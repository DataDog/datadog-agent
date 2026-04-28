// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
)

// fakeResponse is the canned reply for one command on the fake SSH server.
// Stdout is written to the channel; Stderr to the channel's stderr stream;
// ExitStatus is reported to the client (0 = success).
type fakeResponse struct {
	Stdout     string
	Stderr     string
	ExitStatus uint32
}

// ok returns a successful (exit 0) response with the given stdout.
func ok(stdout string) fakeResponse { return fakeResponse{Stdout: stdout} }

// fail returns a failure response with stderr and the given non-zero exit
// status.
func fail(stderr string, exit uint32) fakeResponse {
	return fakeResponse{Stderr: stderr, ExitStatus: exit}
}

// fakeSSHServer is an in-process SSH server backed by a map of canned
// command -> response replies. It is intended for tests that exercise the
// real SSHClient against a server without depending on system sshd or Docker.
//
// The harness deliberately uses ed25519 host keys (fast key generation) and
// tracks every accepted connection so that Stop() can tear them down
// deterministically — this matters when a test wants to assert that NewSession
// fails after the server goes away.
type fakeSSHServer struct {
	listener net.Listener
	hostKey  ssh.Signer
	outputs  map[string]fakeResponse

	expectedUser     string
	expectedPassword string

	mu       sync.Mutex
	received []string
	conns    []net.Conn
	stopped  bool
}

// fakeServerOption configures a fakeSSHServer at startup.
type fakeServerOption func(*fakeSSHServer)

// withCredentials sets the username/password the server expects. Defaults to
// "test" / "hunter2".
func withCredentials(user, password string) fakeServerOption {
	return func(s *fakeSSHServer) {
		s.expectedUser = user
		s.expectedPassword = password
	}
}

// startFakeSSHServer launches an in-process SSH server on 127.0.0.1 with a
// random port. The server is shut down via t.Cleanup, which closes the
// listener and every accepted connection.
func startFakeSSHServer(t *testing.T, outputs map[string]fakeResponse, opts ...fakeServerOption) *fakeSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostKey, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	srv := &fakeSSHServer{
		hostKey:          hostKey,
		outputs:          outputs,
		expectedUser:     "test",
		expectedPassword: "hunter2",
	}
	for _, opt := range opts {
		opt(srv)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == srv.expectedUser && string(pass) == srv.expectedPassword {
				return nil, nil
			}
			return nil, errors.New("invalid credentials")
		},
	}
	cfg.AddHostKey(hostKey)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = lis

	go srv.acceptLoop(cfg)
	t.Cleanup(srv.Stop)

	return srv
}

func (s *fakeSSHServer) acceptLoop(cfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		s.mu.Lock()
		stopped := s.stopped
		if !stopped {
			s.conns = append(s.conns, conn)
		}
		s.mu.Unlock()
		if stopped {
			_ = conn.Close()
			return
		}
		go s.serveConn(conn, cfg)
	}
}

func (s *fakeSSHServer) serveConn(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()

	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()

	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go s.handleSession(ch, chReqs)
	}
}

func (s *fakeSSHServer) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "exec":
			// Per RFC 4254 §6.5: payload is "string", which is uint32 length
			// + bytes. ssh.Unmarshal handles this for us.
			var payload struct{ Command string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				return
			}

			s.mu.Lock()
			s.received = append(s.received, payload.Command)
			s.mu.Unlock()

			_ = req.Reply(true, nil)

			resp, ok := s.outputs[payload.Command]
			if !ok {
				resp = fakeResponse{
					Stderr:     fmt.Sprintf("unknown command: %s\n", payload.Command),
					ExitStatus: 127,
				}
			}
			if resp.Stdout != "" {
				_, _ = ch.Write([]byte(resp.Stdout))
			}
			if resp.Stderr != "" {
				_, _ = ch.Stderr().Write([]byte(resp.Stderr))
			}

			// Send exit-status and close — CombinedOutput on the client
			// blocks until exit-status arrives.
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: resp.ExitStatus}))
			return

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// Stop closes the listener and every connection accepted so far. Idempotent.
// Tests that want to drive a "server went away" scenario call this explicitly;
// otherwise it runs once via t.Cleanup.
func (s *fakeSSHServer) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()

	_ = s.listener.Close()
	for _, c := range conns {
		_ = c.Close()
	}
}

// Addr returns the host:port the server is bound to.
func (s *fakeSSHServer) Addr() string {
	return s.listener.Addr().String()
}

// Host returns just the host portion of the bound address ("127.0.0.1").
func (s *fakeSSHServer) Host() string {
	host, _, _ := net.SplitHostPort(s.Addr())
	return host
}

// Port returns the bound port as a string (matches DeviceInstance.Auth.Port).
func (s *fakeSSHServer) Port() string {
	_, port, _ := net.SplitHostPort(s.Addr())
	return port
}

// Received returns a snapshot of the commands the server has been asked to
// execute, in arrival order.
func (s *fakeSSHServer) Received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.received))
	copy(out, s.received)
	return out
}

// WriteKnownHostsFile writes a known_hosts file containing this server's host
// key, formatted for the bound host:port. Returns the file path.
func (s *fakeSSHServer) WriteKnownHostsFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")

	pub := s.hostKey.PublicKey()
	// OpenSSH non-default-port form: "[host]:port keytype base64(key)\n"
	entry := fmt.Sprintf("[%s]:%s %s %s\n",
		s.Host(), s.Port(),
		pub.Type(),
		base64.StdEncoding.EncodeToString(pub.Marshal()),
	)
	if err := os.WriteFile(path, []byte(entry), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	return path
}

// DeviceInstance builds a DeviceInstance pre-wired to talk to this server,
// using the default credentials (or whichever were configured via
// withCredentials). Callers can mutate the returned struct before use.
func (s *fakeSSHServer) DeviceInstance(t *testing.T) *ncmconfig.DeviceInstance {
	t.Helper()
	knownHosts := s.WriteKnownHostsFile(t)
	port, err := strconv.Atoi(s.Port())
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	return &ncmconfig.DeviceInstance{
		IPAddress: s.Host(),
		Auth: ncmconfig.AuthCredentials{
			Username: s.expectedUser,
			Password: s.expectedPassword,
			Port:     strconv.Itoa(port),
			Protocol: "tcp",
			SSH: &ncmconfig.SSHConfig{
				KnownHostsPath: knownHosts,
			},
		},
	}
}
