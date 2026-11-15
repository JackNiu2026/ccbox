package host

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func boolPtr(v bool) *bool { return &v }

func TestClassifyWindowsVirtualization(t *testing.T) {
	const notPresent, present = uint64(0), uint64(1)
	physical := WindowsVirtualizationFacts{PlatformInstalled: true, QueryValue: notPresent,
		FeatureEnabled: boolPtr(true), HypervisorPresent: boolPtr(false), FirmwareEnabled: boolPtr(true),
		Manufacturer: "LENOVO", Model: "21D6", CPUVendor: "GenuineIntel", Windows11: true}
	with := func(change func(*WindowsVirtualizationFacts)) WindowsVirtualizationFacts {
		f := physical
		change(&f)
		return f
	}
	for _, tc := range []struct {
		name      string
		facts     WindowsVirtualizationFacts
		state     string
		canEnable bool
		firmware  bool
	}{
		{"running", with(func(f *WindowsVirtualizationFacts) { f.QueryValue = present }), VirtReady, false, false},
		{"feature missing", with(func(f *WindowsVirtualizationFacts) { f.PlatformInstalled, f.FeatureEnabled = false, boolPtr(false) }), VirtFeatureOff, true, false},
		{"feature reported off", with(func(f *WindowsVirtualizationFacts) { f.FeatureEnabled = boolPtr(false) }), VirtFeatureOff, true, false},
		{"feature off beats unrelated pending update", with(func(f *WindowsVirtualizationFacts) { f.FeatureEnabled, f.RebootPending = boolPtr(false), true }), VirtFeatureOff, true, false},
		{"enabled, waiting for restart", with(func(f *WindowsVirtualizationFacts) {
			f.PlatformInstalled, f.FeatureEnabled, f.RebootPending = false, nil, true
		}), VirtRebootPending, false, false},
		{"firmware off", with(func(f *WindowsVirtualizationFacts) { f.FirmwareEnabled = boolPtr(false) }), VirtFirmwareOff, false, true},
		{"firmware and feature off", with(func(f *WindowsVirtualizationFacts) { f.FirmwareEnabled, f.PlatformInstalled = boolPtr(false), false }), VirtFirmwareOff, true, true},
		{"firmware flag ignored while a hypervisor runs", with(func(f *WindowsVirtualizationFacts) {
			f.FirmwareEnabled, f.HypervisorPresent, f.QueryStatus = boolPtr(false), boolPtr(true), 0x80370301
		}), VirtQueryFailed, true, false},
		{"launch disabled or VT-x taken", physical, VirtHypervisorOff, true, false},
		{"query rejected", with(func(f *WindowsVirtualizationFacts) { f.QueryStatus = 0x80370301 }), VirtQueryFailed, true, false},
		{"Hyper-V guest", with(func(f *WindowsVirtualizationFacts) {
			f.Manufacturer, f.Model, f.FirmwareEnabled = "Microsoft Corporation", "Virtual Machine", boolPtr(false)
		}), VirtNestedVM, false, false},
		{"VMware guest", with(func(f *WindowsVirtualizationFacts) { f.Manufacturer, f.Model = "VMware, Inc.", "VMware20,1" }), VirtNestedVM, false, false},
		{"Surface is not a VM", with(func(f *WindowsVirtualizationFacts) {
			f.Manufacturer, f.Model = "Microsoft Corporation", "Surface Laptop 5"
		}), VirtHypervisorOff, true, false},
		{"facts unavailable", WindowsVirtualizationFacts{PlatformInstalled: true}, VirtHypervisorOff, true, false},
	} {
		r := ClassifyWindowsVirtualization(tc.facts)
		if r.State != tc.state || r.Ready != (tc.state == VirtReady) || r.CanEnable != tc.canEnable || r.FirmwareRestart != tc.firmware {
			t.Fatalf("%s: got state=%s ready=%v canEnable=%v firmwareRestart=%v", tc.name, r.State, r.Ready, r.CanEnable, r.FirmwareRestart)
		}
		if !r.Ready && (r.Summary == "" || len(r.Steps) == 0) {
			t.Fatalf("%s: a blocked host needs a summary and steps: %+v", tc.name, r)
		}
	}
	if r := ClassifyWindowsVirtualization(with(func(f *WindowsVirtualizationFacts) { f.QueryStatus = 0x80370301 })); !strings.Contains(r.Detail, "0x80370301") {
		t.Fatalf("query failure lost its HRESULT: %+v", r)
	}
	if r := ClassifyWindowsVirtualization(physical); strings.Contains(r.Summary, "固件") {
		t.Fatalf("an enabled-but-not-running hypervisor must not lead with firmware: %s", r.Summary)
	}
}

