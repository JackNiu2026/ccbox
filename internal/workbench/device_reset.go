package workbench

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"

	"github.com/lima-vm/lima/v2/pkg/guestidentity"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/containerenv"
	"github.com/sylvinus/agent-vm/internal/diagnostics"
	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/guestprofile"
	"github.com/sylvinus/agent-vm/internal/host"
	"github.com/sylvinus/agent-vm/internal/vm"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

//go:embed device_reset.py
var deviceResetScript string

var deviceResetIDPattern = regexp.MustCompile(`^[0-9a-f]{8,32}$`)

type deviceResetSummary struct {
	Hostname              string `json:"hostname,omitempty"`
	MACSuffix             string `json:"mac_suffix,omitempty"`
	UUIDPrefix            string `json:"uuid_prefix,omitempty"`
	MachineIDPrefix       string `json:"machine_id_prefix,omitempty"`
	SystemSerialSuffix    string `json:"system_serial_suffix,omitempty"`
	Username              string `json:"username,omitempty"`
	BrowserUsername       string `json:"browser_username,omitempty"`
	AudioDescription      string `json:"audio_description,omitempty"`
	DisplaySize           string `json:"display_size,omitempty"`
	BrowserSize           string `json:"browser_size,omitempty"`
	RuntimeKind           string `json:"runtime_kind,omitempty"`
	SMBIOSSupported       bool   `json:"smbios_supported"`
	VZIdentifierSupported bool   `json:"vz_identifier_supported"`
}

type deviceResetTarget struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	AffectsIdentity bool   `json:"affects_identity"`
	DefaultSelected bool   `json:"default_selected"`
	Present         bool   `json:"present"`
	Bytes           int64  `json:"bytes"`
	Files           int    `json:"files"`
}

type deviceResetBatch struct {
	ID               string   `json:"id"`
	CreatedAt        int64    `json:"created_at"`
	Count            int      `json:"count"`
	ItemIDs          []string `json:"item_ids,omitempty"`
	Bytes            int64    `json:"bytes"`
	PreviousHostname string   `json:"previous_hostname,omitempty"`
}

type deviceResetArchive struct {
	ID                 string                `json:"id"`
	State              string                `json:"state"`
	CreatedAt          int64                 `json:"created_at"`
	RuntimeKind        string                `json:"runtime_kind"`
	PreviousHostname   string                `json:"previous_hostname,omitempty"`
	PreviousUsername   string                `json:"previous_username,omitempty"`
	Bytes              int64                 `json:"bytes"`
	InstanceName       string                `json:"instance_name,omitempty"`
	HasInstance        bool                  `json:"has_instance,omitempty"`
	HasBaseTemplate    bool                  `json:"has_base_template,omitempty"`
	HasBaseVersion     bool                  `json:"has_base_version,omitempty"`
	HasBaseBuiltBy     bool                  `json:"has_base_built_by,omitempty"`
	HasGuestProfile    bool                  `json:"has_guest_profile,omitempty"`
	ContainerArchiveID string                `json:"container_archive_id,omitempty"`
	PreviousProfile    *guestprofile.Profile `json:"previous_profile,omitempty"`
}

type deviceResetResult struct {
	Mode              string             `json:"mode"`
	CompletedAt       int64              `json:"completed_at"`
	Irreversible      bool               `json:"irreversible"`
	ArchiveID         string             `json:"archive_id,omitempty"`
	BatchID           string             `json:"batch_id,omitempty"`
	Before            deviceResetSummary `json:"before"`
	After             deviceResetSummary `json:"after"`
	CleanedItems      []string           `json:"cleaned_items"`
	SkippedItems      []string           `json:"skipped_items,omitempty"`
	VZIdentifierReset bool               `json:"vz_identifier_reset,omitempty"`
	SMBIOSRotated     bool               `json:"smbios_rotated,omitempty"`
	OSRotated         bool               `json:"os_rotated,omitempty"`
}

type deviceResetStatus struct {
	Supported         bool                      `json:"supported"`
	UnsupportedReason string                    `json:"unsupported_reason,omitempty"`
	RuntimeKind       string                    `json:"runtime_kind"`
	HostOS            string                    `json:"host_os"`
	Container         bool                      `json:"container"`
	VMRunning         bool                      `json:"vm_running"`
	Ready             bool                      `json:"ready"`
	LegacyProfile     bool                      `json:"legacy_profile"`
	Current           deviceResetSummary        `json:"current"`
	Targets           []deviceResetTarget       `json:"targets"`
	Protected         []string                  `json:"protected"`
	Archives          []deviceResetArchive      `json:"archives"`
	RecoveryBatches   []deviceResetBatch        `json:"recovery_batches"`
	LastResult        *deviceResetResult        `json:"last_result,omitempty"`
	AudioModels       []guestprofile.AudioModel `json:"audio_models"`
}

type guestResetInspectResponse struct {
	Version         int                 `json:"version"`
	Hostname        string              `json:"hostname"`
	MachineIDPrefix string              `json:"machine_id_prefix"`
	Items           []deviceResetTarget `json:"items"`
	Batches         []deviceResetBatch  `json:"batches"`
}

