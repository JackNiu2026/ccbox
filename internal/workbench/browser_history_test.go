package workbench

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserHistoryRequiresTrustedLocalView(t *testing.T) {
	s := testServer(t)
	s.ready = true
	s.credentials.Token = "guest-test-token"
	id := strings.Repeat("a", 32)
	guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history" || r.URL.Query().Get("id") != id ||
			r.Header.Get("Authorization") != "Bearer "+s.credentials.Token {
			t.Errorf("unexpected request to the guest: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"addresses":[]}`))
	}))
	t.Cleanup(guest.Close)
	s.browserControl = localBrowserClient(t, s.lifetimeContext(), guest)
	path := "/api/browser/history?id=" + id

	for _, test := range []struct {
		name, method, origin, path string
		trusted                    bool
		status                     int
	}{
		{"ordinary cookie", "GET", "", path, false, 404},
		{"untrusted clear", "DELETE", s.origin, path, false, 404},
		{"cross-origin clear", "DELETE", "https://example.com", path, true, 403},
		{"invalid tab", "GET", "", "/api/browser/history?id=../../", true, 400},
		{"trusted read", "GET", "", path, true, 200},
		{"trusted clear", "DELETE", s.origin, path, true, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, s.origin+test.path, nil)
			r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
			if test.trusted {
				r.AddCookie(&http.Cookie{Name: s.cookieName() + "-desktop", Value: trustedViewProof(s.token)})
			}
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			w := httptest.NewRecorder()
			s.handler().ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d, want %d: %s", w.Code, test.status, w.Body)
			}
			if w.Code == http.StatusOK && (w.Header().Get("Cache-Control") != "no-store" || w.Body.String() != `{"addresses":[]}`) {
				t.Fatalf("history response was not delivered privately: %s", w.Body)
			}
		})
	}
}

func TestBrowserHistoryRouteNeverCrossesSSH(t *testing.T) {
	s := testServer(t)
	s.remote = newRemoteGateway(s.profile)
	s.remote.setRemote(true)
	defer s.remote.close()
	path := "/api/browser/history?id=" + strings.Repeat("a", 32)
	if w := request(s, "GET", path, "", "", true); w.Code != http.StatusNotFound {
		t.Fatalf("history route exposed to remote forwarding: %d", w.Code)
	}
}
