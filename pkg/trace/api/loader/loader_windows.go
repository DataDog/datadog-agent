// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package loader contains initialization logic shared with the trace-loader process
package loader

import (
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// Windows socket constants and functions not in syscall package
var (
	wsock32         = syscall.NewLazyDLL("ws2_32.dll")
	procAccept      = wsock32.NewProc("accept")
	procGetsockname = wsock32.NewProc("getsockname")
	procClosesocket = wsock32.NewProc("closesocket")
	procRecv        = wsock32.NewProc("recv")
	procSend        = wsock32.NewProc("send")
	procSetsockopt  = wsock32.NewProc("setsockopt")
)

// Windows socket options
const (
	soRcvTimeo = 0x1006 // SO_RCVTIMEO
	soSndTimeo = 0x1005 // SO_SNDTIMEO
	solSocket  = 0xffff // SOL_SOCKET
	afInet     = 2      // AF_INET
	afInet6    = 23     // AF_INET6
)

// sockaddrInet4 is the Windows SOCKADDR_IN structure
type sockaddrInet4 struct {
	Family uint16
	Port   uint16
	Addr   [4]byte
	Zero   [8]byte
}

// sockaddrInet6 is the Windows SOCKADDR_IN6 structure
type sockaddrInet6 struct {
	Family   uint16
	Port     uint16
	FlowInfo uint32
	Addr     [16]byte
	ScopeID  uint32
}

// GetListenerFromFD creates a new net.Listener from a Windows socket handle
//
// On Windows, socket handles are passed via environment variables (e.g., DD_APM_NET_RECEIVER_FD).
// The handle is inherited from the parent process using SetHandleInformation.
//
// Unlike Unix, Go's net.FileListener doesn't work with Windows socket handles,
// so we create a custom listener that uses raw Winsock API calls.
func GetListenerFromFD(handleStr string, name string) (net.Listener, error) {
	handle, err := strconv.ParseUint(handleStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse socket handle %v: %v", handleStr, err)
	}

	// Create a custom listener that wraps the inherited socket handle
	listener := &winSocketListener{
		handle: syscall.Handle(handle),
		name:   name,
	}

	// Get the local address from the socket
	if err := listener.initAddr(); err != nil {
		return nil, fmt.Errorf("failed to get socket address: %v", err)
	}

	return listener, nil
}

// winSocketListener wraps a Windows socket handle to implement net.Listener
type winSocketListener struct {
	handle syscall.Handle
	name   string
	addr   net.Addr
}

func (l *winSocketListener) initAddr() error {
	// Get the local address from the socket using getsockname
	var sa sockaddrInet4
	saLen := int32(unsafe.Sizeof(sa))

	r1, _, err := procGetsockname.Call(
		uintptr(l.handle),
		uintptr(unsafe.Pointer(&sa)),
		uintptr(unsafe.Pointer(&saLen)),
	)
	if r1 != 0 {
		return fmt.Errorf("getsockname failed: %v", err)
	}

	// Convert to net.Addr
	port := int(sa.Port>>8) | int(sa.Port&0xff)<<8 // network byte order
	l.addr = &net.TCPAddr{
		IP:   net.IPv4(sa.Addr[0], sa.Addr[1], sa.Addr[2], sa.Addr[3]),
		Port: port,
	}
	return nil
}

func (l *winSocketListener) Accept() (net.Conn, error) {
	// Accept a new connection using Winsock accept()
	var sa sockaddrInet4
	saLen := int32(unsafe.Sizeof(sa))

	r1, _, err := procAccept.Call(
		uintptr(l.handle),
		uintptr(unsafe.Pointer(&sa)),
		uintptr(unsafe.Pointer(&saLen)),
	)

	// INVALID_SOCKET = ^uintptr(0)
	if r1 == ^uintptr(0) {
		return nil, &net.OpError{Op: "accept", Net: "tcp", Err: err}
	}

	newHandle := syscall.Handle(r1)

	// Convert remote address
	port := int(sa.Port>>8) | int(sa.Port&0xff)<<8
	remoteAddr := &net.TCPAddr{
		IP:   net.IPv4(sa.Addr[0], sa.Addr[1], sa.Addr[2], sa.Addr[3]),
		Port: port,
	}

	// Create a net.Conn from the accepted socket
	conn := &winSocketConn{
		handle:     newHandle,
		localAddr:  l.addr,
		remoteAddr: remoteAddr,
	}

	return conn, nil
}

func (l *winSocketListener) Close() error {
	r1, _, _ := procClosesocket.Call(uintptr(l.handle))
	if r1 != 0 {
		return fmt.Errorf("closesocket failed")
	}
	return nil
}

func (l *winSocketListener) Addr() net.Addr {
	return l.addr
}

// winSocketConn wraps a Windows socket handle to implement net.Conn
type winSocketConn struct {
	handle     syscall.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *winSocketConn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	r1, _, err := procRecv.Call(
		uintptr(c.handle),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)),
		0, // flags
	)

	// SOCKET_ERROR = -1
	if int32(r1) == -1 {
		return 0, &net.OpError{Op: "read", Net: "tcp", Err: err}
	}

	return int(r1), nil
}

func (c *winSocketConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	r1, _, err := procSend.Call(
		uintptr(c.handle),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)),
		0, // flags
	)

	// SOCKET_ERROR = -1
	if int32(r1) == -1 {
		return 0, &net.OpError{Op: "write", Net: "tcp", Err: err}
	}

	return int(r1), nil
}

func (c *winSocketConn) Close() error {
	r1, _, _ := procClosesocket.Call(uintptr(c.handle))
	if r1 != 0 {
		return fmt.Errorf("closesocket failed")
	}
	return nil
}

func (c *winSocketConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *winSocketConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *winSocketConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *winSocketConn) SetReadDeadline(t time.Time) error {
	return c.setSocketTimeout(soRcvTimeo, t)
}

func (c *winSocketConn) SetWriteDeadline(t time.Time) error {
	return c.setSocketTimeout(soSndTimeo, t)
}

func (c *winSocketConn) setSocketTimeout(opt int, t time.Time) error {
	var timeout int32
	if !t.IsZero() {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		timeout = int32(d.Milliseconds())
	}

	r1, _, err := procSetsockopt.Call(
		uintptr(c.handle),
		uintptr(solSocket),
		uintptr(opt),
		uintptr(unsafe.Pointer(&timeout)),
		uintptr(unsafe.Sizeof(timeout)),
	)

	if r1 != 0 {
		return fmt.Errorf("setsockopt failed: %v", err)
	}
	return nil
}
