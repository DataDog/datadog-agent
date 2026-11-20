// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The supervisor socket is how the processes around a configuration experiment reach the watcher
// that supervises it. Only that watcher listens on it. Two kinds of client connect:
//
//   - the second watcher (W2), launched as the experiment daemon starts, to take supervision over
//     from the first one (W1), which the start hook launched: it sends "armed <token>" once its exit
//     observers are armed, and W1 answers "ack", stops listening and exits, or "nack";
//   - the stop and promote hooks, once they have handed the Agent back, to have the supervisor hand
//     the installer daemon back: they send "end" and the supervisor answers "ok".
//
// The deadline file stays the source of truth. The socket only lets a supervisor act at once
// rather than on its next deadline tick, and lets W1 leave only once W2 is known to be supervising.
// It lives in the root-only state directory beside the deadline, and a connection from any other
// user than the one that owns that state (configExperimentStateOwner) is refused.

// configExperimentSupervisorSocketPath is the supervisor socket. A package-level var so tests can
// point it at a short temporary path: a Unix socket path is limited to 104 bytes on macOS.
var configExperimentSupervisorSocketPath = filepath.Join(paths.ConfigExperimentStatePath, "experiment-supervisor.sock")

// supervisorIOTimeout bounds every read and write on a supervisor connection, and
// supervisorHandshakeTimeout how long W2 waits for W1's answer: W1 answers from its main loop, which
// can be waiting for the installer lock. Vars so tests can shorten them.
var (
	supervisorIOTimeout        = 5 * time.Second
	supervisorHandshakeTimeout = 2 * time.Minute
)

const (
	supervisorMsgArmed = "armed"
	supervisorMsgEnd   = "end"
	supervisorReplyAck = "ack"
	supervisorReplyNak = "nack"
	supervisorReplyOK  = "ok"
)

// errNoSupervisor is returned by the clients when nothing listens on the supervisor socket.
var errNoSupervisor = errors.New("no configuration experiment supervisor is listening")

// supervisorRequest is one message a client sent the listening supervisor. An "armed" request is
// answered by the supervisor's main loop, with reply, which then closes it. An "end" request is
// answered as soon as it is read, and carries no connection: the hook that sent it must not wait on a
// main loop that can be waiting for the hook's own installer command to exit.
type supervisorRequest struct {
	kind  string
	token string
	conn  net.Conn
}

// reply writes the supervisor's answer and closes the connection. It reports whether the answer was
// written.
func (r supervisorRequest) reply(answer string) error {
	if r.conn == nil {
		return nil
	}
	defer r.conn.Close()
	_ = r.conn.SetWriteDeadline(time.Now().Add(supervisorIOTimeout))
	if _, err := r.conn.Write([]byte(answer + "\n")); err != nil {
		log.Warnf("supervisor: could not answer %q: %v", r.kind, err)
		return err
	}
	return nil
}

// supervisorListener is the supervisor's end of the socket. Requests are delivered on requests
// until close.
type supervisorListener struct {
	listener net.Listener
	requests chan supervisorRequest
	done     chan struct{}
}

// listenSupervisor starts listening on the supervisor socket. A socket file nothing answers on is a
// leftover of a supervisor that died, and is replaced. One something answers on belongs to a live
// supervisor, and is an error.
func listenSupervisor() (*supervisorListener, error) {
	path := configExperimentSupervisorSocketPath
	listener, err := net.Listen("unix", path)
	if err != nil && errors.Is(err, syscall.EADDRINUSE) {
		if conn, dialErr := net.DialTimeout("unix", path, supervisorIOTimeout); dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("supervisor: another supervisor listens on %s", path)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, fmt.Errorf("supervisor: could not remove the stale socket %s: %w", path, removeErr)
		}
		listener, err = net.Listen("unix", path)
	}
	if err != nil {
		return nil, fmt.Errorf("supervisor: could not listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("supervisor: could not restrict %s: %w", path, err)
	}
	s := &supervisorListener{listener: listener, requests: make(chan supervisorRequest), done: make(chan struct{})}
	go s.accept()
	return s, nil
}

// accept reads one request from each connection, after checking who sent it, and hands it to the
// main loop. It returns once the listener is closed.
func (s *supervisorListener) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

func (s *supervisorListener) serve(conn net.Conn) {
	if err := checkSupervisorPeer(conn); err != nil {
		log.Warnf("supervisor: refusing a connection: %v", err)
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(supervisorIOTimeout))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		_ = conn.Close()
		return
	}
	request := supervisorRequest{kind: fields[0], conn: conn}
	switch {
	case request.kind == supervisorMsgArmed && len(fields) == 2:
		request.token = fields[1]
	case request.kind == supervisorMsgEnd && len(fields) == 1:
		_ = request.reply(supervisorReplyOK)
		request.conn = nil
	default:
		_ = conn.Close()
		return
	}
	if request.conn != nil {
		_ = conn.SetReadDeadline(time.Time{})
	}
	select {
	case s.requests <- request:
	case <-s.done:
		_ = request.reply(supervisorReplyNak)
	}
}

// close stops listening and removes the socket file, so the next supervisor can listen.
func (s *supervisorListener) close() {
	if s == nil {
		return
	}
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	_ = s.listener.Close()
}

// checkSupervisorPeer refuses a connection from any user other than the owner of the experiment
// state (root).
func checkSupervisorPeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("not a Unix socket connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return fmt.Errorf("could not read the peer's credentials: %w", credErr)
	}
	if int(cred.Uid) != configExperimentStateOwner {
		return fmt.Errorf("peer uid %d is not %d", cred.Uid, configExperimentStateOwner)
	}
	return nil
}

// askSupervisor sends message to the listening supervisor and returns its answer, waiting up to
// timeout for it. It returns errNoSupervisor when nothing listens.
func askSupervisor(message string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("unix", configExperimentSupervisorSocketPath, supervisorIOTimeout)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) {
			return "", errNoSupervisor
		}
		return "", fmt.Errorf("could not reach the supervisor: %w", err)
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(supervisorIOTimeout))
	if _, err := conn.Write([]byte(message + "\n")); err != nil {
		return "", fmt.Errorf("could not send %q to the supervisor: %w", message, err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	answer, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("no answer from the supervisor to %q: %w", message, err)
	}
	return strings.TrimSpace(answer), nil
}

// supervisorListening reports whether a supervisor listens on the socket, without sending it
// anything.
func supervisorListening() bool {
	conn, err := net.DialTimeout("unix", configExperimentSupervisorSocketPath, supervisorIOTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// notifySupervisorEnd tells the supervisor that a stop or promote has handed the Agent back, so it
// hands the installer daemon back as soon as the hook's command has exited. A var so tests can stub
// it.
var notifySupervisorEnd = func() error {
	answer, err := askSupervisor(supervisorMsgEnd, supervisorIOTimeout)
	if err != nil {
		return err
	}
	if answer != supervisorReplyOK {
		return fmt.Errorf("the supervisor answered %q to end", answer)
	}
	return nil
}
