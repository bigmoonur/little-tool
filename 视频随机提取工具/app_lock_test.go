package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
	"github.com/bigmoonur/little-tool/shared/license/applock"
)

func newLockFixture(t *testing.T) *App {
	t.Helper()
	store, err := picker.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewApp(store, nil, nil)
}

func assertLockError(t *testing.T, a *App, st applock.Status, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want || st != a.LockStatus() {
		t.Fatalf("lock error = %+v %v, want current status and %q", st, err, want)
	}
}

func assertLockWait(t *testing.T, a *App, st applock.Status, err error) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), "输错次数太多，请 ") ||
		!strings.HasSuffix(err.Error(), " 秒后再试") || st != a.LockStatus() || st.LockedUntil == 0 {
		t.Fatalf("wait error = %+v %v", st, err)
	}
}

func TestInitIncludesLock(t *testing.T) {
	a := newLockFixture(t)
	for _, enabled := range []bool{false, true} {
		if enabled {
			if _, err := a.SetLock("ignored", "0917"); err != nil {
				t.Fatal(err)
			}
			a.Unlock("wrong")
		}
		info, err := a.Init()
		if err != nil || info.Lock != a.LockStatus() || info.Lock.Enabled != enabled {
			t.Fatalf("Init lock = %+v %v", info.Lock, err)
		}
		b, err := json.Marshal(info)
		var fields map[string]json.RawMessage
		if err != nil || json.Unmarshal(b, &fields) != nil {
			t.Fatalf("Init JSON = %s %v", b, err)
		}
		want, _ := json.Marshal(a.LockStatus())
		if string(fields["lock"]) != string(want) {
			t.Fatalf("Init lock JSON = %s, want %s", fields["lock"], want)
		}
	}
	failed := NewApp(nil, nil, errors.New("unavailable"))
	info, err := failed.Init()
	if err == nil || info.Lock != (applock.Status{}) || info.Lock != failed.LockStatus() {
		t.Fatalf("failed Init lock = %+v %v", info.Lock, err)
	}
}

func TestSetLockChecksCurrentPIN(t *testing.T) {
	a := newLockFixture(t)
	st, err := a.SetLock("ignored", "123")
	assertLockError(t, a, st, err, "密码必须是 4 位数字")
	if st.Enabled {
		t.Fatal("invalid PIN enabled lock")
	}
	if st, err := a.SetLock("ignored", "0917"); err != nil || st != (applock.Status{Enabled: true}) {
		t.Fatalf("enable = %+v %v", st, err)
	}
	st, err = a.SetLock("0000", "2468")
	assertLockError(t, a, st, err, "当前密码不对")
	if st.Failures != 1 {
		t.Fatalf("wrong current PIN did not count: %+v", st)
	}
	st, err = a.SetLock("malformed", "2468")
	assertLockError(t, a, st, err, "当前密码不对")
	if st.Failures != 2 {
		t.Fatalf("malformed current PIN did not count: %+v", st)
	}
	st, err = a.SetLock("0917", "invalid")
	assertLockError(t, a, st, err, "密码必须是 4 位数字")
	if st != (applock.Status{Enabled: true}) {
		t.Fatalf("valid current PIN did not clear failures: %+v", st)
	}
	if r := a.Unlock("0917"); !r.OK {
		t.Fatal("failed change replaced old PIN")
	}
	st, err = a.SetLock("0917", "2468")
	if err != nil || st != (applock.Status{Enabled: true}) {
		t.Fatalf("change = %+v %v", st, err)
	}
	if r := a.Unlock("0917"); r.OK || r.Status.Failures != 1 {
		t.Fatalf("old PIN = %+v", r)
	}
	if r := a.Unlock("2468"); !r.OK || r.Status != (applock.Status{Enabled: true}) {
		t.Fatalf("new PIN = %+v", r)
	}
	reopened := NewApp(a.store, nil, nil)
	if r := reopened.Unlock("2468"); !r.OK || !r.Status.Enabled {
		t.Fatalf("changed PIN was not persisted: %+v", r)
	}
}

