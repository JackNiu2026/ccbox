package host

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The CIM classes below are readable without administrator rights. Any query
// that fails leaves its field null, and classification treats it as unknown.
const virtualizationFactsScript = `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$system = Get-CimInstance Win32_ComputerSystem
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$feature = Get-CimInstance Win32_OptionalFeature -Filter "Name='HypervisorPlatform'"
$featureEnabled = $null
if ($feature) { $featureEnabled = ($feature.InstallState -eq 1) }
$pending = (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or
    (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')
[pscustomobject]@{
    featureEnabled = $featureEnabled
    hypervisorPresent = $system.HypervisorPresent
    firmwareEnabled = $cpu.VirtualizationFirmwareEnabled
    rebootPending = [bool]$pending
    manufacturer = [string]$system.Manufacturer
    model = [string]$system.Model
    cpuVendor = [string]$cpu.Manufacturer
} | ConvertTo-Json -Compress
`

type windowsFactsJSON struct {
	FeatureEnabled    *bool  `json:"featureEnabled"`
	HypervisorPresent *bool  `json:"hypervisorPresent"`
	FirmwareEnabled   *bool  `json:"firmwareEnabled"`
	RebootPending     bool   `json:"rebootPending"`
	Manufacturer      string `json:"manufacturer"`
	Model             string `json:"model"`
	CPUVendor         string `json:"cpuVendor"`
}

func windowsVirtualization(ctx context.Context) VirtualizationReport {
	facts := WindowsVirtualizationFacts{Windows11: windows.RtlGetVersion().BuildNumber >= 22000}
	proc := windows.NewLazySystemDLL("WinHvPlatform.dll").NewProc("WHvGetCapability")
	if proc.Find() == nil {
		facts.PlatformInstalled = true
		var written uint32
		facts.QueryStatus, _, _ = proc.Call(0, uintptr(unsafe.Pointer(&facts.QueryValue)), unsafe.Sizeof(facts.QueryValue), uintptr(unsafe.Pointer(&written)))
		if whvHypervisorPresent(facts.QueryStatus, facts.QueryValue) == nil {
			return readyReport()
		}
	}
	queryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if out, err := hiddenPowerShell(queryCtx, virtualizationFactsScript).Output(); err == nil {
		var parsed windowsFactsJSON
		if json.Unmarshal(out, &parsed) == nil {
			facts.FeatureEnabled, facts.HypervisorPresent, facts.FirmwareEnabled = parsed.FeatureEnabled, parsed.HypervisorPresent, parsed.FirmwareEnabled
			facts.RebootPending, facts.Manufacturer, facts.Model, facts.CPUVendor = parsed.RebootPending, parsed.Manufacturer, parsed.Model, parsed.CPUVendor
		}
	}
	return ClassifyWindowsVirtualization(facts)
}

func systemTool(name string) string {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		dir = `C:\Windows\System32`
	}
	return filepath.Join(dir, name)
}

func hiddenPowerShell(ctx context.Context, script string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, systemTool(`WindowsPowerShell\v1.0\powershell.exe`), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	HideCommandWindow(cmd)
	return cmd
}

// Elevated scripts are fixed text: nothing from the request reaches them.
// Exit codes: 0 done, 21 DISM failed, 22 bcdedit failed, 23 firmware restart
// unsupported.
const enableVirtualizationScript = `
$ErrorActionPreference = 'Stop'
$system = [Environment]::GetFolderPath('System')
& (Join-Path $system 'dism.exe') /Online /Enable-Feature /FeatureName:HypervisorPlatform /All /NoRestart /Quiet | Out-Null
if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne 3010) { exit 21 }
& (Join-Path $system 'bcdedit.exe') /set '{current}' hypervisorlaunchtype auto | Out-Null
if ($LASTEXITCODE -ne 0) { exit 22 }
exit 0
`

const firmwareRestartScript = `
$ErrorActionPreference = 'Stop'
& (Join-Path ([Environment]::GetFolderPath('System')) 'shutdown.exe') /r /fw /t 15 /c 'CCBox：即将重启并进入 BIOS/UEFI 设置' | Out-Null
if ($LASTEXITCODE -ne 0) { exit 23 }
exit 0
`

// The ordinary process starts the elevated one and returns its exit code;
// 48 means the user declined the Windows authorization prompt.
func elevatedLauncher(script string) string {
	return `
$ErrorActionPreference = 'Stop'
try {
    $process = Start-Process -FilePath (Join-Path ([Environment]::GetFolderPath('System')) 'WindowsPowerShell\v1.0\powershell.exe') ` +
		`-ArgumentList @('-NoLogo','-NoProfile','-NonInteractive','-EncodedCommand','` + encodePowerShell(script) + `') -Verb RunAs -WindowStyle Hidden -PassThru
} catch {
    for ($cause = $_.Exception; $cause; $cause = $cause.InnerException) {
        if ($cause -is [ComponentModel.Win32Exception] -and $cause.NativeErrorCode -eq 1223) { exit 48 }
    }
    exit 47
}
try {
    # Keep the native handle before waiting; Windows PowerShell 5.1 can
    # otherwise report a null exit code.
    $null = $process.Handle
    $process.WaitForExit()
    if ($null -eq $process.ExitCode) { exit 1 }
    exit [int]$process.ExitCode
} finally { $process.Dispose() }
`
}

var virtualizationActionMu sync.Mutex

var errVirtualizationBusy = errors.New("另一个虚拟化设置操作正在进行，请稍后再试")

// EnableWindowsVirtualization enables Windows Hypervisor Platform and restores
// the hypervisor launch setting after the user approves the Windows prompt.
// Windows applies both at the next restart.
func EnableWindowsVirtualization(ctx context.Context) error {
	return runElevated(ctx, enableVirtualizationScript)
}

// RestartIntoFirmware restarts into the BIOS/UEFI settings after approval.
func RestartIntoFirmware(ctx context.Context) error {
	return runElevated(ctx, firmwareRestartScript)
}

// RestartWindows schedules an ordinary restart; the user can still cancel it
// with "shutdown /a" during the countdown.
func RestartWindows(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, systemTool("shutdown.exe"), "/r", "/t", "15", "/c", "CCBox：即将重启以完成虚拟化设置")
	HideCommandWindow(cmd)
	if err := cmd.Run(); err != nil {
		return errors.New("无法发起重启，请手动重启电脑")
	}
	return nil
}

func runElevated(ctx context.Context, script string) error {
	if !virtualizationActionMu.TryLock() {
		return errVirtualizationBusy
	}
	defer virtualizationActionMu.Unlock()
	// DISM can take several minutes on a slow disk.
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	err := hiddenPowerShell(runCtx, elevatedLauncher(script)).Run()
	var exited *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exited):
		return elevatedExitError(exited.ExitCode())
	case runCtx.Err() != nil:
		return errors.New("虚拟化设置超时未完成，请重启电脑后再检查")
	default:
		return errors.New("无法启动 Windows 系统工具：" + strings.TrimSpace(err.Error()))
	}
}
