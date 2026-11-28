package workbench

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

const proxyRebindDelay = 30 * time.Second

type proxyInterfaceOutage struct {
	configuration string
	reason        string
}

type proxyBindingCheck struct {
	done chan struct{}
	err  error
}

var physicalProxyInterface = managedproxy.PhysicalInterface
var proxyInterfaceCoreRunning = managedproxy.Running

// The lifecycle lock is held by monitorNetwork. Schedule a regular job so a
// user stop can cancel recovery and pause can still revoke the gate immediately.
func (s *Server) monitorProxyInterface() {
	if s.busy() {
		return
	}
	running := s.vmRunning()
	path := s.proxyBindingPath()
	s.mu.Lock()
	lost, retryAfter := s.proxyNetworkLost, s.proxyRebindAfter
	s.mu.Unlock()
	if time.Now().Before(retryAfter) {
		return
	}
	if !running {
		// The idle core only needs rebinding if its known interface disappeared.
		bound, err := managedproxy.BoundInterface(path)
		if err != nil || bound == "" || (!lost && proxyInterfaceAvailable(bound)) {
			return
		}
	}
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return
	}
	proxy, err := managedproxy.Applied(s.profile, cfg.ProxyURL, cfg.ProxyRevision)
	if err != nil || (!running && !s.coreUsesProxy(s.lifetimeContext(), proxy)) {
		return
	}
	name, err := physicalProxyInterface(s.lifetimeContext())
	if err != nil {
		_ = s.closeProxyGate(err.Error())
		s.mu.Lock()
		s.safety.monitorError = err.Error()
		s.proxyNetworkLost = true
		if !running {
			s.proxyRebindAfter = time.Now().Add(proxyRebindDelay)
			s.proxyInterfaceOutage = proxyInterfaceOutage{configuration: cfg.Fingerprint(), reason: err.Error()}
		}
		s.mu.Unlock()
		return
	}
	if managedproxy.BindingMatches(path, name) {
		if !lost {
			return
		}
		if !running {
			s.mu.Lock()
			outage := s.proxyInterfaceOutage
			recovered := outage.reason != "" && outage.configuration == cfg.Fingerprint()
			if recovered && s.safety.monitorError == outage.reason {
				s.safety.monitorError = ""
				s.proxyNetworkLost = false
				s.proxyRebindAfter = time.Time{}
				s.proxyInterfaceOutage = proxyInterfaceOutage{}
			}
			s.mu.Unlock()
			if recovered {
				return
			}
		}
	}
	// Revoke the previous report before the asynchronous recovery can start.
	if err := s.closeProxyGate("主机网卡或节点路由已变化，正在重新绑定代理并检查出口"); err != nil {
		return
	}
	s.mu.Lock()
	s.proxyNetworkLost = true
	s.proxyInterfaceOutage = proxyInterfaceOutage{}
	s.mu.Unlock()
	_, _ = s.submitJob("proxy-network", s.rebindProxyInterface)
}

func (s *Server) rebindProxyInterface() (returned error) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Keep the outage pending until the selected core and the full check recover.
		s.proxyNetworkLost = returned != nil
		s.proxyRebindAfter = time.Time{}
		if returned != nil {
			s.proxyRebindAfter = time.Now().Add(proxyRebindDelay)
		}
	}()
	s.setMessage("正在识别物理网卡并重新绑定代理…")
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	p, err := managedproxy.Applied(s.profile, cfg.ProxyURL, cfg.ProxyRevision)
	if err != nil {
		return err
	}
	if !s.vmRunning() {
		return s.restartIdleProxy(s.operationContext(), p)
	}
	return s.applyProxy(p, cfg, nil, false)
}

func (s *Server) proxyBindingPath() string {
	return filepath.Join(managedproxy.Dir(s.profile), "config.yaml")
}

func proxyInterfaceAvailable(name string) bool {
	iface, err := net.InterfaceByName(name)
	if err != nil || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil && ip.To4() != nil && ip.IsGlobalUnicast() {
			return true
		}
	}
	return false
}

func (s *Server) coreUsesProxy(ctx context.Context, proxy managedproxy.Store) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return proxyInterfaceCoreRunning(ctx, s.profile, proxy)
}

var errStaleProxyBinding = errors.New("代理核心绑定的出口网卡或节点路由已变化，请检测并启动")

func (s *Server) checkProxyInterfaceBinding(ctx context.Context) error {
	if _, err := managedproxy.BoundInterface(s.proxyBindingPath()); err != nil {
		return err
	}
	name, err := physicalProxyInterface(ctx)
	if err != nil {
		return err
	}
	if !managedproxy.BindingMatches(s.proxyBindingPath(), name) {
		return errStaleProxyBinding
	}
	return nil
}

func (s *Server) ensureProxyInterfaceBeforeUse(ctx context.Context, cfg cage.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := cfg.Fingerprint()
	s.proxyBindingMu.Lock()
	check := s.proxyBindingChecks[key]
	if check == nil {
		activity, done, err := s.beginActivity(context.Background())
		if err != nil {
			s.proxyBindingMu.Unlock()
			return err
		}
		if s.proxyBindingChecks == nil {
			s.proxyBindingChecks = make(map[string]*proxyBindingCheck)
		}
		check = &proxyBindingCheck{done: make(chan struct{})}
		s.proxyBindingChecks[key] = check
		go func() {
			defer done()
			ctx, cancel := context.WithTimeout(activity, proxyRebindDelay)
			defer cancel()
			check.err = s.verifyProxyInterfaceBeforeUse(ctx, cfg)
			s.proxyBindingMu.Lock()
			delete(s.proxyBindingChecks, key)
			close(check.done)
			s.proxyBindingMu.Unlock()
		}()
	}
	s.proxyBindingMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-check.done:
		return check.err
	}
}

// A cancelled observer must not interrupt a rebind that is replacing an idle core.
func (s *Server) verifyProxyInterfaceBeforeUse(ctx context.Context, cfg cage.Config) error {
	proxy, err := managedproxy.Applied(s.profile, cfg.ProxyURL, cfg.ProxyRevision)
	if err != nil || !s.coreUsesProxy(ctx, proxy) {
		return nil
	}
	err = s.checkProxyInterfaceBinding(ctx)
	if !errors.Is(err, errStaleProxyBinding) || s.vmRunning() || !s.op.TryLock() {
		return err
	}
	defer s.op.Unlock()
	if s.vmRunning() {
		return err
	}
	current, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	if current.ProxyURL != cfg.ProxyURL || current.ProxyRevision != cfg.ProxyRevision {
		return errStaleProxyBinding
	}
	proxy, err = managedproxy.Applied(s.profile, current.ProxyURL, current.ProxyRevision)
	if err != nil {
		return err
	}
	if err := s.checkProxyInterfaceBinding(ctx); !errors.Is(err, errStaleProxyBinding) {
		return err
	}
	return s.restartIdleProxy(ctx, proxy)
}

func (s *Server) restartIdleProxy(ctx context.Context, proxy managedproxy.Store) error {
	if err := startProxyCore(ctx, s.profile, proxy); err != nil {
		return err
	}
	s.mu.Lock()
	s.safety.monitorError = ""
	s.proxyNetworkLost = false
	s.proxyRebindAfter = time.Time{}
	s.proxyInterfaceOutage = proxyInterfaceOutage{}
	s.mu.Unlock()
	return nil
}
