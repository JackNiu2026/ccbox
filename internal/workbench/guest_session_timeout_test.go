package workbench

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestGuestSessionStallsFallBackWithoutRepeatingSentCommands(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	// The connection consumes commands but never returns a response.
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	conn := &guestSessionConn{
		cancel:  func() {},
		stdin:   writer,
		waiting: make(map[uint64]chan guestSessionReply),
		ended:   make(chan struct{}),
	}
	session := &guestSession{conn: conn}
	t.Cleanup(session.close)
	unexpectedConnect := func(context.Context) *exec.Cmd {
		t.Fatal("reopened the stalled shared connection before retry delay")
		return nil
	}
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
		_, sent, err := session.run(ctx, t.Context(), unexpectedConnect, "echo run-once")
		cancel()
		if !sent || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timed-out command may have run: sent=%v, err=%v", sent, err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	_, sent, err := session.run(ctx, t.Context(), unexpectedConnect, "echo next")
	if sent || !errors.Is(err, errGuestSessionEnded) {
		t.Fatalf("later command did not fall back: sent=%v, err=%v", sent, err)
	}
}
