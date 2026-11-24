package limaembed

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lima-vm/lima/v2/pkg/logrusutil"

	"github.com/sylvinus/agent-vm/internal/diagnostics"
)

func TestHostagentLogRelaysRedactedQEMUStderr(t *testing.T) {
	var stderr bytes.Buffer
	stream := hostagentLog(diagnostics.New(t.TempDir()), &stderr)
	for _, part := range []string{
		`{"level":"debug","msg":"qemu[stderr]: WHPX initialization failed with hr=80370302 token=`,
		"synthetic-test-secret\"}\n",
		"{\"level\":\"info\",\"msg\":\"a regular hostagent entry\"}\n",
	} {
		if _, err := stream.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	var entry logrusutil.JSON
	if err := json.Unmarshal(stderr.Bytes(), &entry); err != nil {
		t.Fatalf("stderr must contain one valid JSON log record: %v", err)
	}
	if entry.Level != "debug" || !strings.Contains(entry.Msg, "qemu[stderr]: WHPX initialization failed with hr=80370302") ||
		strings.Contains(entry.Msg, "synthetic-test-secret") {
		t.Fatalf("startup output was lost or unredacted: %q", entry.Msg)
	}
}

// The copy of limactl's hostagent command differs from it only as its
// header says: after a Lima upgrade, copy it again.
func TestHostagentCopy(t *testing.T) {
	upstream, err := os.ReadFile("../../third_party/lima/cmd/limactl/hostagent.go")
	if err != nil {
		t.Fatal(err)
	}
	ours, err := os.ReadFile("hostagent.go")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(upstream), "package main", "package limaembed", 1)
	want = strings.Replace(want, "WrapArgsError(cobra.ExactArgs(1))", "cobra.ExactArgs(1)", 1)
	want = strings.Replace(want, "signalCh := make(chan os.Signal, 1)", "signalCh := hostagentSignals", 1)
	want = strings.Replace(want, "signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)", "signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)\n\tstopNotification, err := watchHostagentStop(socket)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer stopNotification()", 1)
	// Our header: the comment block after the license lines.
	got := string(ours)
	start := strings.Index(got, "// Copied from")
	end := strings.Index(got, "package limaembed")
	if start < 0 || end < start {
		t.Fatal("hostagent.go lost its header")
	}
	got = got[:start] + got[end:]
	if got != want {
		t.Error("hostagent.go differs from third_party/lima/cmd/limactl/hostagent.go beyond its header: copy it again")
	}
}
