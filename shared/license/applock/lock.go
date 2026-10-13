// Package applock 保存隐私锁；它不加密用户数据，删除 lock.json 即可移除锁。
package applock

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const iterations = 100000

type Status struct {
	Enabled     bool  `json:"enabled"`
	Failures    int   `json:"failures"`
	LockedUntil int64 `json:"lockedUntil"`
}

type stored struct {
	Salt        []byte `json:"salt"`
	Iterations  int    `json:"iterations"`
	Hash        []byte `json:"hash"`
	Failures    int    `json:"failures"`
	LockedUntil int64  `json:"lockedUntil"`
}

type Lock struct {
	mu    sync.Mutex
	path  string
	state stored
	now   func() time.Time
}

// Open 读取 dir 中的锁；文件不可读或损坏时视为关闭。删除 lock.json 会移除锁。
func Open(dir string) *Lock {
	l := &Lock{path: filepath.Join(dir, "lock.json"), now: time.Now}
	b, err := os.ReadFile(l.path)
	var s stored
	if err == nil && json.Unmarshal(b, &s) == nil && len(s.Salt) == 16 && len(s.Hash) == 32 &&
		s.Iterations == iterations && s.Failures >= 0 && s.LockedUntil >= 0 {
		l.state = s
	}
	return l
}

func (l *Lock) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status(l.now().UnixMilli())
}

func (l *Lock) status(now int64) Status {
	if len(l.state.Hash) == 0 {
		return Status{}
	}
	st := Status{Enabled: true, Failures: l.state.Failures, LockedUntil: l.state.LockedUntil}
	if now >= st.LockedUntil {
		st.LockedUntil = 0
	}
	return st
}

func validPIN(pin string) bool {
	if len(pin) != 4 {
		return false
	}
	for i := range pin {
		if pin[i] < '0' || pin[i] > '9' {
			return false
		}
	}
	return true
}

func (l *Lock) Set(pin string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !validPIN(pin) {
		return errors.New("密码必须是 4 位数字")
	}
	s := stored{Salt: make([]byte, 16), Iterations: iterations}
	if _, err := rand.Read(s.Salt); err != nil {
		return fmt.Errorf("无法生成锁屏密码：%w", err)
	}
	var err error
	s.Hash, err = pbkdf2.Key(sha256.New, pin, s.Salt, s.Iterations, 32)
	if err != nil {
		return fmt.Errorf("无法生成锁屏密码：%w", err)
	}
	if err := l.save(s); err != nil {
		return fmt.Errorf("无法保存锁屏密码：%w", err)
	}
	l.state = s
	return nil
}

func (l *Lock) Verify(pin string) (bool, Status) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now().UnixMilli()
	if len(l.state.Hash) == 0 {
		return true, Status{}
	}
	if now < l.state.LockedUntil {
		return false, l.status(now)
	}
	ok := false
	if validPIN(pin) {
		hash, err := pbkdf2.Key(sha256.New, pin, l.state.Salt, l.state.Iterations, 32)
		ok = err == nil && subtle.ConstantTimeCompare(hash, l.state.Hash) == 1
	}
	s := l.state
	if ok {
		if s.Failures == 0 && s.LockedUntil == 0 {
			return true, l.status(now)
		}
		s.Failures, s.LockedUntil = 0, 0
		// 密码输对了就放行：清零写不进磁盘（磁盘满、只读）也不能把主人挡在外面，下次打开最多多等一会儿。
		l.state = s
		_ = l.save(s)
		return true, l.status(now)
	}
	if s.Failures < int(^uint(0)>>1) {
		s.Failures++
	}
	s.LockedUntil = 0
	if s.Failures >= 5 {
		wait := 30 * time.Second
		for n := 5; n < s.Failures && wait < 10*time.Minute; n++ {
			wait *= 2
		}
		if wait > 10*time.Minute {
			wait = 10 * time.Minute
		}
		s.LockedUntil = now + wait.Milliseconds()
	}
	// 写入失败仍保留本次计数，避免同一进程里因磁盘故障无限重试。
	l.state = s
	_ = l.save(s)
	return false, l.status(now)
}

func (l *Lock) Disable() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("无法关闭锁屏：%w", err)
	}
	l.state = stored{}
	return nil
}

func (l *Lock) save(s stored) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".lock-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), l.path)
}
