package workbench

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

func TestStoppedEnvironmentClearsOnlyItsMissingInterfaceStatus(t *testing.T) {
	s, _ := interfaceMonitorServer(t)
	if err := os.Remove(filepath.Join(s.instanceDir(), "cage-running.json")); err != nil {
		t.Fatal(err)
	}
	oldInterface, oldRunning, oldStart := physicalProxyInterface, proxyInterfaceCoreRunning, startProxyCore
	t.Cleanup(func() {
		physicalProxyInterface, proxyInterfaceCoreRunning, startProxyCore = oldInterface, oldRunning, oldStart
	})
	proxyInterfaceCoreRunning = func(context.Context, string, managedproxy.Store) bool { return true }
	starts := 0
	startProxyCore = func(context.Context, string, managedproxy.Store) error { starts++; return nil }
	physicalProxyInterface = func(context.Context) (string, error) { return "", errors.New("missing uplink") }
	s.monitorProxyInterface()
	if !s.proxyNetworkLost || s.safety.monitorError != "missing uplink" {
		t.Fatal("missing uplink was not recorded")
	}
	s.proxyRebindAfter = time.Now().Add(-time.Second)
	s.networkPaused = true
	physicalProxyInterface = func(context.Context) (string, error) { return "en0", nil }
	s.monitorProxyInterface()
	s.jobs.workers.Wait()
	if s.proxyNetworkLost || s.safety.monitorError != "" || starts != 0 || !s.networkPaused {
		t.Fatalf("unchanged uplink restarted the idle core or changed network intent: starts=%d", starts)
	}
}

func TestConcurrentProxyBindingChecksSurviveFirstRequestCancellation(t *testing.T) {
	s, _ := interfaceMonitorServer(t)
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	oldInterface, oldRunning := physicalProxyInterface, proxyInterfaceCoreRunning
	t.Cleanup(func() { physicalProxyInterface, proxyInterfaceCoreRunning = oldInterface, oldRunning })
	proxyInterfaceCoreRunning = func(context.Context, string, managedproxy.Store) bool { return true }
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	physicalProxyInterface = func(ctx context.Context) (string, error) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			return "en0", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- s.ensureProxyInterfaceBeforeUse(ctx, cfg) }()
	<-entered
	go func() { second <- s.ensureProxyInterfaceBeforeUse(t.Context(), cfg) }()
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Errorf("first check = %v", err)
	}
	// Allow the second handler to join the in-flight request before its
	// simulated OS route lookup can finish.
	select {
	case <-entered:
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-second; err != nil || calls.Load() != 1 {
		t.Fatalf("second check = %v, OS route lookups = %d", err, calls.Load())
	}
}

