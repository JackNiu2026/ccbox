package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (s *Server) tailnet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	action := r.PathValue("action")
	if r.Method == http.MethodGet {
		action = "status"
	}
	if action == "install" {
		var input struct{}
		if !readJSON(w, r, &input) {
			return
		}
		s.launch(w, "tailscale-install", s.installTailnet)
		return
	}
	switch action {
	case "status", "connect", "login", "disconnect", "logout", "add-key", "remove-key", "open-service", "control-server":
	default:
		http.NotFound(w, r)
		return
	}
	var input struct {
		Key        string  `json:"key"`
		Host       string  `json:"host,omitempty"`
		Port       int     `json:"port,omitempty"`
		ControlURL *string `json:"control_url,omitempty"`
	}
	if r.Method != http.MethodGet && !readJSON(w, r, &input) {
		return
	}
	if len(input.Key) > 8192 {
		http.Error(w, "SSH 公钥过长", 400)
		return
	}
	if (action == "control-server") != (input.ControlURL != nil) {
		http.Error(w, "控制服务器设置无效", http.StatusBadRequest)
		return
	}
	if action == "open-service" && (len(input.Host) == 0 || len(input.Host) > 253 || input.Port < 1 || input.Port > 65535) {
		http.Error(w, "请选择私网设备并填写 1–65535 之间的端口", 400)
		return
	}
	s.tailnetMu.Lock()
	defer s.tailnetMu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	var cfg tailnetControlConfig
	var err error
	if action == "control-server" {
		cfg.ControlURL, err = normalizeTailnetControlURL(*input.ControlURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else if cfg, err = readTailnetControlConfig(s.profile); err != nil {
		http.Error(w, "Tailscale 控制服务器设置无法读取，请重新保存控制服务器地址", http.StatusConflict)
		return
	}
	s.mu.Lock()
	ready, token := s.ready, s.credentials.Token
	busy := s.jobs.snapshot().Running
	s.mu.Unlock()
	if !ready || busy {
		http.Error(w, "请先启动 CCBox 环境，并等待当前操作完成", 409)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	guestAction := action
	var controlURL *string
	if action == "control-server" {
		guestAction = "set-control-url"
		controlURL = &cfg.ControlURL
	} else if action == "connect" || action == "login" {
		controlURL = &cfg.ControlURL
	}
	body, _ := json.Marshal(struct {
		Action     string  `json:"action"`
		Key        string  `json:"key,omitempty"`
		Host       string  `json:"host,omitempty"`
		Port       int     `json:"port,omitempty"`
		ControlURL *string `json:"control_url,omitempty"`
	}{guestAction, input.Key, input.Host, input.Port, controlURL})
	data, err := s.guestAPI(ctx, token, http.MethodPost, "/tailnet", body)
	if err != nil {
		http.Error(w, "Tailscale 操作未完成。请更新环境组件后重试；原设备身份和 SSH 公钥会保留。", 502)
		return
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(data, &result) != nil || result == nil {
		http.Error(w, "Tailscale 组件返回了无效状态", 502)
		return
	}
	if b := result["tailnet_error"]; len(b) > 0 {
		var reason string
		_ = json.Unmarshal(b, &reason)
		http.Error(w, reason, 502)
		return
	}
	var view struct {
		Installed  bool   `json:"installed"`
		ControlURL string `json:"control_url"`
		AuthURL    string `json:"auth_url"`
	}
	if err := json.Unmarshal(data, &view); err != nil {
		http.Error(w, "Tailscale 组件返回了无效状态", http.StatusBadGateway)
		return
	}
	if view.AuthURL != "" && (view.ControlURL != cfg.ControlURL || !validTailnetAuthURLFor(view.AuthURL, cfg.ControlURL)) {
		http.Error(w, "Tailscale 登录地址无效，请重新发起登录", 502)
		return
	}
	if action == "control-server" {
		if !view.Installed || view.ControlURL != cfg.ControlURL {
			http.Error(w, "请更新环境组件后重新保存控制服务器地址", http.StatusConflict)
			return
		}
		if err := saveTailnetControlConfig(s.profile, cfg); err != nil {
			http.Error(w, "保存控制服务器地址失败，请重新保存", http.StatusInternalServerError)
			return
		}
	}
	// Use the host's confirmed server for the editor, even while a previous
	// guest component reports that it is unavailable.
	result["control_url"], _ = json.Marshal(cfg.ControlURL)
	jsonReply(w, result)
}

var errTailnetNativeInstall = errors.New("请在设置中更新环境组件，Tailscale 随客机组件一同安装")
