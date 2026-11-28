package workbench

import (
	"context"
	"net/http"
	"slices"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

var startPreflightProxy = managedproxy.StartPreflight

// A candidate core has no authority to revise the running environment's exit
// evidence. Only a probe against the unchanged live core can report it.
func (s *Server) probeCandidateLocation(ctx context.Context, old cage.Config, next savedProfile) (Location, bool, error) {
	store, err := managedproxy.Applied(s.profile, old.ProxyURL, old.ProxyRevision)
	if err != nil {
		return Location{}, false, err
	}
	config := next.Config
	bootstrap := config.BootstrapEndpoints()
	if slices.Equal(old.BootstrapEndpoints(), bootstrap) && slices.Equal(store.NodeResolvers(), bootstrap) {
		if err := s.ensureProxyInterfaceBeforeUse(ctx, old); err != nil {
			return Location{}, false, err
		}
		location, err := probeEnvironmentLocation(ctx, config, next.Preferences)
		return location, true, err
	}
	store.GuestDNS = config.DNSEndpoints()
	store.BootstrapDNS = bootstrap
	preflight, err := startPreflightProxy(ctx, s.profile, store)
	if err != nil {
		return Location{}, false, err
	}
	defer preflight.Close()
	config.ProxyURL = preflight.Endpoint
	location, err := probeEnvironmentLocation(ctx, config, next.Preferences)
	return location, false, err
}

func (s *Server) checkDNS(w http.ResponseWriter, r *http.Request) {
	next, ok := s.proposed(w, r)
	if !ok {
		return
	}
	if err := next.Config.RequireManagedProxy(s.profile); err != nil {
		requestError(w, err)
		return
	}
	if err := s.ensureProxyInterfaceBeforeUse(r.Context(), next.Config); err != nil {
		requestError(w, err)
		return
	}
	result, err := cage.CheckDNS(r.Context(), next.Config)
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, result)
}