func TestFirmwareGuide(t *testing.T) {
	for _, tc := range []struct{ manufacturer, cpu, vendor, keyword, option string }{
		{"LENOVO", "GenuineIntel", "联想", "F2", "Intel Virtualization Technology"},
		{"Dell Inc.", "AuthenticAMD", "戴尔", "F2", "SVM Mode"},
		{"HP", "GenuineIntel", "惠普", "F10", "Intel"},
		{"Hewlett-Packard", "", "惠普", "F10", "Intel Virtualization Technology（也可能写作 Intel VT、VT-x）或 SVM Mode"},
		{"ASUSTeK COMPUTER INC.", "AuthenticAMD", "华硕", "F2", "SVM"},
		{"Micro-Star International Co., Ltd.", "GenuineIntel", "微星", "Del", "Intel"},
		{"Shipping Co", "GenuineIntel", "", "Del 或 F2", "Intel"}, // "hp" must not match inside a word
		{"System manufacturer", "AuthenticAMD", "", "Del 或 F2", "SVM"},
	} {
		g := firmwareGuide(tc.manufacturer, tc.cpu, true)
		if g.Vendor != tc.vendor || !strings.Contains(g.Keys, tc.keyword) || !strings.Contains(g.Option, tc.option) {
			t.Fatalf("%q: %+v", tc.manufacturer, g)
		}
		if len(g.Windows) != 1 || !strings.Contains(g.Windows[0], "UEFI 固件设置") {
			t.Fatalf("%q: no Windows advanced-startup path: %+v", tc.manufacturer, g)
		}
	}
	if !strings.HasPrefix(firmwareGuide("", "", false).Windows[0], "Windows 10") {
		t.Fatal("Windows 10 hosts need the Windows 10 settings path")
	}
}

func TestClassifyLinuxVirtualization(t *testing.T) {
	for _, tc := range []struct {
		facts LinuxVirtualizationFacts
		state string
		step  string
	}{
		{LinuxVirtualizationFacts{KVMExists: true, KVMUsable: true}, VirtReady, ""},
		{LinuxVirtualizationFacts{KVMExists: true}, VirtKVMPermission, "usermod -aG kvm"},
		{LinuxVirtualizationFacts{KVMExists: true, InKVMGroup: true}, VirtKVMPermission, "ls -l /dev/kvm"},
		{LinuxVirtualizationFacts{WSL: 1}, VirtWSL, "wsl --set-version"},
		{LinuxVirtualizationFacts{WSL: 2}, VirtWSL, "wsl --update"},
		{LinuxVirtualizationFacts{}, VirtKVMMissing, "modprobe"},
	} {
		r := ClassifyLinuxVirtualization(tc.facts)
		if r.State != tc.state || r.Ready != (tc.state == VirtReady) || (tc.step != "" && !strings.Contains(strings.Join(r.Steps, "\n"), tc.step)) {
			t.Fatalf("%+v: %+v", tc.facts, r)
		}
		if r.CanEnable || r.FirmwareRestart || r.NeedsRestart {
			t.Fatalf("Linux hosts offer no Windows actions: %+v", r)
		}
	}
}

func TestElevatedExitError(t *testing.T) {
	if elevatedExitError(0) != nil {
		t.Fatal("success reported as failure")
	}
	for code, want := range map[int]string{48: "已取消", 47: "授权窗口", 21: "手动启用", 22: "hypervisorlaunchtype", 23: "按键", 5: "退出码 5"} {
		if err := elevatedExitError(code); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("exit %d: %v", code, err)
		}
	}
}

func TestEncodePowerShell(t *testing.T) {
	script := "Write-Output '虚拟化 ok'"
	raw, err := base64.StdEncoding.DecodeString(encodePowerShell(script))
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("not base64 UTF-16: %v", err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	if string(utf16.Decode(units)) != script {
		t.Fatal("round trip changed the script")
	}
}
