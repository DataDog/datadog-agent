// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// FakeResponse is the canned reply for one command on the fake SSH server.
// Stdout is written to the channel; Stderr to the channel's stderr stream;
// ExitStatus is reported to the client (0 = success).
type FakeResponse struct {
	Stdout     string
	Stderr     string
	ExitStatus uint32
}

// Ok returns a successful (exit 0) response with the given stdout.
func Ok(stdout string) FakeResponse { return FakeResponse{Stdout: stdout} }

// Fail returns a failure response with stderr and the given non-zero exit
// status.
func Fail(stderr string, exit uint32) FakeResponse {
	return FakeResponse{Stderr: stderr, ExitStatus: exit}
}

func FakeData(data map[string]FakeResponse) ShellFunc {
	return func(shell *ShellContext) uint32 {
		command := strings.TrimSpace(shell.command)
		resp, ok := data[command]
		if !ok {
			resp = FakeResponse{
				Stderr:     fmt.Sprintf("unknown command: %s\n", command),
				ExitStatus: 127,
			}
		}
		if resp.Stdout != "" {
			_, _ = io.WriteString(shell.stdout, resp.Stdout)
		}
		if resp.Stderr != "" {
			_, _ = io.WriteString(shell.stderr, resp.Stderr)
		}
		return resp.ExitStatus
	}
}

// SessionFunc builds a ShellFunc scoped to one SSH session. Per-session device
// state must be allocated in the returned closure so it cannot leak.
type SessionFunc func() ShellFunc

// Static adapts a ShellFunc for fakes with no per-session state.
func Static(f ShellFunc) SessionFunc { return func() ShellFunc { return f } }

func FakeASA(runningConfig string, pagerLines int) SessionFunc {
	return func() ShellFunc {
		pagerEnabled := true
		return func(shell *ShellContext) uint32 {
			switch strings.TrimSpace(shell.command) {
			case "":
				return 0
			case "terminal pager 0":
				pagerEnabled = false
				return 0
			case "more system:running-config":
				out := runningConfig
				if pagerEnabled {
					out = paginate(out, pagerLines)
				}
				_, _ = io.WriteString(shell.stdout, out)
				return 0
			default:
				_, _ = fmt.Fprintf(shell.stderr, "ERROR: %% Invalid input detected at '^' marker.\n")
				return 1
			}
		}
	}
}

func paginate(s string, lines int) string {
	parts := strings.SplitAfter(s, "\n")
	if lines <= 0 || len(parts) <= lines {
		return s
	}
	return strings.Join(parts[:lines], "") + "<--- More --->"
}

type ShellContext struct {
	command        string
	stdin          *bufio.Reader
	stdout, stderr io.Writer
}

func NewShellContext(command string, ch ssh.Channel) *ShellContext {
	return &ShellContext{
		command: command,
		stdin:   bufio.NewReader(ch),
		stdout:  ch,
		stderr:  ch.Stderr(),
	}
}

type ShellFunc func(*ShellContext) (returnCode uint32)

// FakeSSHServer is an in-process SSH server backed by a map of canned
// command -> response replies. It is intended for tests that exercise the
// real SSHClient against a server without depending on system sshd or Docker.
//
// The harness deliberately uses ed25519 host keys (fast key generation) and
// tracks every accepted connection so that Stop() can tear them down
// deterministically — this matters when a test wants to assert that NewSession
// fails after the server goes away.
type FakeSSHServer struct {
	listener  net.Listener
	hostKey   ssh.Signer
	getOutput SessionFunc

	banner    string
	prompt    string
	eagerEcho bool

	expectedUser     string
	expectedPassword string

	mu       sync.Mutex
	received []string
	conns    []net.Conn
	stopped  bool
}

// FakeServerOption configures a fakeSSHServer at startup.
type FakeServerOption func(*FakeSSHServer)

// WithCredentials sets the username/password the server expects. Defaults to
// "test" / "hunter2".
func WithCredentials(user, password string) FakeServerOption {
	return func(s *FakeSSHServer) {
		s.expectedUser = user
		s.expectedPassword = password
	}
}

// WithBanner sets text written once when an interactive shell starts.
func WithBanner(banner string) FakeServerOption {
	return func(s *FakeSSHServer) { s.banner = banner }
}

// WithPrompt sets the prompt written after each command, e.g. "fw01# ".
func WithPrompt(prompt string) FakeServerOption {
	return func(s *FakeSSHServer) { s.prompt = prompt }
}

// WithEagerEcho echoes each line as soon as it is read rather than after the
// previous command finishes, as real devices do. A client that writes several
// lines up front then sees their echoes above the output they belong to.
func WithEagerEcho() FakeServerOption {
	return func(s *FakeSSHServer) { s.eagerEcho = true }
}

// StartFakeSSHServer launches an in-process SSH server on 127.0.0.1 with a
// random port. The server is shut down via t.Cleanup, which closes the
// listener and every accepted connection.
func StartFakeSSHServer(t *testing.T, outputs map[string]FakeResponse, opts ...FakeServerOption) *FakeSSHServer {
	return StartFakeSSHServerWithFunc(t, FakeData(outputs), opts...)
}

// StartFakeSSHServerWithFunc starts a server whose ShellFunc is shared by every
// session. Use StartFakeSSHServerWithSessionFunc for per-session state.
func StartFakeSSHServerWithFunc(t *testing.T, getOutput ShellFunc, opts ...FakeServerOption) *FakeSSHServer {
	return StartFakeSSHServerWithSessionFunc(t, Static(getOutput), opts...)
}

