package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKeygen 模拟 Keygen 的几个接口，并像真实服务器一样对响应签名。
type fakeKeygen struct {
	mu                  sync.Mutex
	priv                ed25519.PrivateKey
	licenses            map[string]*fakeLicense // key -> license
	validations         int
	transformValidation func(map[string]any)
}

type fakeLicense struct {
	id       string
	status   string // ACTIVE / SUSPENDED
	max      int
	machines map[string]bool
	expiry   *string
}

func newFake(t *testing.T) (*fakeKeygen, *httptest.Server) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	f := &fakeKeygen{priv: priv, licenses: map[string]*fakeLicense{
		"GOOD-KEY": {id: "lic-good", status: "ACTIVE", max: 2, machines: map[string]bool{}},
		"SUSP-KEY": {id: "lic-susp", status: "SUSPENDED", max: 2, machines: map[string]bool{}},
		"FULL-KEY": {id: "lic-full", status: "ACTIVE", max: 1, machines: map[string]bool{"other": true}},
	}}
	srv := httptest.NewServer(f)
	oldBase, oldKey := apiBase, PublicKey
	apiBase, PublicKey = srv.URL, hex.EncodeToString(pub)
	t.Cleanup(func() {
		srv.Close()
		apiBase, PublicKey = oldBase, oldKey
	})
	return f, srv
}

func (f *fakeKeygen) write(w http.ResponseWriter, r *http.Request, status int, v any) {
	var body []byte
	if v != nil {
		body, _ = json.Marshal(v)
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	digest := sha256.Sum256(body)
	data := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s\ndigest: sha-256=%s",
		strings.ToLower(r.Method), r.URL.RequestURI(), signHost, date, base64.StdEncoding.EncodeToString(digest[:]))
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(f.priv, []byte(data)))
	w.Header().Set("Date", date)
	w.Header().Set("Keygen-Signature", fmt.Sprintf(`keyid="%s", algorithm="ed25519", signature="%s", headers="(request-target) host date digest"`, AccountID, sig))
	w.WriteHeader(status)
	w.Write(body)
}

func (f *fakeKeygen) byAuth(r *http.Request) *fakeLicense {
	return f.licenses[strings.TrimPrefix(r.Header.Get("Authorization"), "License ")]
}

