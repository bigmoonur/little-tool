package license

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"
)

// TestLiveSignature 向真实的 Keygen 服务器验证一个不存在的授权码，确认配置的公钥能校验服务器签名。
// 需要联网，默认跳过：KEYGEN_LIVE=1 go test . -run Live -v
func TestLiveSignature(t *testing.T) {
	if os.Getenv("KEYGEN_LIVE") != "1" {
		t.Skip("set KEYGEN_LIVE=1 to run against api.keygen.sh")
	}
	if PublicKey == "" {
		t.Fatal("PublicKey is empty")
	}
	v, s, err := newClient("live-test").validate("LIVE-SIGNATURE-CHECK", "live-test-fingerprint")
	if errors.Is(err, errSignature) {
		t.Fatalf("signature rejected: public key does not match the account (%s)", s.Signature)
	}
	if err != nil {
		t.Fatal(err)
	}
	if v.Meta.Valid || v.Meta.Code != "NOT_FOUND" {
		t.Fatalf("unexpected result: %+v", v.Meta)
	}
	t.Logf("signature verified, code=%s", v.Meta.Code)

	// 反向检查：换一个错误的公钥必须被拒绝。
	good := PublicKey
	PublicKey = "00" + good[2:]
	defer func() { PublicKey = good }()
	if _, _, err := newClient("live-test").validate("LIVE-SIGNATURE-CHECK", "live-test-fingerprint"); !errors.Is(err, errSignature) {
		t.Fatalf("a wrong public key must be rejected, got %v", err)
	}
}

// TestLiveFlow 用真实授权码走一遍：激活 → 重启后验证 → 离线宽限 → 解除授权。
// 使用临时指纹，结束时一定解除，不长期占用激活名额。
// KEYGEN_LIVE_KEY=<授权码> go test . -run LiveFlow -v
func TestLiveFlow(t *testing.T) {
	key := os.Getenv("KEYGEN_LIVE_KEY")
	if key == "" {
		t.Skip("set KEYGEN_LIVE_KEY to run against api.keygen.sh")
	}
	dir := t.TempDir()
	fp := "live-test-" + NewTestID()
	open := func() *Manager {
		m := New(dir, "live-test")
		m.fp = fp
		return m
	}

	m := open()
	t.Cleanup(func() {
		// 无论测试是否通过，都释放这台测试机器。
		_ = newClient("live-test").deactivate(key, fp)
	})

	st, err := m.Activate(key)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	t.Logf("activated: masked=%s expiry=%q machine=%s", st.Masked, st.Expiry, st.Machine)
	if st.State != StateActive {
		t.Fatalf("activate state: %+v", st)
	}

	md, err := newClient("live-test").machine(key, fp)
	if err != nil {
		t.Fatalf("machine lookup: %v", err)
	}
	t.Logf("machine: id=%s requireHeartbeat=%v heartbeatDuration=%d status=%s",
		md.ID, md.Attributes.RequireHeartbeat, md.Attributes.HeartbeatDuration, md.Attributes.HeartbeatStatus)

	// 重新打开程序：联网验证通过，不需要再次输入。
	m2 := open()
	if st := m2.Refresh(); st.State != StateActive || st.Offline {
		t.Fatalf("restart: %+v", st)
	}

	// 断网：凭签名缓存离线使用。
	base := apiBase
	apiBase = "https://127.0.0.1:1"
	st3 := open().Refresh()
	apiBase = base
	if st3.State != StateActive || !st3.Offline {
		t.Fatalf("offline: %+v", st3)
	}
	t.Logf("offline grace until %s", time.UnixMilli(st3.GraceUntil).Format(time.RFC3339))

	// 解除授权：服务器上的机器被删除，本地不再保存授权码。
	st4, err := m2.Deactivate()
	if err != nil || st4.State != StateInactive {
		t.Fatalf("deactivate: %v %+v", err, st4)
	}
	if _, err := newClient("live-test").machine(key, fp); err == nil {
		t.Fatal("machine still exists after deactivate")
	}
	if st := open().Refresh(); st.State != StateInactive || st.Key != "" {
		t.Fatalf("after deactivate: %+v", st)
	}
}

func NewTestID() string {
	b := make([]byte, 6)
	_, _ = crand.Read(b)
	return hex.EncodeToString(b)
}