func interfaceMonitorServer(t *testing.T) (*Server, *cage.EgressGate) {
	t.Helper()
	s := testServer(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	cfg, _ := loadConfig(s.configPath)
	p, _ := managedproxy.Load(s.profile)
	p.Port = listener.Addr().(*net.TCPAddr).Port
	if err = p.Save(s.profile); err != nil {
		t.Fatal(err)
	}
	cfg.ProxyURL = p.Endpoint()
	if err := writePrivate(s.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := os.MkdirAll(filepath.Join(s.profile, "proxy"), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("interface-name: en0\nmixed-port: " + strconv.Itoa(port) + "\n")
	if err := os.WriteFile(filepath.Join(s.profile, "proxy", "config.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.profile, "state", "lima", cage.InstanceName(s.profile, s.environmentDir()))
	if err := writePrivate(filepath.Join(dir, "cage-running.json"), map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	gate, err := cage.NewEgressGate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := cage.StartGateControl(s.configPath, dir, gate)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	if err := gate.BeginCheck(); err != nil {
		t.Fatal(err)
	}
	return s, gate
}

func TestInterfaceRecoveryRevokesGateAndPreservesManualPause(t *testing.T) {
	previous := physicalProxyInterface
	t.Cleanup(func() { physicalProxyInterface = previous })
	for _, tc := range []struct {
		name, next              string
		paused, lost, staleNode bool
	}{
		{"Wi-Fi to Ethernet", "en7", false, false, false},
		{"Ethernet while paused", "en7", true, false, false},
		{"same interface after outage", "en0", false, true, false},
		{"same uplink with stale node route", "en0", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, gate := interfaceMonitorServer(t)
			if tc.staleNode {
				config := "interface-name: en0\nproxies:\n  - {server: 192.0.2.1, interface-name: old-adapter}\n"
				if err := os.WriteFile(filepath.Join(s.profile, "proxy", "config.yaml"), []byte(config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s.networkPaused, s.proxyNetworkLost = tc.paused, tc.lost
			physicalProxyInterface = func(context.Context) (string, error) { return tc.next, nil }
			// Keep the recovery queued: assert the synchronous revocation before
			// any core can restart, then cancel through the normal stop path.
			s.op.Lock()
			s.monitorProxyInterface()
			s.mu.Lock()
			queued, paused := s.jobs.snapshot().Kind == "proxy-network" && s.jobs.snapshot().Running, s.networkPaused
			s.mu.Unlock()
			closed := gate.Status().Mode == cage.GateClosed
			finished := make(chan struct{})
			s.launch(httptest.NewRecorder(), "stop", func() error { close(finished); return nil })
			s.op.Unlock()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("stop could not cancel network recovery")
			}
			s.op.Lock()
			s.op.Unlock()
			if !queued || !closed || paused != tc.paused {
				t.Fatalf("queued=%v closed=%v paused=%v; wanted pause=%v", queued, closed, paused, tc.paused)
			}
		})
	}
}

func TestMissingPhysicalInterfaceClosesGateWithoutUnboundFallback(t *testing.T) {
	s, gate := interfaceMonitorServer(t)
	previous := physicalProxyInterface
	t.Cleanup(func() { physicalProxyInterface = previous })
	physicalProxyInterface = func(context.Context) (string, error) { return "", errors.New("physical network unavailable") }
	s.monitorProxyInterface()
	if gate.Status().Mode != cage.GateClosed || s.networkPaused || !s.proxyNetworkLost || s.jobs.snapshot().Running || s.currentNetworkErrorLocked() == "" {
		t.Fatal("missing uplink did not preserve offline recovery state")
	}
	data, _ := os.ReadFile(filepath.Join(s.profile, "proxy", "config.yaml"))
	if !strings.Contains(string(data), "interface-name: en0") {
		t.Fatal("missing uplink removed the existing binding")
	}
}

// Test the actual job queue and gate control with a transient proxy validation failure.
func TestSameInterfaceRecoveryRetainsOutageAfterFailedRebind(t *testing.T) {
	s, gate := interfaceMonitorServer(t)
	previousInterface, previousValidate, previousStart := physicalProxyInterface, validateProxyCore, startProxyCore
	t.Cleanup(func() {
		physicalProxyInterface, validateProxyCore, startProxyCore = previousInterface, previousValidate, previousStart
	})
	physicalProxyInterface = func(context.Context) (string, error) { return "en0", nil }
	validations := 0
	validateProxyCore = func(context.Context, string, managedproxy.Store) error {
		validations++
		if validations == 1 {
			return errors.New("uplink is still warming up")
		}
		return nil
	}
	startProxyCore = func(context.Context, string, managedproxy.Store) error { return nil }
	s.networkPaused = true
	s.proxyNetworkLost = true
	tick := func() {
		s.op.Lock()
		s.monitorProxyInterface()
		s.op.Unlock()
		s.jobs.workers.Wait()
	}
	tick()
	if validations != 1 || gate.Status().Mode != cage.GateClosed {
		t.Fatalf("validations=%d gate=%s", validations, gate.Status().Mode)
	}
	if !s.proxyNetworkLost {
		t.Fatal("failed rebind forgot the outage on an unchanged interface")
	}
	tick()
	if validations != 1 {
		t.Fatal("retried before the backoff elapsed")
	}
	s.proxyRebindAfter = time.Now().Add(-time.Second)
	tick()
	if validations != 2 || s.proxyNetworkLost || gate.Status().Mode != cage.GateClosed || !s.networkPaused {
		t.Fatalf("validations=%d lost=%v gate=%s paused=%v", validations, s.proxyNetworkLost, gate.Status().Mode, s.networkPaused)
	}
}

func TestProxyInterfaceMonitorLeavesStoppedEnvironmentAvailableForStart(t *testing.T) {
	s := testServer(t)
	previous := physicalProxyInterface
	t.Cleanup(func() { physicalProxyInterface = previous })
	physicalProxyInterface = func(context.Context) (string, error) { return "eth0", nil }
	s.proxyNetworkLost = true
	if s.vmRunning() {
		t.Fatal("fixture has a running environment")
	}
	s.op.Lock()
	s.monitorProxyInterface()
	job := s.jobs.snapshot()
	s.jobs.cancelRunning()
	s.op.Unlock()
	s.jobs.workers.Wait()
	if job.Running {
		t.Fatalf("%s occupied the job queue before the stopped environment could start", job.Kind)
	}
}

func TestStoppedEnvironmentRebindsRunningCoreAfterUplinkLoss(t *testing.T) {
	s, _ := interfaceMonitorServer(t)
	if err := os.Remove(filepath.Join(s.instanceDir(), "cage-running.json")); err != nil {
		t.Fatal(err)
	}
	previousInterface, previousRunning, previousStart := physicalProxyInterface, proxyInterfaceCoreRunning, startProxyCore
	t.Cleanup(func() {
		physicalProxyInterface, proxyInterfaceCoreRunning, startProxyCore = previousInterface, previousRunning, previousStart
	})
	physicalProxyInterface = func(context.Context) (string, error) { return "replacement-uplink", nil }
	proxyInterfaceCoreRunning = func(context.Context, string, managedproxy.Store) bool { return true }
	starts := 0
	startProxyCore = func(context.Context, string, managedproxy.Store) error {
		starts++
		return nil
	}
	s.networkPaused = true
	s.op.Lock()
	s.monitorProxyInterface()
	s.op.Unlock()
	s.jobs.workers.Wait()
	if starts != 1 || !s.networkPaused || s.vmRunning() {
		t.Fatalf("starts=%d paused=%v vm=%v", starts, s.networkPaused, s.vmRunning())
	}
}
