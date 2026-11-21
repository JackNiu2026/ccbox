package appupdate

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDesktopUpdateUsesOfficialCCBoxDownloads(t *testing.T) {
	const feed = "https://ccbox.org/downloads/updates.json"
	const signedArchive = "https://ccbox.org/downloads/0.2.0/CCBox-0.2.0-windows-amd64-setup.exe"
	const upstreamArchive = "https://github.com/Lee-xeo/Claude-Cage/releases/download/v0.2.0/Cage-0.2.0-windows-amd64-setup.exe"
	if releasesURL != feed {
		t.Errorf("desktop release request goes to %q", releasesURL)
	}
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{feed, true},
		{signedArchive, true},
		{upstreamArchive, false},
		{"https://ccbox.org.evil.example/downloads/updates.json", false},
		{"https://ccbox.org/downloads/0.2.0/Cage-0.2.0-windows-amd64-setup.exe", false},
		{"https://ccbox.org/downloads/0.2.0/%2e%2e/CCBox-0.2.0-windows-amd64-setup.exe", false},
	} {
		u, err := url.Parse(tc.raw)
		if err != nil || allowedURL(u, false) != tc.want {
			t.Errorf("unexpected URL policy for %q: %v", tc.raw, err)
		}
	}
	client := updateHTTPClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != feed {
			t.Fatalf("followed an external redirect to %q", r.URL)
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{upstreamArchive}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	}))
	if _, err := client.Get(feed); err == nil {
		t.Fatal("accepted redirect to an upstream package")
	}
	if _, err := request(context.Background(), client, upstreamArchive); err == nil {
		t.Fatal("accepted upstream package directly")
	}
}
