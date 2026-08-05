// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package fdhandoff

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// datagramTransport is the network used for the DogStatsD socket being handed off.
	datagramTransport = "unixgram"
	// handoffTransport is the network used for the handoff socket itself. A stream
	// socket is required to carry ancillary data reliably.
	handoffTransport = "unix"
	// handoffMessage is sent alongside the file descriptor. At least one byte of
	// regular data must accompany the ancillary data for it to be delivered.
	handoffMessage = "dogstatsd-socket-fd"
	// dialTimeout bounds the time spent waiting for the holder to accept our connection.
	dialTimeout = 5 * time.Second
	// readTimeout bounds the time spent waiting for the holder to send the file descriptor.
	readTimeout = 5 * time.Second
	// waitInterval is the delay between two connection attempts.
	waitInterval = 200 * time.Millisecond
	// socketWriteOnlyMode is the mode applied to the DogStatsD socket, mirroring
	// setSocketWriteOnly in comp/dogstatsd/listeners.
	socketWriteOnlyMode = 0722
	// maxHandoffFDs is the number of file descriptors the ancillary buffer is
	// sized for. We only ever expect one, but a buffer that fits exactly one
	// means any peer sending more triggers MSG_CTRUNC, and the kernel installs
	// the descriptors that fit in a control message we can then no longer parse
	// nor close. Leave enough room to always be able to parse and drain.
	maxHandoffFDs = 8
	// acceptRetryDelay is how long Serve waits before accepting again after a
	// temporary accept failure.
	acceptRetryDelay = 10 * time.Millisecond
)

// waitTimeout bounds the total time spent waiting for the holder to be up. The
// holder and the Agent are started independently, so the holder may not be
// listening yet when the Agent starts. Giving up immediately would leave the
// Agent running with no DogStatsD UDS listener at all until it is restarted, so
// we retry for a while instead. It is a variable so that tests can shorten it.
var waitTimeout = 10 * time.Second

// DefaultWaitTimeout is how long the Agent waits for a holder by default.
func DefaultWaitTimeout() time.Duration { return waitTimeout }

// DatagramSocket is a bound DogStatsD datagram socket, held open by the
// socket-holder process for the lifetime of the host.
type DatagramSocket struct {
	file *os.File
	path string
}

// BindDatagramSocket binds socketPath as a DogStatsD "unixgram" socket.
//
// It removes a stale socket file if one is present, then applies the same setup
// the Agent applies when it binds the socket itself: credential passing for
// origin detection and write-only permissions.
//
// Binding fails if another process is still bound to socketPath, see
// checkDatagramSocketFree.
//
// The returned socket owns a file descriptor that is never unlinked nor closed
// by this package: only Close, which the holder is not expected to call, does.
func BindDatagramSocket(socketPath string) (*DatagramSocket, error) {
	if err := checkDatagramSocketFree(socketPath); err != nil {
		return nil, err
	}
	if err := removeStaleSocket(socketPath); err != nil {
		return nil, err
	}

	conf := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			return setPassCred(c)
		},
	}

	packetConn, err := conf.ListenPacket(context.Background(), datagramTransport, socketPath)
	if err != nil {
		return nil, fmt.Errorf("can't listen on %s: %w", socketPath, err)
	}

	conn, ok := packetConn.(*net.UnixConn)
	if !ok {
		packetConn.Close()
		return nil, fmt.Errorf("unexpected return type from ListenPacket, expected UnixConn: %#v", packetConn)
	}
	// Package net only unlinks unix *listeners* (stream sockets) on close, never
	// datagram connections, so closing conn below leaves the socket file in
	// place. The duplicated file descriptor keeps the socket itself alive.
	defer conn.Close()

	file, err := conn.File()
	if err != nil {
		return nil, fmt.Errorf("can't duplicate the socket file descriptor: %w", err)
	}

	if err := os.Chmod(socketPath, socketWriteOnlyMode); err != nil {
		file.Close()
		return nil, fmt.Errorf("can't set the socket at write only: %w", err)
	}

	return &DatagramSocket{file: file, path: socketPath}, nil
}

// ParseMode parses a socket permission mode written in octal, with or without a
// leading zero. It is stricter than strconv with a zero base on purpose: "770"
// would otherwise be read as decimal, giving mode 0402, which lets every local
// user connect to the handoff socket and read all DogStatsD traffic.
func ParseMode(mode string) (os.FileMode, error) {
	parsed, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid octal permission mode: %w", mode, err)
	}
	if parsed&^uint64(os.ModePerm) != 0 {
		return 0, fmt.Errorf("%q is not a valid permission mode, expected at most three octal digits", mode)
	}
	return os.FileMode(parsed), nil
}

// Path returns the path the socket is bound to.
func (s *DatagramSocket) Path() string {
	return s.path
}

