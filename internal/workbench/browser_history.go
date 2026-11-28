package workbench

import (
	"encoding/hex"
	"net/http"
	"net/url"

	"github.com/sylvinus/agent-vm/internal/containerenv"
	"github.com/sylvinus/agent-vm/internal/guestos"
)

func (s *Server) localBrowserHistory(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) || s.clientOnly || s.macosGuest != nil || s.windowsGuest != nil || containerenv.Configured(s.profile) {
		http.NotFound(w, r)
		return
	}
	if system, err := s.installedLinuxSystem(); err != nil || system != guestos.Debian13 {
		http.NotFound(w, r)
		return
	}
	id := r.URL.Query().Get("id")
	if _, err := hex.DecodeString(id); len(id) != 32 || err != nil {
		http.Error(w, "浏览器标签无效", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	ready, token := s.ready, s.credentials.Token
	s.mu.Unlock()
	if !ready {
		http.Error(w, "浏览器尚未就绪", http.StatusConflict)
		return
	}
	data, err := s.browserClient().request(r.Context(), token, r.Method, "/history?id="+url.QueryEscape(id), nil)
	if err != nil {
		http.Error(w, "浏览历史暂不可用", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
