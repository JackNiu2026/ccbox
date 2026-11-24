package containerenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/guestidentity"
	"github.com/sylvinus/agent-vm/internal/guestprofile"
	"github.com/sylvinus/agent-vm/internal/host"
)

var archiveIDPattern = regexp.MustCompile(`^[0-9a-f]{8,32}$`)

func (r *Runtime) archiveContainerName(archiveID string) (string, error) {
	if !archiveIDPattern.MatchString(archiveID) {
		return "", errors.New("归档标识无效")
	}
	return r.Name() + "-archive-" + archiveID, nil
}

// CurrentIdentity returns the persisted guest identity of the current container.
func (r *Runtime) CurrentIdentity(ctx context.Context) (*guestidentity.Identity, error) {
	container, err := r.inspectContainer(ctx)
	if err != nil {
		return nil, err
	}
	if container == nil {
		return nil, nil
	}
	return containerIdentity(container)
}

// ArchiveCurrent stops nothing itself: the caller must stop the container first.
// It renames the stopped container to an isolated archive name so a fresh
// container can be created without losing rollback ability.
func (r *Runtime) ArchiveCurrent(ctx context.Context, archiveID string) (string, error) {
	target, err := r.archiveContainerName(archiveID)
	if err != nil {
		return "", err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(r.InstanceDir(), "lifecycle.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	container, err := r.inspectContainer(ctx)
	if err != nil {
		return "", err
	}
	if container == nil {
		return "", errors.New("当前没有可归档的容器环境")
	}
	if container.State.Running {
		return "", errors.New("请先停止当前容器环境后再执行换机")
	}
	if _, err := r.output(ctx, "rename", container.ID, target); err != nil {
		return "", errors.Join(ErrArchiveRecoveryIncomplete, fmt.Errorf("归档旧容器失败：%w", err))
	}
	_ = os.Remove(filepath.Join(r.InstanceDir(), "known_hosts"))
	return target, nil
}

// RestoreArchive requires the active name to be free. Live data is never
// discarded to make room for a restore.
func (r *Runtime) RestoreArchive(ctx context.Context, archiveID string) error {
	_, err := r.SwapArchive(ctx, archiveID, "")
	return err
}

// ErrArchiveRecoveryIncomplete means the caller must keep its staging metadata.
var ErrArchiveRecoveryIncomplete = errors.New("容器归档切换尚未完成，数据和恢复记录已保留")

// SwapArchive preserves any stopped current container under replacementID
// before moving the requested archive back to the active name.
func (r *Runtime) SwapArchive(ctx context.Context, archiveID, replacementID string) (bool, error) {
	source, err := r.archiveContainerName(archiveID)
	if err != nil {
		return false, err
	}
	var replacement string
	if replacementID != "" {
		if replacementID == archiveID {
			return false, errors.New("不能使用同一归档作为替换位置")
		}
		replacement, err = r.archiveContainerName(replacementID)
		if err != nil {
			return false, err
		}
	}
	unlock, err := lockRuntime(ctx, filepath.Join(r.InstanceDir(), "lifecycle.lock"))
	if err != nil {
		return false, err
	}
	defer unlock()
	archived, err := r.inspectNamed(ctx, source)
	if err != nil {
		return false, err
	}
	if archived == nil {
		return false, errors.New("未找到对应的旧容器归档")
	}
	if archived.State.Running {
		return false, errors.New("旧容器归档仍在运行，无法直接回滚")
	}
	current, err := r.inspectContainer(ctx)
	if err != nil {
		return false, err
	}
	if current != nil {
		if current.State.Running {
			return false, errors.New("请先停止当前容器后再回滚旧环境")
		}
		if replacement == "" {
			return false, errors.New("当前容器仍存在，请先归档再恢复旧环境")
		}
		if _, err := r.output(ctx, "rename", current.ID, replacement); err != nil {
			return false, errors.Join(ErrArchiveRecoveryIncomplete, err)
		}
	}
	if _, err := r.output(ctx, "rename", archived.ID, r.Name()); err != nil {
		if current == nil {
			return false, errors.Join(ErrArchiveRecoveryIncomplete, err)
		}
		// If the command did switch the name, this rollback fails safely
		// rather than replacing the restored container.
		if _, rollbackErr := r.output(ctx, "rename", current.ID, r.Name()); rollbackErr != nil {
			return false, errors.Join(ErrArchiveRecoveryIncomplete, err, rollbackErr)
		}
		return false, fmt.Errorf("恢复旧容器归档失败：%w", err)
	}
	_ = os.Remove(filepath.Join(r.InstanceDir(), "known_hosts"))
	if err := r.recordStorage(ctx, archived.ID); err != nil {
		return current != nil, errors.Join(ErrArchiveRecoveryIncomplete, err)
	}
	return current != nil, nil
}

// DeleteArchive permanently removes a stopped archived container.
func (r *Runtime) DeleteArchive(ctx context.Context, archiveID string) error {
	target, err := r.archiveContainerName(archiveID)
	if err != nil {
		return err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(r.InstanceDir(), "lifecycle.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	archived, err := r.inspectNamed(ctx, target)
	if err != nil {
		return err
	}
	if archived == nil {
		return nil
	}
	if archived.State.Running {
		return errors.New("归档容器仍在运行，拒绝删除")
	}
	_, err = r.output(ctx, "rm", "--", target)
	return err
}

// PrepareFresh creates a brand-new stopped container using the supplied
// validated profile and guest identity. Used by L3 clean rebuild after the
// previous container has been archived.
func (r *Runtime) PrepareFresh(ctx context.Context, profile *guestprofile.Profile, identity *guestidentity.Identity, log io.Writer) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := host.EnsurePrivateDir(r.InstanceDir()); err != nil {
		return err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(r.InstanceDir(), "lifecycle.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	rootless, err := r.rootlessEngine(ctx)
	if err != nil {
		return err
	}
	if !rootless {
		return errors.New("Cage 只允许使用独立 rootless 容器引擎")
	}
	if err := r.ensureResourceSlice(ctx); err != nil {
		return err
	}
	if existing, err := r.inspectContainer(ctx); err != nil {
		return err
	} else if existing != nil {
		return errors.New("当前容器仍存在，请先归档后再重建")
	}
	for _, device := range []string{"/dev/fuse", "/dev/net/tun"} {
		info, err := os.Stat(device)
		if err != nil || info.Mode()&os.ModeDevice == 0 {
			return fmt.Errorf("服务器缺少容器所需设备 %s", device)
		}
	}
	image, err := r.output(ctx, "image", "inspect", "--format", "{{.Id}}", r.config.Image)
	if err != nil {
		return errors.New("指定的固定容器基础镜像尚未安装，请先完成服务器部署")
	}
	if !imageIDPattern.MatchString(strings.TrimSpace(string(image))) {
		return errors.New("容器基础镜像没有有效 SHA-256 标识")
	}
	if err := guestprofile.Replace(r.profile, profile); err != nil {
		return err
	}
	copyID := *identity
	copyID.Hostname = profile.Hostname
	if err := copyID.Validate(); err != nil {
		return err
	}
	if _, err := r.output(ctx, r.createArguments(&copyID)...); err != nil {
		return err
	}
	container, err := r.inspectContainer(ctx)
	if err != nil {
		return err
	}
	if err := checkCreatedContainer(container, &copyID); err != nil {
		return err
	}
	if err := r.recordStorage(ctx, container.ID); err != nil {
		return err
	}
	if log != nil {
		fmt.Fprintln(log, "Fresh Cage container prepared with a new identity.")
	}
	return nil
}

func (r *Runtime) inspectNamed(ctx context.Context, name string) (*engineContainer, error) {
	output, err := r.output(ctx, "container", "inspect", name)
	if err != nil {
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "no such") || strings.Contains(text, "not found") {
			return nil, nil
		}
		return nil, err
	}
	var found []engineContainer
	if json.Unmarshal(output, &found) != nil || len(found) != 1 || found[0].Config.Labels[profileLabel] != r.identity {
		return nil, errors.New("容器归档与当前 Cage 配置不匹配")
	}
	return &found[0], nil
}
