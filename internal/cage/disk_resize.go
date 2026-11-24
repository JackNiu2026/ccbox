package cage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/sylvinus/agent-vm/internal/host"
)

const diskResizeGuestGeometry = `set -euo pipefail
export LC_ALL=C
test "$(findmnt -n -o FSTYPE /)" = ext4
test ! -e /var/lib/rootfs-resize/request
test ! -e /var/lib/rootfs-resize/done
rootdev="$(readlink -f "$(findmnt -n -o SOURCE /)")"
test -b "$rootdev"
devname="$(basename "$rootdev")"
parent="$(lsblk -dn -o PKNAME "$rootdev")"
test -b "/dev/$parent"
partnum="$(cat "/sys/class/block/$devname/partition")"
part_start="$(cat "/sys/class/block/$devname/start")"
disk_size="$(cat "/sys/class/block/$parent/size")"
`

// GuestDiskGrowScript expands the ext4 root once for an explicit adjustment.
func GuestDiskGrowScript(targetBytes int64) string {
	return diskResizeGuestGeometry + fmt.Sprintf("test \"$((disk_size * 512))\" -eq %d\n", targetBytes) + `
part_size="$(cat "/sys/class/block/$devname/size")"
if [ "$((disk_size - part_start - part_size))" -gt 32768 ]; then
  if command -v growpart >/dev/null 2>&1; then
    sudo growpart "/dev/$parent" "$partnum"
  else
    printf ', +\n' | sudo sfdisk --no-reread -N "$partnum" "/dev/$parent"
    sudo partx --update --nr "$partnum" "/dev/$parent"
  fi
fi
sudo resize2fs "$rootdev"
`
}

// GuestVerifyDiskResizeScript checks the visible device and root filesystem
// after restart, before the host discards the old disk backup.
func GuestVerifyDiskResizeScript(targetBytes int64) string {
	return diskResizeGuestGeometry + fmt.Sprintf("test \"$((disk_size * 512))\" -eq %d\n", targetBytes) + `
part_size="$(cat "/sys/class/block/$devname/size")"
test "$((disk_size - part_start - part_size))" -ge 0
test "$((disk_size - part_start - part_size))" -le 32768
fs_bytes="$(sudo dumpe2fs -h "$rootdev" 2>/dev/null | awk -F: '/^Block count:/{count=$2+0} /^Block size:/{size=$2+0} END {printf "%.0f\n",count*size}')"
test "$fs_bytes" -gt 0
test "$fs_bytes" -le "$((part_size * 512))"
test "$((part_size * 512 - fs_bytes))" -le 33554432
`
}

// GuestDiskInspectShrinkScript inspects the root partition layout and ext4
// minimum size inside the running Linux guest without modifying anything.
const GuestDiskInspectShrinkScript = `set -euo pipefail
export LC_ALL=C
rootdev="$(findmnt -n -o SOURCE /)"
fstype="$(findmnt -n -o FSTYPE /)"
realdev="$(readlink -f "$rootdev")"
devname="$(basename "$realdev")"
parent="$(lsblk -no PKNAME "$realdev" | head -n 1)"
test "$(lsblk -dn -o LOG-SEC "/dev/$parent")" = 512
test "$(lsblk -dn -o PTTYPE "/dev/$parent")" = gpt
partnum="$(cat "/sys/class/block/$devname/partition")"
part_start="$(cat "/sys/class/block/$devname/start")"
part_size="$(cat "/sys/class/block/$devname/size")"
disk_size="$(cat "/sys/class/block/$parent/size")"
other_max_end=0
for p in /sys/class/block/"$parent"/*/partition; do
  [ -r "$p" ] || continue
  dir="$(dirname "$p")"
  b="$(basename "$dir")"
  [ "$b" = "$devname" ] && continue
  s="$(cat "$dir/start")"
  l="$(cat "$dir/size")"
  e=$((s + l - 1))
  if [ "$e" -gt "$other_max_end" ]; then other_max_end="$e"; fi
done
used_bytes="$(df -B1 --output=used / | awk 'NR==2 {print $1}')"
block_size="$(sudo dumpe2fs -h "$realdev" 2>/dev/null | awk -F: '/^Block size:/ {gsub(/[^0-9]/,"",$2); print $2}')"
min_blocks="$(sudo resize2fs -P "$realdev" 2>&1 | awk -F: '/minimum size/ {gsub(/[^0-9]/,"",$2); print $2}')"
printf 'fstype=%s\nrootdev=%s\nparent=%s\npartnum=%s\npart_start=%s\npart_size=%s\ndisk_size=%s\nother_max_end=%s\nused_bytes=%s\nblock_size=%s\nmin_blocks=%s\n' \
  "$fstype" "$realdev" "$parent" "$partnum" "$part_start" "$part_size" "$disk_size" "$other_max_end" "$used_bytes" "$block_size" "$min_blocks"
`

