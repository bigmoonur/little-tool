//go:build darwin

package license

import (
	"errors"
	"os/exec"
	"regexp"
)

var platformUUID = regexp.MustCompile(`"IOPlatformUUID" = "([^"]+)"`)

func rawMachineID() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", err
	}
	m := platformUUID.FindSubmatch(out)
	if m == nil {
		return "", errors.New("IOPlatformUUID not found")
	}
	return string(m[1]), nil
}

const platformName = "macOS"
