package managedproxy

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ErrSourceNodeDNS marks a source rejected for its node resolver list alone:
// its nodes parsed, so "no importable nodes" would misstate the cause.
var ErrSourceNodeDNS = errors.New("unsupported source node DNS")

type sourceNodeDNSError struct{ error }

func (sourceNodeDNSError) Is(target error) bool { return target == ErrSourceNodeDNS }

// Adopt only safe remote HTTPS DoH entries from the subscription's node
// resolver list. Standard non-HTTP Clash/Mihomo resolvers (plain IPv4/IPv6,
// udp://, tcp://, tls://, quic://, h3://, dhcp://, system) are ignored rather
// than blocking import, while HTTP(S) entries are strictly validated so unsafe
// or malformed DoH URLs still fail refresh and keep the previous configuration.
func subscriptionNodeDNS(data []byte) ([]string, error) {
	var input struct {
		DNS yaml.Node `yaml:"dns"`
	}
	if yaml.Unmarshal(data, &input) != nil || input.DNS.Kind != yaml.MappingNode {
		return nil, nil // Share-link lists have no DNS configuration.
	}
	var dns struct {
		Servers []string `yaml:"proxy-server-nameserver"`
	}
	if input.DNS.Decode(&dns) != nil {
		return nil, sourceNodeDNSError{errors.New("订阅的节点 DNS 需要填写 HTTPS DoH 地址列表")}
	}
	var candidates []string
	for _, raw := range dns.Servers {
		trimmed := strings.TrimSpace(raw)
		if isIgnoredNonHTTPResolver(trimmed) {
			continue
		}
		candidates = append(candidates, trimmed)
	}
	servers, err := validateSourceDNS(candidates)
	if err != nil {
		return nil, sourceNodeDNSError{err}
	}
	return servers, nil
}

func isIgnoredNonHTTPResolver(raw string) bool {
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if lower == "system" || strings.HasPrefix(lower, "dhcp://") ||
		strings.HasPrefix(lower, "udp://") || strings.HasPrefix(lower, "tcp://") ||
		strings.HasPrefix(lower, "tls://") || strings.HasPrefix(lower, "quic://") ||
		strings.HasPrefix(lower, "h3://") {
		return true
	}
	// Plain IP or IP:port (e.g. 119.29.29.29, 223.5.5.5:53, [2400:3200::1]).
	if !strings.Contains(raw, "://") {
		host := raw
		if idx := strings.LastIndex(host, ":"); idx != -1 && !strings.Contains(host[idx+1:], "]") {
			if port, err := strconv.Atoi(host[idx+1:]); err == nil && port >= 1 && port <= 65535 {
				host = host[:idx]
			}
		}
		if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return true
		}
	}
	return false
}

// FilterNodeDNS drops standard non-HTTP Clash/Mihomo resolvers (plain IPs,
// udp://, tcp://, tls://, quic://, h3://, dhcp://, system) while preserving
// HTTP(S) and malformed entries so downstream validation still reports
// unsupported DoH configurations accurately.
func FilterNodeDNS(servers []string) []string {
	var out []string
	for _, raw := range servers {
		trimmed := strings.TrimSpace(raw)
		if isIgnoredNonHTTPResolver(trimmed) {
			continue
		}
		if !slices.Contains(out, trimmed) {
			out = append(out, trimmed)
		}
	}
	return out
}

func validateSourceDNS(servers []string) ([]string, error) {
	if len(servers) > 4 {
		return nil, errors.New("订阅的节点 DNS 最多支持 4 个 HTTPS DoH 地址")
	}
	var out []string
	local := newLocalServers()
	for i, raw := range servers {
		raw = strings.TrimSpace(raw)
		u, err := url.Parse(raw)
		if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || local.contains(u.Hostname()) {
			return nil, fmt.Errorf("订阅的第 %d 个节点 DNS 需要远程 HTTPS DoH 地址，不支持用户名、密码或 # 路由参数", i+1)
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("订阅的第 %d 个节点 DNS 端口无效", i+1)
			}
		}
		if !slices.Contains(out, raw) {
			out = append(out, raw)
		}
	}
	return out, nil
}

// The bundled core has a separate resolver/cache for proxy server domains.
// Exact policies there cannot alter business DNS or resolver bootstrapping.
// It keys policies by domain, not node/source; reject conflicting ownership
// rather than silently sending another source's queries to this resolver.
func (s Store) sourceDNSPolicies(blocked map[string]string) (map[string][]string, error) {
	policies := map[string][]string{}
	owners := map[string][]string{}
	for _, src := range s.Sources {
		servers, err := validateSourceDNS(src.NodeDNS)
		if err != nil {
			return nil, err
		}
		custom := len(servers) > 0
		if !custom {
			servers = s.NodeResolvers()
		}
		for _, node := range src.Nodes {
			if blocked[node.ID] != "" {
				continue
			}
			host, _ := node.Config["server"].(string)
			host = strings.ToLower(strings.TrimSuffix(host, "."))
			if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
				continue
			}
			if previous, ok := owners[host]; ok && !slices.Equal(previous, servers) {
				return nil, errors.New("多个来源对同一节点域名指定了不同 DNS，无法隔离解析；请统一这些来源的节点 DNS 或移除冲突来源，原配置已保留")
			}
			owners[host] = servers
			if custom {
				if !exactDNSHost(host) {
					return nil, errors.New("使用订阅节点 DNS 时，节点服务器必须是具体域名或 IP，不能包含通配符或匹配规则")
				}
				policies[host] = servers
			}
		}
	}
	return policies, nil
}

func exactDNSHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