type DiskShrinkInspection struct {
	FSType      string
	RootDev     string
	ParentDisk  string
	PartNum     int
	PartStart   int64
	PartSectors int64
	DiskSectors int64
	OtherMaxEnd int64
	UsedBytes   int64
	FSBlockSize int64
	FSMinBlocks int64
}

type DiskShrinkPlan struct {
	Nonce             string
	TargetDiskGiB     int
	TargetDiskBytes   int64
	TargetDiskSectors int64
	TargetPartSectors int64
	TargetPartEndLBA  int64
	TargetFSBlocks    int64
	PartNum           int
	ParentDisk        string
}

var safeDeviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func ParseDiskShrinkInspection(output []byte) (DiskShrinkInspection, error) {
	var in DiskShrinkInspection
	values := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			values[k] = strings.TrimSpace(v)
		}
	}
	in.FSType = values["fstype"]
	in.RootDev = values["rootdev"]
	in.ParentDisk = values["parent"]
	parseInt := func(key string) (int64, error) {
		v, ok := values[key]
		if !ok || v == "" {
			return 0, fmt.Errorf("missing field %s", key)
		}
		return strconv.ParseInt(v, 10, 64)
	}
	var err error
	var partNum int64
	if partNum, err = parseInt("partnum"); err != nil {
		return in, err
	}
	in.PartNum = int(partNum)
	if in.PartStart, err = parseInt("part_start"); err != nil {
		return in, err
	}
	if in.PartSectors, err = parseInt("part_size"); err != nil {
		return in, err
	}
	if in.DiskSectors, err = parseInt("disk_size"); err != nil {
		return in, err
	}
	if in.OtherMaxEnd, err = parseInt("other_max_end"); err != nil {
		return in, err
	}
	if in.UsedBytes, err = parseInt("used_bytes"); err != nil {
		return in, err
	}
	if in.FSBlockSize, err = parseInt("block_size"); err != nil {
		return in, err
	}
	if in.FSMinBlocks, err = parseInt("min_blocks"); err != nil {
		return in, err
	}
	return in, nil
}

