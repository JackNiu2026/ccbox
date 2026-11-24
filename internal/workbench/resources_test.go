package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/host"
)

func TestDiskRestoreRequiresLimaToConfirmStopped(t *testing.T) {
	for _, state := range []string{"stopped", "driver PID without a hostagent", "unreadable driver PID", "missing configuration", "CLI holds VM lock"} {
		t.Run(state, func(t *testing.T) {
			s := testServer(t)
			t.Setenv("LIMA_HOME", "")
			t.Setenv("CAGE_LOGICAL_LIMA_HOME", "")
			dir := s.instanceDir()
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			disk, config := filepath.Join(dir, "diffdisk"), filepath.Join(dir, "lima.yaml")
			write := func(path, value string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(config, "vmType: qemu\nimages:\n- location: /unused-fixture-image\ndisk: 24GiB\n")
			write(disk, "original disk")
			if err := cage.BeginDiskResize(context.Background(), dir, "old-job"); err != nil {
				t.Fatal(err)
			}
			write(disk, "partially resized disk")
			switch state {
			case "driver PID without a hostagent":
				// The test process keeps the PID live but cannot be mistaken for QEMU.
				write(filepath.Join(dir, "qemu.pid"), strconv.Itoa(os.Getpid()))
			case "unreadable driver PID":
				write(filepath.Join(dir, "qemu.pid"), "invalid")
			case "missing configuration":
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
			case "CLI holds VM lock":
				locks := filepath.Join(s.profile, "state", "locks")
				if err := os.MkdirAll(locks, 0o700); err != nil {
					t.Fatal(err)
				}
				f, err := os.OpenFile(filepath.Join(locks, filepath.Base(dir)), os.O_CREATE|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				if err := lockController(f); err != nil {
					t.Fatal(err)
				}
				defer unlockController(f)
			}
			if s.vmRunning() {
				t.Fatal("fixture unexpectedly has a running record")
			}
			w := request(s, "POST", "/api/environment/resources", s.origin, `{"disk_action":"restore"}`, true)
			if w.Code != http.StatusOK {
				t.Fatal(w.Code, w.Body.String())
			}
			s.jobs.workers.Wait()
			job := s.jobs.snapshot()
			got, err := os.ReadFile(disk)
			if err != nil {
				t.Fatal(err)
			}
			pending := cage.DiskResizePending(dir)
			if state == "stopped" {
				if job.Error != "" || pending || string(got) != "original disk" {
					t.Fatalf("stopped VM did not restore its backup: %q, pending=%t, job error=%s", got, pending, job.Error)
				}
			} else if job.Error == "" || !pending || string(got) != "partially resized disk" {
				t.Fatalf("unverified VM state must preserve both disk and backup: %q, pending=%t, job error=%s", got, pending, job.Error)
			}
		})
	}
}

func TestResourceAllocationSaveAndStartup(t *testing.T) {
	oldMem, oldCPU := host.MemGiB, host.CPUs
	t.Cleanup(func() { host.MemGiB, host.CPUs = oldMem, oldCPU })
	host.MemGiB = func() int { return 24 }
	host.CPUs = func() int { return 10 }
	t.Setenv("AGENT_VM_HOST_SHARE", "2")
	s := testServer(t)
	check := func(memory, cpus int) {
		t.Helper()
		m, c, err := cage.ConfiguredResourceLimits(s.configPath)
		if err != nil || m != memory || c != cpus {
			t.Fatalf("startup limits = %d/%d, %v", m, c, err)
		}
	}
	check(8, 5)
	for _, body := range []string{
		`{"allocation":{"automatic":false,"memory_gib":0,"cpus":2}}`,
		`{"allocation":{"automatic":false,"memory_gib":13,"cpus":2}}`,
		`{"allocation":{"automatic":false,"memory_gib":4,"cpus":6}}`,
		`{"allocation":{"automatic":false,"memory_gib":4.5,"cpus":2}}`,
	} {
		if w := request(s, "POST", "/api/environment/resources", s.origin, body, true); w.Code == 200 {
			t.Fatalf("accepted invalid allocation: %s", body)
		}
		check(8, 5)
	}
	before, _ := os.ReadFile(s.configPath)
	if w := request(s, "POST", "/api/environment/resources", s.origin, `{"allocation":{"automatic":false,"memory_gib":4,"cpus":2}}`, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	check(4, 2)
	after, _ := os.ReadFile(s.configPath)
	if string(before) != string(after) {
		t.Fatal("resource save changed network configuration")
	}
	w := request(s, "GET", "/api/environment/resources", "", "", true)
	var status struct {
		Automatic bool `json:"automatic"`
		Memory    int  `json:"memory_gib"`
		CPUs      int  `json:"cpus"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || status.Automatic || status.Memory != 4 || status.CPUs != 2 {
		t.Fatal(w.Body.String(), err)
	}
	s.op.Lock()
	w = request(s, "POST", "/api/environment/resources", s.origin, `{"allocation":{"automatic":true}}`, true)
	s.op.Unlock()
	if w.Code != 409 {
		t.Fatal("save must not race an environment operation", w.Code)
	}
	check(4, 2)
	if w = request(s, "POST", "/api/environment/resources", s.origin, `{"allocation":{"automatic":true}}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	check(8, 5)
}