func TestDisableLockChecksCurrentPIN(t *testing.T) {
	a := newLockFixture(t)
	if st, err := a.DisableLock("ignored"); err != nil || st != (applock.Status{}) {
		t.Fatalf("already disabled = %+v %v", st, err)
	}
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	st, err := a.DisableLock("wrong")
	assertLockError(t, a, st, err, "当前密码不对")
	if !st.Enabled || st.Failures != 1 {
		t.Fatalf("wrong disable = %+v", st)
	}
	st, err = a.DisableLock("0917")
	if err != nil || st != (applock.Status{}) {
		t.Fatalf("disable = %+v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(a.store.Dir(), "lock.json")); !os.IsNotExist(err) {
		t.Fatalf("DisableLock did not remove file: %v", err)
	}
	if r := NewApp(a.store, nil, nil).Unlock("anything"); !r.OK || r.Status != (applock.Status{}) {
		t.Fatalf("disabled lock after reopening = %+v", r)
	}
}

func TestDisableAlreadyOffDoesNotWrite(t *testing.T) {
	a := newLockFixture(t)
	path := filepath.Join(a.store.Dir(), "lock.json")
	before := []byte("corrupt lock")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	a = NewApp(a.store, nil, nil)
	if st, err := a.DisableLock("ignored"); err != nil || st != (applock.Status{}) {
		t.Fatalf("disabled corrupt lock = %+v %v", st, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("already-off DisableLock wrote state: %q %v", after, err)
	}
}

func TestConcurrentPINChangesRequireLatestPIN(t *testing.T) {
	a := newLockFixture(t)
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	type result struct {
		pin string
		st  applock.Status
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, pin := range []string{"2468", "1357"} {
		wg.Go(func() {
			st, err := a.SetLock("0917", pin)
			results <- result{pin, st, err}
		})
	}
	wg.Wait()
	winner := ""
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			if winner != "" || r.st != (applock.Status{Enabled: true}) {
				t.Fatalf("multiple concurrent changes succeeded: %+v", r)
			}
			winner = r.pin
		} else if r.err.Error() != "当前密码不对" || r.st != (applock.Status{Enabled: true, Failures: 1}) {
			t.Fatalf("concurrent change rejection = %+v", r)
		}
	}
	if winner == "" || a.LockStatus() != (applock.Status{Enabled: true, Failures: 1}) {
		t.Fatalf("concurrent changes lost verification/count: winner %q, %+v", winner, a.LockStatus())
	}
	if r := a.Unlock(winner); !r.OK || r.Status != (applock.Status{Enabled: true}) {
		t.Fatalf("winning PIN was not saved: %+v", r)
	}
}

func TestWaitingAppliesToAllPINChecks(t *testing.T) {
	a := newLockFixture(t)
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	r := a.Unlock("wrong")
	if r.OK || r.Status.Failures != 1 {
		t.Fatalf("wrong unlock = %+v", r)
	}
	st, err := a.SetLock("wrong", "2468")
	assertLockError(t, a, st, err, "当前密码不对")
	st, err = a.DisableLock("wrong")
	assertLockError(t, a, st, err, "当前密码不对")
	st, err = a.SetLock("wrong", "2468")
	assertLockError(t, a, st, err, "当前密码不对")
	st, err = a.DisableLock("wrong")
	assertLockWait(t, a, st, err)
	if st.Failures != 5 {
		t.Fatalf("shared failure count = %+v", st)
	}
	before := st
	if r := a.Unlock("0917"); r.OK || r.Status != before {
		t.Fatalf("unlock bypassed wait = %+v", r)
	}
	st, err = a.SetLock("0917", "2468")
	assertLockWait(t, a, st, err)
	if st != before {
		t.Fatal("SetLock counted refused attempt")
	}
	st, err = a.DisableLock("0917")
	assertLockWait(t, a, st, err)
	if st != before {
		t.Fatal("DisableLock counted refused attempt")
	}
}

func TestLockWaitSecondsRoundUp(t *testing.T) {
	const now = int64(1700000000000)
	for _, tc := range []struct {
		remaining int64
		want      string
	}{
		{-1, "当前密码不对"},
		{0, "当前密码不对"},
		{1, "输错次数太多，请 1 秒后再试"},
		{999, "输错次数太多，请 1 秒后再试"},
		{1000, "输错次数太多，请 1 秒后再试"},
		{1001, "输错次数太多，请 2 秒后再试"},
		{29999, "输错次数太多，请 30 秒后再试"},
	} {
		st := applock.Status{Enabled: true, LockedUntil: now + tc.remaining}
		if err := currentLockError(st, now); err.Error() != tc.want {
			t.Fatalf("remaining %dms = %v, want %q", tc.remaining, err, tc.want)
		}
	}
}

func TestLockUnavailableDataDirectory(t *testing.T) {
	for _, a := range []*App{NewApp(nil, nil, errors.New("unavailable")), NewApp(nil, nil, nil)} {
		if st := a.LockStatus(); st != (applock.Status{}) {
			t.Fatalf("unavailable lock = %+v", st)
		}
		if r := a.Unlock("anything"); !r.OK || r.Status != (applock.Status{}) {
			t.Fatalf("unavailable lock must be off: %+v", r)
		}
		for _, call := range []func() (applock.Status, error){
			func() (applock.Status, error) { return a.SetLock("", "0917") },
			func() (applock.Status, error) { return a.DisableLock("") },
			func() (applock.Status, error) { return a.ResetLockWithLicense("SAVED-KEY") },
		} {
			st, err := call()
			assertLockError(t, a, st, err, "无法打开数据文件夹")
		}
	}
}

func TestResetLockWithoutSavedLicense(t *testing.T) {
	a := newLockFixture(t)
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	a.Unlock("wrong")
	before := a.LockStatus()
	st, err := a.ResetLockWithLicense("anything")
	assertLockError(t, a, st, err, "没有可用于重置的授权码")
	if st != before {
		t.Fatal("no-key reset changed lock")
	}
}

func checkSavedLicenseReset(t *testing.T, a *App) {
	t.Helper()
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		a.Unlock("wrong")
	}
	before := a.LockStatus()
	for _, key := range []string{"", "short", "SAVED-key", "SAVED-KEY-EXTRA"} {
		st, err := a.ResetLockWithLicense(key)
		assertLockError(t, a, st, err, "授权码不对")
		if st != before {
			t.Fatal("wrong license reset changed lock")
		}
	}
	st, err := a.ResetLockWithLicense(" \tSAVED-KEY\n ")
	if err != nil || st != (applock.Status{}) {
		t.Fatalf("right saved key reset = %+v %v", st, err)
	}
	if time.Now().UnixMilli() >= before.LockedUntil {
		t.Fatal("test did not exercise reset during wait")
	}
	if r := NewApp(a.store, nil, nil).Unlock("anything"); !r.OK || r.Status != (applock.Status{}) {
		t.Fatalf("license reset was not persisted: %+v", r)
	}
}