func (f *fakeKeygen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := "/v1/accounts/" + AccountID
	switch {
	case r.Method == "POST" && r.URL.Path == base+"/licenses/actions/validate-key":
		f.validations++
		var req struct {
			Meta struct {
				Key   string         `json:"key"`
				Nonce int64          `json:"nonce"`
				Scope map[string]any `json:"scope"`
			} `json:"meta"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		meta := map[string]any{"nonce": req.Meta.Nonce, "scope": req.Meta.Scope}
		l := f.licenses[req.Meta.Key]
		fp, _ := req.Meta.Scope["fingerprint"].(string)
		var data any
		switch {
		case l == nil:
			meta["valid"], meta["code"] = false, "NOT_FOUND"
		case l.status == "SUSPENDED":
			meta["valid"], meta["code"] = false, "SUSPENDED"
		case len(l.machines) == 0:
			meta["valid"], meta["code"] = false, "NO_MACHINE"
		case !l.machines[fp]:
			meta["valid"], meta["code"] = false, "FINGERPRINT_SCOPE_MISMATCH"
		default:
			meta["valid"], meta["code"] = true, "VALID"
		}
		if l != nil {
			data = map[string]any{"id": l.id, "type": "licenses", "attributes": map[string]any{"status": l.status, "expiry": l.expiry}}
		}
		out := map[string]any{"meta": meta, "data": data}
		if f.transformValidation != nil {
			f.transformValidation(out)
		}
		f.write(w, r, 200, out)

	case r.Method == "POST" && r.URL.Path == base+"/machines":
		l := f.byAuth(r)
		var req struct {
			Data struct {
				Attributes struct {
					Fingerprint string `json:"fingerprint"`
				} `json:"attributes"`
			} `json:"data"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		fp := req.Data.Attributes.Fingerprint
		switch {
		case l == nil:
			f.write(w, r, 401, map[string]any{"errors": []any{map[string]any{"code": "LICENSE_INVALID"}}})
		case l.machines[fp]:
			f.write(w, r, 422, map[string]any{"errors": []any{map[string]any{"code": "FINGERPRINT_TAKEN"}}})
		case len(l.machines) >= l.max:
			f.write(w, r, 422, map[string]any{"errors": []any{map[string]any{"code": "MACHINE_LIMIT_EXCEEDED"}}})
		default:
			l.machines[fp] = true
			f.write(w, r, 201, map[string]any{"data": map[string]any{"id": "m-" + fp[:6], "attributes": map[string]any{"requireHeartbeat": false}}})
		}

	case strings.HasPrefix(r.URL.Path, base+"/machines/"):
		l := f.byAuth(r)
		fp := strings.TrimPrefix(r.URL.Path, base+"/machines/")
		if l == nil || !l.machines[fp] {
			f.write(w, r, 404, map[string]any{"errors": []any{map[string]any{"code": "NOT_FOUND"}}})
			return
		}
		if r.Method == "DELETE" {
			delete(l.machines, fp)
			f.write(w, r, 204, nil)
			return
		}
		f.write(w, r, 200, map[string]any{"data": map[string]any{"id": "m", "attributes": map[string]any{"requireHeartbeat": false}}})

	default:
		http.NotFound(w, r)
	}
}

func TestActivateValidateAndRestart(t *testing.T) {
	f, _ := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")

	if st := m.Refresh(); st.State != StateInactive || st.Key != "" {
		t.Fatalf("no key should be inactive: %+v", st)
	}
	st, err := m.Activate("  GOOD-KEY ")
	if err != nil || st.State != StateActive || st.Offline {
		t.Fatalf("activate: %v %+v", err, st)
	}
	if !f.licenses["GOOD-KEY"].machines[m.fp] {
		t.Fatal("machine was not registered")
	}

	// 重新打开程序：直接联网验证通过。
	m2 := New(dir, "test")
	if st := m2.Refresh(); st.State != StateActive || st.Masked == "" {
		t.Fatalf("restart: %+v", st)
	}
}

func TestActivateErrors(t *testing.T) {
	newFake(t)
	m := New(t.TempDir(), "test")
	cases := map[string]string{"NOPE": "NOT_FOUND", "SUSP-KEY": "SUSPENDED", "FULL-KEY": "MACHINE_LIMIT_EXCEEDED"}
	for key, code := range cases {
		_, err := m.Activate(key)
		var ke *KeygenError
		if !errors.As(err, &ke) || ke.Code != code {
			t.Fatalf("%s: got %v", key, err)
		}
		if m.Active() {
			t.Fatalf("%s should not activate", key)
		}
	}
}

func TestOfflineGraceAndTamper(t *testing.T) {
	_, srv := newFake(t)
	dir := t.TempDir()
	m := New(dir, "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}
	srv.Close() // 断网

	st := m.Refresh()
	if st.State != StateActive || !st.Offline || st.GraceUntil == 0 {
		t.Fatalf("offline within grace: %+v", st)
	}

	m.now = func() time.Time { return time.Now().Add(OfflineGrace + time.Hour) }
	if st := m.Refresh(); st.State != StateInactive || !st.Network {
		t.Fatalf("offline after grace: %+v", st)
	}

	// 修改缓存内容后签名失效，不能离线使用。
	m.now = time.Now
	b, _ := os.ReadFile(dir + "/license.json")
	var s stored
	_ = json.Unmarshal(b, &s)
	s.Last.Date = time.Now().Add(48 * time.Hour).UTC().Format(http.TimeFormat)
	b, _ = json.Marshal(s)
	_ = os.WriteFile(dir+"/license.json", b, 0o600)
	m3 := New(dir, "test")
	if st := m3.Refresh(); st.State != StateInactive {
		t.Fatalf("tampered cache must not be accepted: %+v", st)
	}
}

func TestForgedServerRejected(t *testing.T) {
	newFake(t)
	other, _, _ := ed25519.GenerateKey(nil)
	PublicKey = hex.EncodeToString(other) // 程序里配置的公钥与“服务器”不一致
	m := New(t.TempDir(), "test")
	if _, err := m.Activate("GOOD-KEY"); !errors.Is(err, errSignature) {
		t.Fatalf("expected signature error, got %v", err)
	}
}

func TestDeactivateAndAutoReactivate(t *testing.T) {
	f, _ := newFake(t)
	m := New(t.TempDir(), "test")
	if _, err := m.Activate("GOOD-KEY"); err != nil {
		t.Fatal(err)
	}

	// 服务器端释放了本机（例如心跳超时），下次验证时自动重新登记。
	f.mu.Lock()
	delete(f.licenses["GOOD-KEY"].machines, m.fp)
	f.mu.Unlock()
	if st := m.Refresh(); st.State != StateActive {
		t.Fatalf("auto reactivate: %+v", st)
	}

	st, err := m.Deactivate()
	if err != nil || st.State != StateInactive {
		t.Fatalf("deactivate: %v %+v", err, st)
	}
	if f.licenses["GOOD-KEY"].machines[m.fp] {
		t.Fatal("machine still registered on server")
	}
	if st := m.Refresh(); st.State != StateInactive || st.Key != "" {
		t.Fatalf("after deactivate the key must be forgotten: %+v", st)
	}
}
