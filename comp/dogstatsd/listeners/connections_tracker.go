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

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	return &ConnectionTracker{
		connections:       make(map[net.Conn]struct{}),
		connToTrack:       make(chan net.Conn),
		connToClose:       make(chan net.Conn),
		stopChan:          make(chan struct{}),
		stoppedChan:       make(chan struct{}),
		closeDelay:        closeDelay,
		activeConnections: atomic.NewInt32(0),
		name:              name,
	}
}

// Start starts the connection tracker.
func (t *ConnectionTracker) Start() {
	go t.HandleConnections()
}

// Track tracks a connection.
func (t *ConnectionTracker) Track(conn net.Conn) {
	t.connToTrack <- conn
}

// Close closes a connection.
func (t *ConnectionTracker) Close(conn net.Conn) {
	t.connToClose <- conn
}

// HandleConnections handles connections.
func (t *ConnectionTracker) HandleConnections() {
	requestStop := false
	for stop := false; !stop; {
		select {
		case conn := <-t.connToTrack:
			log.Debugf("dogstatsd-%s: tracking new connection %s", t.name, conn.RemoteAddr().String())

			if requestStop {
				//Close it immediately if we are shutting down.
				conn.Close()
			} else {
				t.connections[conn] = struct{}{}
				t.activeConnections.Inc()
			}
		case conn := <-t.connToClose:
			if conn != nil {
				if err := conn.Close(); err != nil {
					log.Warnf("dogstatsd-%s: failed to close connection: %v", t.name, err)
				}
			}

			if _, ok := t.connections[conn]; !ok {
				log.Warnf("dogstatsd-%s: connection wasn't tracked", t.name)
			} else {
				delete(t.connections, conn)
				t.activeConnections.Dec()
			}
		case <-t.stopChan:
			log.Infof("dogstatsd-%s: stopping connections", t.name)
			requestStop = true

			var err error
			for c := range t.connections {
				// First, when possible, we close the write end of the connection to notify
				// the client that we are shutting down.
				switch c := c.(type) {
				case *net.TCPConn:
					err = c.CloseWrite()
				case *net.UnixConn:
					err = c.CloseWrite()
				}

				if err != nil {
					log.Warnf("dogstatsd-%s: failed to shutdown connection (write): %v", t.name, err)
					err = nil
				}
			}

			time.Sleep(t.closeDelay)

			for c := range t.connections {
				// Then, we finish closing the connection.
				switch c := c.(type) {
				case *net.TCPConn:
					err = c.CloseRead()
				case *net.UnixConn:
					err = c.CloseRead()
				default:
					// We don't have a choice, setting a 0 timeout would likely be a retryable error.
					err = c.Close()
				}

				if err != nil {
					log.Warnf("dogstatsd-%s: failed to shutdown connection (read): %v", t.name, err)
					err = nil
				}
			}
		case <-time.After(1 * time.Second):
			// We don't want to block forever on the select, so we add a timeout.
		}

		// Stop if we are requested to stop and all connections are closed. We might drop
		// some work in the channels but that should be fine.
		if requestStop && len(t.connections) == 0 {
			log.Debugf("dogstatsd-%s: all connections closed", t.name)
			stop = true
		}
	}
	t.stoppedChan <- struct{}{}
}

// Stop stops the connection tracker.
// To be called one the listener is stopped, after the server socket has been close.
func (t *ConnectionTracker) Stop() {
	if t.activeConnections.Load() == 0 {
		return
	}

	// Request closing connections
	t.stopChan <- struct{}{}

	// Wait until all connections are closed
	<-t.stoppedChan
}