type guestResetActionResponse struct {
	Status          string `json:"status"`
	BatchID         string `json:"batch_id"`
	Irreversible    bool   `json:"irreversible"`
	OSRotated       bool   `json:"os_rotated"`
	Hostname        string `json:"hostname"`
	MachineIDPrefix string `json:"machine_id_prefix"`
	Outcomes        []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"outcomes"`
}

type deviceResetHooks struct {
	runGuestReset  func(context.Context, map[string]any) ([]byte, error)
	stopEnv        func() error
	startEnv       func(bool) error
	currentLima    func(context.Context, string) (vm.IdentityInfo, error)
	rotateLima     func(context.Context, string, *guestidentity.Identity) (*guestidentity.Identity, bool, error)
	archiveDirSize func(string) int64
}

func defaultDeviceResetTargets() []deviceResetTarget {
	return []deviceResetTarget{
		{
			ID:              "browser_profile",
			Title:           "浏览器完整资料与登录态",
			Description:     "Cookie、LocalStorage、IndexedDB、浏览器设备 ID（Local State）、历史与登录配置",
			AffectsIdentity: true,
			DefaultSelected: true,
		},
		{
			ID:              "claude_code",
			Title:           "Claude Code 登录与设备状态",
			Description:     "CLI 认证凭据、用户/设备标识与本地缓存",
			AffectsIdentity: true,
			DefaultSelected: true,
		},
		{
			ID:              "claude_desktop",
			Title:           "Claude Desktop 登录与应用数据",
			Description:     "桌面端 Cookie、存储、GPU 缓存与本地设备状态",
			AffectsIdentity: true,
			DefaultSelected: true,
		},
		{
			ID:              "editor_state",
			Title:           "内置编辑器状态与缓存",
			Description:     "工作区界面布局、最近打开记录与编辑器缓存（不影响项目文件）",
			AffectsIdentity: false,
			DefaultSelected: true,
		},
		{
			ID:              "system_traces",
			Title:           "终端历史与系统临时痕迹",
			Description:     "Shell 命令历史、临时状态与旧控制扩展目录",
			AffectsIdentity: false,
			DefaultSelected: true,
		},
	}
}

func validDeviceResetTarget(id string) bool {
	for _, t := range defaultDeviceResetTargets() {
		if t.ID == id {
			return true
		}
	}
	return false
}

func protectedDeviceResetItems() []string {
	return []string{
		"宿主机上所有项目文件夹（双向共享，绝不删除或修改）",
		"代理订阅、节点配置与出口规则",
		"CCBox 账户、收藏与应用通用设置",
	}
}

func (s *Server) registerDeviceReset(mux *discoverableMux) {
	mux.HandleFunc("GET /api/environment/device-reset", s.deviceResetStatusHandler)
	mux.HandleFunc("POST /api/environment/device-reset/preview", s.deviceResetPreviewHandler)
	mux.HandleFunc("POST /api/environment/device-reset/execute", s.deviceResetExecuteHandler)
	mux.HandleFunc("POST /api/environment/device-reset/restore", s.deviceResetRestoreHandler)
	mux.HandleFunc("POST /api/environment/device-reset/delete-archive", s.deviceResetDeleteArchiveHandler)
}

func (s *Server) deviceResetArchivesRoot() string {
	return filepath.Join(s.profile, "state", "device-reset-archives")
}

// An interrupted archive may already contain the only copy of the old disk.
// Keep ordinary startup from creating another VM over an unfinished switch.
func (s *Server) checkDeviceResetArchives() error {
	root := s.deviceResetArchivesRoot()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法确认旧环境归档状态：%w", err)
	}
	for _, entry := range entries {
		if !deviceResetIDPattern.MatchString(entry.Name()) {
			continue
		}
		var archive deviceResetArchive
		if entry.IsDir() {
			data, err := os.ReadFile(filepath.Join(root, entry.Name(), "manifest.json"))
			if err == nil && json.Unmarshal(data, &archive) == nil && archive.ID == entry.Name() && archive.State == "ready" {
				continue
			}
		}
		return errors.New("上次换机归档尚未完成。旧环境数据已保留，请从设置导出诊断包后恢复")
	}
	return nil
}

func (s *Server) deviceResetLastResultPath() string {
	return filepath.Join(s.profile, "state", "device-reset-last.json")
}

func (s *Server) deviceResetLimaHome() string {
	return filepath.Join(s.profile, "state", "lima")
}

func (s *Server) limaBackend() (*vm.Lima, error) {
	return vm.NewLima(s.deviceResetLimaHome(), filepath.Join(s.profile, "state"))
}

func (s *Server) hookCurrentLima(ctx context.Context, name string) (vm.IdentityInfo, error) {
	if s.deviceReset.currentLima != nil {
		return s.deviceReset.currentLima(ctx, name)
	}
	b, err := s.limaBackend()
	if err != nil {
		return vm.IdentityInfo{}, err
	}
	return b.CurrentIdentity(ctx, name)
}

func (s *Server) hookRotateLima(ctx context.Context, name string, next *guestidentity.Identity) (*guestidentity.Identity, bool, error) {
	if s.deviceReset.rotateLima != nil {
		return s.deviceReset.rotateLima(ctx, name, next)
	}
	b, err := s.limaBackend()
	if err != nil {
		return nil, false, err
	}
	return b.RotateIdentity(ctx, name, next)
}

func (s *Server) hookStopEnv() error {
	if s.deviceReset.stopEnv != nil {
		return s.deviceReset.stopEnv()
	}
	return s.stopEnvironment()
}

func (s *Server) hookStartEnv(syncTimezone bool) error {
	if s.deviceReset.startEnv != nil {
		return s.deviceReset.startEnv(syncTimezone)
	}
	return s.startEnvironment(syncTimezone)
}

func (s *Server) runGuestDeviceReset(ctx context.Context, request map[string]any) ([]byte, error) {
	if s.deviceReset.runGuestReset != nil {
		return s.deviceReset.runGuestReset(ctx, request)
	}
	p, err := guestprofile.Load(s.profile)
	if err != nil {
		return nil, err
	}
	input, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	python := []string{"sudo", "-n", "/usr/bin/python3", "-I", "-c", deviceResetScript, p.WorkHomePath(), p.BrowserHomePath()}
	return s.guest(ctx, "exec "+shellescape.QuoteCommand(python)+" <<'DEVICE_RESET_INPUT'\n"+string(input)+"\nDEVICE_RESET_INPUT\n")
}

func macSuffix(mac string) string {
	parts := strings.Split(mac, ":")
	if len(parts) == 6 {
		return "..:" + parts[4] + ":" + parts[5]
	}
	return ""
}

func prefixOf(text string, n int) string {
	if len(text) < n {
		return text
	}
	return text[:n]
}

func suffixOf(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return ".." + text[len(text)-n:]
}

func buildDeviceSummary(profile *guestprofile.Profile, id *guestidentity.Identity, runtimeKind, guestHostname, guestMachineIDPrefix string) deviceResetSummary {
	summary := deviceResetSummary{
		RuntimeKind:           runtimeKind,
		SMBIOSSupported:       runtimeKind == "qemu",
		VZIdentifierSupported: runtimeKind == "vz",
		MachineIDPrefix:       guestMachineIDPrefix,
	}
	if profile != nil {
		summary.Hostname = profile.Hostname
		summary.Username = profile.Username
		summary.BrowserUsername = profile.BrowserUsername
		summary.AudioDescription = profile.AudioDescription
		if profile.DisplayWidth > 0 && profile.DisplayHeight > 0 {
			summary.DisplaySize = fmt.Sprintf("%dx%d", profile.DisplayWidth, profile.DisplayHeight)
		}
		if profile.BrowserWidth > 0 && profile.BrowserHeight > 0 {
			summary.BrowserSize = fmt.Sprintf("%dx%d", profile.BrowserWidth, profile.BrowserHeight)
		}
	} else {
		summary.Username = "cage"
		summary.BrowserUsername = "cage-browser"
	}
	if id != nil {
		if id.Hostname != "" {
			summary.Hostname = id.Hostname
		}
		summary.MACSuffix = macSuffix(id.MACAddress)
		summary.UUIDPrefix = prefixOf(id.UUID, 8)
		if id.SystemSerial != "" {
			summary.SystemSerialSuffix = suffixOf(id.SystemSerial, 4)
		}
		if summary.MachineIDPrefix == "" && len(id.MachineID()) >= 8 {
			summary.MachineIDPrefix = id.MachineID()[:8]
		}
	}
	if guestHostname != "" {
		summary.Hostname = guestHostname
	}
	return summary
}

func (s *Server) detectRuntimeAndIdentity(ctx context.Context) (runtimeKind string, id *guestidentity.Identity, err error) {
	if containerenv.Configured(s.profile) {
		env, loadErr := containerenv.Load(s.profile)
		if loadErr != nil {
			return "container", nil, loadErr
		}
		cid, idErr := env.CurrentIdentity(ctx)
		return "container", cid, idErr
	}
	instName := cage.InstanceName(s.profile, s.environmentDir())
	info, infoErr := s.hookCurrentLima(ctx, instName)
	if infoErr != nil {
		if runtime.GOOS == "darwin" {
			return "vz", nil, nil
		}
		return "qemu", nil, nil
	}
	kind := info.VMType
	if kind == "" {
		if runtime.GOOS == "darwin" {
			kind = "vz"
		} else {
			kind = "qemu"
		}
	}
	return kind, info.Identity, nil
}

func (s *Server) loadLastDeviceResetResult() *deviceResetResult {
	data, err := os.ReadFile(s.deviceResetLastResultPath())
	if err != nil {
		return nil
	}
	var result deviceResetResult
	if json.Unmarshal(data, &result) != nil || result.Mode == "" {
		return nil
	}
	return &result
}

func (s *Server) saveLastDeviceResetResult(result *deviceResetResult) {
	if result == nil {
		return
	}
	_ = host.EnsurePrivateDir(filepath.Dir(s.deviceResetLastResultPath()))
	_ = writePrivate(s.deviceResetLastResultPath(), result)
}

func (s *Server) listDeviceResetArchives() []deviceResetArchive {
	root := s.deviceResetArchivesRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return []deviceResetArchive{}
	}
	archives := make([]deviceResetArchive, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !deviceResetIDPattern.MatchString(entry.Name()) {
			continue
		}
		manifestPath := filepath.Join(root, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var archive deviceResetArchive
		if json.Unmarshal(data, &archive) != nil || archive.State != "ready" || archive.ID != entry.Name() {
			continue
		}
		archives = append(archives, archive)
	}
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].CreatedAt > archives[j].CreatedAt
	})
	return archives
}

func (s *Server) measureDirBytes(dir string) int64 {
	if s.deviceReset.archiveDirSize != nil {
		return s.deviceReset.archiveDirSize(dir)
	}
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *Server) deviceResetAvailability() error {
	if s.clientOnly || s.windowsGuest != nil || s.macosGuest != nil {
		return errors.New("环境换机支持本机管理的 Debian 13 客机")
	}
	system, err := s.installedLinuxSystem()
	if err != nil {
		return err
	}
	if system != guestos.Debian13 {
		return errors.New("环境换机支持 Debian 13 客机。现有系统仍可继续使用")
	}
	if err := s.requireStorageReady(); err != nil {
		return err
	}
	installation, err := guestos.LoadInstallation(s.profile)
	if err != nil {
		return err
	}
	if installation != nil {
		return errors.New("请先完成当前环境的安装，再发起换机")
	}
	if err := s.checkDeviceResetArchives(); err != nil {
		return err
	}
	return cage.AllowDiskResizeStart(s.instanceDir(), "")
}

func (s *Server) deviceResetStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在已信任的 CCBox 窗口管理环境换机", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status := deviceResetStatus{
		Supported:       true,
		HostOS:          runtime.GOOS,
		Container:       containerenv.Configured(s.profile),
		Targets:         defaultDeviceResetTargets(),
		Protected:       protectedDeviceResetItems(),
		Archives:        s.listDeviceResetArchives(),
		RecoveryBatches: []deviceResetBatch{},
		LastResult:      s.loadLastDeviceResetResult(),
		AudioModels:     guestprofile.AudioModels(),
	}
	if err := s.deviceResetAvailability(); err != nil {
		status.Supported = false
		status.UnsupportedReason = err.Error()
		jsonReply(w, status)
		return
	}

	s.mu.Lock()
	ready := s.ready && !s.closing
	s.mu.Unlock()
	status.Ready = ready
	status.VMRunning = ready || s.vmRunning()

	profile, err := guestprofile.Load(s.profile)
	if err != nil {
		requestError(w, err)
		return
	}
	status.LegacyProfile = profile == nil

	runtimeKind, id, _ := s.detectRuntimeAndIdentity(ctx)
	status.RuntimeKind = runtimeKind

	var guestHostname, guestMidPrefix string
	if ready {
		if data, inspectErr := s.runGuestDeviceReset(ctx, map[string]any{"action": "inspect"}); inspectErr == nil {
			var inspected guestResetInspectResponse
			if json.Unmarshal(data, &inspected) == nil && inspected.Version == 1 {
				guestHostname = inspected.Hostname
				guestMidPrefix = inspected.MachineIDPrefix
				if len(inspected.Items) > 0 {
					status.Targets = inspected.Items
				}
				if inspected.Batches != nil {
					status.RecoveryBatches = inspected.Batches
				}
			}
		}
	}
	status.Current = buildDeviceSummary(profile, id, runtimeKind, guestHostname, guestMidPrefix)
	jsonReply(w, status)
}

func (s *Server) deviceResetPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在已信任的 CCBox 窗口预览新机配置", http.StatusForbidden)
		return
	}
	var options guestprofile.Options
	if !readJSON(w, r, &options) {
		return
	}
	if err := options.ValidatePlatform("linux"); err != nil {
		requestError(w, err)
		return
	}
	if options.GPUBackend == "virgl" && (runtime.GOOS != "linux" || containerenv.Configured(s.profile)) {
		requestError(w, errors.New("VirGL 目前仅支持 Linux 主机上的 QEMU 客机"))
		return
	}
	p, err := guestprofile.New(options)
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, p.Options)
}

func (s *Server) deviceResetExecuteHandler(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在已信任的 CCBox 窗口执行环境换机", http.StatusForbidden)
		return
	}
	if err := s.deviceResetAvailability(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var input struct {
		Mode                string                `json:"mode"`
		Items               []string              `json:"items"`
		Irreversible        bool                  `json:"irreversible"`
		ConfirmIrreversible bool                  `json:"confirm_irreversible"`
		CustomProfile       *guestprofile.Options `json:"custom_profile,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "换机请求参数无效", http.StatusBadRequest)
		return
	}
	if input.Mode != "quick" && input.Mode != "rebuild" {
		http.Error(w, "换机模式无效，请选择快速换机或彻底换新机", http.StatusBadRequest)
		return
	}
	if input.Irreversible || input.ConfirmIrreversible {
		http.Error(w, "换机必须保留可恢复归档", http.StatusBadRequest)
		return
	}
	seen := make(map[string]bool)
	cleanedItems := make([]string, 0, len(input.Items))
	for _, id := range input.Items {
		if !validDeviceResetTarget(id) || seen[id] {
			http.Error(w, "清理目标选择无效", http.StatusBadRequest)
			return
		}
		seen[id] = true
		cleanedItems = append(cleanedItems, id)
	}
	var customOpts guestprofile.Options
	if input.Mode == "rebuild" && input.CustomProfile != nil {
		customOpts = *input.CustomProfile
		if err := customOpts.Validate(); err != nil {
			requestError(w, err)
			return
		}
		if err := customOpts.ValidatePlatform("linux"); err != nil {
			requestError(w, err)
			return
		}
		if customOpts.GPUBackend == "virgl" && (runtime.GOOS != "linux" || containerenv.Configured(s.profile)) {
			requestError(w, errors.New("VirGL 目前仅支持 Linux 主机上的 QEMU 客机"))
			return
		}
	}
	if input.Mode == "quick" {
		s.mu.Lock()
		ready := s.ready && !s.closing
		s.mu.Unlock()
		if !ready {
			http.Error(w, "原地快速换机需要当前环境处于运行就绪状态；请先启动环境，或改用「彻底换新机」", http.StatusConflict)
			return
		}
	}

	s.launch(w, "device-reset", func() error {
		if input.Mode == "quick" {
			return s.executeQuickDeviceReset(cleanedItems, input.Irreversible)
		}
		return s.executeRebuildDeviceReset(customOpts, input.Irreversible)
	})
}

