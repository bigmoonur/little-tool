package license

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	StateChecking = "checking"
	StateActive   = "active"
	StateInactive = "inactive"
)

// Status 是发给界面的授权状态。
type Status struct {
	State      string `json:"state"`
	Key        string `json:"key"`        // 已保存的授权码，未激活时用于预填
	Masked     string `json:"masked"`     // 显示用的部分隐藏的授权码
	Expiry     string `json:"expiry"`     // RFC3339，空表示不过期
	Offline    bool   `json:"offline"`    // 凭离线宽限期使用中
	GraceUntil int64  `json:"graceUntil"` // 离线可用截止时间（毫秒）
	Code       string `json:"code"`
	Message    string `json:"message"`
	Network    bool   `json:"network"` // 因无法联网而未激活
	Machine    string `json:"machine"`
}

// stored 是 license.json 的内容。
type stored struct {
	Key       string  `json:"key"`
	LicenseID string  `json:"licenseId"`
	Last      *signed `json:"last,omitempty"` // 最近一次验证通过的签名响应，用于离线宽限期
	Fallback  string  `json:"fallback,omitempty"`
}

type Manager struct {
	mu        sync.Mutex
	path      string
	fp        string
	host      string
	c         *client
	saved     stored
	status    Status
	ready     chan struct{}
	readyOnce sync.Once
	hbStop    chan struct{}
	now       func() time.Time
}

// New 读取保存的授权信息并计算本机指纹，不联网。
func New(dataDir, version string) *Manager {
	m := &Manager{
		path:   filepath.Join(dataDir, "license.json"),
		c:      newClient(version),
		ready:  make(chan struct{}),
		now:    time.Now,
		status: Status{State: StateChecking},
	}
	m.load()
	raw, err := rawMachineID()
	if err != nil || strings.TrimSpace(raw) == "" {
		// 读不到系统标识时用随机标识代替，保存在授权文件里。
		if m.saved.Fallback == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			m.saved.Fallback = hex.EncodeToString(b)
			_ = m.save()
		}
		raw = m.saved.Fallback
	}
	sum := sha256.Sum256([]byte("material-picker:" + strings.TrimSpace(raw)))
	m.fp = hex.EncodeToString(sum[:])
	m.host, _ = os.Hostname()
	return m
}

func (m *Manager) load() {
	b, err := os.ReadFile(m.path)
	if err == nil {
		_ = json.Unmarshal(b, &m.saved)
	}
}

