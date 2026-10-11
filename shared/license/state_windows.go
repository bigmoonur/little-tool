//go:build windows

package license

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI 绑定当前 Windows 用户。没有 UI 提示，也不需要额外密码或管理员权限。
func protectState(b []byte) ([]byte, error)   { return dpapi(b, false) }
func unprotectState(b []byte) ([]byte, error) { return dpapi(b, true) }

func dpapi(b []byte, decrypt bool) ([]byte, error) {
	var in, out windows.DataBlob
	if len(b) > 0 {
		in.Size, in.Data = uint32(len(b)), &b[0]
	}
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
