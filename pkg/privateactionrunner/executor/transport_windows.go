// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package executor

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// Listen creates the executor's listening named pipe.
func Listen(address string) (net.Listener, error) {
	return winio.ListenPipe(address, &winio.PipeConfig{})
}

// Dial connects to the executor's named pipe.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := winio.DialPipe(address, &timeout)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case conn := <-connCh:
		return conn, nil
	}
}
