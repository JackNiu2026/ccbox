package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

func TestGuestConnectionRequiresSuccessfulMatchingManifest(t *testing.T) {
	required := cage.RequiredGuestManifest()
	for _, name := range []string{"current", "missing", "old-revision", "old-protocol", "broken"} {
		t.Run(name, func(t *testing.T) {
			manifest := required
			if name == "old-revision" {
				manifest.Revision = "previous"
			}
			if name == "old-protocol" {
				manifest.Protocol++
			}
			encoded, _ := json.Marshal(manifest)
			if name == "missing" {
				encoded = []byte("null")
			}
			if name == "broken" {
				encoded = []byte("{")
			}
			credentials, current, err := parseGuestConnection(append([]byte("{\"token\":\"guest-test\"}\npassword\n"), encoded...))
			if name == "current" {
				if err != nil || current == nil || credentials.Token != "guest-test" || credentials.VNCPassword != "password" {
					t.Fatal(current, err)
				}
			} else if !errors.Is(err, errGuestUpdateRequired) {
				t.Fatalf("stale guest accepted: %v", err)
			}
		})
	}
}

func TestGuestConnectionRejectsInterruptedMaintenanceEvenWithCurrentManifest(t *testing.T) {
	manifest, err := json.Marshal(cage.RequiredGuestManifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"updating":true}`, string(manifest) + "\nupdating"} {
		_, got, err := parseGuestConnection([]byte("{\"token\":\"test\"}\npassword\n" + body))
		if !errors.Is(err, errGuestUpdateRequired) || !strings.Contains(err.Error(), "更新中断") || got == nil || got.Current() {
			t.Fatalf("maintenance snapshot %q bypassed repair: current=%+v error=%v", body, got, err)
		}
	}
}

func TestGuestConnectionKeepsShipped0200GuestUsable(t *testing.T) {
	shipped := cage.GuestManifest{
		Protocol: 1, AppVersion: "0.20.0",
		Revision: "934074036b1e9f32d67b9b30c7cd0aa9a25481db88d51ec14c7189cf267f7f6e",
	}
	for _, test := range []struct {
		name    string
		change  func(*cage.GuestManifest)
		allowed bool
	}{
		{"shipped", func(*cage.GuestManifest) {}, true},
		{"local Windows preview", func(m *cage.GuestManifest) {
			m.Revision = "490b498591785b10a317e3b479eb0e446fa8969d416c374f193b5617ffa73d9d"
		}, true},
		{"interrupted", func(m *cage.GuestManifest) { m.Updating = true }, false},
		{"other protocol", func(m *cage.GuestManifest) { m.Protocol++ }, false},
		{"other version", func(m *cage.GuestManifest) { m.AppVersion = "0.19.3" }, false},
		{"unidentified scripts", func(m *cage.GuestManifest) { m.Revision = strings.Repeat("a", 64) }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := shipped
			test.change(&manifest)
			encoded, _ := json.Marshal(manifest)
			got, current, err := parseGuestConnection(append([]byte("{\"token\":\"existing-test-token\"}\nexisting-test-password\n"), encoded...))
			if test.allowed {
				if err != nil || current == nil || current.Current() ||
					got.Token != "existing-test-token" || got.VNCPassword != "existing-test-password" {
					t.Fatalf("compatible guest cannot reuse its credentials: manifest=%+v error=%v", current, err)
				}
			} else if !errors.Is(err, errGuestUpdateRequired) {
				t.Fatalf("unexpected legacy guest accepted: %v", err)
			}
		})
	}
}

func TestVersionQueryPreservesNewerMaintenanceDecision(t *testing.T) {
	s := testServer(t)
	s.noteGuestUpdateRequired(0, errGuestMaintenanceInterrupted.Error())
	if !s.guestUpdateRequired || s.guestUpdateError != errGuestMaintenanceInterrupted.Error() {
		t.Fatal("guest maintenance requirement did not reach status")
	}
	s.mu.Lock()
	s.setGuestUpdateLocked(false, "")
	generation := s.guestUpdateGeneration
	s.mu.Unlock()

	s.noteGuestUpdateRequired(0, errGuestUpdateRequired.Error())
	if s.guestUpdateRequired {
		t.Fatal("old version query overrode a newer attach")
	}
	s.setJobForTest(Job{ID: "update", Kind: "environment-update", Running: true})
	s.noteGuestUpdateRequired(generation, errGuestUpdateRequired.Error())
	if s.guestUpdateRequired {
		t.Fatal("version query overrode ongoing maintenance")
	}
}

func grantedToolUpdateServer(t *testing.T) (*Server, *cage.EgressGate) {
	t.Helper()
	s, gate := interfaceMonitorServer(t)
	grantTestGate(t, gate)
	s.ready = true
	required := cage.RequiredGuestManifest()
	s.guestManifest = &required
	s.setJobForTest(Job{ID: "tools", Kind: "environment-tools", Running: true})
	s.publishGate(gate.Status())
	return s, gate
}

func grantTestGate(t *testing.T, gate *cage.EgressGate) {
	t.Helper()
	state := gate.Status()
	report := cage.SafetyReport{Policy: cage.SafetyPolicyVersion, ID: "tool-update-test", RunID: state.RunID, CheckID: state.CheckID, Configuration: state.Configuration, CreatedAt: time.Now()}
	for _, id := range []string{"egress_policy", "proxy_transport", "dns_transport", "guest_settings", "browser_settings", "filesystem_boundary", "management_boundary", "fail_closed", "udp_policy", "ipv6_policy", "exit_consistency"} {
		report.Checks = append(report.Checks, cage.SafetyCheck{ID: id, Title: id, Critical: true, State: cage.CheckPassed, Evidence: "synthetic isolated fixture", CheckedAt: report.CreatedAt})
	}
	if err := gate.Grant(report); err != nil {
		t.Fatal(err)
	}
}

func TestToolUpdateLeaseHonorsPauseStopAndConfigurationAtRenewal(t *testing.T) {
	for _, jobKind := range []string{"environment-tools", "claude-desktop-install"} {
		t.Run(jobKind, func(t *testing.T) {
			for _, condition := range []string{"allowed", "paused", "stop", "configuration-changed"} {
				t.Run(condition, func(t *testing.T) {
					s, gate := grantedToolUpdateServer(t)
					s.setJobForTest(Job{ID: "tools", Kind: jobKind, Running: true})
					client, err := s.gateClient()
					if err != nil {
						t.Fatal(err)
					}
					before := gate.Status()
					// Windows can grant and renew within the same wall-clock tick.
					// Advance past the grant tick before requiring a later deadline.
					for !time.Now().Add(cage.GateLease).After(before.Until) {
						time.Sleep(time.Millisecond)
					}
					switch condition {
					case "paused":
						if err := s.pauseNetworkNow("test user pause"); err != nil {
							t.Fatal(err)
						}
					case "stop":
						s.setJobForTest(Job{ID: "stop", Kind: "stop", Running: true})
					case "configuration-changed":
						cfg, err := loadConfig(s.configPath)
						if err != nil {
							t.Fatal(err)
						}
						cfg.Timezone = "Etc/UTC"
						if cfg.Fingerprint() == before.Configuration {
							cfg.Timezone = "Asia/Tokyo"
						}
						if err := writePrivate(s.configPath, cfg); err != nil {
							t.Fatal(err)
						}
					}
					s.finishSafetyMonitor(context.Background(), client, before, "192.0.2.1", "")
					after := gate.Status()
					switch condition {
					case "allowed":
						if after.Mode != cage.GateOpen || !after.Until.After(before.Until) {
							t.Fatal("tool update lease was not renewed")
						}
					case "paused", "configuration-changed":
						if after.Mode != cage.GateClosed {
							t.Fatal("denied network was renewed")
						}
					case "stop":
						if after.Until != before.Until {
							t.Fatal("stopping job allowed a lease renewal")
						}
					}
				})
			}
		})
	}
}

func TestToolUpdateDoesNotRestorePausedOrExpiredNetworking(t *testing.T) {
	for _, paused := range []bool{true, false} {
		s, gate := grantedToolUpdateServer(t)
		if paused {
			if err := s.pauseNetworkNow("manual"); err != nil {
				t.Fatal(err)
			}
		} else {
			gate.Close("expired")
		}
		s.mu.Lock()
		allowed := s.toolsNetworkAllowedLocked()
		s.mu.Unlock()
		if !allowed {
			t.Fatal("business network pause blocked maintenance")
		}
		if gate.Status().Mode != cage.GateClosed {
			t.Fatal("tool update restored closed network")
		}
	}
}

func TestEnvironmentVersionRoutesRequireHostSession(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/api/environment/versions", "/api/environment/update", "/api/environment/tools"} {
		method := "POST"
		if strings.HasSuffix(path, "versions") {
			method = "GET"
		}
		if w := request(s, method, path, s.origin, "{}", false); w.Code != 401 {
			t.Fatalf("untrusted version action accepted: %d", w.Code)
		}
	}
	w := request(s, "GET", "/api/environment/versions", s.origin, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"stopped"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
