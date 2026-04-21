// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flightrecorderimpl implements the flight recorder component.
package flightrecorderimpl

import (
	"encoding/binary"
	"net"
	"sync"
	"time"
)

// Transport abstracts the wire protocol so the Unix-socket implementation can be
// swapped for a zero-copy shared-memory transport (Iceoryx2) without any logic changes.
type Transport interface {
	// Send writes b to the transport. If the connection is not established it
	// returns an error immediately without blocking.
	Send(b []byte) error
	// Close shuts down the transport.
	Close() error
}

// unixConn manages a single Unix domain socket connection. Each pipeline gets
// its own unixConn, so pipelines never contend on the same socket.
//
// The mutex is held for the entire Send call. This is acceptable because each
// pipeline has at most 2 flush goroutines (entries + contexts), and context
// flushes are rare after warm-up.
type unixConn struct {
	socketPath string

	mu           sync.Mutex
	conn         net.Conn
	closed       bool
	disconnected bool

	// onDisconnect is called exactly once when dialing fails (sidecar gone).
	// Set by activate() before any Send.
	onDisconnect func()
}

func newUnixConn(socketPath string) *unixConn {
	return &unixConn{socketPath: socketPath}
}

// Send writes b as a length-prefixed frame. The connection is lazily dialed
// on first use. On write error, the connection is replaced immediately with
// a fresh dial. If dialing fails, the sidecar is considered gone.
func (c *unixConn) Send(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return errNotConnected
	}

	// Lazy dial.
	if c.conn == nil {
		conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
		if err != nil {
			c.triggerDisconnect()
			return errNotConnected
		}
		c.conn = conn
	}

	// Write length-prefixed frame using writev (net.Buffers).
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(b)))
	bufs := net.Buffers{prefix[:], b}
	_, err := bufs.WriteTo(c.conn)
	if err != nil {
		// Connection dead — close and try one immediate reconnect.
		c.conn.Close() //nolint:errcheck
		c.conn = nil

		conn, dialErr := net.DialTimeout("unix", c.socketPath, 2*time.Second)
		if dialErr != nil {
			c.triggerDisconnect()
			return errNotConnected
		}
		c.conn = conn
		// Retry the write on the fresh connection.
		bufs = net.Buffers{prefix[:], b}
		_, err = bufs.WriteTo(c.conn)
		if err != nil {
			c.conn.Close() //nolint:errcheck
			c.conn = nil
			return err
		}
	}
	return nil
}

// Close shuts down the connection.
func (c *unixConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// triggerDisconnect fires the onDisconnect callback exactly once.
// Must be called with mu held.
func (c *unixConn) triggerDisconnect() {
	if c.disconnected {
		return
	}
	c.disconnected = true
	if c.onDisconnect != nil {
		c.onDisconnect()
	}
}

// errNotConnected is returned by Send when there is no live connection.
var errNotConnected = &transportError{"not connected to flightrecorder socket"}

type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }
