package workbench

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sylvinus/agent-vm/internal/host"
)

const tailnetControlFile = "tailscale.json"

type tailnetControlConfig struct {
	ControlURL string `json:"control_url,omitempty"`
}

func normalizeTailnetControlURL(raw string) (string, error) {
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

func readTailnetControlConfig(profile string) (tailnetControlConfig, error) {
	var cfg tailnetControlConfig
	path := filepath.Join(profile, tailnetControlFile)
	if err := host.CheckPrivateFile(path); os.IsNotExist(err) {
		return cfg, nil
	} else if err != nil {
		return cfg, err
	}
	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()
	const maxConfig = 16 << 10
	if info, err := file.Stat(); err != nil || info.Size() > maxConfig {
		return cfg, errors.New("invalid control configuration file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxConfig))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return cfg, errors.New("invalid control configuration data")
	}
	cfg.ControlURL, err = normalizeTailnetControlURL(cfg.ControlURL)
	return cfg, err
}

func saveTailnetControlConfig(profile string, cfg tailnetControlConfig) error {
	return writePrivate(filepath.Join(profile, tailnetControlFile), cfg)
}

func validTailnetAuthURL(raw string) bool {
	return validTailnetAuthURLFor(raw, "")
}

func validTailnetAuthURLFor(raw, controlURL string) bool {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || strings.Contains(raw, "#") {
		return false
	}
	if controlURL == "" {
		return u.Host == "login.tailscale.com" && len(u.Path) > 3 && (strings.HasPrefix(u.Path, "/a/") || strings.HasPrefix(u.Path, "/c/"))
	}
	origin, err := normalizeTailnetControlURL("https://" + u.Host)
	return err == nil && origin == controlURL && u.Path != "" && u.Path != "/"
}