// PlanDiskShrink validates that shrinking the guest rootfs and disk image to
// targetDiskGiB is safe and returns exact sector/block targets.
func PlanDiskShrink(insp DiskShrinkInspection, targetDiskGiB int) (DiskShrinkPlan, error) {
	if insp.FSType != "ext4" {
		return DiskShrinkPlan{}, fmt.Errorf("当前根文件系统类型为 %q，暂仅支持缩小 ext4 磁盘", insp.FSType)
	}
	if !safeDeviceNameRe.MatchString(insp.ParentDisk) || insp.PartNum < 1 || insp.PartNum > 128 {
		return DiskShrinkPlan{}, errors.New("无法识别环境根分区设备")
	}
	if insp.FSBlockSize < 1024 || insp.FSBlockSize > 65536 || insp.FSBlockSize&(insp.FSBlockSize-1) != 0 {
		return DiskShrinkPlan{}, errors.New("无效的文件系统块大小")
	}
	if insp.PartStart <= 0 || insp.PartSectors <= 0 || insp.DiskSectors <= insp.PartStart {
		return DiskShrinkPlan{}, errors.New("无法读取有效的分区表范围")
	}
	if insp.OtherMaxEnd >= insp.PartStart {
		return DiskShrinkPlan{}, errors.New("根分区不是磁盘末尾分区，为保护数据无法直接缩小")
	}
	if targetDiskGiB < MinDiskGiB {
		return DiskShrinkPlan{}, fmt.Errorf("硬盘上限不能低于 %d GiB", MinDiskGiB)
	}
	targetDiskBytes := int64(targetDiskGiB) << 30
	targetDiskSectors := targetDiskBytes / gptSectorSize
	if targetDiskSectors >= insp.DiskSectors {
		return DiskShrinkPlan{}, errors.New("目标容量未小于当前硬盘容量")
	}
	// Reserve at least 1 MiB at disk tail for Backup GPT (33 sectors) + alignment.
	const alignSectors = 2048 // 1 MiB
	newLastUsable := targetDiskSectors - 34
	targetPartEnd := (newLastUsable/alignSectors)*alignSectors - 1
	if targetPartEnd <= insp.PartStart+alignSectors*32 {
		return DiskShrinkPlan{}, errors.New("目标硬盘容量不足以容纳系统分区")
	}
	targetPartSectors := targetPartEnd - insp.PartStart + 1
	targetPartBytes := targetPartSectors * gptSectorSize
	// Leave 16 MiB slack between ext4 end and partition end so sfdisk never clips ext4.
	const fsSlackBytes = 16 << 20
	if targetPartBytes <= fsSlackBytes {
		return DiskShrinkPlan{}, errors.New("目标分区空间过小")
	}
	targetFSBytes := targetPartBytes - fsSlackBytes
	targetFSBlocks := targetFSBytes / insp.FSBlockSize
	minFSBytes := insp.FSMinBlocks * insp.FSBlockSize
	requiredByUsed := insp.UsedBytes + DiskShrinkHeadroomBytes
	requiredByFS := minFSBytes + (2 << 30)
	required := max(requiredByUsed, requiredByFS)
	if targetFSBytes < required {
		needGiB := int((required + insp.PartStart*gptSectorSize + (64 << 20) + (1 << 30) - 1) >> 30)
		if needGiB < MinDiskGiB {
			needGiB = MinDiskGiB
		}
		return DiskShrinkPlan{}, fmt.Errorf("当前环境已用 %.1f GiB，至少需要 %d GiB 硬盘上限以保留安全余量。请先清理环境内文件，或设置不低于 %d GiB 的容量。", float64(insp.UsedBytes)/(1<<30), needGiB, needGiB)
	}
	var rawNonce [12]byte
	if _, err := rand.Read(rawNonce[:]); err != nil {
		return DiskShrinkPlan{}, err
	}
	return DiskShrinkPlan{
		Nonce:             hex.EncodeToString(rawNonce[:]),
		TargetDiskGiB:     targetDiskGiB,
		TargetDiskBytes:   targetDiskBytes,
		TargetDiskSectors: targetDiskSectors,
		TargetPartSectors: targetPartSectors,
		TargetPartEndLBA:  targetPartEnd,
		TargetFSBlocks:    targetFSBlocks,
		PartNum:           insp.PartNum,
		ParentDisk:        insp.ParentDisk,
	}, nil
}

