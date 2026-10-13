package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// netError 表示无法连接授权服务器（或服务器暂时不可用），可以进入离线宽限期。
type netError struct{ err error }

func (e *netError) Error() string { return "无法连接授权服务器，请检查网络后重试" }
func (e *netError) Unwrap() error { return e.err }

var errSignature = errors.New("授权服务器的响应未通过校验，请检查网络环境后重试")

// signed 是一次 API 响应中参与签名校验的部分，也用于离线缓存。
type signed struct {
	Method    string `json:"method"`
	Target    string `json:"target"`
	Date      string `json:"date"`
	Signature string `json:"signature"`
	Body      string `json:"body"`
}

type apiError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

type licenseData struct {
	ID         string `json:"id"`
	Attributes struct {
		Expiry      *string `json:"expiry"`
		Status      string  `json:"status"`
		MaxMachines *int    `json:"maxMachines"`
	} `json:"attributes"`
}

type validation struct {
	Data *licenseData `json:"data"`
	Meta struct {
		Valid  bool   `json:"valid"`
		Detail string `json:"detail"`
		Code   string `json:"code"`
		Nonce  int64  `json:"nonce"`
		Scope  struct {
			Fingerprint string `json:"fingerprint"`
			Product     string `json:"product"`
			Policy      string `json:"policy"`
		} `json:"scope"`
	} `json:"meta"`
	Errors []apiError `json:"errors"`
}

type machineData struct {
	ID         string `json:"id"`
	Attributes struct {
		RequireHeartbeat  bool   `json:"requireHeartbeat"`
		HeartbeatDuration int    `json:"heartbeatDuration"`
		HeartbeatStatus   string `json:"heartbeatStatus"`
	} `json:"attributes"`
}

type client struct {
	http      *http.Client
	ua        string
	baseURL   string
	publicKey string
}

func newClient(version string) *client {
	return &client{http: &http.Client{Timeout: 20 * time.Second}, ua: "MaterialPicker/" + version, baseURL: apiBase, publicKey: PublicKey}
}

// request 发送请求并校验响应签名。HTTP 状态码不在这里判断，由调用方处理。
func (c *client) request(method, path, key string, payload any) (int, signed, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, signed{}, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return 0, signed{}, err
	}
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("User-Agent", c.ua)
	if payload != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}
	if key != "" {
		req.Header.Set("Authorization", "License "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, signed{}, &netError{err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, signed{}, &netError{err}
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return 0, signed{}, &netError{fmt.Errorf("授权服务器暂时不可用（%d）", resp.StatusCode)}
	}
	s := signed{
		Method:    method,
		Target:    path,
		Date:      resp.Header.Get("Date"),
		Signature: resp.Header.Get("Keygen-Signature"),
		Body:      string(raw),
	}
	if err := verifyWithKey(s, c.publicKey); err != nil {
		return 0, signed{}, err
	}
	return resp.StatusCode, s, nil
}

var sigParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// verify 按 Keygen 的规则校验 Ed25519 签名，缺少公钥时也必须拒绝。
func verify(s signed) error {
	return verifyWithKey(s, PublicKey)
}

func verifyWithKey(s signed, publicKey string) error {
	pub, err := hex.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("授权公钥配置错误")
	}
	params := map[string]string{}
	for _, m := range sigParam.FindAllStringSubmatch(s.Signature, -1) {
		params[m[1]] = m[2]
	}
	if params["algorithm"] != "ed25519" || params["keyid"] != AccountID || params["headers"] != "(request-target) host date digest" {
		return errSignature
	}
	sig, err := base64.StdEncoding.DecodeString(params["signature"])
	if err != nil {
		return errSignature
	}
	digest := sha256.Sum256([]byte(s.Body))
	data := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s\ndigest: sha-256=%s",
		strings.ToLower(s.Method), s.Target, signHost, s.Date, base64.StdEncoding.EncodeToString(digest[:]))
	if !ed25519.Verify(pub, []byte(data), sig) {
		return errSignature
	}
	if _, err := http.ParseTime(s.Date); err != nil {
		return errSignature
	}
	return nil
}

func accountPath(rest string) string { return "/v1/accounts/" + AccountID + rest }

// validate 校验授权码，限定本产品、本策略和本机指纹，并用随机数防止重放旧响应。
func (c *client) validate(key, fingerprint string) (validation, signed, error) {
	// 保持在 JSON 精确整数范围内，使用密码学随机源生成挑战值。
	n, err := rand.Int(rand.Reader, big.NewInt(1<<52))
	if err != nil {
		return validation{}, signed{}, fmt.Errorf("无法生成授权验证随机数：%w", err)
	}
	nonce := n.Int64()
	payload := map[string]any{
		"meta": map[string]any{
			"key":   key,
			"nonce": nonce,
			"scope": map[string]any{"product": ProductID, "policy": PolicyID, "fingerprint": fingerprint},
		},
	}
	status, s, err := c.request("POST", accountPath("/licenses/actions/validate-key"), "", payload)
	if err != nil {
		return validation{}, s, err
	}
	var v validation
	if err := json.Unmarshal([]byte(s.Body), &v); err != nil {
		return validation{}, s, errSignature
	}
	if len(v.Errors) > 0 {
		return v, s, apiErr(v.Errors)
	}
	if status < 200 || status >= 300 || v.Meta.Nonce != nonce || v.Meta.Scope.Fingerprint != fingerprint || v.Meta.Scope.Product != ProductID || v.Meta.Scope.Policy != PolicyID {
		return validation{}, s, errSignature
	}
	if v.Meta.Valid && (v.Data == nil || v.Data.ID == "") {
		return validation{}, s, errSignature
	}
	if v.Meta.Valid && v.Data.Attributes.Expiry != nil {
		if _, err := time.Parse(time.RFC3339, *v.Data.Attributes.Expiry); err != nil {
			return validation{}, s, errSignature
		}
	}
	return v, s, nil
}

