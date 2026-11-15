package host

import (
	"context"
	"os"
	"runtime"
	"strings"
)

// Virtualization states reported before a Linux guest is installed. Each one
// maps to a different remedy, so the user is never sent to firmware settings
// when a Windows feature or a restart is all that is missing.
const (
	VirtReady           = "ready"
	VirtFeatureOff      = "feature-off"    // Windows Hypervisor Platform is not enabled
	VirtRebootPending   = "reboot-pending" // a feature change waits for a restart
	VirtHypervisorOff   = "hypervisor-off" // enabled, but the hypervisor did not start
	VirtFirmwareOff     = "firmware-off"   // CPU virtualization is disabled in firmware
	VirtNestedVM        = "nested-vm"      // this computer is itself a virtual machine
	VirtQueryFailed     = "query-failed"   // Windows did not answer the status query
	VirtKVMMissing      = "kvm-missing"    // Linux: /dev/kvm does not exist
	VirtKVMPermission   = "kvm-permission" // Linux: /dev/kvm exists but is not usable
	VirtWSL             = "wsl"            // Linux inside WSL without nested virtualization
	firmwareAdvancedW11 = "Windows 11：设置 → 系统 → 恢复 → 高级启动「立即重新启动」→ 疑难解答 → 高级选项 → UEFI 固件设置"
	firmwareAdvancedW10 = "Windows 10：设置 → 更新和安全 → 恢复 → 高级启动「立即重新启动」→ 疑难解答 → 高级选项 → UEFI 固件设置"
)

// FirmwareGuide tells the user how to reach the CPU virtualization switch.
type FirmwareGuide struct {
	Vendor   string   `json:"vendor,omitempty"`
	Keys     string   `json:"keys"`
	Option   string   `json:"option"`
	Location string   `json:"location,omitempty"`
	Windows  []string `json:"windows,omitempty"`
}

// VirtualizationReport is what the installer shows before downloading a
// Linux guest image.
type VirtualizationReport struct {
	State   string   `json:"state"`
	Ready   bool     `json:"ready"`
	Summary string   `json:"summary"`
	Steps   []string `json:"steps,omitempty"`
	// CanEnable offers the one-click fix: enable Windows Hypervisor Platform
	// and restore the hypervisor launch setting, with administrator consent.
	CanEnable bool `json:"can_enable,omitempty"`
	// NeedsRestart offers a restart after a change that Windows applies at boot.
	NeedsRestart bool `json:"needs_restart,omitempty"`
	// FirmwareRestart offers restarting straight into firmware settings.
	FirmwareRestart bool           `json:"firmware_restart,omitempty"`
	Firmware        *FirmwareGuide `json:"firmware,omitempty"`
	Detail          string         `json:"detail,omitempty"`
}

// CheckVirtualization reports whether this host can run a Linux guest.
func CheckVirtualization(ctx context.Context) VirtualizationReport {
	switch runtime.GOOS {
	case "windows":
		return windowsVirtualization(ctx)
	case "linux":
		_, statErr := os.Stat("/dev/kvm")
		return ClassifyLinuxVirtualization(LinuxVirtualizationFacts{
			KVMExists: statErr == nil, KVMUsable: statErr == nil && canReadWrite("/dev/kvm"),
			InKVMGroup: inGroup("kvm"), WSL: WSLVersion(),
		})
	default:
		return readyReport()
	}
}

func readyReport() VirtualizationReport {
	return VirtualizationReport{State: VirtReady, Ready: true, Summary: "这台电脑的虚拟化已就绪。"}
}

// WindowsVirtualizationFacts are gathered without administrator rights.
// Pointer fields are nil when Windows did not answer that query.
type WindowsVirtualizationFacts struct {
	PlatformInstalled bool    // WinHvPlatform.dll is loadable
	QueryStatus       uintptr // HRESULT of WHvGetCapability(HypervisorPresent)
	QueryValue        uint64
	FeatureEnabled    *bool // Win32_OptionalFeature HypervisorPlatform
	HypervisorPresent *bool // Win32_ComputerSystem.HypervisorPresent
	FirmwareEnabled   *bool // Win32_Processor.VirtualizationFirmwareEnabled
	RebootPending     bool
	Manufacturer      string
	Model             string
	CPUVendor         string // Win32_Processor.Manufacturer
	Windows11         bool
}