// StartFakeSSHServerWithSessionFunc starts a server that builds a fresh
// ShellFunc per session, so per-session device state is modeled correctly.
func StartFakeSSHServerWithSessionFunc(t *testing.T, getOutput SessionFunc, opts ...FakeServerOption) *FakeSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostKey, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	srv := &FakeSSHServer{
		hostKey:          hostKey,
		getOutput:        getOutput,
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

func (s *FakeSSHServer) acceptLoop(cfg *ssh.ServerConfig) {
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

func (s *FakeSSHServer) serveConn(conn net.Conn, cfg *ssh.ServerConfig) {
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

func (s *FakeSSHServer) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()

	// One ShellFunc per session, so per-session device state cannot leak.
	run := s.getOutput()

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
			exitStatus := run(NewShellContext(payload.Command, ch))

			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: exitStatus}))
			return

		case "pty-req":
			_ = req.Reply(true, nil)

		case "shell":
			_ = req.Reply(true, nil)
			s.runShell(ch, run)
			return

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// runShell reads commands a line at a time and runs each against the same
// ShellFunc, so state set by one is visible to the next. "exit" ends it.
func (s *FakeSSHServer) runShell(ch ssh.Channel, run ShellFunc) {
	if s.banner != "" {
		_, _ = io.WriteString(ch, s.banner)
	}
	if s.prompt != "" {
		_, _ = io.WriteString(ch, s.prompt)
	}

	done := make(chan struct{})
	defer close(done)
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(ch)
		for scanner.Scan() {
			command := strings.TrimRight(scanner.Text(), "\r")

			s.mu.Lock()
			s.received = append(s.received, command)
			s.mu.Unlock()

			if s.eagerEcho {
				_, _ = io.WriteString(ch, command+"\n")
			}
			select {
			case lines <- command:
			case <-done:
				return
			}
		}
	}()

	var exitStatus uint32
	for command := range lines {
		if !s.eagerEcho {
			_, _ = io.WriteString(ch, command+"\n")
		}
		if strings.TrimSpace(command) == "exit" {
			_, _ = io.WriteString(ch, "\nLogoff\n")
			break
		}
		exitStatus = run(NewShellContext(command, ch))
		if s.prompt != "" {
			_, _ = io.WriteString(ch, s.prompt)
		}
	}
	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: exitStatus}))
}

// Stop closes the listener and every connection accepted so far. Idempotent.
// Tests that want to drive a "server went away" scenario call this explicitly;
// otherwise it runs once via t.Cleanup.
func (s *FakeSSHServer) Stop() {
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
func (s *FakeSSHServer) Addr() string {
	return s.listener.Addr().String()
}

// Host returns just the host portion of the bound address ("127.0.0.1").
func (s *FakeSSHServer) Host() string {
	host, _, _ := net.SplitHostPort(s.Addr())
	return host
}

// Port returns the bound port as a string (matches DeviceInstance.Auth.Port).
func (s *FakeSSHServer) Port() string {
	_, port, _ := net.SplitHostPort(s.Addr())
	return port
}

// User returns the username this server expects.
func (s *FakeSSHServer) User() string {
	return s.expectedUser
}

// Password returns the password this server expects.
func (s *FakeSSHServer) Password() string {
	return s.expectedPassword
}

// Received returns a snapshot of the commands the server has been asked to
// execute, in arrival order.
func (s *FakeSSHServer) Received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.received))
	copy(out, s.received)
	return out
}

// WriteKnownHostsFile writes a known_hosts file containing this server's host
// key, formatted for the bound host:port. Returns the file path.
func (s *FakeSSHServer) WriteKnownHostsFile(path string) error {
	pub := s.hostKey.PublicKey()
	// OpenSSH non-default-port form: "[host]:port keytype base64(key)\n"
	entry := fmt.Sprintf("[%s]:%s %s %s\n",
		s.Host(), s.Port(),
		pub.Type(),
		base64.StdEncoding.EncodeToString(pub.Marshal()),
	)
	if err := os.WriteFile(path, []byte(entry), 0600); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}
	return nil
}

func (s *FakeSSHServer) MakeConfig(knownHostsPath string) (*ssh.ClientConfig, error) {
	hostKey := ssh.InsecureIgnoreHostKey()
	if knownHostsPath != "" {
		var err error
		hostKey, err = knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("error parsing known_hosts file from path: %w", err)
		}
	}
	return &ssh.ClientConfig{
		User:            s.User(),
		Auth:            []ssh.AuthMethod{ssh.Password(s.Password())},
		HostKeyCallback: hostKey,
		Timeout:         0,
	}, nil
}

func (s *FakeSSHServer) Dial(knownHostsPath string) (*ssh.Client, error) {
	sshConfig, err := s.MakeConfig(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", s.Addr(), err)
	}
	client, err := ssh.Dial("tcp", s.Addr(), sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", s.Addr(), err)
	}
	return client, nil
}

func MakeKnownHostsFile(t testing.TB, s *FakeSSHServer) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	if err := s.WriteKnownHostsFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func MustConnect(t *testing.T, srv *FakeSSHServer) *ssh.Client {
	t.Helper()
	client, err := srv.Dial(MakeKnownHostsFile(t, srv))
	if err != nil {
		t.Fatalf("Unable to connect to fake server: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
	})

	return client
}
