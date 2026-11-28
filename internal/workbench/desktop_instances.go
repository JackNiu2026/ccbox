package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/containerenv"
	"github.com/sylvinus/agent-vm/internal/guestos"
)

type desktopSlot string

func numberedDesktopSlot(value string) (desktopSlot, bool) {
	switch value {
	case "2", "3", "4":
		return desktopSlot(value), true
	}
	return "", false
}

func (slot desktopSlot) unit() string    { return "ccbox-claude-desktop@" + string(slot) + ".service" }
func (slot desktopSlot) runtime() string { return "/run/ccbox-claude-desktop-" + string(slot) }
func (slot desktopSlot) home() string {
	return `"$HOME/.local/share/ccbox-desktop-instances/` + string(slot) + `/home"`
}

func numberedDesktopResizeScript(slot desktopSlot, width, height int, scale string) string {
	number, _ := strconv.Atoi(string(slot))
	return strings.NewReplacer(
		desktopPhoneResizeGuard, "",
		"DISPLAY=:78", fmt.Sprintf("DISPLAY=:%d", 77+number),
		"/run/cage-desktop/", slot.runtime()+"/",
	).Replace(desktopResizeScript(width, height, scale))
}

func (s *Server) desktopInstance(w http.ResponseWriter, r *http.Request) {
	slot, ok := numberedDesktopSlot(r.PathValue("slot"))
	if !ok || !s.hasTrustedView(r) || s.clientOnly || s.macosGuest != nil || s.windowsGuest != nil || containerenv.Configured(s.profile) {
		http.NotFound(w, r)
		return
	}
	if system, err := s.installedLinuxSystem(); err != nil || system != guestos.Debian13 {
		http.NotFound(w, r)
		return
	}
	if !s.vmRunning() {
		http.Error(w, "请先启动 CCBox 环境", http.StatusConflict)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	action := r.PathValue("action")
	if r.Method == http.MethodGet && action == "vnc" {
		s.streamClaudeDesktopVNC(w, r, slot.runtime())
		return
	}
	if r.Method == http.MethodPost && (action == "start" || action == "stop") {
		var input struct{}
		if readJSON(w, r, &input) {
			s.launch(w, "claude-desktop-instance-"+action, func() error {
				return s.changeDesktopInstance(slot, action)
			})
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	switch {
	case r.Method == http.MethodGet && action == "":
		script := strings.NewReplacer("cage-claude-desktop.service", slot.unit(), "/run/cage-desktop/", slot.runtime()+"/").Replace(claudeDesktopSnapshot)
		var state claudeDesktopState
		data, err := s.guestQuick(ctx, script)
		if err != nil || json.Unmarshal(data, &state) != nil {
			http.Error(w, "无法读取此 Claude Desktop 实例的状态", http.StatusBadGateway)
			return
		}
		jsonReply(w, state)
	case r.Method == http.MethodGet && action == "vnc-credentials":
		script := "set -eu\nsystemctl is-active --quiet " + slot.unit() +
			"\ncat " + slot.home() + "/.config/claude-cage/desktop-password\n" +
			"printf '\\n'\ncat " + slot.runtime() + "/base-display-scale\n" +
			"if test -f " + slot.runtime() + "/xsettings.pid; then echo dynamic; fi"
		data, err := s.guestQuick(ctx, script)
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if err != nil || len(lines) < 2 || len(lines[0]) != 8 {
			http.Error(w, "此 Claude Desktop 实例尚未启动", http.StatusConflict)
			return
		}
		density := 2.0
		if lines[1] == "1.5" {
			density = 1.5
		}
		jsonReply(w, map[string]any{"password": lines[0], "display_scale": density, "dynamic_scale": len(lines) == 3 && lines[2] == "dynamic"})
	case r.Method == http.MethodPost && action == "resize":
		var input struct {
			Width  int     `json:"width"`
			Height int     `json:"height"`
			Scale  float64 `json:"scale"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if input.Width < 320 || input.Height < 240 || input.Width > 8192 || input.Height > 8192 || !validDisplayDensity(input.Scale) {
			http.Error(w, "画面尺寸无效", http.StatusBadRequest)
			return
		}
		scale := ""
		if input.Scale != 0 {
			scale = strconv.FormatFloat(input.Scale, 'f', -1, 64)
		}
		data, err := s.guestQuick(ctx, numberedDesktopResizeScript(slot, input.Width, input.Height, scale))
		if err != nil {
			http.Error(w, "无法调整此 Claude Desktop 实例的画面", http.StatusBadGateway)
			return
		}
		reply := map[string]any{"ok": true}
		if first, _, _ := strings.Cut(string(data), "\n"); scale != "" && strings.TrimSpace(first) == "scale "+scale {
			reply["scale"] = input.Scale
		}
		jsonReply(w, reply)
	case r.Method == http.MethodPost && action == "input":
		var input browserInput
		if !readJSON(w, r, &input, 65536) {
			return
		}
		if input.Target != "desktop" || input.ID != "desktop" || len(input.Text) > 16384 {
			http.Error(w, "输入目标或文字无效", http.StatusBadRequest)
			return
		}
		body, _ := json.Marshal(input)
		command := "exec env HOME=" + slot.home() + " /usr/local/bin/cage-native-input --request"
		command, err := s.linuxTemplate(command)
		if err != nil {
			http.Error(w, "客机输入配置无效", http.StatusConflict)
			return
		}
		cmd := s.sshCommand(ctx, "-T", s.sshTarget(), "--", command)
		cmd.Stdin = bytes.NewReader(append(body, '\n'))
		data, err := cmd.Output()
		if err != nil || len(data) > 32768 || !json.Valid(data) {
			if input.Mode == "status" {
				jsonReply(w, map[string]bool{"available": false})
				return
			}
			http.Error(w, "此实例的输入会话已断开，请重新点击输入框", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) changeDesktopInstance(slot desktopSlot, action string) error {
	if action == "stop" {
		return s.runDesktopScript("sudo systemctl stop "+slot.unit(), 30*time.Second)
	}
	s.setMessage("正在打开独立 Claude Desktop 实例…")
	key, err := s.account.desktopKey()
	if err != nil {
		return err
	}
	defer clear(key)
	provision := `test "$(dpkg-query -W -f='${Status}' claude-desktop 2>/dev/null)" = 'install ok installed' || { echo '请先从默认实例安装 Claude Desktop。'; exit 1; }
` + cage.ClaudeDesktopProvisionScript()
	if err = s.runDesktopScript(provision, 20*time.Second); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.operationContext(), 55*time.Second)
	defer cancel()
	cmd := s.sshCommand(ctx, "-T", s.sshTarget(), "--", "/usr/local/bin/cage-claude-start "+string(slot))
	cmd.Stdin = bytes.NewReader(key)
	cmd.Stdout, cmd.Stderr = s.jobs.execution(), s.jobs.execution()
	if err = cmd.Run(); err != nil {
		return executionFailure("打开独立 Claude Desktop", err, s.jobs.snapshot().Log)
	}
	return nil
}
