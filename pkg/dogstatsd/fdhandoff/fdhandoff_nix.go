// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package fdhandoff

import (
	"context"
	"fmt"
	"net"
	"os"
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
	// socketWriteOnlyMode is the mode applied to the DogStatsD socket, mirroring
	// setSocketWriteOnly in comp/dogstatsd/listeners.
	socketWriteOnlyMode = 0722
)

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
// The returned socket owns a file descriptor that is never unlinked nor closed
// by this package: only Close, which the holder is not expected to call, does.
func BindDatagramSocket(socketPath string) (*DatagramSocket, error) {
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
}

// NewServer returns a Server listening on handoffPath, ready to hand out the
// file descriptor of the given socket. The handoff socket is created with the
// given mode: it must allow the user the Agent runs as to connect.
func NewServer(handoffPath string, mode os.FileMode, socket *DatagramSocket) (*Server, error) {
	if err := removeStaleSocket(handoffPath); err != nil {
		return nil, err
	}

	addr, err := net.ResolveUnixAddr(handoffTransport, handoffPath)
	if err != nil {
		return nil, fmt.Errorf("can't ResolveUnixAddr: %w", err)
	}

	ln, err := net.ListenUnix(handoffTransport, addr)
	if err != nil {
		return nil, fmt.Errorf("can't listen on %s: %w", handoffPath, err)
	}

	if err := os.Chmod(handoffPath, mode); err != nil {
		ln.Close()
		return nil, fmt.Errorf("can't set the handoff socket permissions: %w", err)
	}

	return &Server{socket: socket, ln: ln}, nil
}

// Addr returns the address the handoff socket is listening on.
func (s *Server) Addr() net.Addr {
	return s.ln.Addr()
}

// Serve accepts connections and sends the socket file descriptor to each of
// them. It only returns when the handoff socket can no longer be accepted on,
// in particular after Close has been called.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.AcceptUnix()
		if err != nil {
			return err
		}

		if err := s.send(conn); err != nil && s.ErrorHandler != nil {
			s.ErrorHandler(err)
		}

		if err := conn.Close(); err != nil && s.ErrorHandler != nil {
			s.ErrorHandler(fmt.Errorf("can't close the handoff connection: %w", err))
		}
	}
}

// Close stops the handoff socket. The socket being handed out is left untouched.
func (s *Server) Close() error {
	return s.ln.Close()
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
func receiveFile(handoffPath string) (*os.File, error) {
	conn, err := net.DialTimeout(handoffTransport, handoffPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("can't connect to the handoff socket %s: %w", handoffPath, err)
	}
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("unexpected return type from Dial, expected UnixConn: %#v", conn)
	}

	if err := unixConn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return nil, fmt.Errorf("can't set a read deadline on the handoff socket: %w", err)
	}

	buf := make([]byte, len(handoffMessage))
	oob := make([]byte, unix.CmsgSpace(4))
	n, oobn, _, _, err := unixConn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, fmt.Errorf("can't read from the handoff socket: %w", err)
	}
	if n == 0 && oobn == 0 {
		return nil, fmt.Errorf("the handoff socket %s closed the connection without sending a file descriptor", handoffPath)
	}

	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, fmt.Errorf("can't parse the handoff ancillary data: %w", err)
	}

	for _, message := range messages {
		if message.Header.Level != unix.SOL_SOCKET || message.Header.Type != unix.SCM_RIGHTS {
			continue
		}

		fds, err := unix.ParseUnixRights(&message)
		if err != nil {
			return nil, fmt.Errorf("can't parse the handoff file descriptors: %w", err)
		}

		// We only expect a single file descriptor, close any extra one so that
		// we don't leak it.
		for _, fd := range fds[1:] {
			_ = unix.Close(fd)
		}
		if len(fds) > 0 {
			return os.NewFile(uintptr(fds[0]), handoffPath), nil
		}
	}

	return nil, fmt.Errorf("no file descriptor received from the handoff socket %s", handoffPath)
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
