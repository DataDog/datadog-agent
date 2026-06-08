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
	"path/filepath"
	"time"
)

func defaultSocketPath(runPath string) string {
	return filepath.Join(runPath, "private-action-runner-executor.sock")
}

func listen(address string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(address), 0755); err != nil {
		return nil, err
	}
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return net.Listen("unix", address)
}

func dial(ctx context.Context, address string, _ time.Duration) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", address)
}
