//go:build windows

package picker

import "golang.org/x/sys/windows"

func freeSpace(path string) (uint64, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var free uint64
	if windows.GetDiskFreeSpaceEx(p, &free, nil, nil) != nil {
		return 0, false
	}
	return free, true
}

// hide 把临时文件夹设为隐藏，避免在资源管理器里被误操作。
func hide(path string) {
	if p, err := windows.UTF16PtrFromString(path); err == nil {
		attrs, err := windows.GetFileAttributes(p)
		if err == nil {
			_ = windows.SetFileAttributes(p, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
		}
	}
}
