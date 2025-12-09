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
)

// GetListenerFromFD creates a new net.Listener from a Windows socket handle
//
// On Windows, socket handles are passed via environment variables (e.g., DD_APM_NET_RECEIVER_FD).
// The handle is inherited from the parent process using SetHandleInformation.
//
// Unlike Unix, Go's net.FileListener doesn't work with Windows socket handles,
// so we create a custom listener that uses the raw socket handle directly.
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
	sa, err := syscall.Getsockname(l.handle)
	if err != nil {
		return err
	}
	l.addr = sockaddrToTCPAddr(sa)
	return nil
}

func (l *winSocketListener) Accept() (net.Conn, error) {
	// Accept a new connection on the socket
	newHandle, sa, err := syscall.Accept(l.handle)
	if err != nil {
		return nil, &net.OpError{Op: "accept", Net: "tcp", Err: err}
	}

	// Create a net.Conn from the accepted socket
	conn := &winSocketConn{
		handle:     newHandle,
		localAddr:  l.addr,
		remoteAddr: sockaddrToTCPAddr(sa),
	}

	return conn, nil
}

func (l *winSocketListener) Close() error {
	return syscall.Closesocket(l.handle)
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
	n, err := syscall.Read(c.handle, b)
	if err != nil {
		return n, &net.OpError{Op: "read", Net: "tcp", Err: err}
	}
	return n, nil
}

func (c *winSocketConn) Write(b []byte) (int, error) {
	n, err := syscall.Write(c.handle, b)
	if err != nil {
		return n, &net.OpError{Op: "write", Net: "tcp", Err: err}
	}
	return n, nil
}

func (c *winSocketConn) Close() error {
	return syscall.Closesocket(c.handle)
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

// Windows socket options not defined in syscall package
const (
	soRcvTimeo = 0x1006 // SO_RCVTIMEO
	soSndTimeo = 0x1005 // SO_SNDTIMEO
)

func (c *winSocketConn) SetReadDeadline(t time.Time) error {
	return setSocketTimeout(c.handle, soRcvTimeo, t)
}

func (c *winSocketConn) SetWriteDeadline(t time.Time) error {
	return setSocketTimeout(c.handle, soSndTimeo, t)
}

func setSocketTimeout(handle syscall.Handle, opt int, t time.Time) error {
	var timeout int32
	if !t.IsZero() {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		timeout = int32(d.Milliseconds())
	}
	return syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, opt, int(timeout))
}

// sockaddrToTCPAddr converts a syscall.Sockaddr to a *net.TCPAddr
func sockaddrToTCPAddr(sa syscall.Sockaddr) *net.TCPAddr {
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		return &net.TCPAddr{
			IP:   net.IPv4(sa.Addr[0], sa.Addr[1], sa.Addr[2], sa.Addr[3]),
			Port: sa.Port,
		}
	case *syscall.SockaddrInet6:
		return &net.TCPAddr{
			IP:   sa.Addr[:],
			Port: sa.Port,
			Zone: zoneToString(int(sa.ZoneId)),
		}
	default:
		return nil
	}
}

func zoneToString(zone int) string {
	if zone == 0 {
		return ""
	}
	if ifi, err := net.InterfaceByIndex(zone); err == nil {
		return ifi.Name
	}
	return strconv.Itoa(zone)
}