func TestResetLockWithSavedLicense(t *testing.T) {
	a := newLockFixture(t)
	a.lic = &testLicense{status: license.Status{State: license.StateInactive, Key: "SAVED-KEY"}}
	checkSavedLicenseReset(t, a)
}

func TestLockMethodsDuringCopy(t *testing.T) {
	a := newLockFixture(t)
	a.lic = &testLicense{status: license.Status{State: license.StateInactive, Key: "SAVED-KEY"}}
	a.busy, a.pendingID = true, "prepared"
	if st, err := a.SetLock("", "0917"); err != nil || !st.Enabled {
		t.Fatalf("enable during copy = %+v %v", st, err)
	}
	if r := a.Unlock("0917"); !r.OK {
		t.Fatalf("unlock during copy = %+v", r)
	}
	if st, err := a.SetLock("0917", "2468"); err != nil || !st.Enabled {
		t.Fatalf("change during copy = %+v %v", st, err)
	}
	if st, err := a.DisableLock("2468"); err != nil || st.Enabled {
		t.Fatalf("disable during copy = %+v %v", st, err)
	}
	if _, err := a.SetLock("", "0917"); err != nil {
		t.Fatal(err)
	}
	if st, err := a.ResetLockWithLicense("SAVED-KEY"); err != nil || st.Enabled {
		t.Fatalf("reset during copy = %+v %v", st, err)
	}
	if !a.busy || a.pendingID != "prepared" {
		t.Fatal("lock operations changed copy state")
	}
}
