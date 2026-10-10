//go:build windows

package license

import "golang.org/x/sys/windows/registry"

// rawMachineID 读取 Windows 安装时生成的 MachineGuid，普通用户权限即可读取。
func rawMachineID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("MachineGuid")
	return v, err
}

const platformName = "Windows"
