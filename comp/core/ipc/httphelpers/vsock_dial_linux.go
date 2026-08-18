// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package httphelpers

import (
	"context"
	"fmt"
	"net"

	"github.com/mdlayher/socket"
	"golang.org/x/sys/unix"
)

func dialVSockContext(ctx context.Context, cid, port uint32) (net.Conn, error) {
	// Use socket directly so Connect observes the transport context, which carries
	// the server timeout. vsock.Dial connects with context.Background instead.
	conn, err := socket.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0, "vsock", nil)
	if err != nil {
		return nil, err
	}

	remote, err := conn.Connect(ctx, &unix.SockaddrVM{CID: cid, Port: port})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	local, err := conn.Getsockname()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	localAddr, ok := local.(*unix.SockaddrVM)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected local vsock address %T", local)
	}
	remoteAddr, ok := remote.(*unix.SockaddrVM)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected remote vsock address %T", remote)
	}

	return &vsockConn{
		Conn:       conn,
		localAddr:  vsockAddr{cid: localAddr.CID, port: localAddr.Port},
		remoteAddr: vsockAddr{cid: remoteAddr.CID, port: remoteAddr.Port},
	}, nil
}

// vsockConn adapts socket.Conn to net.Conn. socket.Conn has context-aware
// Connect, but does not expose the addresses required by net.Conn.
type vsockConn struct {
	*socket.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *vsockConn) LocalAddr() net.Addr  { return c.localAddr }
func (c *vsockConn) RemoteAddr() net.Addr { return c.remoteAddr }

type vsockAddr struct {
	cid  uint32
	port uint32
}

func (a vsockAddr) Network() string { return "vsock" }
func (a vsockAddr) String() string  { return fmt.Sprintf("%d:%d", a.cid, a.port) }