// GuestInstallShrinkHookAndStageScript installs the offline initramfs shrink
// hook (when missing or outdated) and stages the validated shrink request.
func GuestInstallShrinkHookAndStageScript(plan DiskShrinkPlan) string {
	return fmt.Sprintf(`set -euo pipefail
sudo mkdir -p /var/lib/rootfs-resize /etc/initramfs-tools/hooks /etc/initramfs-tools/scripts/local-premount
hook_changed=0
tmp_hook="$(mktemp)"
cat >"$tmp_hook" <<'HOOK_EOF'
#!/bin/sh
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "${1:-}" in prereqs) prereqs; exit 0 ;; esac
. /usr/share/initramfs-tools/hook-functions
copy_exec /sbin/e2fsck /sbin
copy_exec /sbin/resize2fs /sbin
copy_exec /sbin/sfdisk /sbin
HOOK_EOF
if ! cmp -s "$tmp_hook" /etc/initramfs-tools/hooks/rootfs-shrink 2>/dev/null; then
  sudo install -m 0755 "$tmp_hook" /etc/initramfs-tools/hooks/rootfs-shrink
  hook_changed=1
fi
rm -f "$tmp_hook"

tmp_script="$(mktemp)"
cat >"$tmp_script" <<'PREMOUNT_EOF'
#!/bin/sh
PREREQ=""
prereqs() { echo "$PREREQ"; }
case "${1:-}" in prereqs) prereqs; exit 0 ;; esac
. /scripts/functions
DEV="$(resolve_device "$ROOT")"
if [ -z "$DEV" ] || [ ! -b "$DEV" ]; then exit 0; fi
mkdir -p /tmp/rootfs-resize-check
if ! mount -o ro "$DEV" /tmp/rootfs-resize-check 2>/dev/null; then
  exit 0
fi
REQ=""
if [ -f /tmp/rootfs-resize-check/var/lib/rootfs-resize/request ]; then
  REQ="$(cat /tmp/rootfs-resize-check/var/lib/rootfs-resize/request)"
fi
umount /tmp/rootfs-resize-check || true
if [ -z "$REQ" ]; then exit 0; fi
NONCE=""
TARGET_PART_SECTORS=""
TARGET_FS_BLOCKS=""
PART_NUM=""
PARENT_DISK=""
for line in $REQ; do
  case "$line" in
    NONCE=*) NONCE="${line#NONCE=}" ;;
    TARGET_PART_SECTORS=*) TARGET_PART_SECTORS="${line#TARGET_PART_SECTORS=}" ;;
    TARGET_FS_BLOCKS=*) TARGET_FS_BLOCKS="${line#TARGET_FS_BLOCKS=}" ;;
    PART_NUM=*) PART_NUM="${line#PART_NUM=}" ;;
    PARENT_DISK=*) PARENT_DISK="${line#PARENT_DISK=}" ;;
  esac
done
write_done() {
  status="$1"
  reason="$2"
  if mount -o rw "$DEV" /tmp/rootfs-resize-check 2>/dev/null; then
    rm -f /tmp/rootfs-resize-check/var/lib/rootfs-resize/request
    printf 'NONCE=%%s\nSTATUS=%%s\nREASON=%%s\n' "$NONCE" "$status" "$reason" > /tmp/rootfs-resize-check/var/lib/rootfs-resize/done
    sync
    umount /tmp/rootfs-resize-check || true
  fi
}
e2fsck -f -p "$DEV"
rc=$?
if [ "$rc" -ge 4 ]; then
  write_done "error" "fsck_pre_failed_$rc"
  exit 0
fi
if ! resize2fs "$DEV" "$TARGET_FS_BLOCKS"; then
  write_done "error" "resize2fs_failed"
  exit 0
fi
e2fsck -f -p "$DEV"
rc=$?
if [ "$rc" -ge 4 ]; then
  write_done "error" "fsck_post_failed_$rc"
  exit 0
fi
if ! printf ',%%s\n' "$TARGET_PART_SECTORS" | sfdisk --no-reread -N "$PART_NUM" "/dev/$PARENT_DISK"; then
  write_done "error" "sfdisk_failed"
  exit 0
fi
write_done "ok" "shrunk"
exit 0
PREMOUNT_EOF
if ! cmp -s "$tmp_script" /etc/initramfs-tools/scripts/local-premount/rootfs-shrink 2>/dev/null; then
  sudo install -m 0755 "$tmp_script" /etc/initramfs-tools/scripts/local-premount/rootfs-shrink
  hook_changed=1
fi
rm -f "$tmp_script"

if [ "$hook_changed" -eq 1 ]; then
  sudo update-initramfs -u >/dev/null
fi
sudo rm -f /var/lib/rootfs-resize/done
sudo tee /var/lib/rootfs-resize/request >/dev/null <<'REQ_EOF'
NONCE=%s
TARGET_PART_SECTORS=%d
TARGET_FS_BLOCKS=%d
PART_NUM=%d
PARENT_DISK=%s
REQ_EOF
sudo fstrim -v / >/dev/null 2>&1 || true
sync
`, plan.Nonce, plan.TargetPartSectors, plan.TargetFSBlocks, plan.PartNum, plan.ParentDisk)
}

