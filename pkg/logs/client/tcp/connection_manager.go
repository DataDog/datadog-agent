// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package tcp

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

const (
	maxExpBackoffCount    = 7
	connectionTimeout     = 20 * time.Second
	statusConnectionError = "connection_error"
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	endpoint  config.Endpoint
	mutex     sync.Mutex
	firstConn sync.Once
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(endpoint config.Endpoint) *ConnectionManager {
	panic("not called")
}

type tlsTimeoutError struct{}

func (tlsTimeoutError) Error() string {
	panic("not called")
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection(ctx context.Context) (net.Conn, error) {
	panic("not called")
}

func (cm *ConnectionManager) handshakeWithTimeout(conn *tls.Conn, timeout time.Duration) error {
	panic("not called")
}

// address returns the address of the server to send logs to.
func (cm *ConnectionManager) address() string {
	panic("not called")
}

// ShouldReset returns whether the connection should be reset, depending on the endpoint's config
// and the passed connection creation time.
func (cm *ConnectionManager) ShouldReset(connCreationTime time.Time) bool {
	panic("not called")
}

// CloseConnection closes a connection on the client side
func (cm *ConnectionManager) CloseConnection(conn net.Conn) {
	panic("not called")
}

// handleServerClose lets the connection manager detect when a connection
// has been closed by the server, and closes it for the client.
// This is not strictly necessary but a good safeguard against callers
// that might not handle errors properly.
func (cm *ConnectionManager) handleServerClose(conn net.Conn) {
	panic("not called")
}

// backoff implements a randomized exponential backoff in case of connection failure
// each invocation will trigger a sleep between [2^(retries-1), 2^retries) second
// the exponent is capped at 7, which translates to max sleep between ~1min and ~2min
func (cm *ConnectionManager) backoff(ctx context.Context, retries uint) {
	panic("not called")
}