// ClassifyWindowsVirtualization turns the gathered facts into one remedy.
// The order matters: a running hypervisor wins, then conditions the user
// cannot fix from Windows, then the ones a restart or the one-click fix
// resolves.
func ClassifyWindowsVirtualization(f WindowsVirtualizationFacts) VirtualizationReport {
	queryErr := error(nil)
	if f.PlatformInstalled {
		queryErr = whvHypervisorPresent(f.QueryStatus, f.QueryValue)
		if queryErr == nil {
			return readyReport()
		}
	}
	featureOff := !f.PlatformInstalled || (f.FeatureEnabled != nil && !*f.FeatureEnabled)
	guide := firmwareGuide(f.Manufacturer, f.CPUVendor, f.Windows11)
	switch {
	case isVirtualMachine(f.Manufacturer, f.Model):
		return VirtualizationReport{State: VirtNestedVM,
			Summary: "这台电脑本身是虚拟机或云电脑，宿主没有为它开放嵌套虚拟化。",
			Steps: []string{
				"在宿主或云平台为这台虚拟机开启嵌套虚拟化（Hyper-V：Set-VMProcessor -ExposeVirtualizationExtensions $true；VMware：勾选“虚拟化 Intel VT-x/EPT 或 AMD-V/RVI”）。",
				"无法修改宿主设置时，请在实体电脑上安装 CCBox，或使用云端部署（通过 SSH 连接 Linux 服务器）。",
			},
			CanEnable: featureOff}
	// Windows reports the firmware switch as off whenever a hypervisor is
	// already running, so it is only meaningful without one.
	case f.FirmwareEnabled != nil && !*f.FirmwareEnabled && (f.HypervisorPresent == nil || !*f.HypervisorPresent):
		steps := []string{
			"重启电脑并进入 BIOS/UEFI 设置：" + guide.Keys + "。",
			"找到「" + guide.Option + "」，改为 Enabled（启用），保存并退出（通常按 F10）。",
		}
		if guide.Location != "" {
			steps[1] = "在「" + guide.Location + "」中找到「" + guide.Option + "」，改为 Enabled（启用），保存并退出（通常按 F10）。"
		}
		if featureOff {
			steps = append(steps, "回到 Windows 后，点击「一键启用」开启 Windows 虚拟机监控程序平台，并按提示再重启一次。")
		}
		return VirtualizationReport{State: VirtFirmwareOff,
			Summary: "这台电脑的 CPU 虚拟化在 BIOS/UEFI 中处于关闭状态，需要先在固件设置中开启。",
			Steps:   steps, Firmware: &guide, FirmwareRestart: true, CanEnable: featureOff}
	case f.RebootPending && !(f.FeatureEnabled != nil && !*f.FeatureEnabled):
		return VirtualizationReport{State: VirtRebootPending,
			Summary:      "Windows 有尚未生效的功能或更新变更，重启后虚拟化才能启用。",
			Steps:        []string{"保存其他程序中的工作，点击「立即重启」或手动重启电脑。", "重启后回到 CCBox，重新选择系统并继续安装。"},
			NeedsRestart: true}
	case featureOff:
		return VirtualizationReport{State: VirtFeatureOff,
			Summary: "Windows 虚拟机监控程序平台（Windows Hypervisor Platform）尚未启用。",
			Steps: []string{
				"点击「一键启用」，在 Windows 弹出的管理员授权窗口中选择“是”。",
				"启用完成后重启电脑，回到 CCBox 继续安装。",
				"也可以手动启用：控制面板 → 程序 → 启用或关闭 Windows 功能 → 勾选「Windows 虚拟机监控程序平台」→ 确定并重启。",
			},
			CanEnable: true}
	case queryErr != nil && !errorsIsNotRunning(queryErr):
		return VirtualizationReport{State: VirtQueryFailed,
			Summary: "CCBox 无法读取 Windows 的虚拟化状态。",
			Steps: []string{
				"点击「一键启用」确认相关设置，然后重启电脑后重试。",
				"仍然失败时，在设置 → 日志与诊断中导出诊断包，并附上下方的错误代码。",
			},
			CanEnable: true, Detail: queryErr.Error()}
	default:
		return VirtualizationReport{State: VirtHypervisorOff,
			Summary: "Windows 虚拟机监控程序平台已启用，但虚拟化没有随系统启动。",
			Steps: []string{
				"点击「一键启用」恢复系统启动时加载虚拟化（部分安卓模拟器、旧版 VirtualBox 或 VMware 会关闭这一设置），然后重启电脑。",
				"重启后仍然如此时，检查安全软件是否占用了硬件虚拟化，例如 360 的「核晶防护」，关闭后再重启。",
				"如果以上都不是，可能是 BIOS/UEFI 中的 CPU 虚拟化未开启：" + guide.Keys + "，找到「" + guide.Option + "」并启用。",
			},
			CanEnable: true, Firmware: &guide}
	}
}

