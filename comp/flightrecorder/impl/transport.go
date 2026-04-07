// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flightrecorderimpl implements the flight recorder component.
package flightrecorderimpl

import (
	"context"
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

// unixTransport manages a Unix domain socket connection to the sidecar.
//
// On write errors, the transport silently reconnects to the same socket
// path without tearing down the batcher or hook subscriptions. Only if
// reconnection fails is the full disconnect signaled.
//
// No write deadline is set — WriteTo blocks until all bytes are written
// or the connection breaks. This is safe because the sidecar's async
// handler drains the socket into an rtrb ring in microseconds (dedicated
// writer threads handle the slow Parquet I/O separately). A deadline
// would risk interrupting WriteTo mid-frame, corrupting the length-
// prefixed framing and requiring connection replacement.
type unixTransport struct {
	socketPath string

	mu           sync.Mutex
	conn         net.Conn
	disconnected chan struct{} // closed when reconnect fails (fatal)

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// onDisconnect is called when reconnection fails (sidecar is gone).
	// Triggers full teardown (hooks, batcher). Set by the caller after
	// creation. Must be safe to call from any goroutine.
	onDisconnect func()
}

// newUnixTransport creates a unixTransport and starts the background serve loop.
// The transport connects to the socket immediately (the discovery loop already
// verified the socket is reachable).
func newUnixTransport(parentCtx context.Context, socketPath string) *unixTransport {
	ctx, cancel := context.WithCancel(parentCtx)
	t := &unixTransport{
		socketPath:   socketPath,
		disconnected: make(chan struct{}),
		cancel:       cancel,
	}
	t.wg.Add(1)
	go t.serveLoop(ctx)
	return t
}

// serveLoop connects to the socket and blocks until a fatal disconnect
// or the context is cancelled.
func (t *unixTransport) serveLoop(ctx context.Context) {
	defer t.wg.Done()
	defer t.cancel()

	conn, err := net.DialTimeout("unix", t.socketPath, 5*time.Second)
	if err != nil {
		if t.onDisconnect != nil {
			t.onDisconnect()
		}
		return
	}

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	select {
	case <-ctx.Done():
		conn.Close() //nolint:errcheck
		t.mu.Lock()
		t.conn = nil
		t.mu.Unlock()
	case <-t.disconnected:
		// reconnect() failed — sidecar is gone. Full teardown.
		if t.onDisconnect != nil {
			t.onDisconnect()
		}
	}
}

// Send writes b to the Unix socket as a length-prefixed frame.
//
// No write deadline — WriteTo blocks until all bytes are written or the
// connection breaks. The sidecar's async handler drains the socket fast
// enough that blocking is transient (microseconds). See CONNECTION_DESIGN.md.
//
// On any write error (broken pipe, connection reset), the transport
// silently reconnects. The current frame is lost but the batcher and
// hook subscriptions stay alive.
func (t *unixTransport) Send(b []byte) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return errNotConnected
	}

	// Write a length-prefixed frame using writev (net.Buffers) to avoid
	// copying the payload just to prepend 4 bytes.
	// WriteTo loops internally until all bytes are written or error.
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(b)))
	bufs := net.Buffers{prefix[:], b}
	_, err := bufs.WriteTo(conn)
	if err != nil {
		// Connection is dead (broken pipe, reset, etc.).
		// WriteTo only returns after attempting all bytes, so the
		// stream may be corrupt. Replace the connection silently.
		t.reconnect(conn)
	}
	return err
}

// reconnect closes the old connection and opens a new one to the same
// socket path. The batcher and hook subscriptions are NOT affected.
//
// If reconnection fails, signals the serve loop for full teardown.
func (t *unixTransport) reconnect(oldConn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != oldConn {
		return // another goroutine already reconnected
	}

	t.conn = nil
	oldConn.Close() //nolint:errcheck

	newConn, err := net.DialTimeout("unix", t.socketPath, 2*time.Second)
	if err != nil {
		// Sidecar is gone — signal fatal disconnect.
		select {
		case <-t.disconnected:
		default:
			close(t.disconnected)
		}
		return
	}

	t.conn = newConn
}

// Close cancels the serve loop and closes any open connection.
func (t *unixTransport) Close() error {
	t.cancel()
	t.wg.Wait()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

// errNotConnected is returned by Send when there is no live connection.
var errNotConnected = &transportError{"not connected to flightrecorder socket"}

type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }
