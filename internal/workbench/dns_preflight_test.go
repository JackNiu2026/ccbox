package workbench

import (
	"context"
	"slices"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

func TestCandidateDNSProbeCannotUpdateActiveExitEvidence(t *testing.T) {
	s := testServer(t)
	old, err := loadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	next := old
	next.DNS = cage.NewDNSOptions(nil, []string{"https://9.9.9.9/dns-query"}, false)
	priorStart, priorProbe := startPreflightProxy, probeEnvironmentLocation
	t.Cleanup(func() { startPreflightProxy, probeEnvironmentLocation = priorStart, priorProbe })
	const temporary = "socks5://127.0.0.1:32123"
	startPreflightProxy = func(_ context.Context, _ string, candidate managedproxy.Store) (*managedproxy.PreflightProxy, error) {
		if !slices.Equal(candidate.BootstrapDNS, next.BootstrapEndpoints()) {
			t.Fatal("temporary core did not receive the candidate resolvers")
		}
		return &managedproxy.PreflightProxy{Endpoint: temporary}, nil
	}
	probeEnvironmentLocation = func(_ context.Context, config cage.Config, _ Preferences) (Location, error) {
		if config.ProxyURL != temporary {
			t.Fatal("candidate test used the active core")
		}
		return Location{IP: "192.0.2.2"}, nil
	}
	location, active, err := s.probeCandidateLocation(t.Context(), old, savedProfile{Config: next})
	if err != nil || active || location.IP != "192.0.2.2" {
		t.Fatalf("candidate result was treated as live exit evidence: active=%v location=%+v err=%v", active, location, err)
	}
	if saved, err := loadConfig(s.configPath); err != nil || saved != old {
		t.Fatalf("running configuration changed during the candidate test: %v", err)
	}
}
