package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/appupdate/catalog"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAdminControlsDesktopUpdateChannel(t *testing.T) {
	client, _, admin := newTestAdmin(t)
	current := "0.19.2"
	hash := strings.Repeat("a", 64)
	corrupt := false
	admin.updates.client.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		base := catalog.DownloadBase(current)
		archive := "CCBox-" + current + "-windows-amd64-setup.exe"
		intel := "CCBox-" + current + "-macos-amd64.dmg"
		sums := hash + "  " + archive + "\n" + hash + "  " + intel + "\n"
		if corrupt {
			sums = "invalid\n"
		}
		var body string
		switch r.URL.String() {
		case catalog.SiteOrigin + "/downloads/catalog.json":
			data, _ := json.Marshal([]catalog.Release{{Tag: "v" + current, Assets: []catalog.Asset{
				{Name: archive, URL: base + archive, Size: 12345, State: "uploaded", Manual: true},
				{Name: intel, URL: base + intel, Size: 12345, State: "uploaded", Manual: true},
				{Name: "SHA256SUMS", URL: base + "SHA256SUMS", Size: int64(len(sums)), State: "uploaded"},
			}}})
			body = string(data)
		case base + "SHA256SUMS":
			body = sums
		case base + archive, base + intel:
			if r.Method != http.MethodHead {
				t.Fatalf("unexpected installer method: %s", r.Method)
			}
			return &http.Response{StatusCode: 200, ContentLength: 12345, Body: http.NoBody}, nil
		default:
			t.Fatalf("unexpected release URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	admin.updates.register(client.mux)
	feed := func(mux *http.ServeMux) []catalog.Release {
		t.Helper()
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", catalog.ReleasesURL, nil))
		if response.Code != 200 || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("feed response: %d %v", response.Code, response.Header())
		}
		return decode[[]catalog.Release](t, response)
	}
	apply := func(version string, enabled bool, revision int, origin string) int {
		t.Helper()
		return client.do("POST", "api/updates", origin, map[string]any{"version": version, "enabled": enabled, "revision": revision}).Code
	}
	if len(feed(client.mux)) != 0 || apply(current, true, 0, testOrigin) != 401 {
		t.Fatal("a new channel must be paused and publication must require an admin session")
	}
	response := client.do("POST", "api/login", testOrigin, goodLogin)
	client.cookie = response.Result().Cookies()[0]
	if apply(current, true, 0, "https://other.example") != 403 || apply("0.19.1", true, 0, testOrigin) != 409 {
		t.Fatal("untrusted origin or stale catalog accepted")
	}
	if apply(current, true, 0, testOrigin) != 200 {
		t.Fatal("verified version did not publish")
	}
	published := feed(client.mux)
	if len(published) != 1 || published[0].Tag != "v0.19.2" || !published[0].Assets[0].Manual || published[0].Assets[0].Digest != "sha256:"+hash {
		t.Fatalf("invalid publication: %+v", published)
	}
	if selected, err := catalog.Select(published, "0.19.1", "darwin", "amd64"); err != nil || selected == nil || selected.Archive.Digest != "sha256:"+hash {
		t.Fatalf("verified Intel macOS installer missing: %+v %v", selected, err)
	}
	current = "0.19.3"
	corrupt = true
	if apply(current, true, 1, testOrigin) != 502 || feed(client.mux)[0].Tag != "v0.19.2" {
		t.Fatal("a new upload or invalid checksum changed the live push")
	}
	corrupt = false
	restarted := http.NewServeMux()
	newUpdateChannel(admin.updates.dir).register(restarted)
	if feed(restarted)[0].Tag != "v0.19.2" || apply("0.19.2", false, 0, testOrigin) != 409 {
		t.Fatal("publication did not survive restart or a stale edit disabled it")
	}
	if apply("0.19.2", false, 1, testOrigin) != 200 || len(feed(restarted)) != 0 {
		t.Fatal("pause did not remove the live push")
	}
}
