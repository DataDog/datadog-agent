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

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Transport abstracts the wire protocol so the Unix-socket implementation can be
// swapped for a zero-copy shared-memory transport (Iceoryx2) without any logic changes.
type Transport interface {
	// Send writes b to the transport. If the connection is not established it
	// returns an error immediately without blocking.
	Send(b []byte) error
	// Close shuts down the transport and cancels the reconnect loop.
	Close() error
}

const (
	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
)

type unixTransport struct {
	socketPath string

	mu           sync.Mutex
	conn         net.Conn
	disconnected chan struct{} // closed by Send when it detects a broken connection

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// reconnectStats is called after each reconnect attempt (used in tests).
	reconnectStats func(attempt int, delay time.Duration)
	// onReconnect is called whenever a successful connection is made (increments Reconnects counter).
	onReconnect func()
}

// newUnixTransport creates a unixTransport and starts the background reconnect loop.
func newUnixTransport(socketPath string, onReconnect func()) *unixTransport {
	ctx, cancel := context.WithCancel(context.Background())
	t := &unixTransport{
		socketPath:  socketPath,
		cancel:      cancel,
		onReconnect: onReconnect,
	}
	t.wg.Add(1)
	go t.reconnectLoop(ctx)
	return t
}

func (t *unixTransport) reconnectLoop(ctx context.Context) {
	defer t.wg.Done()
	delay := reconnectInitial
	attempt := 0
	for {
		// Wait before attempting (first attempt also waits so the caller has time
		// to set up the server in tests).
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		conn, err := net.DialTimeout("unix", t.socketPath, 5*time.Second)
		if err != nil {
			attempt++
			if t.reconnectStats != nil {
				t.reconnectStats(attempt, delay)
			}
			delay *= 2
			if delay > reconnectMax {
				delay = reconnectMax
			}
			continue
		}

		// Connected.
		pkglog.Infof("flightrecorder: connected to %s", t.socketPath)
		if t.onReconnect != nil {
			t.onReconnect()
		}
		disconnected := make(chan struct{})
		t.mu.Lock()
		t.conn = conn
		t.disconnected = disconnected
		t.mu.Unlock()

		// Block until the connection is closed (detected by a failed send) or
		// the context is cancelled.
		select {
		case <-ctx.Done():
			conn.Close() //nolint:errcheck
			t.mu.Lock()
			t.conn = nil
			t.mu.Unlock()
			return
		case <-disconnected:
			// Send detected a broken connection; loop back to reconnect.
			delay = reconnectInitial
			attempt = 0
		}
	}
}

// Send writes b to the Unix socket. It returns an error immediately if not connected.
func (t *unixTransport) Send(b []byte) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return errNotConnected
	}

	// Write a length-prefixed frame using writev (net.Buffers) to avoid
	// copying the payload just to prepend 4 bytes.
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(b)))
	bufs := net.Buffers{prefix[:], b}
	_, err := bufs.WriteTo(conn)
	if err != nil {
		// Mark connection as dead and signal the reconnect loop.
		t.mu.Lock()
		if t.conn == conn {
			t.conn = nil
			conn.Close() //nolint:errcheck
			if t.disconnected != nil {
				close(t.disconnected)
				t.disconnected = nil
			}
		}
		t.mu.Unlock()
	}
	return err
}

// Close cancels the reconnect loop and closes any open connection.
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
