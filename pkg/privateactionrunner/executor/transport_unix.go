// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package executor

import (
	"context"
	"net"
	"os"
	"time"
)

// DefaultSocketPath is the default local socket the executor listens on and the
// control plane dials. It is baked into the process-manager process definition
// (out of scope here) rather than passed at start time.
const DefaultSocketPath = "/opt/datadog-agent/run/par-executor.sock"

// Listen creates the executor's listening socket. On Unix this is a Unix domain
// socket; a stale socket file from a previous run is removed first.
func Listen(address string) (net.Listener, error) {
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return net.Listen("unix", address)
}

// Dial connects to the executor's socket. Used by the control plane's gRPC client
// (and by tests) to reach the executor.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "unix", address)
}
