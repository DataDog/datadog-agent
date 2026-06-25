// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package executor

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// DefaultSocketPath is the platform-specific socket path used when the
// operator did not configure one.
func DefaultSocketPath() string {
	return "/opt/datadog-agent/run/par-executor.sock"
}

// Listen opens a listener on the given socket path, cleaning up any stale
// socket file left over from a previous run.
func Listen(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, errors.New("empty socket path")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	// Restrict access to the agent user; the executor and orchestrator
	// additionally authenticate via the bearer token.
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return l, nil
}

// DialTarget returns the gRPC target string a client uses to dial the
// given socket path.
func DialTarget(socketPath string) string {
	return "unix:" + socketPath
}

// SignalProcess sends SIGTERM to the running child so it has a chance to
// drain before the WaitDelay-driven SIGKILL.
func SignalProcess(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Signal(syscall.SIGTERM)
}
