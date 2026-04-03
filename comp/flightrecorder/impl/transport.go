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

// unixTransport is a single-use Unix socket transport. It connects to the
// socket path once and serves until the connection is lost. On disconnect,
// it calls onDisconnect and exits. A new transport is created for each
// activation cycle by the discovery loop.
type unixTransport struct {
	socketPath string

	mu           sync.Mutex
	conn         net.Conn
	disconnected chan struct{} // closed by Send when it detects a broken connection

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// onDisconnect is called when the connection is lost. Set by the caller
	// after creation. Must be safe to call from any goroutine.
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

// serveLoop connects to the socket and blocks until the connection is lost
// or the context is cancelled. On disconnect, it calls onDisconnect.
func (t *unixTransport) serveLoop(ctx context.Context) {
	defer t.wg.Done()
	defer t.cancel() // release child context resources

	conn, err := net.DialTimeout("unix", t.socketPath, 5*time.Second)
	if err != nil {
		// Socket disappeared between discovery probe and connect — trigger teardown.
		if t.onDisconnect != nil {
			t.onDisconnect()
		}
		return
	}

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	// Block until the connection is lost or the context is cancelled.
	select {
	case <-ctx.Done():
		conn.Close() //nolint:errcheck
		t.mu.Lock()
		t.conn = nil
		t.mu.Unlock()
		return
	case <-t.disconnected:
		// Send detected a broken connection — trigger teardown.
		if t.onDisconnect != nil {
			t.onDisconnect()
		}
	}
}

// Send writes b to the Unix socket. It returns an error immediately if not connected.
//
// A 5-second write deadline prevents blocking the flush goroutine forever.
// On timeout, the frame is dropped but the connection stays alive — a timeout
// is a transient backpressure signal, not a permanent failure. The connection
// is only torn down on non-timeout errors (broken pipe, connection reset).
func (t *unixTransport) Send(b []byte) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return errNotConnected
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck

	// Write a length-prefixed frame using writev (net.Buffers) to avoid
	// copying the payload just to prepend 4 bytes.
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(b)))
	bufs := net.Buffers{prefix[:], b}
	_, err := bufs.WriteTo(conn)
	if err != nil {
		// Timeout: sidecar is slow but alive. Drop the frame, keep connection.
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return err
		}
		// Non-timeout error (broken pipe, reset): connection is dead.
		t.mu.Lock()
		if t.conn == conn {
			t.conn = nil
			conn.Close() //nolint:errcheck
			select {
			case <-t.disconnected:
			default:
				close(t.disconnected)
			}
		}
		t.mu.Unlock()
	}
	return err
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
