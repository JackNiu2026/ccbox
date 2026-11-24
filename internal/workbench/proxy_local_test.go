package workbench

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/proxyimport"
)

func TestProxyLocalSourcesRequireDesktopAndHideCredentials(t *testing.T) {
	s := testServer(t)
	previous := discoverProxySources
	t.Cleanup(func() { discoverProxySources = previous })
	calls := 0
	discoverProxySources = func(key []byte) ([]proxyimport.Config, error) {
		calls++
		if string(key) != s.token {
			t.Fatal("source discovery did not use the desktop controller identity")
		}
		return []proxyimport.Config{{ID: "source-revision", Client: "FlClash", Name: "Test", Nodes: 1,
			Data: []byte("sample-node-password"), URL: "https://example.test/sub?token=sample-subscription-token"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, s.origin+"/api/proxy/local-sources", nil)
	w := httptest.NewRecorder()
	s.proxyLocalSources(w, req)
	if w.Code != http.StatusForbidden || calls != 0 {
		t.Fatal("an ordinary browser request triggered local discovery")
	}
	req.AddCookie(&http.Cookie{Name: s.cookieName() + "-desktop", Value: trustedViewProof(s.token)})
	w = httptest.NewRecorder()
	s.proxyLocalSources(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "source-revision") || strings.Contains(w.Body.String(), "sample-") {
		t.Fatalf("unexpected discovery response: %d %s", w.Code, w.Body.String())
	}
	if _, err := s.resolveLocalProxySource("obsolete-revision"); err == nil {
		t.Fatal("an obsolete local source identifier was accepted")
	}
}
