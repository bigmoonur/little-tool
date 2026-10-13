package applock

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func lockedFixture(t *testing.T) (*Lock, string, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	l := Open(dir)
	now := time.Unix(1700000000, 0)
	l.now = func() time.Time { return now }
	if err := l.Set("0917"); err != nil {
		t.Fatal(err)
	}
	return l, dir, &now
}

func readStored(t *testing.T, dir string) (stored, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s stored
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s, b
}

func TestPINStorageAndVerification(t *testing.T) {
	l, dir, _ := lockedFixture(t)
	s, b := readStored(t, dir)
	if bytes.Contains(b, []byte(`"0917"`)) || len(s.Salt) != 16 || s.Iterations != 100000 || len(s.Hash) != 32 {
		t.Fatalf("PIN storage format = %s", b)
	}
	wantHash, err := pbkdf2.Key(sha256.New, "0917", s.Salt, 100000, 32)
	if err != nil || !bytes.Equal(s.Hash, wantHash) {
		t.Fatalf("stored hash is not PBKDF2-HMAC-SHA256: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dir, "lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o", fi.Mode().Perm())
	}
	if ok, st := l.Verify("0917"); !ok || st != (Status{Enabled: true}) {
		t.Fatalf("right PIN = %v %+v", ok, st)
	}
	for i, pin := range []string{"0918", "", "123", "12345", "１２３４"} {
		if ok, st := l.Verify(pin); ok || st.Failures != i+1 {
			t.Fatalf("wrong/malformed PIN %q = %v %+v", pin, ok, st)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "lock.json" {
		t.Fatalf("atomic write left temporary files: %v %v", entries, err)
	}
}

func TestFailureWaitGrowthAndRecovery(t *testing.T) {
	l, dir, now := lockedFixture(t)
	waits := []time.Duration{0, 0, 0, 0, 30 * time.Second, time.Minute, 2 * time.Minute,
		4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, wait := range waits {
		ok, st := l.Verify("wrong")
		if ok || st.Failures != i+1 || !st.Enabled {
			t.Fatalf("attempt %d = %v %+v", i+1, ok, st)
		}
		wantUntil := int64(0)
		if wait > 0 {
			wantUntil = now.Add(wait).UnixMilli()
		}
		if st.LockedUntil != wantUntil {
			t.Fatalf("attempt %d wait = %d, want %d", i+1, st.LockedUntil, wantUntil)
		}
		saved, before := readStored(t, dir)
		if saved.Failures != st.Failures || saved.LockedUntil != wantUntil {
			t.Fatalf("attempt not persisted: %+v", saved)
		}
		if wait == 0 {
			continue
		}
		for _, pin := range []string{"0917", "wrong"} {
			if ok, during := l.Verify(pin); ok || during != st {
				t.Fatalf("attempt during wait changed state: %v %+v", ok, during)
			}
		}
		_, after := readStored(t, dir)
		if !bytes.Equal(before, after) {
			t.Fatal("refused attempts rewrote the file")
		}
		*now = time.UnixMilli(wantUntil - 1)
		if ok, during := l.Verify("0917"); ok || during != st {
			t.Fatalf("wait ended early: %v %+v", ok, during)
		}
		*now = time.UnixMilli(wantUntil)
		if expired := l.Status(); expired.LockedUntil != 0 || expired.Failures != i+1 {
			t.Fatalf("expired wait = %+v", expired)
		}
	}
	if ok, st := l.Verify("0917"); !ok || st != (Status{Enabled: true}) {
		t.Fatalf("right PIN after wait = %v %+v", ok, st)
	}
	if st := Open(dir).Status(); st != (Status{Enabled: true}) {
		t.Fatalf("successful verification not persisted: %+v", st)
	}
	if ok, st := l.Verify("wrong"); ok || st.Failures != 1 || st.LockedUntil != 0 {
		t.Fatalf("new sequence after recovery = %v %+v", ok, st)
	}
}

func TestFailuresSurviveOpen(t *testing.T) {
	l, dir, now := lockedFixture(t)
	for i := 1; i <= 5; i++ {
		_, st := l.Verify("wrong")
		l = Open(dir)
		l.now = func() time.Time { return *now }
		if reopened := l.Status(); reopened != st || reopened.Failures != i {
			t.Fatalf("reopened after attempt %d = %+v, want %+v", i, reopened, st)
		}
	}
	st := l.Status()
	if ok, after := l.Verify("0917"); ok || after != st {
		t.Fatalf("restart bypassed wait: %v %+v", ok, after)
	}
	*now = time.UnixMilli(st.LockedUntil)
	if ok, after := l.Verify("0917"); !ok || after != (Status{Enabled: true}) {
		t.Fatalf("restarted wait did not end: %v %+v", ok, after)
	}
}

func TestMissingUnreadableAndCorruptFiles(t *testing.T) {
	valid, _, _ := lockedFixture(t)
	s := valid.state
	corrupt := map[string][]byte{
		"invalid JSON": []byte("{"),
		"null":         []byte("null"),
		"empty object": []byte("{}"),
		"wrong type":   []byte(`{"salt":123}`),
	}
	for _, name := range []string{"salt", "hash", "iterations", "failures", "lockedUntil"} {
		broken := s
		switch name {
		case "salt":
			broken.Salt = []byte{1}
		case "hash":
			broken.Hash = []byte{1}
		case "iterations":
			broken.Iterations = 1
		case "failures":
			broken.Failures = -1
		case "lockedUntil":
			broken.LockedUntil = -1
		}
		corrupt[name], _ = json.Marshal(broken)
	}
	for name, b := range corrupt {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "lock.json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
			l := Open(dir)
			if ok, st := l.Verify("anything"); !ok || st != (Status{}) || l.Status() != (Status{}) {
				t.Fatalf("corrupt lock is enabled: %v %+v", ok, st)
			}
		})
	}
	for _, unreadable := range []bool{false, true} {
		dir := t.TempDir()
		if unreadable {
			if err := os.Mkdir(filepath.Join(dir, "lock.json"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		l := Open(dir)
		if ok, st := l.Verify("anything"); !ok || st != (Status{}) {
			t.Fatalf("missing/unreadable file is enabled: %v %+v", ok, st)
		}
	}
}

func TestSetValidationAndReset(t *testing.T) {
	l, dir, _ := lockedFixture(t)
	for i := 0; i < 5; i++ {
		l.Verify("wrong")
	}
	before := l.Status()
	old, beforeFile := readStored(t, dir)
	for _, pin := range []string{"", "123", "12345", "12a4", "１２３４", "١٢٣٤", "123\n", " 1234", "12\x004"} {
		if err := l.Set(pin); err == nil || err.Error() != "密码必须是 4 位数字" {
			t.Fatalf("Set(%q) = %v", pin, err)
		}
		_, afterFile := readStored(t, dir)
		if l.Status() != before || !bytes.Equal(beforeFile, afterFile) {
			t.Fatalf("invalid Set(%q) changed lock", pin)
		}
	}
	if err := l.Set("0000"); err != nil {
		t.Fatal(err)
	}
	if st := l.Status(); st != (Status{Enabled: true}) {
		t.Fatalf("Set did not clear wait/count: %+v", st)
	}
	fresh, _ := readStored(t, dir)
	if bytes.Equal(old.Salt, fresh.Salt) || bytes.Equal(old.Hash, fresh.Hash) {
		t.Fatal("Set reused salt or hash")
	}
	l = Open(dir)
	if ok, _ := l.Verify("0917"); ok {
		t.Fatal("old PIN still works")
	}
	if ok, st := l.Verify("0000"); !ok || st != (Status{Enabled: true}) {
		t.Fatalf("new PIN does not work: %v %+v", ok, st)
	}
}

func TestDisableAndDelete(t *testing.T) {
	l, dir, _ := lockedFixture(t)
	l.Verify("wrong")
	if err := l.Disable(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lock.json")); !os.IsNotExist(err) {
		t.Fatalf("Disable left file: %v", err)
	}
	for _, lock := range []*Lock{l, Open(dir)} {
		if ok, st := lock.Verify("anything"); !ok || st != (Status{}) || lock.Status() != (Status{}) {
			t.Fatalf("disabled lock = %v %+v", ok, st)
		}
		if err := lock.Disable(); err != nil {
			t.Fatalf("repeated Disable = %v", err)
		}
	}
	if err := l.Set("0917"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "lock.json")); err != nil {
		t.Fatal(err)
	}
	if st := Open(dir).Status(); st != (Status{}) {
		t.Fatalf("deleting file did not remove lock: %+v", st)
	}
}

func TestStatusDoesNotAliasState(t *testing.T) {
	l, dir, _ := lockedFixture(t)
	before := l.Status()
	_, after := l.Verify("wrong")
	if before != (Status{Enabled: true}) {
		t.Fatalf("verification changed previous status: %+v", before)
	}
	after.Enabled, after.Failures, after.LockedUntil = false, 99, 123
	if st := l.Status(); st != (Status{Enabled: true, Failures: 1}) || Open(dir).Status() != st {
		t.Fatalf("editing returned status changed state: %+v", st)
	}
}

func TestDiskErrorsNeitherOpenNorJamTheLock(t *testing.T) {
	l, dir, _ := lockedFixture(t)
	path := l.path
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	l.path = filepath.Join(blocked, "lock.json")
	if err := l.Set("0000"); err == nil || l.Status() != (Status{Enabled: true}) {
		t.Fatalf("failed Set changed state: %v %+v", err, l.Status())
	}
	if ok, st := l.Verify("wrong"); ok || st.Failures != 1 {
		t.Fatalf("failed write discarded count: %v %+v", ok, st)
	}
	// 密码输对了就放行，即使清零写不进磁盘；不能因为磁盘故障把主人挡在外面。
	if ok, st := l.Verify("0917"); !ok || st != (Status{Enabled: true}) {
		t.Fatalf("right PIN refused because the count could not be saved: %v %+v", ok, st)
	}
	if err := l.Disable(); err == nil || !l.Status().Enabled {
		t.Fatalf("failed Disable removed lock: %v %+v", err, l.Status())
	}
	l.path = path
	if ok, st := l.Verify("0917"); !ok || st != (Status{Enabled: true}) {
		t.Fatalf("lock did not recover after disk error: %v %+v", ok, st)
	}
}

func TestConcurrentVerify(t *testing.T) {
	l, dir, now := lockedFixture(t)
	const count = 20
	results := make(chan verifyResult, count)
	var wg sync.WaitGroup
	run := func(pin string) {
		for i := 0; i < count; i++ {
			wg.Go(func() {
				l.Status()
				ok, st := l.Verify(pin)
				results <- verifyResult{ok, st}
			})
		}
		wg.Wait()
	}
	run("0918")
	for i := 0; i < count; i++ {
		r := <-results
		if r.ok || r.status.Failures < 1 || r.status.Failures > 5 {
			t.Fatalf("concurrent wrong attempt = %+v", r)
		}
	}
	st := l.Status()
	if st.Failures != 5 || st.LockedUntil != now.Add(30*time.Second).UnixMilli() {
		t.Fatalf("concurrent failures = %+v", st)
	}
	reopened := Open(dir)
	reopened.now = func() time.Time { return *now }
	if reopened.Status() != st {
		t.Fatal("concurrent count was not persisted")
	}
	*now = time.UnixMilli(st.LockedUntil)
	run("0917")
	for i := 0; i < count; i++ {
		r := <-results
		if !r.ok || r.status != (Status{Enabled: true}) {
			t.Fatalf("concurrent right attempt = %+v", r)
		}
	}
	if Open(dir).Status() != (Status{Enabled: true}) {
		t.Fatal("concurrent successful verification did not persist reset")
	}
}

type verifyResult struct {
	ok     bool
	status Status
}
