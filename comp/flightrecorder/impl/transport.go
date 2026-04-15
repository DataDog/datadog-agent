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

// pooledTransport manages a pool of Unix domain socket connections to the
// sidecar. Each flush goroutine acquires a connection from the pool for the
// duration of a Send(), so concurrent flushes never block each other.
//
// Connections are created lazily on first Acquire and returned to the pool
// after use. On write error, the bad connection is discarded and a new one
// is dialed. If dialing fails, the sidecar is considered gone and a fatal
// disconnect is signaled.
type pooledTransport struct {
	socketPath string

	mu    sync.Mutex
	conns []net.Conn // idle connections

	closed bool

	// onDisconnect is called exactly once when the sidecar is unreachable
	// (dial fails). Triggers full teardown (hooks, batcher).
	onDisconnect func()
	disconnected bool
}

func newPooledTransport(socketPath string) *pooledTransport {
	return &pooledTransport{
		socketPath: socketPath,
	}
}

// acquire returns an idle connection from the pool, or dials a new one.
func (t *pooledTransport) acquire() (net.Conn, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, errNotConnected
	}
	if len(t.conns) > 0 {
		conn := t.conns[len(t.conns)-1]
		t.conns = t.conns[:len(t.conns)-1]
		t.mu.Unlock()
		return conn, nil
	}
	t.mu.Unlock()

	// Dial outside the lock — this may block for up to 2s.
	conn, err := net.DialTimeout("unix", t.socketPath, 2*time.Second)
	if err != nil {
		t.fatalDisconnect()
		return nil, errNotConnected
	}
	return conn, nil
}

// release returns a healthy connection to the pool.
func (t *pooledTransport) release(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		conn.Close() //nolint:errcheck
		return
	}
	t.conns = append(t.conns, conn)
}

// discard closes a bad connection without returning it to the pool.
func (t *pooledTransport) discard(conn net.Conn) {
	conn.Close() //nolint:errcheck
}

// fatalDisconnect signals that the sidecar is unreachable. Called at most once.
func (t *pooledTransport) fatalDisconnect() {
	t.mu.Lock()
	if t.disconnected {
		t.mu.Unlock()
		return
	}
	t.disconnected = true
	fn := t.onDisconnect
	t.mu.Unlock()

	if fn != nil {
		fn()
	}
}

// Send writes b to the Unix socket as a length-prefixed frame.
//
// Acquires a connection from the pool, writes, and returns it. If the write
// fails, the connection is discarded and a new one will be dialed on the
// next Send. Concurrent goroutines each get their own connection — no
// goroutine blocks another.
func (t *pooledTransport) Send(b []byte) error {
	conn, err := t.acquire()
	if err != nil {
		return err
	}

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(b)))
	bufs := net.Buffers{prefix[:], b}
	_, err = bufs.WriteTo(conn)
	if err != nil {
		t.discard(conn)
		return err
	}

	t.release(conn)
	return nil
}

// Close drains the pool and closes all idle connections.
func (t *pooledTransport) Close() error {
	t.mu.Lock()
	t.closed = true
	conns := t.conns
	t.conns = nil
	t.mu.Unlock()

	var firstErr error
	for _, c := range conns {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// errNotConnected is returned by Send when there is no live connection.
var errNotConnected = &transportError{"not connected to flightrecorder socket"}

type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }
