package license

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestSuspensionCannotBeUndoneByOfflineOrCacheRestore(t *testing.T) {
	f, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.licenses["GOOD-KEY"].status = "SUSPENDED"
	f.mu.Unlock()
	if st := m.Refresh(); st.State != StateInactive || st.Code != "SUSPENDED" {
		t.Fatalf("suspension: %+v", st)
	}
	srv.Close()
	if st := m.Refresh(); st.State != StateInactive {
		t.Fatalf("offline resurrected license: %+v", st)
	}
	if err := os.WriteFile(m.path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if st := New(dir, "test").Refresh(); st.State != StateInactive {
		t.Fatalf("old license.json bypassed independent rejection record: %+v", st)
	}
}

func TestSignatureFailureRequiresSuccessfulOnlineRecovery(t *testing.T) {
	f, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	_, wrong, _ := ed25519.GenerateKey(nil)
	f.mu.Lock()
	good := f.priv
	f.priv = wrong
	f.mu.Unlock()
	if st := m.Refresh(); st.State != StateInactive || st.Offline {
		t.Fatalf("bad live signature must not allow offline fallback: %+v", st)
	}
	f.mu.Lock()
	f.priv = good
	f.mu.Unlock()
	if st := m.Refresh(); st.State != StateActive {
		t.Fatalf("online recovery: %+v", st)
	}
	srv.Close()
	if st := New(dir, "test").Refresh(); st.State != StateActive || !st.Offline {
		t.Fatalf("normal offline use after recovery: %+v", st)
	}
}

func TestEveryOperationChecksDeadlineWithoutNetwork(t *testing.T) {
	f, srv := newFake(t)
	m := New(t.TempDir(), "test")
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if st := m.Refresh(); st.State != StateActive || !st.Offline {
		t.Fatalf("offline: %+v", st)
	}
	f.mu.Lock()
	before := f.validations
	f.mu.Unlock()
	clock = clock.Add(OfflineGrace - time.Minute)
	if !m.Active() {
		t.Fatal("normal seven-day grace was shortened")
	}
	clock = clock.Add(2 * time.Minute)
	if m.Active() {
		t.Fatal("core operations remained active beyond deadline without Refresh")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.validations != before {
		t.Fatal("core operation unexpectedly performed network validation")
	}
}

func TestLicenseExpiryIsEarlierThanOfflineGrace(t *testing.T) {
	f, srv := newFake(t)
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	f.licenses["GOOD-KEY"].expiry = &exp
	m := New(t.TempDir(), "test")
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	st := m.Refresh()
	want, _ := time.Parse(time.RFC3339, exp)
	if st.GraceUntil != want.UnixMilli() {
		t.Fatalf("grace should end at license expiry: %+v", st)
	}
	clock = clock.Add(2 * time.Hour)
	if m.Active() {
		t.Fatal("expired paid license remained usable")
	}
}

func TestClockRollbackAcrossRestartRequiresOnlineRecovery(t *testing.T) {
	_, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	clock = clock.Add(5 * 24 * time.Hour)
	if !m.Active() {
		t.Fatal("still inside grace")
	}
	if st := New(dir, "test").Refresh(); st.State != StateInactive {
		t.Fatalf("rollback to before highest used time was accepted: %+v", st)
	}
	// 少量正常自动校时允许使用，但不会向后延长宽限期。
	m2 := New(dir, "test")
	seen := time.UnixMilli(m2.local.SeenAt)
	m2.now = func() time.Time { return seen.Add(-2 * time.Minute) }
	if st := m2.Refresh(); st.State != StateActive {
		t.Fatalf("small clock correction: %+v", st)
	}
}

func TestExpiredCacheCannotBeRevivedByClockRollback(t *testing.T) {
	_, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	clock = clock.Add(OfflineGrace + time.Hour)
	if st := m.Refresh(); st.State != StateInactive {
		t.Fatalf("expiry: %+v", st)
	}
	if st := New(dir, "test").Refresh(); st.State != StateInactive {
		t.Fatalf("clock rollback revived expired cache: %+v", st)
	}
}

func TestOnlineValidationToleratesIncorrectLocalClock(t *testing.T) {
	_, srv := newFake(t)
	m := New(t.TempDir(), "test")
	clock := time.Now().Add(-365 * 24 * time.Hour)
	m.now = func() time.Time { return clock }
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	if !m.Active() {
		t.Fatal("signed server time should override wrong local wall clock")
	}
	srv.Close()
	clock = clock.Add(time.Hour)
	if st := m.Refresh(); st.State != StateActive || !st.Offline {
		t.Fatalf("server-time anchor did not survive normal offline transition: %+v", st)
	}
}

func TestLegacyCacheMigrationAndDamagedStateRecovery(t *testing.T) {
	_, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(m.statePath); err != nil {
		t.Fatal(err)
	}
	// 模拟升级前已有 license.json、尚无独立状态文件的正常客户。
	m2 := New(dir, "test")
	st := m2.failed("GOOD-KEY", &netError{})
	if st.State != StateActive || !st.Offline {
		t.Fatalf("old signed cache migration: %+v", st)
	}
	if err := os.WriteFile(m.statePath, []byte("damaged state"), 0o600); err != nil {
		t.Fatal(err)
	}
	m3 := New(dir, "test")
	if st := m3.failed("GOOD-KEY", &netError{}); st.State != StateInactive {
		t.Fatalf("damaged state allowed offline use: %+v", st)
	}
	if st := m3.Refresh(); st.State != StateActive {
		t.Fatalf("online repairs damaged local state: %+v", st)
	}
	srv.Close()
	if st := New(dir, "test").Refresh(); st.State != StateActive {
		t.Fatalf("repaired offline cache: %+v", st)
	}
}

func TestMistypedReplacementKeyDoesNotRevokeExistingLicense(t *testing.T) {
	newFake(t)
	m := New(t.TempDir(), "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Activate("MISTYPED-NEW-KEY"); err == nil {
		t.Fatal("bad key accepted")
	}
	if !m.Active() {
		t.Fatal("mistyped new key disabled existing paid license")
	}
}

func TestPublicKeyIsRequiredAndStateProtectionRoundTrips(t *testing.T) {
	if err := verifyWithKey(signed{}, ""); err == nil {
		t.Fatal("empty public key was accepted")
	}
	b := []byte(`{"seenAt":1234567890}`)
	protected, err := protectState(b)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := unprotectState(protected)
	if err != nil || !bytes.Equal(plain, b) {
		t.Fatalf("state protection round trip: %v", err)
	}
	if runtime.GOOS == "windows" {
		if bytes.Equal(protected, b) {
			t.Fatal("Windows state was not protected")
		}
		protected[len(protected)-1] ^= 1
		if _, err := unprotectState(protected); err == nil {
			t.Fatal("DPAPI accepted modified state")
		}
	}
}

func TestSignedValidationMustMatchChallengeAndFullScope(t *testing.T) {
	cases := map[string]func(map[string]any){
		"policy": func(out map[string]any) {
			out["meta"].(map[string]any)["scope"].(map[string]any)["policy"] = "other-policy"
		},
		"product": func(out map[string]any) {
			out["meta"].(map[string]any)["scope"].(map[string]any)["product"] = "other-product"
		},
		"fingerprint": func(out map[string]any) {
			out["meta"].(map[string]any)["scope"].(map[string]any)["fingerprint"] = "other-machine"
		},
		"nonce":        func(out map[string]any) { out["meta"].(map[string]any)["nonce"] = -1 },
		"missing data": func(out map[string]any) { out["data"] = nil },
	}
	for name, transform := range cases {
		t.Run(name, func(t *testing.T) {
			f, _ := newFake(t)
			m := New(t.TempDir(), "test")
			if _, err := m.Activate("GOOD-KEY"); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			f.transformValidation = transform
			f.mu.Unlock()
			if st := m.Refresh(); st.State != StateInactive {
				t.Fatalf("valid signature with wrong challenge/scope was accepted: %+v", st)
			}
		})
	}
}
