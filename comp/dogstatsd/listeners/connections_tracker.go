// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package listeners implements the StatsdListener interfaces.
package listeners

import (
	"net"
	"time"

	"go.uber.org/atomic"
)

// ConnectionTracker tracks connections and closes them gracefully.
type ConnectionTracker struct {
	connections       map[net.Conn]struct{}
	connToTrack       chan net.Conn
	connToClose       chan net.Conn
	stopChan          chan struct{}
	stoppedChan       chan struct{}
	closeDelay        time.Duration
	activeConnections *atomic.Int32
	name              string
}

// NewConnectionTracker creates a new ConnectionTracker.
// closeDelay is the time to wait before closing a connection. First it will be shutdown for write, which
// will notify the client that we are disconnecting, then it will be closed. This gives some time to
// consume the remaining packets.
func NewConnectionTracker(name string, closeDelay time.Duration) *ConnectionTracker {
	panic("not called")
}

// Start starts the connection tracker.
func (t *ConnectionTracker) Start() {
	panic("not called")
}

// Track tracks a connection.
func (t *ConnectionTracker) Track(conn net.Conn) {
	panic("not called")
}

// Close closes a connection.
func (t *ConnectionTracker) Close(conn net.Conn) {
	panic("not called")
}

// HandleConnections handles connections.
func (t *ConnectionTracker) HandleConnections() {
	panic("not called")
}

// Stop stops the connection tracker.
// To be called one the listener is stopped, after the server socket has been close.
func (t *ConnectionTracker) Stop() {
	panic("not called")
}