func (s *Server) executeQuickDeviceReset(items []string, irreversible bool) error {
	ctx := s.operationContext()
	s.setMessage("正在读取当前环境身份并生成新硬件与系统指纹…")

	oldProfile, err := guestprofile.Load(s.profile)
	if err != nil {
		return err
	}
	runtimeKind, oldIdentity, _ := s.detectRuntimeAndIdentity(ctx)

	var beforeHostname, beforeMidPrefix string
	if data, inspectErr := s.runGuestDeviceReset(ctx, map[string]any{"action": "inspect"}); inspectErr == nil {
		var inspected guestResetInspectResponse
		if json.Unmarshal(data, &inspected) == nil && inspected.Version == 1 {
			beforeHostname = inspected.Hostname
			beforeMidPrefix = inspected.MachineIDPrefix
		}
	}
	beforeSummary := buildDeviceSummary(oldProfile, oldIdentity, runtimeKind, beforeHostname, beforeMidPrefix)

	nextIdentity, err := guestidentity.New()
	if err != nil {
		return fmt.Errorf("生成新机器身份失败：%w", err)
	}
	nextProfile, err := guestprofile.RotateInPlace(oldProfile, guestprofile.Options{})
	if err != nil {
		return fmt.Errorf("生成新设备配置失败：%w", err)
	}
	if nextProfile != nil {
		nextIdentity.Hostname = nextProfile.Hostname
	}

	s.setMessage("正在客机内停止客户端、轮换系统标识并清理所选数据…")
	guestReq := map[string]any{
		"action":       "reset",
		"items":        items,
		"irreversible": irreversible,
		"rotate_os":    true,
		"hostname":     nextIdentity.Hostname,
		"machine_id":   nextIdentity.MachineID(),
	}
	if oldIdentity != nil && oldIdentity.MACAddress != "" && runtimeKind != "container" {
		guestReq["old_mac"] = oldIdentity.MACAddress
		guestReq["new_mac"] = nextIdentity.MACAddress
	}
	if oldProfile != nil && oldProfile.ExtensionSuffix != "" {
		guestReq["old_extension_suffix"] = oldProfile.ExtensionSuffix
	}
	rawOutcome, err := s.runGuestDeviceReset(ctx, guestReq)
	if err != nil {
		return fmt.Errorf("客机内重置失败，请重试：%w", err)
	}
	var outcome guestResetActionResponse
	if json.Unmarshal(rawOutcome, &outcome) != nil || outcome.Status == "" {
		return errors.New("客机内重置返回了无效结果")
	}

	s.setMessage("正在停止环境以应用新硬件身份…")
	if err := s.hookStopEnv(); err != nil {
		return fmt.Errorf("停止环境失败：%w", err)
	}

	vzReset := false
	smbiosRotated := false
	if runtimeKind != "container" {
		instName := cage.InstanceName(s.profile, s.environmentDir())
		var rotErr error
		_, vzReset, rotErr = s.hookRotateLima(ctx, instName, nextIdentity)
		if rotErr != nil {
			return fmt.Errorf("写入新虚拟机硬件身份失败：%w", rotErr)
		}
		smbiosRotated = runtimeKind == "qemu" && nextIdentity.SystemSerial != ""
	}
	if nextProfile != nil {
		if err := guestprofile.Replace(s.profile, nextProfile); err != nil {
			if runtimeKind != "container" && oldIdentity != nil {
				_, _, _ = s.hookRotateLima(ctx, cage.InstanceName(s.profile, s.environmentDir()), oldIdentity)
			}
			return fmt.Errorf("保存新设备配置失败：%w", err)
		}
	}

	s.setMessage("正在以全新身份重新启动环境…")
	startErr := s.hookStartEnv(false)
	var netErr networkCheckError
	if startErr != nil && !errors.As(startErr, &netErr) {
		return fmt.Errorf("新身份已写入，但重启环境未就绪：%w", startErr)
	}

	cleaned := make([]string, 0, len(items))
	skipped := []string{}
	for _, o := range outcome.Outcomes {
		if o.Status == "cleaned" {
			cleaned = append(cleaned, o.ID)
		} else if o.Status == "error" {
			skipped = append(skipped, o.ID)
		}
	}
	afterSummary := buildDeviceSummary(nextProfile, nextIdentity, runtimeKind, nextIdentity.Hostname, prefixOf(nextIdentity.MachineID(), 8))
	s.saveLastDeviceResetResult(&deviceResetResult{
		Mode:              "quick",
		CompletedAt:       time.Now().Unix(),
		Irreversible:      irreversible,
		BatchID:           outcome.BatchID,
		Before:            beforeSummary,
		After:             afterSummary,
		CleanedItems:      cleaned,
		SkippedItems:      skipped,
		VZIdentifierReset: vzReset,
		SMBIOSRotated:     smbiosRotated,
		OSRotated:         outcome.OSRotated,
	})
	diagnostics.Log("info", "device-reset", fmt.Sprintf("quick reset completed mode=quick runtime=%s irreversible=%v", runtimeKind, irreversible))
	if startErr != nil {
		return fmt.Errorf("换机已完成，环境已启动但外网保持关闭（建议切换代理节点后重新检测）：%w", startErr)
	}
	return nil
}