// activate 在授权下登记本机。
func (c *client) activate(key, licenseID, fingerprint, name, platform string) (machineData, error) {
	payload := map[string]any{
		"data": map[string]any{
			"type": "machines",
			"attributes": map[string]any{
				"fingerprint": fingerprint,
				"name":        name,
				"platform":    platform,
			},
			"relationships": map[string]any{
				"license": map[string]any{"data": map[string]any{"type": "licenses", "id": licenseID}},
			},
		},
	}
	status, s, err := c.request("POST", accountPath("/machines"), key, payload)
	if err != nil {
		return machineData{}, err
	}
	var out struct {
		Data   machineData `json:"data"`
		Errors []apiError  `json:"errors"`
	}
	_ = json.Unmarshal([]byte(s.Body), &out)
	if status >= 400 || len(out.Errors) > 0 {
		return machineData{}, apiErr(out.Errors)
	}
	return out.Data, nil
}

// machine 读取本机记录；fingerprint 可以直接作为机器 ID 使用。
func (c *client) machine(key, fingerprint string) (machineData, error) {
	status, s, err := c.request("GET", accountPath("/machines/"+fingerprint), key, nil)
	if err != nil {
		return machineData{}, err
	}
	var out struct {
		Data   machineData `json:"data"`
		Errors []apiError  `json:"errors"`
	}
	_ = json.Unmarshal([]byte(s.Body), &out)
	if status >= 400 || len(out.Errors) > 0 {
		return machineData{}, apiErr(out.Errors)
	}
	return out.Data, nil
}

func (c *client) ping(key, fingerprint string) error {
	status, s, err := c.request("POST", accountPath("/machines/"+fingerprint+"/actions/ping"), key, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		var out struct{ Errors []apiError }
		_ = json.Unmarshal([]byte(s.Body), &out)
		return apiErr(out.Errors)
	}
	return nil
}

// deactivate 删除本机记录。机器已不存在时视为成功。
func (c *client) deactivate(key, fingerprint string) error {
	status, s, err := c.request("DELETE", accountPath("/machines/"+fingerprint), key, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status < 300 {
		return nil
	}
	var out struct{ Errors []apiError }
	_ = json.Unmarshal([]byte(s.Body), &out)
	return apiErr(out.Errors)
}

// KeygenError 是授权服务器返回的业务错误。
type KeygenError struct {
	Code string
	Msg  string
}

func (e *KeygenError) Error() string { return e.Msg }

func apiErr(errs []apiError) error {
	if len(errs) == 0 {
		return &KeygenError{Code: "UNKNOWN", Msg: "授权服务器返回了未知错误，请稍后重试"}
	}
	e := errs[0]
	return &KeygenError{Code: e.Code, Msg: errorMessage(e.Code, e.Detail)}
}

// codeMessage 把验证结果代码翻译成给用户看的话。
func codeMessage(code, detail string) string {
	switch code {
	case "NOT_FOUND":
		return "授权码不存在，请检查是否输入正确"
	case "SUSPENDED":
		return "这个授权已被暂停，请联系卖家"
	case "EXPIRED":
		return "这个授权已过期，请联系卖家续期"
	case "OVERDUE":
		return "这个授权已逾期，请联系卖家"
	case "BANNED":
		return "这个授权已被停用，请联系卖家"
	case "PRODUCT_SCOPE_MISMATCH", "POLICY_SCOPE_MISMATCH":
		return "这个授权码不适用于本程序"
	case "TOO_MANY_MACHINES", "MACHINE_LIMIT_EXCEEDED":
		return "这个授权码的可激活电脑数已用完。请先在旧电脑上“解除本机授权”，或联系卖家"
	case "NO_MACHINE", "NO_MACHINES", "FINGERPRINT_SCOPE_MISMATCH":
		return "这台电脑还没有激活"
	case "HEARTBEAT_DEAD":
		return "这台电脑的授权因长时间未联网已失效，请重新激活"
	case "HEARTBEAT_NOT_STARTED":
		return "授权尚未完成初始化，请重试"
	}
	if detail != "" {
		return fmt.Sprintf("授权验证未通过（%s：%s）", code, detail)
	}
	return fmt.Sprintf("授权验证未通过（%s）", code)
}

func errorMessage(code, detail string) string {
	switch {
	case strings.Contains(code, "LIMIT"):
		return codeMessage("MACHINE_LIMIT_EXCEEDED", "")
	case code == "TOKEN_INVALID", code == "FORBIDDEN", code == "LICENSE_INVALID":
		return fmt.Sprintf("授权服务器拒绝了激活请求，请联系卖家（%s）", code)
	}
	return codeMessage(code, detail)
}
