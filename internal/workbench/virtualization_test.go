package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/host"
)

// Build hosts have no KVM. Other tests in this package exercise installs, so
// they see a ready host unless a test asks otherwise.
func init() {
	checkVirtualization = func(context.Context) host.VirtualizationReport {
		return host.VirtualizationReport{State: host.VirtReady, Ready: true}
	}
}

func stubVirtualization(t *testing.T, report host.VirtualizationReport) {
	t.Helper()
	previous := checkVirtualization
	checkVirtualization = func(context.Context) host.VirtualizationReport { return report }
	t.Cleanup(func() { checkVirtualization = previous })
}

// trustedRequest is the local CCBox window: session plus trusted-view proof.
func trustedRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	r.Header.Set("Origin", s.origin)
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	r.AddCookie(&http.Cookie{Name: s.cookieName() + "-desktop", Value: trustedViewProof(s.token)})
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}

var featureOff = host.VirtualizationReport{State: host.VirtFeatureOff, Summary: "Windows 虚拟机监控程序平台尚未启用。", Steps: []string{"一键启用"}, CanEnable: true}

func startRequest(t *testing.T, s *Server, body string) (*httptest.ResponseRecorder, Job) {
	t.Helper()
	// Hold the operation lock so an accepted job cannot touch a real VM.
	s.op.Lock()
	r := httptest.NewRequest(http.MethodPost, "/api/start", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: s.cookieName() + "-desktop", Value: trustedViewProof(s.token)})
	w := httptest.NewRecorder()
	s.start(w, r)
	job := s.jobs.snapshot()
	s.jobs.cancelRunning()
	s.op.Unlock()
	s.jobs.workers.Wait()
	return w, job
}

func TestLinuxInstallsStopBeforeDownloadWhenVirtualizationIsUnavailable(t *testing.T) {
	for _, body := range []string{`{"guest_system":"ubuntu-22.04"}`, `{"guest_system":"debian-13"}`, `{}`} {
		s := testServer(t)
		stubVirtualization(t, featureOff)
		w, job := startRequest(t, s, body)
		if w.Code != http.StatusConflict || job.ID != "" || !strings.Contains(w.Body.String(), featureOff.Summary) {
			t.Fatalf("%s: fresh install not stopped before download: %d %q job=%+v", body, w.Code, w.Body.String(), job)
		}
	}
}

func TestReadyHostStillInstalls(t *testing.T) {
	s := testServer(t)
	stubVirtualization(t, host.VirtualizationReport{State: host.VirtReady, Ready: true})
	w, job := startRequest(t, s, `{"guest_system":"ubuntu-22.04"}`)
	if w.Code != http.StatusOK || job.Kind != "guest-install" {
		t.Fatalf("ready host refused: %d %s", w.Code, w.Body.String())
	}
}

func TestExistingEnvironmentStartIsNotGated(t *testing.T) {
	s := testServer(t)
	if err := os.MkdirAll(filepath.Join(s.profile, "state", "lima", "existing"), 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	previous := checkVirtualization
	checkVirtualization = func(context.Context) host.VirtualizationReport { called = true; return featureOff }
	t.Cleanup(func() { checkVirtualization = previous })
	w, job := startRequest(t, s, `{}`)
	if called || w.Code != http.StatusOK || job.Kind != "start" {
		t.Fatalf("existing environment start changed: checked=%v %d %s", called, w.Code, w.Body.String())
	}
}

func TestVirtualizationReportRequiresTrustedWindow(t *testing.T) {
	s := testServer(t)
	stubVirtualization(t, featureOff)
	// A signed-in page that is not the local desktop window.
	if w := request(s, http.MethodGet, "/api/environment/virtualization", s.origin, "", true); w.Code != http.StatusForbidden {
		t.Fatalf("untrusted page read diagnosis: %d", w.Code)
	}
	w := trustedRequest(s, http.MethodGet, "/api/environment/virtualization", "")
	var report host.VirtualizationReport
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &report) != nil || report.State != host.VirtFeatureOff || !report.CanEnable {
		t.Fatalf("diagnosis: %d %s", w.Code, w.Body.String())
	}
	if w := request(s, http.MethodPost, "/api/environment/virtualization", s.origin, `{"action":"enable"}`, true); w.Code != http.StatusForbidden {
		t.Fatalf("untrusted page changed system settings: %d", w.Code)
	}
}

func TestVirtualizationActions(t *testing.T) {
	previousHost, previousEnable, previousRestart, previousFirmware := windowsHost, enableVirtualization, restartWindows, restartIntoFirmware
	t.Cleanup(func() {
		windowsHost, enableVirtualization, restartWindows, restartIntoFirmware = previousHost, previousEnable, previousRestart, previousFirmware
	})
	calls := []string{}
	enableErr := error(nil)
	enableVirtualization = func(context.Context) error { calls = append(calls, "enable"); return enableErr }
	restartWindows = func(context.Context) error { calls = append(calls, "restart"); return nil }
	restartIntoFirmware = func(context.Context) error { calls = append(calls, "firmware"); return nil }
	post := func(s *Server, action string) *httptest.ResponseRecorder {
		return trustedRequest(s, http.MethodPost, "/api/environment/virtualization", `{"action":"`+action+`"}`)
	}

	s := testServer(t)
	windowsHost = false
	if w := post(s, "enable"); w.Code != http.StatusConflict || len(calls) != 0 {
		t.Fatalf("non-Windows host ran a Windows action: %d %v", w.Code, calls)
	}
	windowsHost = true

	stubVirtualization(t, host.VirtualizationReport{State: host.VirtReady, Ready: true})
	if w := post(s, "enable"); w.Code != http.StatusConflict || len(calls) != 0 {
		t.Fatalf("enabled a ready host: %d %v", w.Code, calls)
	}
	if w := post(s, "firmware"); w.Code != http.StatusConflict || len(calls) != 0 {
		t.Fatalf("firmware restart offered without a firmware problem: %d %v", w.Code, calls)
	}
	if w := post(s, "shutdown"); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown action: %d", w.Code)
	}

	stubVirtualization(t, featureOff)
	w := post(s, "enable")
	var after host.VirtualizationReport
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &after) != nil || !after.NeedsRestart || after.Ready || strings.Join(calls, ",") != "enable" {
		t.Fatalf("enable: %d %s %v", w.Code, w.Body.String(), calls)
	}
	enableErr = errors.New("已取消 Windows 管理员授权，设置没有改变")
	if w := post(s, "enable"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "已取消") {
		t.Fatalf("declined authorization: %d %s", w.Code, w.Body.String())
	}
	if w := post(s, "restart"); w.Code != http.StatusOK || calls[len(calls)-1] != "restart" {
		t.Fatalf("restart: %d %v", w.Code, calls)
	}

	stubVirtualization(t, host.VirtualizationReport{State: host.VirtFirmwareOff, FirmwareRestart: true, Firmware: &host.FirmwareGuide{Keys: "F2"}})
	if w := post(s, "firmware"); w.Code != http.StatusOK || calls[len(calls)-1] != "firmware" {
		t.Fatalf("firmware restart: %d %v", w.Code, calls)
	}
}
