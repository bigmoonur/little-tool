package license

import (
	"encoding/json"
	"os"
	"time"
)

// localState 独立于旧版 license.json，防止还原旧缓存丢掉已收到的拒绝结果。
// Windows 使用当前用户的 DPAPI 保护；它用于提高本地篡改成本，不能替代服务器。
type localState struct {
	SeenAt  int64           `json:"seenAt"`
	Blocked map[string]bool `json:"blocked,omitempty"`
}

func (m *Manager) loadState() {
	b, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return // 兼容已有用户的签名缓存，首次成功验证后自动迁移。
	}
	if err == nil {
		b, err = unprotectState(b)
	}
	if err == nil {
		err = json.Unmarshal(b, &m.local)
	}
	m.stateInvalid = err != nil
}

// 调用者持有 mu。定期保存最高已使用时间，避免每次操作都写磁盘。
func (m *Manager) saveState(force bool) error {
	now := m.now()
	if !force && !m.stateSavedAt.IsZero() && now.Sub(m.stateSavedAt) < time.Minute {
		return nil
	}
	b, err := json.Marshal(m.local)
	if err == nil {
		b, err = protectState(b)
	}
	if err == nil {
		err = os.WriteFile(m.statePath+".tmp", b, 0o600)
	}
	if err == nil {
		err = os.Rename(m.statePath+".tmp", m.statePath)
	}
	if err == nil {
		m.stateSavedAt = now
	}
	return err
}

// serverAt 和 clockStarted 构成服务器时间 + 进程内单调时钟。
// 在线验证成功后，本机时钟不准确也不影响本次运行中的正常使用。
func (m *Manager) effectiveNow() time.Time {
	if m.serverAt.IsZero() {
		return m.now()
	}
	elapsed := m.now().Sub(m.clockStarted)
	if elapsed < 0 {
		elapsed = 0
	}
	return m.serverAt.Add(elapsed)
}

// invalidate 只撤销正在使用的授权，输入另一个错误的授权码不会影响原授权。
func (m *Manager) invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saved.Key != key {
		return
	}
	if m.local.Blocked == nil {
		m.local.Blocked = make(map[string]bool)
	}
	if m.saved.LicenseID != "" {
		m.local.Blocked[m.saved.LicenseID] = true
	}
	m.saved.Last = nil
	if m.hbStop != nil {
		close(m.hbStop)
		m.hbStop = nil
	}
	_ = m.save()
	_ = m.saveState(true)
}