func errorsIsNotRunning(err error) bool { return err == errHypervisorNotRunning }

var virtualMachineMarkers = []string{"vmware", "virtualbox", "innotek", "qemu", "kvm", "xen", "parallels", "bochs",
	"amazon ec2", "google compute engine", "alibaba cloud", "tencent cloud", "openstack"}

func isVirtualMachine(manufacturer, model string) bool {
	m := strings.ToLower(manufacturer + " " + model)
	if strings.Contains(strings.ToLower(manufacturer), "microsoft") && strings.Contains(strings.ToLower(model), "virtual machine") {
		return true
	}
	for _, marker := range virtualMachineMarkers {
		if strings.Contains(m, marker) {
			return true
		}
	}
	return false
}

type vendorFirmware struct {
	match    []string
	name     string
	keys     string
	location string
}

// Common keys and menus; the exact layout varies by model and BIOS version.
var firmwareVendors = []vendorFirmware{
	{[]string{"lenovo"}, "联想", "开机出现标志时连续按 F2（部分机型为 Fn+F2）；ThinkPad 和台式机按 F1；也可在关机状态按机身的 Novo 小孔选择 BIOS Setup", "Configuration 或 Security → Virtualization"},
	{[]string{"dell"}, "戴尔", "开机出现 Dell 标志时连续按 F2", "Virtualization Support"},
	{[]string{"hp", "hewlett"}, "惠普", "开机时连续按 Esc 打开启动菜单，再按 F10 进入 BIOS 设置", "Advanced → System Options，或 Security → System Security"},
	{[]string{"asus"}, "华硕", "开机时连续按 F2（台式机主板多为 Del）", "Advanced → CPU Configuration"},
	{[]string{"acer"}, "宏碁", "开机时连续按 F2", "Main 或 Advanced"},
	{[]string{"micro-star", "msi"}, "微星", "开机时连续按 Del", "OC → CPU Features，或 Overclocking → Advanced CPU Configuration"},
	{[]string{"gigabyte"}, "技嘉", "开机时连续按 Del", "Tweaker → Advanced CPU Settings，或 Settings → Miscellaneous"},
	{[]string{"asrock"}, "华擎", "开机时连续按 F2 或 Del", "Advanced → CPU Configuration"},
	{[]string{"huawei"}, "华为", "开机出现标志时连续按 F2", ""},
	{[]string{"honor"}, "荣耀", "开机出现标志时连续按 F2", ""},
	{[]string{"xiaomi", "timi", "redmi"}, "小米", "开机出现标志时连续按 F2", ""},
	{[]string{"microsoft"}, "微软 Surface", "关机后按住音量加键，再按电源键，看到 Surface 标志后松开音量键", ""},
	{[]string{"mechrevo"}, "机械革命", "开机时连续按 F2", ""},
	{[]string{"hasee"}, "神舟", "开机时连续按 F2", ""},
	{[]string{"thunderobot"}, "雷神", "开机时连续按 F2", ""},
}

