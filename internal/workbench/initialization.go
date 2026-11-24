package workbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/guestos"
)

type initializationRecord struct {
	Version   int  `json:"version"`
	Completed bool `json:"completed"`
	Started   bool `json:"started,omitempty"`
}

type initializationView struct {
	Required bool `json:"required"`
	Started  bool `json:"started"`
	Tracked  bool `json:"tracked"`
	Prepared bool `json:"prepared"`
}

func (s *Server) initializationState() (initializationView, error) {
	if !s.localVMStorage() || s.storageRecoveryStatus().Required {
		return initializationView{}, nil
	}
	data, err := os.ReadFile(filepath.Join(s.profile, "initialization.json"))
	if err == nil {
		var record initializationRecord
		if json.Unmarshal(data, &record) != nil || record.Version != 1 {
			return initializationView{}, errors.New("初始化记录无法读取，请保留现有数据并导出诊断包")
		}
		pending, err := guestos.LoadInstallation(s.profile)
		if err != nil {
			return initializationView{}, err
		}
		unboundMac := pending != nil && pending.System == guestos.MacOS && s.macosGuest == nil
		return initializationView{Required: !record.Completed || s.macSetupPending() || unboundMac, Started: record.Started || unboundMac, Tracked: true, Prepared: s.environmentPrepared()}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return initializationView{}, err
	}
	// Existing installations predate this screen. A directory or partial disk
	// alone is not proof of a completed Linux installation.
	pending, err := guestos.LoadInstallation(s.profile)
	if err != nil {
		return initializationView{}, err
	}
	if s.macosGuest != nil || s.windowsGuest != nil {
		return initializationView{Required: s.macSetupPending()}, nil
	}
	return initializationView{Required: pending != nil || !s.environmentPrepared(), Started: pending != nil}, nil
}

func (s *Server) finishInitialization() error {
	if s.macSetupPending() {
		return errors.New("请先完成客机的系统权限确认")
	}
	s.mu.Lock()
	ready, gate := s.ready, s.safety.Gate
	s.mu.Unlock()
	if !ready || !s.vmRunning() {
		return errors.New("客机尚未就绪，请继续安装并完成启动检查")
	}
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	if err := cfg.RequireManagedProxy(s.profile); err != nil {
		return err
	}
	if gate == nil || gate.Mode != cage.GateOpen || !gate.PolicyVerified || gate.Fault || gate.Report == nil || !gate.Until.After(time.Now()) || gate.Configuration != cfg.Fingerprint() || gate.Report.Authorizes(gate.RunID, cfg.Fingerprint(), time.Now()) != nil {
		return errors.New("环境网络检查尚未通过，请在初始化界面重新检查")
	}
	return writePrivate(filepath.Join(s.profile, "initialization.json"), initializationRecord{Version: 1, Completed: true})
}

// Earlier builds removed the initial-provision marker even while the guest's
// update marker was present. A user resuming tracked onboarding can finish
// that transaction in the existing guest. Ordinary starts of installed
// environments continue to require explicit component maintenance.
func (s *Server) canResumeInterruptedProvision(jobKind string, err error) bool {
	if (jobKind != "start" && jobKind != "guest-install") || !errors.Is(err, errGuestMaintenanceInterrupted) {
		return false
	}
	view, stateErr := s.initializationState()
	return stateErr == nil && view.Tracked && view.Required && view.Started
}

func (s *Server) initialization(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		state, err := s.initializationState()
		if err != nil {
			requestError(w, err)
			return
		}
		jsonReply(w, state)
		return
	}
	if !s.hasTrustedView(r) || !s.localVMStorage() {
		http.Error(w, "请在本机 CCBox 窗口完成初始化", http.StatusForbidden)
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if !s.lockForInitialization(w, r) {
		return
	}
	defer s.op.Unlock()
	if job := s.jobs.snapshot(); job.Running {
		http.Error(w, initializationBusyMessage(job), http.StatusConflict)
		return
	}
	state, err := s.initializationState()
	if err != nil {
		requestError(w, err)
		return
	}
	switch input.Action {
	case "begin":
		if state.Required {
			err = writePrivate(filepath.Join(s.profile, "initialization.json"), initializationRecord{Version: 1, Started: state.Started})
		}
	case "install":
		if state.Required {
			err = writePrivate(filepath.Join(s.profile, "initialization.json"), initializationRecord{Version: 1, Started: true})
		}
	case "complete":
		err = s.finishInitialization()
	default:
		err = errors.New("未知的初始化操作")
	}
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, map[string]bool{"completed": input.Action == "complete" || !state.Required})
}

func initializationBusyMessage(job Job) string {
	return fmt.Sprintf("当前正在%s，完成后即可继续。", jobName(job.Kind))
}

func (s *Server) lockForInitialization(w http.ResponseWriter, r *http.Request) bool {
	// Health and network monitors also hold op briefly without publishing a job.
	// Wait for those reads, while leaving real jobs in control of the environment.
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()
	for r.Context().Err() == nil {
		if job := s.jobs.snapshot(); job.Running {
			http.Error(w, initializationBusyMessage(job), http.StatusConflict)
			return false
		}
		if s.op.TryLock() {
			return true
		}
		select {
		case <-r.Context().Done():
			return false
		case <-deadline.C:
			http.Error(w, "后台环境检查尚未完成。请稍后重试，当前设置已保留。", http.StatusServiceUnavailable)
			return false
		case <-retry.C:
		}
	}
	return false
}
