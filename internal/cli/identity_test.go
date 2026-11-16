package cli

import (
	"bytes"
	"strings"
	"testing"
)

// A program that embeds the shared commands names itself and says how it
// updates: CCBox must not send its users to agent-vm's installer.
func TestEmbeddingProgramIdentity(t *testing.T) {
	savedCommand := Command
	t.Cleanup(func() { Command = savedCommand })
	Command = "claude-cage"

	var stdout, stderr bytes.Buffer
	io := IO{Stdout: &stdout, Stderr: &stderr}
	if code := CCBoxVersion([]string{"--min", "99.0.0"}, io); code != 1 {
		t.Fatalf("an unmet minimum returned %d", code)
	}
	got := stderr.String()
	for _, want := range []string{"Error: CCBox ", "Update it:  get the latest installer from your CCBox distributor"} {
		if !strings.Contains(got, want) {
			t.Errorf("the version error lacks %q: %q", want, got)
		}
	}
	if strings.Contains(got, "agent-vm") {
		t.Errorf("the version error names agent-vm: %q", got)
	}

	stderr.Reset()
	if code := licensesCmd([]string{"x"}, io); code != 2 || !strings.Contains(stderr.String(), "Usage: claude-cage licenses") {
		t.Errorf("licenses usage: %d %q", code, stderr.String())
	}
}
