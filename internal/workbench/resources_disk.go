package workbench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/host"
	"github.com/sylvinus/agent-vm/internal/vm"
)

var errDiskShrinkUnavailable = errors.New("暂不支持缩小虚拟硬盘，现有磁盘仍可扩容或从已有备份恢复")

type resourceDisk struct {
	supported bool
	info      cage.DiskImageInfo
	recovery  bool
}

func (d resourceDisk) currentGiB() int {
	return int((d.info.VirtualBytes + (1 << 30) - 1) >> 30)
}

func (s *Server) readResourceDisk() (resourceDisk, error) {
	if !s.localVMStorage() || s.macosGuest != nil || s.windowsGuest != nil {
		return resourceDisk{}, nil
	}
	system, err := s.installedLinuxSystem()
	if err != nil || system != guestos.Debian13 {
		return resourceDisk{}, err
	}
	dir := s.instanceDir()
	disk := resourceDisk{supported: true, recovery: cage.DiskResizePending(dir)}
	disk.info, err = cage.DiskImageUsage(dir)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Lstat(filepath.Join(dir, "lima.yaml")); statErr == nil && !disk.recovery {
			return resourceDisk{}, errors.New("找不到已有环境的虚拟磁盘")
		}
		err = nil
	}
	return disk, err
}

func (d resourceDisk) diskLimit(allocation cage.ResourceAllocation, instanceDir string, used uint64) (int, error) {
	if allocation.DiskGiB != 0 && !d.supported {
		return 0, errors.New("当前环境不支持调整虚拟硬盘")
	}
	return allocation.DiskLimit(instanceDir, uint64(d.info.VirtualBytes), used)
}

func (d resourceDisk) changePending(allocation cage.ResourceAllocation, limit int) bool {
	return d.supported && !d.recovery && !allocation.Automatic && allocation.DiskGiB > 0 &&
		d.info.VirtualBytes > 0 && allocation.DiskGiB > d.currentGiB() && int64(limit)<<30 > d.info.VirtualBytes
}

func (s *Server) startEnvironmentForDisk() error {
	err := s.startEnvironment(false)
	// A network-only check can fail while a functional VM is available to
	// verify its disk. The existing check keeps the external gate closed.
	var checkErr networkCheckError
	if errors.As(err, &checkErr) && s.vmRunning() {
		return nil
	}
	return err
}

func (s *Server) inspectDiskVM(ctx context.Context) (*vm.Instance, error) {
	lima, err := s.limaBackend()
	if err != nil {
		return nil, err
	}
	dir := s.instanceDir()
	instance, err := lima.Get(ctx, filepath.Base(dir))
	if err != nil {
		return nil, fmt.Errorf("无法确认虚拟机已停止，虚拟磁盘和备份已保留：%w", err)
	}
	if filepath.Clean(instance.Dir) != filepath.Clean(dir) || instance.ConfigErr != nil {
		return nil, errors.New("无法核对虚拟机配置，虚拟磁盘和备份已保留")
	}
	return instance, nil
}

func (s *Server) stopEnvironmentForDisk(ctx context.Context) error {
	instance, err := s.inspectDiskVM(ctx)
	if err != nil {
		return err
	}
	if instance.Status == vm.Stopped {
		return nil
	}
	if instance.Status != vm.Running && !instance.Orphaned {
		return errors.New("无法确认虚拟机驱动已停止，虚拟磁盘和备份已保留")
	}
	if err := s.stopEnvironment(); err != nil {
		return err
	}
	instance, err = s.inspectDiskVM(ctx)
	if err != nil {
		return err
	}
	if instance.Status != vm.Stopped {
		return errors.New("环境尚未完全停止，无法调整虚拟硬盘")
	}
	return nil
}

func (s *Server) withStoppedDisk(ctx context.Context, mutate func() error) error {
	if err := s.stopEnvironmentForDisk(ctx); err != nil {
		return err
	}
	// Use the same per-instance lock as CLI startup while doing cold disk I/O.
	// Stop before taking it, then recheck in case a CLI start was already queued.
	locks := filepath.Join(s.profile, "state", "locks")
	if err := os.MkdirAll(locks, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(locks, filepath.Base(s.instanceDir())), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := lockController(lock); err != nil {
		return fmt.Errorf("无法锁定当前虚拟机，虚拟磁盘和备份已保留：%w", err)
	}
	defer unlockController(lock)
	instance, err := s.inspectDiskVM(ctx)
	if err != nil {
		return err
	}
	if instance.Status != vm.Stopped {
		return errors.New("虚拟机已重新启动，虚拟磁盘和备份已保留")
	}
	return mutate()
}

func (s *Server) controlDiskGuest(timeout time.Duration, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(s.operationContext(), timeout)
	defer cancel()
	return s.guestControl(ctx, script, "disk-resize")
}

func (s *Server) adjustEnvironmentDisk(disk resourceDisk, targetGiB int) (returned error) {
	ctx := s.operationContext()
	dir := s.instanceDir()
	targetBytes := int64(targetGiB) << 30
	if targetBytes < disk.info.VirtualBytes {
		return errDiskShrinkUnavailable
	}
	additional := disk.info.HostAllocatedBytes + (1 << 30)
	if free, ok := host.FreeGiB(filepath.Dir(disk.info.Path)); ok && uint64(free)<<30 < uint64(additional) {
		return fmt.Errorf("可用磁盘空间不足，至少需要 %d GiB 临时空间保存恢复备份", (additional+(1<<30)-1)>>30)
	}
	if err := s.withStoppedDisk(ctx, func() error {
		s.setMessage("正在备份调整前的虚拟磁盘…")
		return cage.BeginDiskResize(ctx, dir, s.jobs.execution().id)
	}); err != nil {
		return err
	}
	defer func() {
		if returned == nil || !cage.DiskResizePending(dir) {
			return
		}
		rollback, cancel := context.WithTimeout(s.lifetimeContext(), 30*time.Minute)
		defer cancel()
		if err := s.withStoppedDisk(rollback, func() error { return cage.RestoreDiskResize(rollback, dir) }); err != nil {
			returned = errors.Join(returned, fmt.Errorf("自动恢复未完成，已保留硬盘备份：%w", err))
			return
		}
		returned = fmt.Errorf("硬盘调整未完成，已恢复调整前的磁盘：%w", returned)
	}()

	if err := cage.SetDiskResizeLimit(dir, targetGiB); err != nil {
		return err
	}

	if err := s.startEnvironmentForDisk(); err != nil {
		return err
	}
	if _, err := s.controlDiskGuest(90*time.Second, cage.GuestDiskGrowScript(targetBytes)); err != nil {
		return fmt.Errorf("环境未能扩展根文件系统：%w", err)
	}
	final, err := cage.DiskImageUsage(dir)
	if err != nil || final.VirtualBytes != targetBytes {
		return fmt.Errorf("实际虚拟磁盘容量与目标不符：%v", err)
	}
	if _, err := s.controlDiskGuest(30*time.Second, cage.GuestVerifyDiskResizeScript(targetBytes)); err != nil {
		return fmt.Errorf("重启后未能确认客机磁盘容量与根文件系统：%w", err)
	}
	return cage.CommitDiskResize(dir)
}
