package license

import (
	"context"
	"os/exec"
	"regexp"
	"time"
)

var platformUUID = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([0-9A-Fa-f-]+)"`)

func machineIdentity() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	match := platformUUID.FindSubmatch(data)
	if err != nil || len(match) != 2 {
		return "", err
	}
	return string(match[1]), nil
}
