//go:build !windows && !darwin

package license

import (
	"os"
	"strings"
)

func rawMachineID() (string, error) {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) != "" {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", os.ErrNotExist
}

const platformName = "Linux"
