//go:build !windows

package picker

import "syscall"

func freeSpace(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, false
	}
	return st.Bavail * uint64(st.Bsize), true
}

// hide 在 macOS / Linux 上不需要处理：以点开头的文件夹本来就是隐藏的。
func hide(string) {}
