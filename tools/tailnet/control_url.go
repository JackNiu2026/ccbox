package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// An empty origin chooses the built-in Tailscale control server.
func normalizeControlURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if len(raw) > 256 || err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil ||
		u.ForceQuery || u.RawQuery != "" || strings.Contains(raw, "#") || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
		return "", errors.New("控制服务器地址须为 HTTPS，只填写协议、域名和端口")
	}
	name, port := strings.ToLower(u.Hostname()), u.Port()
	if name == "" || strings.ContainsAny(name, " \t\r\n%") || strings.HasSuffix(u.Host, ":") ||
		strings.ContainsRune(name, ':') && net.ParseIP(name) == nil {
		return "", errors.New("控制服务器地址缺少有效主机名或端口")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("控制服务器端口须在 1–65535 之间")
		}
		if n != 443 {
			return "https://" + net.JoinHostPort(name, strconv.Itoa(n)), nil
		}
	}
	if name == "controlplane.tailscale.com" {
		return "", nil
	}
	if strings.ContainsRune(name, ':') {
		name = "[" + name + "]"
	}
	return "https://" + name, nil
}

func (n *node) stateDir() (string, error) {
	if n.config.ControlURL == "" {
		return n.dir, nil
	}
	sum := sha256.Sum256([]byte(n.config.ControlURL))
	parent := filepath.Join(n.dir, "servers")
	dir := filepath.Join(parent, hex.EncodeToString(sum[:16]))
	for _, path := range []string{parent, dir} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return "", err
		}
		if err := protectStateDir(path); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// Called with the node lock held. The outgoing identity stays on disk.
func (n *node) setControlURL(raw string) error {
	controlURL, err := normalizeControlURL(raw)
	if err != nil || controlURL == n.config.ControlURL {
		return err
	}
	previous := n.config
	n.config.ControlURL = controlURL
	n.config.Enabled = false
	if err := n.save(); err != nil {
		n.config = previous
		return err
	}
	n.stop()
	_ = n.writeSSHConfig(nil)
	return nil
}
