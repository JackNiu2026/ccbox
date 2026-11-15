package host

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

// encodePowerShell produces the UTF-16LE Base64 form -EncodedCommand takes.
func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 2*len(units))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(raw[2*i:], unit)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// elevatedExitError explains the exit code of an elevated virtualization
// action. It is platform-independent so the mapping is tested everywhere.
func elevatedExitError(code int) error {
	switch code {
	case 0:
		return nil
	case 48:
		return errors.New("已取消 Windows 管理员授权，设置没有改变；需要时请重新点击并在授权窗口中选择“是”")
	case 47:
		return errors.New("无法打开 Windows 管理员授权窗口，请使用有管理员权限的 Windows 账户重试")
	case 21:
		return errors.New("Windows 未能启用虚拟机监控程序平台，请改为手动启用：控制面板 → 程序 → 启用或关闭 Windows 功能 → 勾选「Windows 虚拟机监控程序平台」")
	case 22:
		return errors.New("已启用 Windows 虚拟机监控程序平台，但未能恢复系统启动设置；请以管理员身份运行 bcdedit /set {current} hypervisorlaunchtype auto 后重启")
	case 23:
		return errors.New("这台电脑不支持从 Windows 直接进入固件设置，请重启时按提示的按键进入 BIOS/UEFI")
	default:
		return fmt.Errorf("虚拟化设置未完成（退出码 %d），请重启电脑后重新检查", code)
	}
}
