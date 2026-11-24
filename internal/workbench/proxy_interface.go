package workbench

import (
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

const proxyRebindDelay = 30 * time.Second

var physicalProxyInterface = managedproxy.PhysicalInterface

// The lifecycle lock is held by monitorNetwork. Schedule a regular job so a
// user stop can cancel recovery and pause can still revoke the gate immediately.
func (s *Server) monitorProxyInterface() {
	// A stopped guest has no network grant to recover. Its next start will
	// detect the current interface and start the core if needed.
	if s.busy() || !s.vmRunning() {
		return
	}
	cfg, err := loadConfig(s.configPath)
	if err != nil || cfg.RequireManagedProxy(s.profile) != nil {
		return
	}
	name, err := physicalProxyInterface(s.lifetimeContext())
	if err != nil {
		_ = s.closeProxyGate(err.Error())
		s.mu.Lock()
		s.safety.monitorError = err.Error()
		s.proxyNetworkLost = true
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	lost, retryAfter := s.proxyNetworkLost, s.proxyRebindAfter
	s.mu.Unlock()
	path := filepath.Join(managedproxy.Dir(s.profile), "config.yaml")
	if time.Now().Before(retryAfter) || (!lost && managedproxy.BindingMatches(path, name)) {
		return
	}
	// Revoke the previous report before the asynchronous recovery can start.
	if err := s.closeProxyGate("主机网卡或节点路由已变化，正在重新绑定代理并检查出口"); err != nil {
		return
	}
	s.mu.Lock()
	s.proxyNetworkLost = true
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
	return s.applyProxy(p, cfg, nil, false)
}
