//go:build !windows

package license

// 非 Windows 开发环境仍限制文件权限，不依赖 Windows 的用户凭据。
func protectState(b []byte) ([]byte, error)   { return b, nil }
func unprotectState(b []byte) ([]byte, error) { return b, nil }
