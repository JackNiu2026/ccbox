package workbench

import (
	"net/http"
	"strings"
	"testing"
)

func TestDesktopInstanceTargetsStaySeparate(t *testing.T) {
	for _, value := range []string{"", "0", "1", "02", "5", "../2", "2\n"} {
		if _, ok := numberedDesktopSlot(value); ok {
			t.Errorf("accepted invalid instance %q", value)
		}
	}
	for _, value := range []string{"2", "3", "4"} {
		slot, ok := numberedDesktopSlot(value)
		if !ok || slot.unit() != "ccbox-claude-desktop@"+value+".service" || slot.runtime() != "/run/ccbox-claude-desktop-"+value {
			t.Fatalf("incorrect resources for instance %q: %+v", value, slot)
		}
		script := numberedDesktopResizeScript(slot, 800, 600, "1.5")
		for _, bad := range []string{"/run/cage-desktop/", "DISPLAY=:78", "/run/cage-phone-desktop/"} {
			if strings.Contains(script, bad) {
				t.Errorf("instance %q resize touches another desktop: %s", value, bad)
			}
		}
	}
}

func TestDesktopInstanceCannotUseOrdinaryOrRemoteSession(t *testing.T) {
	s := testServer(t)
	path := "/api/claude-desktop/instances/2/vnc-credentials"
	if w := request(s, "GET", path, "", "", true); w.Code != http.StatusNotFound {
		t.Fatalf("credentials available without native desktop proof: %d", w.Code)
	}
	s.remote = newRemoteGateway(s.profile)
	s.remote.setRemote(true)
	defer s.remote.close()
	if w := request(s, "POST", "/api/claude-desktop/instances/2/start", s.origin, "{}", true); w.Code != http.StatusNotFound {
		t.Fatalf("instance action exposed to SSH forwarding: %d", w.Code)
	}
}
