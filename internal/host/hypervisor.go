package host

import (
	"errors"
	"fmt"
)

var errHypervisorNotRunning = errors.New("Windows 虚拟化尚未就绪：请在固件中启用 CPU 虚拟化，启用 Windows Hypervisor Platform 并重启")

// whvHypervisorPresent interprets WHvGetCapability(WHvCapabilityCodeHypervisorPresent).
//
// Microsoft documents the output buffer as large enough for a 64-bit value;
// the capability itself is a Windows BOOL in its first four bytes. A failed
// query is reported as such rather than as disabled virtualization, so a
// working host is never told to change its firmware settings.
func whvHypervisorPresent(status uintptr, value uint64) error {
	if hresult := uint32(status); int32(hresult) < 0 {
		return fmt.Errorf("无法查询 Windows 虚拟化状态（HRESULT %#08x）：请确认已启用 Windows Hypervisor Platform（Windows 虚拟机监控程序平台）并重启；仍失败时请在设置中导出诊断包", hresult)
	}
	if uint32(value) == 0 {
		return errHypervisorNotRunning
	}
	return nil
}
