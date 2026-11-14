package license

import "os"

func machineIdentity() (string, error) {
	value, err := os.ReadFile("/etc/machine-id")
	return string(value), err
}
