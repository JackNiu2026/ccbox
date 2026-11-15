package workbench

import (
	"context"
	"net/http"
	"runtime"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/host"
)

// Replaced in tests: build hosts have no KVM and are not Windows.
var (
	checkVirtualization  = host.CheckVirtualization
	enableVirtualization = host.EnableWindowsVirtualization
	restartIntoFirmware  = host.RestartIntoFirmware
	restartWindows       = host.RestartWindows
	windowsHost          = runtime.GOOS == "windows"
)

// virtualizationBlocked checks a host before a Linux guest image is
// downloaded. Existing environments keep their own start-time checks.
func virtualizationBlocked(ctx context.Context) (host.VirtualizationReport, bool) {
	report := checkVirtualization(ctx)
	return report, !report.Ready
}

func writeVirtualizationBlocked(w http.ResponseWriter, report host.VirtualizationReport) {
	http.Error(w, report.Summary+"请按客户机系统选项下方的提示处理后再安装。", http.StatusConflict)
}

// freshLinuxInstall reports whether starting now would create the first
// environment, which is when a Linux guest image is downloaded.
func (s *Server) freshLinuxInstall() bool {
	loc, err := cage.GetStorageLocation(s.profile)
	return err == nil && !loc.HasEnvironment
}

func (s *Server) virtualization(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.hasTrustedView(r) || !s.localVMStorage() {
		http.Error(w, "请在本机 CCBox 窗口检查虚拟化", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		jsonReply(w, checkVirtualization(r.Context()))
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if !readJSON(w, r, &input, 256) {
		return
	}
	if !windowsHost {
		http.Error(w, "此操作仅适用于 Windows", http.StatusConflict)
		return
	}
	report := checkVirtualization(r.Context())
	var err error
	switch input.Action {
	case "enable":
		if !report.CanEnable {
			http.Error(w, "当前状态无需启用，请按页面提示处理", http.StatusConflict)
			return
		}
		err = enableVirtualization(r.Context())
	case "restart", "firmware":
		if s.busy() {
			http.Error(w, "有任务正在运行，请等待完成或停止后再重启", http.StatusConflict)
			return
		}
		if input.Action == "firmware" {
			if !report.FirmwareRestart && report.Firmware == nil {
				http.Error(w, "当前状态无需进入固件设置", http.StatusConflict)
				return
			}
			err = restartIntoFirmware(r.Context())
		} else {
			err = restartWindows(r.Context())
		}
	default:
		http.Error(w, "未知的虚拟化操作", http.StatusBadRequest)
		return
	}
	if err != nil {
		requestError(w, err)
		return
	}
	if input.Action != "enable" {
		jsonReply(w, map[string]bool{"restarting": true})
		return
	}
	// Windows applies the feature and launch setting at the next restart.
	after := checkVirtualization(r.Context())
	if !after.Ready {
		after = host.VirtualizationReport{State: host.VirtRebootPending,
			Summary: "设置已完成，重启电脑后生效。", NeedsRestart: true,
			Steps: []string{"保存其他程序中的工作，点击「立即重启」或手动重启电脑。", "重启后回到 CCBox，重新选择系统并继续安装。"}}
	}
	jsonReply(w, after)
}
