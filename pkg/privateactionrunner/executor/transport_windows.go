// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package executor

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
)

// defaultSocketPath is the platform-specific transport path used when the
// operator did not configure one. On Windows the path is a named pipe.
func defaultSocketPath() string {
	return `\\.\pipe\datadog-par-executor`
}

// Listen opens a named-pipe listener at the given path.
func Listen(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, errors.New("empty pipe path")
	}
	cfg := &winio.PipeConfig{
		MessageMode:      false,
		InputBufferSize:  65536,
		OutputBufferSize: 65536,
	}
	l, err := winio.ListenPipe(socketPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	return l, nil
}

// dialTarget returns the gRPC target string a client uses to dial the given
// pipe.
func dialTarget(socketPath string) string {
	return "pipe:" + socketPath
}

// signalProcess best-effort tries to ask the child to exit gracefully.
// Windows lacks SIGTERM; os.Process.Kill is the only built-in option and
// the WaitDelay timer will force termination anyway.
func signalProcess(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}
