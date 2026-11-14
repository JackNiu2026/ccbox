package license

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// DeviceID exposes a product-scoped digest, never the raw OS identifier.
func DeviceID() (string, error) {
	value, err := machineIdentity()
	if err != nil || strings.TrimSpace(value) == "" {
		return "", errors.New("无法读取此设备的系统标识")
	}
	sum := sha256.Sum256([]byte("org.ccbox.desktop/license/v1\x00" + strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:]), nil
}