// SetReadBuffer sets the size of the operating system's receive buffer for the
// socket. The Agent applies its own dogstatsd_so_rcvbuf on the descriptor it
// adopts, this is only useful to size the buffer while the Agent is down.
func (s *DatagramSocket) SetReadBuffer(bytes int) error {
	var serr error
	rawConn, err := s.file.SyscallConn()
	if err != nil {
		return err
	}
	if err := rawConn.Control(func(fd uintptr) {
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, bytes)
	}); err != nil {
		return err
	}
	return serr
}

// Close closes the socket file descriptor. The socket file is left in place so
// that it can be rebound later.
func (s *DatagramSocket) Close() error {
	return s.file.Close()
}

// Server hands out the file descriptor of a DogStatsD datagram socket over a
// unix stream socket.
type Server struct {
	// ErrorHandler, when set, is called with the errors that affect a single
	// handoff and do not stop the server.
	ErrorHandler func(error)

	socket *DatagramSocket
	ln     *net.UnixListener
	path   string
}

// NewServer returns a Server listening on handoffPath, ready to hand out the
// file descriptor of the given socket. The handoff socket is created with the
// given mode: it must allow the user the Agent runs as to connect.
func NewServer(handoffPath string, mode os.FileMode, socket *DatagramSocket) (*Server, error) {
	if err := removeStaleSocket(handoffPath); err != nil {
		return nil, err
	}

	// Bind on a temporary path next to handoffPath and only move the socket into
	// place once its permissions are set. net.ListenUnix creates the socket with
	// 0777 masked by the umask, so binding handoffPath directly and chmod-ing it
	// afterwards leaves a window during which any local user may connect and be
	// handed a file descriptor that reads every DogStatsD datagram.
	tmpPath := fmt.Sprintf("%s.%d.tmp", handoffPath, os.Getpid())
	if err := removeStaleSocket(tmpPath); err != nil {
		return nil, err
	}

	addr, err := net.ResolveUnixAddr(handoffTransport, tmpPath)
	if err != nil {
		return nil, fmt.Errorf("can't ResolveUnixAddr: %w", err)
	}

	ln, err := net.ListenUnix(handoffTransport, addr)
	if err != nil {
		return nil, fmt.Errorf("can't listen on %s: %w", handoffPath, err)
	}
	// The socket is about to be renamed, so package net must not try to unlink
	// the path it was bound to: Close does it instead.
	ln.SetUnlinkOnClose(false)

	if err := os.Chmod(tmpPath, mode); err != nil {
		ln.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("can't set the handoff socket permissions: %w", err)
	}

	// Renaming a bound unix socket keeps it listening: only the name clients
	// connect through changes, and it appears with its final permissions.
	if err := os.Rename(tmpPath, handoffPath); err != nil {
		ln.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("can't move the handoff socket to %s: %w", handoffPath, err)
	}

	return &Server{socket: socket, ln: ln, path: handoffPath}, nil
}

// Addr returns the address the handoff socket is listening on.
func (s *Server) Addr() net.Addr {
	return &net.UnixAddr{Name: s.path, Net: handoffTransport}
}

// Serve accepts connections and sends the socket file descriptor to each of
// them. It only returns when the handoff socket can no longer be accepted on,
// in particular after Close has been called.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.AcceptUnix()
		if err != nil {
			// Running out of file descriptors, or a client going away between
			// the connection and the accept, must not take the holder down: it
			// owns the DogStatsD socket and nothing would hand it out again.
			if isTemporaryAcceptError(err) {
				s.reportError(fmt.Errorf("can't accept on the handoff socket, retrying: %w", err))
				time.Sleep(acceptRetryDelay)
				continue
			}
			return err
		}

		if err := s.send(conn); err != nil {
			s.reportError(err)
		}

		if err := conn.Close(); err != nil {
			s.reportError(fmt.Errorf("can't close the handoff connection: %w", err))
		}
	}
}

// Close stops the handoff socket. The socket being handed out is left untouched.
func (s *Server) Close() error {
	err := s.ln.Close()
	// The listener was renamed after being bound, so package net does not unlink
	// it for us.
	if rerr := os.Remove(s.path); rerr != nil && !os.IsNotExist(rerr) && err == nil {
		err = fmt.Errorf("can't remove the handoff socket %s: %w", s.path, rerr)
	}
	return err
}

func (s *Server) reportError(err error) {
	if s.ErrorHandler != nil {
		s.ErrorHandler(err)
	}
}

// isTemporaryAcceptError reports whether an accept failure is worth retrying
// rather than a reason to stop serving.
func isTemporaryAcceptError(err error) bool {
	if errors.Is(err, net.ErrClosed) {
		return false
	}
	return errors.Is(err, unix.EMFILE) || errors.Is(err, unix.ENFILE) ||
		errors.Is(err, unix.ENOBUFS) || errors.Is(err, unix.ENOMEM) ||
		errors.Is(err, unix.ECONNABORTED) || errors.Is(err, unix.EINTR)
}