func firmwareGuide(manufacturer, cpuVendor string, windows11 bool) FirmwareGuide {
	option := "Intel Virtualization Technology（也可能写作 Intel VT、VT-x）或 SVM Mode（AMD 处理器，也可能写作 AMD-V）"
	switch cpu := strings.ToLower(cpuVendor); {
	case strings.Contains(cpu, "intel"):
		option = "Intel Virtualization Technology（也可能写作 Intel VT、VT-x）"
	case strings.Contains(cpu, "amd"):
		option = "SVM Mode（也可能写作 AMD-V、SVM、Virtualization）"
	}
	alternative := firmwareAdvancedW10
	if windows11 {
		alternative = firmwareAdvancedW11
	}
	guide := FirmwareGuide{Keys: "开机时连续按 Del 或 F2（品牌机也常用 F1、F10 或 Esc）", Option: option, Windows: []string{alternative}}
	m := strings.ToLower(manufacturer)
	for _, vendor := range firmwareVendors {
		for _, match := range vendor.match {
			if matchesVendor(m, match) {
				guide.Vendor, guide.Keys, guide.Location = vendor.name, vendor.keys, vendor.location
				return guide
			}
		}
	}
	return guide
}

// matchesVendor avoids "hp" matching inside unrelated words.
func matchesVendor(manufacturer, match string) bool {
	if len(match) > 3 {
		return strings.Contains(manufacturer, match)
	}
	for _, field := range strings.FieldsFunc(manufacturer, func(r rune) bool { return r < 'a' || r > 'z' }) {
		if field == match {
			return true
		}
	}
	return false
}

// LinuxVirtualizationFacts mirror the checks LinuxPrereqs performs at start.
type LinuxVirtualizationFacts struct {
	KVMExists  bool
	KVMUsable  bool
	InKVMGroup bool
	WSL        int
}

func ClassifyLinuxVirtualization(f LinuxVirtualizationFacts) VirtualizationReport {
	switch {
	case f.KVMExists && f.KVMUsable:
		return readyReport()
	case f.KVMExists && !f.InKVMGroup:
		return VirtualizationReport{State: VirtKVMPermission,
			Summary: "当前用户没有使用 KVM 虚拟化的权限。",
			Steps:   []string{"在终端运行：sudo usermod -aG kvm \"$USER\"", "注销并重新登录（或重启电脑）后，回到 CCBox 继续安装。"}}
	case f.KVMExists:
		return VirtualizationReport{State: VirtKVMPermission,
			Summary: "当前用户已在 kvm 组中，但仍无法访问 /dev/kvm。",
			Steps:   []string{"在终端运行 ls -l /dev/kvm 检查设备权限，确认属组为 kvm 且组可读写。", "修改后注销并重新登录，再回到 CCBox 继续安装。"}}
	case f.WSL == 1:
		return VirtualizationReport{State: VirtWSL,
			Summary: "当前运行在 WSL1 中，WSL1 无法运行虚拟机。",
			Steps:   []string{"在 Windows 中运行：wsl --set-version <发行版名称> 2，把发行版升级到 WSL2。"}}
	case f.WSL == 2:
		return VirtualizationReport{State: VirtWSL,
			Summary: "当前运行在 WSL2 中，但 Windows 没有向它开放嵌套虚拟化（KVM）。",
			Steps:   []string{"在 Windows 中运行 wsl --update 更新 WSL，并确认 Windows 主机已开启虚拟化。", "也可以直接安装 Windows 版 CCBox。"}}
	default:
		guide := firmwareGuide("", "", false)
		guide.Windows = nil
		return VirtualizationReport{State: VirtKVMMissing,
			Summary: "未找到 KVM 设备（/dev/kvm），CPU 虚拟化可能在 BIOS/UEFI 中关闭，或当前系统本身是没有开放嵌套虚拟化的虚拟机。",
			Steps: []string{
				"重启电脑并进入 BIOS/UEFI 设置：" + guide.Keys + "，找到「" + guide.Option + "」并启用。",
				"在终端运行 sudo modprobe kvm_intel（AMD 处理器为 kvm_amd），确认内核已加载 KVM 模块。",
				"如果这台电脑是虚拟机，请在宿主上为它开启嵌套虚拟化。",
			},
			Firmware: &guide}
	}
}