func randomArchiveID() (string, error) {
	var b [8]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func moveIfExists(src, dst string) (bool, error) {
	if _, err := os.Lstat(src); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := os.Rename(src, dst); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Server) executeRebuildDeviceReset(customOpts guestprofile.Options, irreversible bool) error {
	ctx := s.operationContext()
	s.setMessage("正在准备新机配置与安全归档…")

	oldProfile, err := guestprofile.Load(s.profile)
	if err != nil {
		return err
	}
	runtimeKind, oldIdentity, _ := s.detectRuntimeAndIdentity(ctx)
	beforeSummary := buildDeviceSummary(oldProfile, oldIdentity, runtimeKind, "", "")

	nextProfile, err := guestprofile.New(customOpts)
	if err != nil {
		return fmt.Errorf("生成新机配置失败：%w", err)
	}
	archiveID, err := randomArchiveID()
	if err != nil {
		return err
	}

	s.setMessage("正在停止当前环境…")
	if err := s.hookStopEnv(); err != nil {
		return fmt.Errorf("停止当前环境失败，原环境未改动：%w", err)
	}

	archivesRoot := s.deviceResetArchivesRoot()
	if err := host.EnsurePrivateDir(archivesRoot); err != nil {
		return err
	}
	archiveDir := filepath.Join(archivesRoot, archiveID)
	if err := host.EnsurePrivateDir(archiveDir); err != nil {
		return err
	}

	manifest := deviceResetArchive{
		ID:               archiveID,
		State:            "staging",
		CreatedAt:        time.Now().Unix(),
		RuntimeKind:      runtimeKind,
		PreviousHostname: beforeSummary.Hostname,
		PreviousUsername: beforeSummary.Username,
		PreviousProfile:  oldProfile,
	}

	if runtimeKind == "container" {
		return s.executeContainerRebuild(ctx, nextProfile, beforeSummary, archiveDir, manifest, irreversible)
	}
	return s.executeLimaRebuild(ctx, nextProfile, beforeSummary, archiveDir, manifest, irreversible)
}

func (s *Server) executeLimaRebuild(ctx context.Context, nextProfile *guestprofile.Profile, beforeSummary deviceResetSummary, archiveDir string, manifest deviceResetArchive, irreversible bool) error {
	limaHome := s.deviceResetLimaHome()
	instName := cage.InstanceName(s.profile, s.environmentDir())
	manifest.InstanceName = instName
	stateDir := filepath.Join(s.profile, "state")

	instSrc := filepath.Join(limaHome, instName)
	baseSrc := filepath.Join(limaHome, vmname.Template)
	baseVerSrc := filepath.Join(stateDir, ".agent-vm-base-version")
	baseBuiltSrc := filepath.Join(stateDir, ".agent-vm-base-built-by")
	profileSrc := filepath.Join(s.profile, guestprofile.Filename)

	instDst := filepath.Join(archiveDir, "lima-instance")
	baseDst := filepath.Join(archiveDir, "lima-base")
	baseVerDst := filepath.Join(archiveDir, "base-version")
	baseBuiltDst := filepath.Join(archiveDir, "base-built-by")
	profileDst := filepath.Join(archiveDir, "guest-profile.json")

	// existed records what was really present before any change. Rollback only
	// deletes a source path it replaced or that did not exist before; a path
	// whose archive move failed is still the user's original and must stay.
	exists := func(path string) bool {
		_, err := os.Lstat(path)
		return err == nil
	}
	hadInst, hadBase, hadVer, hadBuilt, hadProf := exists(instSrc), exists(baseSrc), exists(baseVerSrc), exists(baseBuiltSrc), exists(profileSrc)
	restoreOne := func(moved bool, dst, src string) bool {
		if !moved {
			return true
		}
		return os.Rename(dst, src) == nil
	}
	rollback := func() {
		s.setMessage("新机构建未完成，正在自动回滚原环境…")
		if err := s.hookStopEnv(); err != nil {
			diagnostics.Log("error", "device-reset", "could not stop replacement VM; original archive kept")
			return
		}
		removeNew := func(had, moved bool, path string, remove func(string) error) {
			if !had || moved {
				_ = remove(path)
			}
		}
		removeNew(hadInst, manifest.HasInstance, instSrc, os.RemoveAll)
		removeNew(hadBase, manifest.HasBaseTemplate, baseSrc, os.RemoveAll)
		_ = os.RemoveAll(filepath.Join(limaHome, "base-building"))
		removeNew(hadVer, manifest.HasBaseVersion, baseVerSrc, os.Remove)
		removeNew(hadBuilt, manifest.HasBaseBuiltBy, baseBuiltSrc, os.Remove)
		removeNew(hadProf, manifest.HasGuestProfile, profileSrc, os.Remove)
		ok := restoreOne(manifest.HasInstance, instDst, instSrc)
		ok = restoreOne(manifest.HasBaseTemplate, baseDst, baseSrc) && ok
		ok = restoreOne(manifest.HasBaseVersion, baseVerDst, baseVerSrc) && ok
		ok = restoreOne(manifest.HasBaseBuiltBy, baseBuiltDst, baseBuiltSrc) && ok
		ok = restoreOne(manifest.HasGuestProfile, profileDst, profileSrc) && ok
		if ok {
			_ = os.RemoveAll(archiveDir)
		} else {
			// Never delete the archive when any piece could not be put back.
			diagnostics.Log("error", "device-reset", "rollback incomplete; archive kept for manual recovery")
		}
	}

	var err error
	if manifest.HasInstance, err = moveIfExists(instSrc, instDst); err != nil {
		rollback()
		return fmt.Errorf("归档旧工作实例失败：%w", err)
	}
	if manifest.HasBaseTemplate, err = moveIfExists(baseSrc, baseDst); err != nil {
		rollback()
		return fmt.Errorf("归档旧基础模板失败：%w", err)
	}
	if manifest.HasBaseVersion, err = moveIfExists(baseVerSrc, baseVerDst); err != nil {
		rollback()
		return fmt.Errorf("归档模板版本标记失败：%w", err)
	}
	if manifest.HasBaseBuiltBy, err = moveIfExists(baseBuiltSrc, baseBuiltDst); err != nil {
		rollback()
		return fmt.Errorf("归档模板构建标记失败：%w", err)
	}
	if manifest.HasGuestProfile, err = moveIfExists(profileSrc, profileDst); err != nil {
		rollback()
		return fmt.Errorf("归档旧客机配置失败：%w", err)
	}
	if err := writePrivate(filepath.Join(archiveDir, "manifest.json"), manifest); err != nil {
		rollback()
		return err
	}

	if err := guestprofile.Replace(s.profile, nextProfile); err != nil {
		rollback()
		return fmt.Errorf("写入新机配置失败：%w", err)
	}

	s.setMessage("正在从本地缓存镜像重建全新客机，可能需要数分钟…")
	startErr := s.hookStartEnv(false)
	var netErr networkCheckError
	if startErr != nil && !errors.As(startErr, &netErr) {
		rollback()
		return fmt.Errorf("重建新机失败，请检查原环境或恢复归档：%w", startErr)
	}

	newRuntime, newIdentity, _ := s.detectRuntimeAndIdentity(ctx)
	finalArchiveID := manifest.ID
	if irreversible {
		s.setMessage("新机已验证就绪，正在彻底清理旧环境归档…")
		if rmErr := os.RemoveAll(archiveDir); rmErr != nil {
			diagnostics.Log("warn", "device-reset", "failed to remove archive after irreversible rebuild: "+rmErr.Error())
		}
		finalArchiveID = ""
	} else {
		manifest.State = "ready"
		manifest.Bytes = s.measureDirBytes(archiveDir)
		_ = writePrivate(filepath.Join(archiveDir, "manifest.json"), manifest)
	}

	afterSummary := buildDeviceSummary(nextProfile, newIdentity, newRuntime, nextProfile.Hostname, "")
	allIDs := make([]string, 0, len(defaultDeviceResetTargets()))
	for _, t := range defaultDeviceResetTargets() {
		allIDs = append(allIDs, t.ID)
	}
	s.saveLastDeviceResetResult(&deviceResetResult{
		Mode:              "rebuild",
		CompletedAt:       time.Now().Unix(),
		Irreversible:      irreversible,
		ArchiveID:         finalArchiveID,
		Before:            beforeSummary,
		After:             afterSummary,
		CleanedItems:      allIDs,
		VZIdentifierReset: newRuntime == "vz",
		SMBIOSRotated:     newRuntime == "qemu",
		OSRotated:         true,
	})
	diagnostics.Log("info", "device-reset", fmt.Sprintf("rebuild completed runtime=%s irreversible=%v archive=%s", newRuntime, irreversible, finalArchiveID))
	if startErr != nil {
		return fmt.Errorf("新机已重建完成，外网保持关闭（建议切换代理节点后重新检测）：%w", startErr)
	}
	return nil
}

func (s *Server) executeContainerRebuild(ctx context.Context, nextProfile *guestprofile.Profile, beforeSummary deviceResetSummary, archiveDir string, manifest deviceResetArchive, irreversible bool) error {
	env, err := containerenv.Load(s.profile)
	if err != nil {
		_ = os.RemoveAll(archiveDir)
		return err
	}
	profileSrc := filepath.Join(s.profile, guestprofile.Filename)
	profileDst := filepath.Join(archiveDir, "guest-profile.json")
	archivedContainer := false
	rollback := func(cause error) error {
		s.setMessage("新容器重建未完成，正在自动回滚原容器…")
		if archivedContainer {
			recovery, cancel := context.WithTimeout(s.lifetimeContext(), 45*time.Second)
			defer cancel()
			if err := env.Stop(recovery); err != nil {
				return errors.Join(cause, containerenv.ErrArchiveRecoveryIncomplete, err)
			}
			replacementID, err := randomArchiveID()
			if err != nil {
				return errors.Join(cause, containerenv.ErrArchiveRecoveryIncomplete, err)
			}
			if err := s.restoreContainerArchive(recovery, env, manifest, archiveDir, replacementID, true); err != nil {
				return errors.Join(cause, containerenv.ErrArchiveRecoveryIncomplete, err)
			}
			return fmt.Errorf("新容器重建未完成，已恢复原容器：%w", cause)
		}
		if manifest.HasGuestProfile {
			if err := os.Rename(profileDst, profileSrc); err != nil {
				return errors.Join(cause, containerenv.ErrArchiveRecoveryIncomplete, err)
			}
		}
		_ = os.RemoveAll(archiveDir)
		return cause
	}

	if manifest.HasGuestProfile, err = moveIfExists(profileSrc, profileDst); err != nil {
		return rollback(err)
	}
	if _, err := env.ArchiveCurrent(ctx, manifest.ID); err != nil {
		if errors.Is(err, containerenv.ErrArchiveRecoveryIncomplete) {
			return err
		}
		return rollback(err)
	}
	archivedContainer = true
	manifest.ContainerArchiveID = manifest.ID
	if err := writePrivate(filepath.Join(archiveDir, "manifest.json"), manifest); err != nil {
		return rollback(err)
	}

	nextIdentity, err := guestidentity.New()
	if err != nil {
		return rollback(err)
	}
	nextIdentity.Hostname = nextProfile.Hostname
	s.setMessage("正在基于固定基础镜像创建全新容器环境…")
	if err := env.PrepareFresh(ctx, nextProfile, nextIdentity, nil); err != nil {
		return rollback(fmt.Errorf("创建新容器失败：%w", err))
	}
	startErr := s.hookStartEnv(false)
	var netErr networkCheckError
	if startErr != nil && !errors.As(startErr, &netErr) {
		return rollback(fmt.Errorf("启动新容器失败：%w", startErr))
	}

	finalArchiveID := manifest.ID
	if irreversible {
		_ = env.DeleteArchive(ctx, manifest.ID)
		_ = os.RemoveAll(archiveDir)
		finalArchiveID = ""
	} else {
		manifest.State = "ready"
		_ = writePrivate(filepath.Join(archiveDir, "manifest.json"), manifest)
	}

	afterSummary := buildDeviceSummary(nextProfile, nextIdentity, "container", nextProfile.Hostname, prefixOf(nextIdentity.MachineID(), 8))
	allIDs := make([]string, 0, len(defaultDeviceResetTargets()))
	for _, t := range defaultDeviceResetTargets() {
		allIDs = append(allIDs, t.ID)
	}
	s.saveLastDeviceResetResult(&deviceResetResult{
		Mode:         "rebuild",
		CompletedAt:  time.Now().Unix(),
		Irreversible: irreversible,
		ArchiveID:    finalArchiveID,
		Before:       beforeSummary,
		After:        afterSummary,
		CleanedItems: allIDs,
		OSRotated:    true,
	})
	if startErr != nil {
		return fmt.Errorf("新容器已重建完成，外网保持关闭（建议切换代理节点后重新检测）：%w", startErr)
	}
	return nil
}

func (s *Server) deviceResetRestoreHandler(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在已信任的 CCBox 窗口恢复旧环境", http.StatusForbidden)
		return
	}
	if err := s.deviceResetAvailability(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var input struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "恢复请求无效", http.StatusBadRequest)
		return
	}
	if (input.Kind != "archive" && input.Kind != "batch") || !deviceResetIDPattern.MatchString(input.ID) {
		http.Error(w, "恢复目标标识无效", http.StatusBadRequest)
		return
	}
	if input.Kind == "batch" {
		if !s.op.TryLock() {
			http.Error(w, "环境正在执行其他操作，请稍后重试", http.StatusConflict)
			return
		}
		defer s.op.Unlock()
		s.mu.Lock()
		ready := s.ready && !s.closing
		s.mu.Unlock()
		if !ready {
			http.Error(w, "请先启动当前环境后再恢复数据批次", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		data, err := s.runGuestDeviceReset(ctx, map[string]any{"action": "restore", "batch": input.ID})
		if err != nil {
			requestError(w, errors.New("恢复批次失败，已有数据与恢复副本均保留"))
			return
		}
		jsonReply(w, json.RawMessage(data))
		return
	}

	s.launch(w, "device-reset-restore", func() error {
		return s.restoreDeviceArchive(input.ID)
	})
}

func (s *Server) restoreDeviceArchive(archiveID string) error {
	ctx := s.operationContext()
	archiveDir := filepath.Join(s.deviceResetArchivesRoot(), archiveID)
	data, err := os.ReadFile(filepath.Join(archiveDir, "manifest.json"))
	if err != nil {
		return errors.New("未找到指定的旧环境归档")
	}
	var manifest deviceResetArchive
	if json.Unmarshal(data, &manifest) != nil || manifest.State != "ready" || manifest.ID != archiveID {
		return errors.New("旧环境归档元数据无效，已停止恢复以保护现有数据")
	}

	s.setMessage("正在停止当前环境以回滚旧环境归档…")
	if err := s.hookStopEnv(); err != nil {
		return fmt.Errorf("停止当前环境失败：%w", err)
	}

	// The environment being replaced is never discarded: it is kept as a new
	// ready archive so the rollback itself can be undone.
	swapID, err := randomArchiveID()
	if err != nil {
		return err
	}
	swapDir := filepath.Join(s.deviceResetArchivesRoot(), swapID)

	if manifest.RuntimeKind == "container" {
		env, err := containerenv.Load(s.profile)
		if err != nil {
			return err
		}
		if err := s.restoreContainerArchive(ctx, env, manifest, archiveDir, swapID, false); err != nil {
			return err
		}
		s.setMessage("正在启动恢复后的容器环境…")
		return s.hookStartEnv(false)
	}

	limaHome := s.deviceResetLimaHome()
	instName := manifest.InstanceName
	if instName == "" {
		instName = cage.InstanceName(s.profile, s.environmentDir())
	}
	stateDir := filepath.Join(s.profile, "state")

	if err := host.EnsurePrivateDir(swapDir); err != nil {
		return err
	}
	instSrc := filepath.Join(limaHome, instName)
	baseSrc := filepath.Join(limaHome, vmname.Template)
	baseVerSrc := filepath.Join(stateDir, ".agent-vm-base-version")
	baseBuiltSrc := filepath.Join(stateDir, ".agent-vm-base-built-by")
	profileSrc := filepath.Join(s.profile, guestprofile.Filename)

	swap := deviceResetArchive{ID: swapID, State: "staging", CreatedAt: time.Now().Unix(), RuntimeKind: manifest.RuntimeKind, InstanceName: instName}
	if cur, _ := guestprofile.Load(s.profile); cur != nil {
		swap.PreviousProfile = cur
		swap.PreviousHostname = cur.Hostname
		swap.PreviousUsername = cur.Username
	}
	type pair struct {
		src, dst string
		moved    *bool
	}
	curPairs := []pair{
		{instSrc, filepath.Join(swapDir, "lima-instance"), &swap.HasInstance},
		{baseSrc, filepath.Join(swapDir, "lima-base"), &swap.HasBaseTemplate},
		{baseVerSrc, filepath.Join(swapDir, "base-version"), &swap.HasBaseVersion},
		{baseBuiltSrc, filepath.Join(swapDir, "base-built-by"), &swap.HasBaseBuiltBy},
		{profileSrc, filepath.Join(swapDir, "guest-profile.json"), &swap.HasGuestProfile},
	}
	archPairs := []pair{
		{filepath.Join(archiveDir, "lima-instance"), instSrc, &manifest.HasInstance},
		{filepath.Join(archiveDir, "lima-base"), baseSrc, &manifest.HasBaseTemplate},
		{filepath.Join(archiveDir, "base-version"), baseVerSrc, &manifest.HasBaseVersion},
		{filepath.Join(archiveDir, "base-built-by"), baseBuiltSrc, &manifest.HasBaseBuiltBy},
		{filepath.Join(archiveDir, "guest-profile.json"), profileSrc, &manifest.HasGuestProfile},
	}
	// Persist the intent first so an interrupted swap leaves a self-describing
	// staging directory instead of unexplained files.
	_ = writePrivate(filepath.Join(swapDir, "manifest.json"), swap)

	// undo puts every moved piece back. Nothing is deleted: the only removals
	// are of the (empty) swap directory after all pieces returned.
	var restoredFromArchive []pair
	undo := func() {
		for i := len(restoredFromArchive) - 1; i >= 0; i-- {
			p := restoredFromArchive[i]
			_ = os.Rename(p.dst, p.src)
		}
		ok := true
		for _, p := range curPairs {
			if *p.moved && os.Rename(p.dst, p.src) != nil {
				ok = false
			}
		}
		if ok {
			_ = os.Remove(filepath.Join(swapDir, "manifest.json"))
			_ = os.Remove(swapDir)
		}
	}

	for _, p := range curPairs {
		moved, err := moveIfExists(p.src, p.dst)
		if err != nil {
			undo()
			return fmt.Errorf("暂存当前环境失败，未做任何改动：%w", err)
		}
		*p.moved = moved
	}
	for _, p := range archPairs {
		if !*p.moved {
			continue
		}
		if err := os.Rename(p.src, p.dst); err != nil {
			undo()
			return fmt.Errorf("恢复旧环境失败，已还原当前环境：%w", err)
		}
		restoredFromArchive = append(restoredFromArchive, p)
	}

	// Keep the replaced environment as a normal rollback target.
	if swap.HasInstance || swap.HasBaseTemplate || swap.HasGuestProfile {
		swap.State = "ready"
		swap.Bytes = s.measureDirBytes(swapDir)
		_ = writePrivate(filepath.Join(swapDir, "manifest.json"), swap)
	} else {
		_ = os.Remove(filepath.Join(swapDir, "manifest.json"))
		_ = os.Remove(swapDir)
	}
	_ = os.RemoveAll(archiveDir)
	s.setMessage("正在启动恢复后的旧环境…")
	return s.hookStartEnv(false)
}

type containerArchiveSwapper interface {
	SwapArchive(ctx context.Context, archiveID, replacementID string) (bool, error)
}

func (s *Server) restoreContainerArchive(ctx context.Context, env containerArchiveSwapper, manifest deviceResetArchive, archiveDir, swapID string, rebuilding bool) error {
	archivedProfile := filepath.Join(archiveDir, "guest-profile.json")
	if manifest.HasGuestProfile {
		info, err := os.Lstat(archivedProfile)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("旧容器归档缺少有效的客机配置，当前环境已保留")
		}
	}
	swapDir := filepath.Join(s.deviceResetArchivesRoot(), swapID)
	if err := host.EnsurePrivateDir(swapDir); err != nil {
		return err
	}
	swapPath := filepath.Join(swapDir, "manifest.json")
	swapProfile := filepath.Join(swapDir, "guest-profile.json")
	liveProfile := filepath.Join(s.profile, guestprofile.Filename)
	swap := deviceResetArchive{ID: swapID, State: "staging", CreatedAt: time.Now().Unix(), RuntimeKind: "container", ContainerArchiveID: swapID}
	if current, err := guestprofile.Load(s.profile); err == nil && current != nil {
		swap.PreviousHostname = current.Hostname
	}
	if err := writePrivate(swapPath, swap); err != nil {
		_ = os.Remove(swapDir)
		return err
	}
	rollback := func(err error) error {
		if swap.HasGuestProfile {
			if moveErr := os.Rename(swapProfile, liveProfile); moveErr != nil {
				return errors.Join(containerenv.ErrArchiveRecoveryIncomplete, err, moveErr)
			}
		}
		_ = os.Remove(swapPath)
		_ = os.Remove(swapDir)
		return err
	}
	moved, err := moveIfExists(liveProfile, swapProfile)
	if err != nil {
		return rollback(err)
	}
	swap.HasGuestProfile = moved
	if err := writePrivate(swapPath, swap); err != nil {
		return rollback(err)
	}
	preserved, err := env.SwapArchive(ctx, manifest.ContainerArchiveID, swapID)
	if errors.Is(err, containerenv.ErrArchiveRecoveryIncomplete) {
		return err
	}
	if err != nil {
		return rollback(err)
	}
	// A failed rebuild can have created its new profile but no new
	// container. An ordinary restore must preserve a pre-existing profile.
	if !preserved && moved && !rebuilding {
		return errors.Join(containerenv.ErrArchiveRecoveryIncomplete, errors.New("原客机配置已保留，但未找到对应容器"))
	}
	if manifest.HasGuestProfile {
		if err := os.Rename(archivedProfile, liveProfile); err != nil {
			return errors.Join(containerenv.ErrArchiveRecoveryIncomplete, err)
		}
	}
	// Retain the staging guard until both the active profile and the archive
	// index match the container engine's names.
	if err := os.RemoveAll(archiveDir); err != nil {
		return errors.Join(containerenv.ErrArchiveRecoveryIncomplete, err)
	}
	if preserved {
		swap.State = "ready"
		return writePrivate(swapPath, swap)
	}
	return os.RemoveAll(swapDir)
}

func (s *Server) deviceResetDeleteArchiveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.hasTrustedView(r) {
		http.Error(w, "请在已信任的 CCBox 窗口管理旧环境归档", http.StatusForbidden)
		return
	}
	if s.clientOnly {
		http.Error(w, "请在虚拟机所在电脑上管理旧环境归档", http.StatusConflict)
		return
	}
	var input struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "删除归档请求无效", http.StatusBadRequest)
		return
	}
	if (input.Kind != "archive" && input.Kind != "batch") || !deviceResetIDPattern.MatchString(input.ID) {
		http.Error(w, "归档标识无效", http.StatusBadRequest)
		return
	}
	if !s.op.TryLock() {
		http.Error(w, "环境正在执行其他操作，请稍后重试", http.StatusConflict)
		return
	}
	defer s.op.Unlock()

	if input.Kind == "batch" {
		s.mu.Lock()
		ready := s.ready && !s.closing
		s.mu.Unlock()
		if !ready {
			http.Error(w, "请先启动当前环境后再删除恢复批次", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		data, err := s.runGuestDeviceReset(ctx, map[string]any{"action": "delete_batch", "batch": input.ID})
		if err != nil {
			requestError(w, errors.New("删除恢复批次失败"))
			return
		}
		jsonReply(w, json.RawMessage(data))
		return
	}

	archiveDir := filepath.Join(s.deviceResetArchivesRoot(), input.ID)
	data, err := os.ReadFile(filepath.Join(archiveDir, "manifest.json"))
	if err != nil {
		http.Error(w, "未找到指定的旧环境归档", http.StatusNotFound)
		return
	}
	var manifest deviceResetArchive
	if json.Unmarshal(data, &manifest) == nil && manifest.RuntimeKind == "container" && manifest.ContainerArchiveID != "" {
		if env, loadErr := containerenv.Load(s.profile); loadErr == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := env.DeleteArchive(ctx, manifest.ContainerArchiveID); err != nil {
				requestError(w, fmt.Errorf("删除归档容器失败：%w", err))
				return
			}
		}
	}
	if err := os.RemoveAll(archiveDir); err != nil {
		requestError(w, fmt.Errorf("删除归档目录失败：%w", err))
		return
	}
	jsonReply(w, map[string]any{"status": "ok", "id": input.ID})
}
