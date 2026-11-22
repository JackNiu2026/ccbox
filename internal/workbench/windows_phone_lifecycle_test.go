package workbench

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/mobilelink"
	"github.com/sylvinus/agent-vm/internal/relay"
	"github.com/sylvinus/agent-vm/internal/remotelink"
)

// Restart the production service with a legacy pairing record. The retired
// phone transport must stay off without deleting the existing record.
func TestWindowsPhoneProductionDoesNotRestoreRetiredPairing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX process interrupt; native Windows phone entry is tested separately")
	}
	const helper = "CAGE_TEST_WINDOWS_PHONE_SERVICE"
	if config := os.Getenv(helper); config != "" {
		if err := Serve(config); err != nil {
			t.Fatal(err)
		}
		// Keep the owning process alive after Serve returns. Cleanup must belong
		// to the service, not be an accidental consequence of process exit.
		if marker := os.Getenv("CAGE_TEST_WINDOWS_PHONE_RETURNED"); marker != "" {
			if err := os.WriteFile(marker, []byte("returned"), 0600); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(os.Getenv("CAGE_TEST_WINDOWS_PHONE_RELEASE")); err == nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("parent did not release the returned service")
		}
		return
	}
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binding := guestos.WindowsProfile{Version: 1, VMID: uuid.New(), GUIUserSID: "S-1-5-21-123-456-789-1001", GUIUser: "Guest", InstanceDirectory: filepath.Join(profile, "never-created-vm"), PairingFile: filepath.Join(profile, "unused-pair.dpapi"), RelayExecutable: executable}
	if err := guestos.BindWindowsProfile(profile, binding); err != nil {
		t.Fatal(err)
	}
	edge, err := relay.New(relay.Config{PublicRooms: true, AdminToken: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	relayHTTP := httptest.NewServer(edge)
	t.Cleanup(func() { edge.Close(); relayHTTP.Close() })
	hostSeed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(hostSeed); err != nil {
		t.Fatal(err)
	}
	phone, err := remotelink.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	lookup := make([]byte, 32)
	if _, err := rand.Read(lookup); err != nil {
		t.Fatal(err)
	}
	paired := phoneRecord{Seed: hostSeed, Pin: phone.Pin, Relay: relayHTTP.URL, Lookup: base64.RawURLEncoding.EncodeToString(lookup), Admin: strings.Repeat("x", 32)}
	phoneFile := filepath.Join(profile, "mobile-paired-device.json")
	if err := writePrivate(phoneFile, paired); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(phoneFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	for restart := 0; restart < 2; restart++ {
		returned := filepath.Join(profile, fmt.Sprintf("serve-returned-%d", restart))
		release := filepath.Join(profile, fmt.Sprintf("serve-release-%d", restart))
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestWindowsPhoneProductionDoesNotRestoreRetiredPairing$", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), helper+"="+filepath.Join(profile, "config.json"), "CAGE_TEST_WINDOWS_PHONE_RETURNED="+returned, "CAGE_TEST_WINDOWS_PHONE_RELEASE="+release)
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		finished := false
		t.Cleanup(func() {
			if !finished {
				_ = cmd.Process.Kill()
				<-done
			}
		})
		var control controlFile
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			data, err := os.ReadFile(filepath.Join(profile, "control.json"))
			if err == nil && json.Unmarshal(data, &control) == nil && control.PID == cmd.Process.Pid {
				break
			}
			select {
			case err := <-done:
				finished = true
				t.Fatalf("production Windows service exited early: %v", err)
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
		if control.PID != cmd.Process.Pid {
			t.Fatal("production Windows service did not publish its controller")
		}
		deadline = time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if _, err := mobilelink.LookupRoom(ctx, relayHTTP.URL, paired.Lookup); !errors.Is(err, mobilelink.ErrNoInvitation) {
				t.Fatalf("production Serve exposed a retired phone invitation or lookup failed: %v", err)
			}
			time.Sleep(25 * time.Millisecond)
		}
		// This Windows binding has no verifiable resident VM on this POSIX host.
		// Trigger Serve's real signal shutdown instead of faking an idle handoff.
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		deadline = time.Now().Add(4 * time.Second)
		for {
			if _, err := os.Stat(returned); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("production Serve did not return after interrupt")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := os.WriteFile(release, []byte("verified"), 0600); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			finished = true
			if err != nil {
				t.Fatalf("production service did not stop cleanly: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Windows phone service did not stop with production Serve")
		}
		current, err := os.ReadFile(phoneFile)
		if err != nil || !bytes.Equal(current, original) {
			t.Fatal("service restart changed or removed persistent pairing")
		}
	}
	if _, err := os.Stat(binding.InstanceDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("phone lifecycle created a guest VM directory")
	}
}
