package workbench

import (
	"encoding/json"
	"testing"

	"github.com/sylvinus/agent-vm/internal/version"
)

func TestAboutUsesLocalBuild(t *testing.T) {
	s := testServer(t)
	// A selected remote host must never own this software's build information.
	s.remote = newRemoteGateway(s.profile)
	s.remote.setRemote(true)
	t.Cleanup(s.remote.close)
	if got := request(s, "GET", "/api/client/about", "", "", false).Code; got != 401 {
		t.Fatalf("unauthenticated: %d", got)
	}
	if got := request(s, "POST", "/api/client/about", "https://untrusted.example", "{}", true).Code; got != 403 {
		t.Fatalf("origin: %d", got)
	}
	w := request(s, "GET", "/api/client/about", "", "", true)
	var build version.Build
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &build) != nil || build.Version != version.Version {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}

}
