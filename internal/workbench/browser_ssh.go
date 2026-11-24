package workbench

import (
	"context"
	"io"
	"net"
	"sync"
)

// Browser control and display use this instance's authenticated SSH
// connection, independent of host port forwarding. address is a fixed guest
// loopback endpoint chosen by the caller.
func (s *Server) dialGuestLoopback(ctx context.Context, lifetime context.Context, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	processContext, cancel := context.WithCancel(lifetime)
	cmd := s.sshCommand(processContext, "-T", "-W", address, s.sshTarget())
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	local, remote := net.Pipe()
	conn := &browserSSHConnection{Conn: local, cancel: cancel}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := cmd.Start(); err != nil {
		conn.Close()
		remote.Close()
		input.Close()
		output.Close()
		return nil, err
	}
	written := make(chan struct{})
	go func() {
		defer close(written)
		_, _ = io.Copy(input, remote)
		_ = input.Close()
	}()
	go func() {
		_, _ = io.Copy(remote, output)
		_ = remote.Close()
		cancel()
		<-written
		_ = cmd.Wait()
	}()
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

type browserSSHConnection struct {
	net.Conn
	cancel context.CancelFunc
	once   sync.Once
}

func (c *browserSSHConnection) Close() error {
	c.once.Do(c.cancel)
	return c.Conn.Close()
}
