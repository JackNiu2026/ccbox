package workbench

import (
	"errors"
	"net/http"
	"strings"

	"github.com/sylvinus/agent-vm/internal/proxyimport"
)

var discoverProxySources = proxyimport.Discover

func (s *Server) proxyLocalSources(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在 CCBox 本机窗口导入本机代理配置", http.StatusForbidden)
		return
	}
	sources, err := discoverProxySources([]byte(s.token))
	if err != nil {
		requestError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonReply(w, sources)
}

func (s *Server) resolveLocalProxySource(id string) (proxyimport.Config, error) {
	sources, err := discoverProxySources([]byte(s.token))
	if err != nil {
		return proxyimport.Config{}, err
	}
	for _, source := range sources {
		if source.ID != id {
			continue
		}
		if source.Error != "" || source.Nodes == 0 || len(source.Data) == 0 {
			return proxyimport.Config{}, errors.New("本机代理配置没有可导入的节点，请检查原客户端")
		}
		source.Name = strings.TrimSpace(source.Name)
		if source.Name == "" || len(source.Name) > 100 {
			source.Name = source.Client
		}
		return source, nil
	}
	return proxyimport.Config{}, errors.New("本机代理配置已变化，请重新扫描后选择")
}
