package workbench

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

func TestBrowserUsesInstanceSSHWithHostPortOccupied(t *testing.T) {
	var leaked atomic.Bool
	unrelated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		leaked.Store(true)
		http.Error(w, "unrelated application", http.StatusForbidden)
	}))
	defer unrelated.Close()
	streamClosed := make(chan struct{})
	guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer isolated-test-token" {
			t.Error("guest request lost authentication")
		}
		if r.URL.Path == "/events" {
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(streamClosed)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer guest.Close()
	const displayGreeting = "RFB 003.008\n"
	displays := make(chan ssh.Channel, 2)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	var connections atomic.Int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					conn.Close()
					return
				}
				defer server.Close()
				connections.Add(1)
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					var target struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					if incoming.ChannelType() != "direct-tcpip" || ssh.Unmarshal(incoming.ExtraData(), &target) != nil || target.Host != "127.0.0.1" || (target.Port != 18500 && target.Port != 15900) {
						_ = incoming.Reject(ssh.Prohibited, "unexpected destination")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						return
					}
					go ssh.DiscardRequests(requests)
					if target.Port == 15900 {
						displays <- channel
						_, _ = io.WriteString(channel, displayGreeting)
						_, _ = io.Copy(io.Discard, channel)
						_ = channel.Close()
						continue
					}
					remote, err := net.Dial("tcp", guest.Listener.Addr().String())
					if err != nil {
						channel.Close()
						return
					}
					go func() { _, _ = io.Copy(remote, channel); remote.Close(); channel.Close() }()
					go func() { _, _ = io.Copy(channel, remote); remote.Close(); channel.Close() }()
				}
			}()
		}
	}()
	s := testServer(t)
	dir := s.instanceDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	sshConfig := fmt.Sprintf("Host *\n HostName 127.0.0.1\n Port %d\n User cage\n StrictHostKeyChecking no\n UserKnownHostsFile %s\n", listener.Addr().(*net.TCPAddr).Port, os.DevNull)
	if err := os.WriteFile(filepath.Join(dir, "ssh.config"), []byte(sshConfig), 0600); err != nil {
		t.Fatal(err)
	}
	client := s.browserClient()
	defer client.close()
	// Model an occupied host port at the socket boundary without binding or
	// contacting an existing host service. The production SSH dialer ignores
	// this host address; the old TCP dialer reaches the unrelated application.
	transport := client.http.Transport.(*http.Transport)
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dial(ctx, network, unrelated.Listener.Addr().String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := client.request(ctx, "isolated-test-token", http.MethodGet, "/tabs", nil)
	if err != nil || string(data) != `{"ok":true}` || connections.Load() == 0 || leaked.Load() {
		t.Fatalf("browser did not use instance SSH: data=%q err=%v SSH=%d host-port-contacted=%t", data, err, connections.Load(), leaked.Load())
	}
	t.Run("display reconnect uses the same guest", func(t *testing.T) {
		// Own the host port for this test; never contact a running VM or app.
		occupied, err := net.Listen("tcp", "127.0.0.1:15900")
		if err != nil {
			t.Skip("local display port already in use")
		}
		defer occupied.Close()
		var hostPortContacted atomic.Bool
		go func() {
			conn, err := occupied.Accept()
			if err == nil {
				hostPortContacted.Store(true)
				_ = conn.Close()
			}
		}()
		s.ready = true
		s.credentials.Token = "isolated-test-token"
		server := httptest.NewServer(http.HandlerFunc(s.vnc))
		s.origin = server.URL
		defer server.Close()
		socket := "ws" + strings.TrimPrefix(server.URL, "http")
		for i := 0; i < 2; i++ {
			ws, _, err := websocket.DefaultDialer.DialContext(ctx, socket, http.Header{"Origin": {server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
			kind, message, err := ws.ReadMessage()
			if err != nil || kind != websocket.BinaryMessage || string(message) != displayGreeting {
				_ = ws.Close()
				t.Fatalf("guest display greeting missing on connection %d: %v (SSH connections=%d, display channels=%d, host port contacted=%t)", i+1, err, connections.Load(), len(displays), hostPortContacted.Load())
			}
			display := <-displays
			if i == 0 {
				// Drop the active guest stream after its first frame. The
				// next browser request reconnects to the same guest.
				_ = display.Close()
				if _, _, err := ws.ReadMessage(); err == nil {
					_ = ws.Close()
					t.Fatal("browser did not observe the guest stream closing")
				} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					_ = ws.Close()
					t.Fatal("closed guest stream left the browser connection hanging")
				}
			}
			_ = ws.Close()
		}
		if hostPortContacted.Load() {
			t.Fatal("display bypassed its authenticated guest SSH transport")
		}
	})
	body, err := client.events(ctx, "isolated-test-token")
	if err != nil {
		t.Fatal(err)
	}
	client.close()
	body.Close()
	select {
	case <-streamClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("SSH browser subscription survived client closure")
	}
}
