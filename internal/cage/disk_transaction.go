package cage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const diskResizeDir = ".ccbox-disk-resize"

type diskResizeRecord struct {
	Job  string `json:"job"`
	Disk string `json:"disk"`
}

func resizeRecord(instanceDir string) (diskResizeRecord, error) {
	var record diskResizeRecord
	dir := filepath.Join(instanceDir, diskResizeDir)
	info, err := os.Lstat(dir)
	if err != nil {
		return record, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return record, errors.New("虚拟磁盘恢复目录无效")
	}
	data, err := os.ReadFile(filepath.Join(dir, "resize.json"))
	if err != nil {
		return record, fmt.Errorf("无法读取虚拟磁盘恢复记录：%v", err)
	}
	if err := json.Unmarshal(data, &record); err != nil || record.Job == "" || record.Disk != "disk" && record.Disk != "diffdisk" {
		return record, errors.New("虚拟磁盘恢复记录无效")
	}
	return record, nil
}

func DiskResizePending(instanceDir string) bool {
	_, err := resizeRecord(instanceDir)
	return !errors.Is(err, os.ErrNotExist)
}

func AllowDiskResizeStart(instanceDir, job string) error {
	record, err := resizeRecord(instanceDir)
	if errors.Is(err, os.ErrNotExist) || err == nil && job != "" && job == record.Job {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("硬盘调整未完成，请先在设置的计算资源中恢复调整前的磁盘")
}

// BeginDiskResize must run with the VM stopped, before touching guest or host
// disk data. Publishing the completed recovery directory is the commit point.
func BeginDiskResize(ctx context.Context, instanceDir, job string) error {
	if job == "" || DiskResizePending(instanceDir) {
		return errors.New("已有硬盘恢复记录，请先处理上次调整")
	}
	disk, err := InstanceDiskPath(instanceDir)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(instanceDir)
	if err != nil {
		return err
	}
	name := filepath.Base(disk)
	if filepath.Dir(disk) != root || name != "disk" && name != "diffdisk" {
		return errors.New("已有虚拟磁盘位于外部存储，无法安全调整")
	}
	config := filepath.Join(root, "lima.yaml")
	info, err := os.Lstat(config)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("无法确认已有环境的磁盘配置")
	}
	yaml, err := os.ReadFile(config)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".ccbox-disk-resize-preparing-")
	if err != nil {
		return err
	}
	defer removeDiskResizeFiles(stage)
	if err = cloneDiskBackup(ctx, disk, filepath.Join(stage, "disk")); err != nil {
		return fmt.Errorf("无法备份虚拟磁盘，原环境保持不变：%w", err)
	}
	if err = writeAtomicFile(filepath.Join(stage, "lima.yaml"), yaml, 0o600); err != nil {
		return err
	}
	b, err := json.Marshal(diskResizeRecord{Job: job, Disk: name})
	if err != nil {
		return err
	}
	if err = writeAtomicFile(filepath.Join(stage, "resize.json"), b, 0o600); err != nil {
		return err
	}
	if err = syncStorageDir(stage); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(stage, filepath.Join(root, diskResizeDir)); err != nil {
		return err
	}
	return syncStorageDir(root)
}

func SetDiskResizeLimit(instanceDir string, diskGiB int) error {
	if _, err := resizeRecord(instanceDir); err != nil {
		return err
	}
	config := filepath.Join(instanceDir, "lima.yaml")
	old, err := os.ReadFile(config)
	if err != nil {
		return err
	}
	updated, err := replaceLimaDiskLine(old, diskGiB)
	if err != nil {
		return err
	}
	return writeAtomicFile(config, updated, 0o600)
}

// RestoreDiskResize installs recovery data only while the VM is stopped. The
// published backup remains available if either replacement fails.
func RestoreDiskResize(ctx context.Context, instanceDir string) error {
	record, err := resizeRecord(instanceDir)
	if err != nil {
		return err
	}
	stage := filepath.Join(instanceDir, diskResizeDir)
	diskBackup := filepath.Join(stage, "disk")
	for _, path := range []string{diskBackup, filepath.Join(stage, "lima.yaml")} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("硬盘恢复文件不完整")
		}
	}
	yaml, err := os.ReadFile(filepath.Join(stage, "lima.yaml"))
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp(instanceDir, ".ccbox-disk-restore-")
	if err != nil {
		return err
	}
	defer removeDiskResizeFiles(work)
	disk := filepath.Join(work, "disk")
	// Keep the backup name through both replacements. A same-volume link does
	// not require space for a second dense recovery copy.
	if err := os.Link(diskBackup, disk); err != nil {
		if err = cloneDiskBackup(ctx, diskBackup, disk); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(instanceDir, record.Disk)
	if current, statErr := os.Lstat(target); statErr == nil && !current.Mode().IsRegular() {
		return errors.New("原虚拟磁盘路径发生变化，拒绝覆盖")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	// On a retry, the target may already be linked to the verified backup.
	if existing, e := os.Stat(target); e == nil {
		if backup, be := os.Stat(diskBackup); be == nil && os.SameFile(existing, backup) {
			disk = ""
		}
	}
	if disk != "" {
		if err = os.Rename(disk, target); err != nil {
			return err
		}
	}
	if err = writeAtomicFile(filepath.Join(instanceDir, "lima.yaml"), yaml, 0o600); err != nil {
		return err
	}
	if err = syncStorageDir(instanceDir); err != nil {
		return err
	}
	return CommitDiskResize(instanceDir)
}

func CommitDiskResize(instanceDir string) error {
	dest, err := os.MkdirTemp(instanceDir, ".ccbox-disk-resize-finished-")
	if err != nil {
		return err
	}
	defer os.Remove(dest)
	finished := filepath.Join(dest, "backup")
	if err = os.Rename(filepath.Join(instanceDir, diskResizeDir), finished); err != nil {
		return err
	}
	if err = syncStorageDir(instanceDir); err != nil {
		return err
	}
	removeDiskResizeFiles(finished)
	return nil
}

func removeDiskResizeFiles(dir string) {
	for _, name := range []string{"resize.json", "lima.yaml", "disk"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	_ = os.Remove(dir)
}
