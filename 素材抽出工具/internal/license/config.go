// Package license 通过 Keygen（keygen.sh）实现联网授权：授权码校验、本机激活、心跳和离线宽限期。
//
// 客户端只使用授权码本身认证（Authorization: License <key>），程序里不包含任何管理员或产品令牌。
package license

import "time"

const (
	AccountID = "89c2e867-ded1-4c13-a5ba-60e8680cd251"
	ProductID = "43905060-7f84-413f-a0f6-c41ec8f7e310"
	PolicyID  = "cd7d2964-705e-40b5-a4f6-5db80e7835d5"

	// OfflineGrace 是联网验证失败时，凭上次成功验证结果继续使用的时间。
	OfflineGrace = 7 * 24 * time.Hour
	// RecheckInterval 是程序运行期间重新联网验证的间隔。
	RecheckInterval = 6 * time.Hour

	signHost = "api.keygen.sh"
)

// PublicKey 是 Keygen 账户的 Ed25519 公钥（64 位十六进制），在 Keygen 后台 Settings 页面复制。
// 填写后，每个服务器响应和离线缓存都必须通过签名校验，可以防止伪造授权服务器或修改缓存；
// 留空则不校验签名。
var PublicKey = "9de093ff188ddcbaceb1cc6246d372ef7970cd6d9ab4385ad7e5c95e74e9a089"

var apiBase = "https://api.keygen.sh"