func (m *Manager) save() error {
	b, err := json.Marshal(m.saved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// Start 在后台完成首次验证，之后定期重新验证；状态变化时调用 onChange。
func (m *Manager) Start(onChange func(Status)) {
	go func() {
		m.Refresh()
		m.readyOnce.Do(func() { close(m.ready) })
		t := time.NewTicker(RecheckInterval)
		defer t.Stop()
		for range t.C {
			prev := m.Status()
			st := m.Refresh()
			if onChange != nil && (prev.State != st.State || prev.Offline != st.Offline) {
				onChange(st)
			}
		}
	}()
}

// Wait 等待首次验证完成后返回状态。
func (m *Manager) Wait() Status {
	<-m.ready
	return m.Status()
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) Active() bool { return m.Status().State == StateActive }

func (m *Manager) set(st Status) Status {
	if st.Machine == "" {
		st.Machine = m.host
	}
	m.mu.Lock()
	m.status = st
	m.mu.Unlock()
	return st
}

func (m *Manager) stored() stored {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved
}

// 这些结果表示本机没有登记（或登记已因心跳超时被清除），可以重新登记。
var needsMachine = map[string]bool{
	"NO_MACHINE": true, "NO_MACHINES": true, "FINGERPRINT_SCOPE_MISMATCH": true, "HEARTBEAT_DEAD": true,
}

// Refresh 联网重新验证已保存的授权码。
func (m *Manager) Refresh() Status {
	saved := m.stored()
	if saved.Key == "" {
		return m.set(Status{State: StateInactive})
	}
	v, s, err := m.c.validate(saved.Key, m.fp)
	if err == nil && !v.Meta.Valid && needsMachine[v.Meta.Code] && saved.LicenseID != "" {
		// 这台电脑以前激活过（例如心跳超时被自动释放），直接重新登记，不必让用户再输一次授权码。
		st, aerr := m.activate(saved.Key)
		if aerr == nil {
			return st
		}
		return m.set(m.failed(saved.Key, aerr))
	}
	if err == nil && v.Meta.Code == "HEARTBEAT_NOT_STARTED" {
		if m.c.ping(saved.Key, m.fp) == nil {
			v, s, err = m.c.validate(saved.Key, m.fp)
		}
	}
	switch {
	case err != nil:
		return m.set(m.failed(saved.Key, err))
	case !v.Meta.Valid:
		return m.set(m.inactive(saved.Key, v.Meta.Code, codeMessage(v.Meta.Code, v.Meta.Detail)))
	}
	return m.set(m.accept(saved.Key, v, s))
}

// Activate 用授权码激活本机。
func (m *Manager) Activate(key string) (Status, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return m.Status(), errors.New("请输入授权码")
	}
	st, err := m.activate(key)
	if err != nil {
		return m.Status(), err
	}
	return st, nil
}

func (m *Manager) activate(key string) (Status, error) {
	v, s, err := m.c.validate(key, m.fp)
	if err != nil {
		return Status{}, err
	}
	if !v.Meta.Valid {
		switch {
		case needsMachine[v.Meta.Code] && v.Meta.Code != "HEARTBEAT_DEAD":
			if v.Data == nil {
				return Status{}, &KeygenError{Code: v.Meta.Code, Msg: codeMessage(v.Meta.Code, v.Meta.Detail)}
			}
			if _, err := m.c.activate(key, v.Data.ID, m.fp, m.host, platformName); err != nil {
				var ke *KeygenError
				// 已登记过同一指纹时继续验证即可。
				if !errors.As(err, &ke) || !strings.Contains(ke.Code, "TAKEN") {
					return Status{}, err
				}
			}
		case v.Meta.Code == "HEARTBEAT_NOT_STARTED" || v.Meta.Code == "HEARTBEAT_DEAD":
			_ = m.c.ping(key, m.fp)
		default:
			return Status{}, &KeygenError{Code: v.Meta.Code, Msg: codeMessage(v.Meta.Code, v.Meta.Detail)}
		}
		v, s, err = m.c.validate(key, m.fp)
		if err == nil && v.Meta.Code == "HEARTBEAT_NOT_STARTED" {
			_ = m.c.ping(key, m.fp)
			v, s, err = m.c.validate(key, m.fp)
		}
		if err != nil {
			return Status{}, err
		}
		if !v.Meta.Valid {
			return Status{}, &KeygenError{Code: v.Meta.Code, Msg: codeMessage(v.Meta.Code, v.Meta.Detail)}
		}
	}
	return m.set(m.accept(key, v, s)), nil
}

// Deactivate 解除本机授权，释放一个激活名额。
func (m *Manager) Deactivate() (Status, error) {
	saved := m.stored()
	if saved.Key != "" {
		if err := m.c.deactivate(saved.Key, m.fp); err != nil {
			return m.Status(), err
		}
	}
	m.mu.Lock()
	if m.hbStop != nil {
		close(m.hbStop)
		m.hbStop = nil
	}
	m.saved = stored{Fallback: m.saved.Fallback}
	err := m.save()
	m.mu.Unlock()
	return m.set(Status{State: StateInactive}), err
}

func (m *Manager) accept(key string, v validation, s signed) Status {
	m.mu.Lock()
	m.saved.Key = key
	m.saved.LicenseID = v.Data.ID
	m.saved.Last = &s
	_ = m.save()
	m.mu.Unlock()
	go m.heartbeat(key)
	return Status{State: StateActive, Key: key, Masked: mask(key), Expiry: expiryOf(v)}
}

func (m *Manager) inactive(key, code, msg string) Status {
	return Status{State: StateInactive, Key: key, Masked: mask(key), Code: code, Message: msg}
}

// failed 处理联网失败：上次验证通过且在宽限期内时继续可用。
func (m *Manager) failed(key string, err error) Status {
	var ne *netError
	if !errors.As(err, &ne) && !errors.Is(err, errSignature) {
		var ke *KeygenError
		code := ""
		if errors.As(err, &ke) {
			code = ke.Code
		}
		return m.inactive(key, code, err.Error())
	}
	saved := m.stored()
	if until, v, ok := m.offlineUntil(saved); ok {
		return Status{
			State: StateActive, Key: key, Masked: mask(key), Expiry: expiryOf(v),
			Offline: true, GraceUntil: until.UnixMilli(), Message: err.Error(),
		}
	}
	st := m.inactive(key, "", err.Error())
	st.Network = true
	if saved.Last != nil {
		st.Message = "已经很久没有联网验证授权了，请连接网络后重试"
	}
	return st
}

// offlineUntil 检查离线缓存：签名有效、属于本机和本授权、未过宽限期、授权本身未到期。
func (m *Manager) offlineUntil(saved stored) (time.Time, validation, bool) {
	var v validation
	if saved.Last == nil || verify(*saved.Last) != nil {
		return time.Time{}, v, false
	}
	if json.Unmarshal([]byte(saved.Last.Body), &v) != nil || !v.Meta.Valid || v.Data == nil {
		return time.Time{}, v, false
	}
	if v.Data.ID != saved.LicenseID || v.Meta.Scope.Fingerprint != m.fp || v.Meta.Scope.Product != ProductID {
		return time.Time{}, v, false
	}
	at, err := http.ParseTime(saved.Last.Date)
	if err != nil {
		return time.Time{}, v, false
	}
	now := m.now()
	until := at.Add(OfflineGrace)
	if now.After(until) || now.Before(at.Add(-24*time.Hour)) {
		return time.Time{}, v, false
	}
	if exp := expiryOf(v); exp != "" {
		if t, err := time.Parse(time.RFC3339, exp); err == nil && now.After(t) {
			return time.Time{}, v, false
		}
	}
	return until, v, true
}

// heartbeat 只在策略要求心跳时定期发送；不需要心跳时不能发送，否则会开启心跳监控。
func (m *Manager) heartbeat(key string) {
	md, err := m.c.machine(key, m.fp)
	if err != nil || !md.Attributes.RequireHeartbeat {
		return
	}
	m.mu.Lock()
	if m.hbStop != nil {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.hbStop = stop
	m.mu.Unlock()

	interval := time.Duration(md.Attributes.HeartbeatDuration) * time.Second / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	_ = m.c.ping(key, m.fp)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = m.c.ping(key, m.fp)
		}
	}
}

func expiryOf(v validation) string {
	if v.Data != nil && v.Data.Attributes.Expiry != nil {
		return *v.Data.Attributes.Expiry
	}
	return ""
}

func mask(key string) string {
	// Keygen 授权码形如 8F5BC0-F09717-…-F352F0-V3：只显示第一段和最后一段。
	if parts := strings.Split(key, "-"); len(parts) >= 3 {
		return parts[0] + "-••••-" + parts[len(parts)-1]
	}
	r := []rune(key)
	if len(r) <= 10 {
		if len(r) <= 2 {
			return key
		}
		return string(r[:2]) + "…"
	}
	return string(r[:6]) + "…" + string(r[len(r)-4:])
}