// GuestRelocateGPTAndTrimScript verifies the initramfs shrink completion
// receipt, relocates the Backup GPT header/entries to the exact target disk
// end (working identically for both raw and qcow2 backing images), cleans up
// the completion marker, and trims free blocks before host shutdown.
func GuestRelocateGPTAndTrimScript(plan DiskShrinkPlan) string {
	return fmt.Sprintf(`set -euo pipefail
if [ ! -f /var/lib/rootfs-resize/done ]; then
  echo "missing shrink completion receipt" >&2
  exit 1
fi
cat /var/lib/rootfs-resize/done
grep -qx 'NONCE=%s' /var/lib/rootfs-resize/done
grep -qx 'STATUS=ok' /var/lib/rootfs-resize/done
sudo python3 - "/dev/%s" "%d" <<'GPT_EOF'
import struct, sys, zlib

dev_path = sys.argv[1]
target_bytes = int(sys.argv[2])
sector = 512
assert target_bytes >= (1 << 30) and target_bytes %% sector == 0
target_sectors = target_bytes // sector

with open(dev_path, "r+b", buffering=0) as f:
    f.seek(sector)
    hdr = bytearray(f.read(sector))
    assert hdr[0:8] == b"EFI PART", "missing primary GPT signature"
    hdr_size = struct.unpack_from("<I", hdr, 12)[0]
    assert 92 <= hdr_size <= sector
    part_lba, num_parts, part_sz, part_crc = struct.unpack_from("<QIII", hdr, 72)
    f.seek(part_lba * sector)
    entries = f.read(num_parts * part_sz)
    assert (zlib.crc32(entries) & 0xffffffff) == part_crc, "partition array CRC mismatch"
    table_sectors = (len(entries) + sector - 1) // sector
    backup_hdr_lba = target_sectors - 1
    backup_entries_lba = backup_hdr_lba - table_sectors
    last_usable = backup_entries_lba - 1
    for i in range(num_parts):
        entry = entries[i * part_sz : (i + 1) * part_sz]
        if entry[:16] == b"\x00" * 16:
            continue
        first_lba, last_lba = struct.unpack_from("<QQ", entry, 32)
        if last_lba > last_usable:
            raise SystemExit(f"partition {i+1} end {last_lba} exceeds target usable end {last_usable}")
    struct.pack_into("<Q", hdr, 24, 1)
    struct.pack_into("<Q", hdr, 32, backup_hdr_lba)
    struct.pack_into("<Q", hdr, 48, last_usable)
    struct.pack_into("<I", hdr, 16, 0)
    struct.pack_into("<I", hdr, 16, zlib.crc32(hdr[:hdr_size]) & 0xffffffff)

    backup = bytearray(hdr)
    struct.pack_into("<Q", backup, 24, backup_hdr_lba)
    struct.pack_into("<Q", backup, 32, 1)
    struct.pack_into("<Q", backup, 72, backup_entries_lba)
    struct.pack_into("<I", backup, 16, 0)
    struct.pack_into("<I", backup, 16, zlib.crc32(backup[:hdr_size]) & 0xffffffff)

    padded = entries + b"\x00" * (table_sectors * sector - len(entries))
    f.seek(backup_entries_lba * sector)
    f.write(padded)
    f.seek(backup_hdr_lba * sector)
    f.write(backup)
    f.seek(sector)
    f.write(hdr)
    f.flush()
GPT_EOF
sudo rm -f /var/lib/rootfs-resize/done /var/lib/rootfs-resize/request
sudo fstrim -v / >/dev/null 2>&1 || true
sync
`, plan.Nonce, plan.ParentDisk, plan.TargetDiskBytes)
}

