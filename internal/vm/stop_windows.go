package vm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/store"
	"github.com/sylvinus/agent-vm/internal/host"
	"golang.org/x/sys/windows"
)

func stopInstance(ctx context.Context, inst *limatype.Instance) error {
	requested, err := host.RequestInstanceStop(inst.Dir)
	if err != nil {
		return err
	}
	if !requested {
		if current, inspectErr := store.Inspect(ctx, inst.Name); inspectErr == nil {
			switch windowsStopPoll(current) {
			case stopPollStopped:
				return nil
			case stopPollStopOrphan:
				return stopOrphanedInstance(ctx, current)
			}
		}
		return fmt.Errorf("CCBox 环境的停止通知不可用，已保留环境。请查看虚拟机后台日志")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute+10*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 CCBox 环境保存并停止超时: %w", ctx.Err())
		case <-ticker.C:
			current, err := store.Inspect(ctx, inst.Name)
			if err != nil {
				return err
			}
			switch windowsStopPoll(current) {
			case stopPollStopped:
				return nil
			case stopPollStopOrphan:
				return stopOrphanedInstance(ctx, current)
			case stopPollBrokenFail:
				return fmt.Errorf("CCBox 虚拟机后台已退出，无法确认环境安全停止（%s）", current.Status)
			}
		}
	}
}

func windowsStopPoll(inst *limatype.Instance) stopPollAction {
	orphaned := inst.Status == limatype.StatusBroken && inst.HostAgentPID == 0 && fromLima(inst).Orphaned
	return evaluateWindowsStopPoll(inst.Status, inst.HostAgentPID, inst.DriverPID, orphaned)
}

func stopOrphanedInstance(ctx context.Context, inst *limatype.Instance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if inst.VMType != limatype.QEMU || inst.DriverPID <= 0 {
		return fmt.Errorf("unsupported orphaned VM driver")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(inst.DriverPID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(handle, 0, &image[0], &size); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(windows.UTF16ToString(image[:size])), "qemu-system-x86_64.exe") || !isQEMU(inst.DriverPID, inst.Dir, inst.Name) {
		return fmt.Errorf("orphaned VM process identity changed; no process was stopped")
	}
	waitExit := func(interval time.Duration) (bool, error) {
		milliseconds := uint32(0)
		if interval > 0 {
			milliseconds = uint32(max(1, interval.Milliseconds()))
		}
		state, err := windows.WaitForSingleObject(handle, milliseconds)
		return state == windows.WAIT_OBJECT_0, err
	}
	forceStop := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := windows.TerminateProcess(handle, 1); err != nil {
			if exited, _ := waitExit(0); !exited {
				return err
			}
			return nil
		}
		exited, err := waitExit(5 * time.Second)
		if err != nil {
			return err
		}
		if !exited {
			return fmt.Errorf("orphaned QEMU did not stop")
		}
		return nil
	}
	return stopOrphanedWithHooks(ctx, inst.Dir, qmpSocketCandidatesWindows(inst.Dir), waitExit, forceStop)
}