func (s *Server) send(conn *net.UnixConn) error {
	var serr error
	rawConn, err := s.socket.file.SyscallConn()
	if err != nil {
		return fmt.Errorf("can't access the socket file descriptor: %w", err)
	}

	// The file descriptor is only valid for the duration of the Control call, so
	// the sendmsg has to happen inside of it.
	if err := rawConn.Control(func(fd uintptr) {
		rights := unix.UnixRights(int(fd))
		_, _, serr = conn.WriteMsgUnix([]byte(handoffMessage), rights, nil)
	}); err != nil {
		return fmt.Errorf("can't access the socket file descriptor: %w", err)
	}
	if serr != nil {
		return fmt.Errorf("can't send the socket file descriptor: %w", serr)
	}

	return nil
}

// receiveFile connects to the handoff socket and reads a single file descriptor
// out of the ancillary data sent by the holder.
func receiveFileWithin(handoffPath string, waitFor time.Duration) (*os.File, error) {
	conn, err := dialHandoff(handoffPath, waitFor)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return nil, fmt.Errorf("can't set a read deadline on the handoff socket: %w", err)
	}

	buf := make([]byte, len(handoffMessage))
	oob := make([]byte, unix.CmsgSpace(4*maxHandoffFDs))
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, fmt.Errorf("can't read from the handoff socket: %w", err)
	}
	if n == 0 && oobn == 0 {
		return nil, fmt.Errorf("the handoff socket %s closed the connection without sending a file descriptor", handoffPath)
	}

	// Collect every descriptor the peer sent before doing anything else: the
	// kernel has already installed them all in this process, so returning early
	// on an unexpected message would leak them.
	fds, err := parseUnixRights(oob[:oobn])
	defer func() {
		// Only the first descriptor is handed to the caller, on success.
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("can't parse the handoff ancillary data: %w", err)
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return nil, fmt.Errorf("the handoff socket %s sent more ancillary data than expected, refusing the truncated handoff", handoffPath)
	}
	if len(fds) == 0 {
		return nil, fmt.Errorf("no file descriptor received from the handoff socket %s", handoffPath)
	}

	file := os.NewFile(uintptr(fds[0]), handoffPath)
	fds = fds[1:]
	return file, nil
}

// parseUnixRights returns every file descriptor found in the SCM_RIGHTS
// messages of oob. Descriptors parsed before an error is hit are returned too,
// so that the caller can close them.
func parseUnixRights(oob []byte) ([]int, error) {
	messages, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}

	var fds []int
	for i := range messages {
		if messages[i].Header.Level != unix.SOL_SOCKET || messages[i].Header.Type != unix.SCM_RIGHTS {
			continue
		}
		messageFds, err := unix.ParseUnixRights(&messages[i])
		if err != nil {
			return fds, err
		}
		fds = append(fds, messageFds...)
	}
	return fds, nil
}

// dialHandoff connects to the handoff socket, retrying until waitTimeout has
// elapsed while the holder is not listening yet. The holder and the Agent are
// started independently, and failing here leaves the Agent with no DogStatsD
// UDS listener at all until it is restarted.
func dialHandoff(handoffPath string, waitFor time.Duration) (*net.UnixConn, error) {
	deadline := time.Now().Add(waitFor)
	for {
		conn, err := net.DialTimeout(handoffTransport, handoffPath, dialTimeout)
		if err == nil {
			unixConn, ok := conn.(*net.UnixConn)
			if !ok {
				conn.Close()
				return nil, fmt.Errorf("unexpected return type from Dial, expected UnixConn: %#v", conn)
			}
			return unixConn, nil
		}

		if !holderNotUp(err) {
			return nil, fmt.Errorf("can't connect to the handoff socket %s: %w", handoffPath, err)
		}
		if !time.Now().Add(waitInterval).Before(deadline) {
			// The holder never showed up. Report it distinctly so the caller can
			// tell "nobody owns the socket" from "the handoff itself failed".
			return nil, fmt.Errorf("%w: %s: %w", ErrHolderUnavailable, handoffPath, err)
		}
		time.Sleep(waitInterval)
	}
}

// holderNotUp reports whether a dial error means the holder is not listening
// yet, as opposed to a permanent problem such as a permission denial.
func holderNotUp(err error) bool {
	return errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ECONNREFUSED)
}

// checkDatagramSocketFree returns an error if a process is still bound to the
// datagram socket at path.
//
// Rebinding the socket destroys its inode: clients keep writing to the old one
// and an Agent that adopted the previous descriptor silently stops receiving
// anything, which is the very failure the handoff exists to avoid. Connecting
// to a datagram socket nobody is bound to fails with ECONNREFUSED, so a
// successful connection means the socket is still in use.
func checkDatagramSocketFree(path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	conn, err := net.DialTimeout(datagramTransport, path, dialTimeout)
	if err != nil {
		return nil
	}
	conn.Close()
	return fmt.Errorf("cannot bind %s: another process is still bound to it, stop it first (an Agent binding the socket itself, or another socket holder)", path)
}

// removeStaleSocket removes the socket file at path if it exists, mirroring
// setupSocketBeforeListen in comp/dogstatsd/listeners.
func removeStaleSocket(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if fileInfo.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("cannot reuse %s socket path: path already exists and is not a UNIX socket", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove stale UNIX socket: %w", err)
	}
	return nil
}