// ShrinkColdDiskImage compacts a stopped disk after the guest has shrunk its
// filesystem and emitted a verified receipt. Never replace a recovery backup
// created from the old, unchanged disk at the beginning of the operation.
func ShrinkColdDiskImage(ctx context.Context, instanceDir string, targetDiskGiB int) error {
	if targetDiskGiB < MinDiskGiB || targetDiskGiB > MaxDiskCapGiB {
		return fmt.Errorf("硬盘上限须为 %d–%d GiB", MinDiskGiB, MaxDiskCapGiB)
	}
	if _, err := resizeRecord(instanceDir); err != nil {
		return err
	}
	targetBytes := int64(targetDiskGiB) << 30
	info, err := DiskImageUsage(instanceDir)
	if err != nil {
		return err
	}
	if targetBytes >= info.VirtualBytes {
		return errors.New("目标容量未小于当前硬盘容量")
	}
	switch info.Format {
	case "raw":
		f, err := os.OpenFile(info.Path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		err = VerifyAndRelocateGPT(f, targetBytes)
		if err == nil {
			err = f.Truncate(targetBytes)
		}
		if err == nil {
			err = f.Sync()
		}
		if err = errors.Join(err, f.Close()); err != nil {
			return fmt.Errorf("校验或缩小虚拟磁盘失败：%w", err)
		}
		// Compaction is best-effort on filesystems without sparse hole support.
		_, _ = punchZeroHoles(ctx, info.Path)
	case "qcow2":
		qemuImg, err := findQemuImg()
		if err != nil {
			return fmt.Errorf("找不到 qemu-img，无法紧缩虚拟磁盘：%w", err)
		}
		dir, err := os.MkdirTemp(filepath.Dir(info.Path), ".ccbox-disk-compact-")
		if err != nil {
			return err
		}
		defer os.Remove(dir)
		staged := filepath.Join(dir, "disk")
		defer os.Remove(staged)
		if out, err := runHidden(ctx, qemuImg, "convert", "-f", "qcow2", "-O", "qcow2", info.Path, staged); err != nil {
			return fmt.Errorf("紧缩虚拟磁盘失败：%w (%s)", err, strings.TrimSpace(string(out)))
		}
		if out, err := runHidden(ctx, qemuImg, "resize", "--shrink", "-f", "qcow2", staged, strconv.FormatInt(targetBytes, 10)); err != nil {
			return fmt.Errorf("缩小虚拟磁盘上限失败：%w (%s)", err, strings.TrimSpace(string(out)))
		}
		if out, err := runHidden(ctx, qemuImg, "check", "-f", "qcow2", staged); err != nil {
			return fmt.Errorf("缩小后磁盘校验失败：%w (%s)", err, strings.TrimSpace(string(out)))
		}
		f, err := os.OpenFile(staged, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			return err
		}
		if err = os.Rename(staged, info.Path); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持缩小格式为 %q 的虚拟磁盘", info.Format)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = SetDiskResizeLimit(instanceDir, targetDiskGiB); err != nil {
		return err
	}
	return syncStorageDir(instanceDir)
}

var limaDiskLineRe = regexp.MustCompile(`(?m)^disk:\s*.*$`)

func replaceLimaDiskLine(data []byte, diskGiB int) ([]byte, error) {
	replacement := fmt.Sprintf("disk: %q", strconv.Itoa(diskGiB)+"GiB")
	if limaDiskLineRe.Match(data) {
		return limaDiskLineRe.ReplaceAll(data, []byte(replacement)), nil
	}
	trimmed := bytes.TrimRight(data, "\n")
	return append(trimmed, []byte("\n"+replacement+"\n")...), nil
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".disk-yaml-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := os.Chmod(tmp, mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func findQemuImg() (string, error) {
	exeName := "qemu-img"
	if runtime.GOOS == "windows" {
		exeName = "qemu-img.exe"
	}
	if p, err := exec.LookPath(exeName); err == nil {
		return p, nil
	}
	if self, err := os.Executable(); err == nil {
		for _, candidate := range []string{
			filepath.Join(filepath.Dir(self), exeName),
			filepath.Join(filepath.Dir(self), "qemu", exeName),
			filepath.Join(filepath.Dir(self), "intel-macos", "bin", exeName),
		} {
			if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", os.ErrNotExist
}

func runHidden(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	host.HideCommandWindow(cmd)
	return cmd.CombinedOutput()
}

func copySingleSparseFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.Close()
			_ = os.Remove(dst)
		}
	}()
	_ = sparseStorageFile(out)
	buf := make([]byte, 1<<20)
	zero := make([]byte, len(buf))
	var offset int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, rErr := in.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if !bytes.Equal(chunk, zero[:n]) {
				if _, err = out.WriteAt(chunk, offset); err != nil {
					return err
				}
			}
			offset += int64(n)
		}
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			err = rErr
			return err
		}
	}
	if err = out.Truncate(info.Size()); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	err = out.Close()
	return err
}
