package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bigmoonur/little-tool/shared/license/applock"
)

type LockResult struct {
	OK     bool           `json:"ok"`
	Status applock.Status `json:"status"`
}

func (a *App) LockStatus() applock.Status {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	return a.lockStatus()
}

func (a *App) lockStatus() applock.Status {
	if a.lock == nil {
		return applock.Status{}
	}
	return a.lock.Status()
}

func (a *App) Unlock(pin string) LockResult {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	if a.lock == nil {
		return LockResult{OK: true, Status: applock.Status{}}
	}
	ok, st := a.lock.Verify(pin)
	return LockResult{OK: ok, Status: st}
}

func (a *App) SetLock(current, pin string) (applock.Status, error) {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	if a.lock == nil {
		return a.lockStatus(), errors.New("无法打开数据文件夹")
	}
	if st, err := a.verifyCurrentLock(current); err != nil {
		return st, err
	}
	err := a.lock.Set(pin)
	return a.lockStatus(), err
}

func (a *App) DisableLock(current string) (applock.Status, error) {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	if a.lock == nil {
		return a.lockStatus(), errors.New("无法打开数据文件夹")
	}
	if st := a.lockStatus(); !st.Enabled {
		return st, nil
	}
	if st, err := a.verifyCurrentLock(current); err != nil {
		return st, err
	}
	err := a.lock.Disable()
	return a.lockStatus(), err
}

func (a *App) verifyCurrentLock(current string) (applock.Status, error) {
	st := a.lockStatus()
	if !st.Enabled {
		return st, nil
	}
	ok, st := a.lock.Verify(current)
	if ok {
		return st, nil
	}
	return st, currentLockError(st, time.Now().UnixMilli())
}

func currentLockError(st applock.Status, now int64) error {
	if remaining := st.LockedUntil - now; remaining > 0 {
		return fmt.Errorf("输错次数太多，请 %d 秒后再试", (remaining+999)/1000)
	}
	return errors.New("当前密码不对")
}

func (a *App) ResetLockWithLicense(key string) (applock.Status, error) {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	if a.lock == nil {
		return a.lockStatus(), errors.New("无法打开数据文件夹")
	}
	saved := ""
	if a.lic != nil {
		saved = a.lic.Status().Key
	}
	if saved == "" {
		return a.lockStatus(), errors.New("没有可用于重置的授权码")
	}
	// 固定长度摘要让比较不因授权码长度不同而提前返回。
	gotHash := sha256.Sum256([]byte(strings.TrimSpace(key)))
	savedHash := sha256.Sum256([]byte(saved))
	if subtle.ConstantTimeCompare(gotHash[:], savedHash[:]) != 1 {
		return a.lockStatus(), errors.New("授权码不对")
	}
	err := a.lock.Disable()
	return a.lockStatus(), err
}
