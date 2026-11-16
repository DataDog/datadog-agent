// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package executor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// socketMode restricts the executor socket to its owner. dd-procmgrd starts
// par-control and the executor under its own UID, so no other user needs access.
const socketMode os.FileMode = 0o600

// chmodSocket is replaced in tests to exercise permission-setup failures.
var chmodSocket = os.Chmod

// Listen creates the executor's Unix socket, replacing any stale socket file.
//
// The socket is bound inside a private 0700 directory next to address,
// restricted to socketMode, and only then renamed over address, so it is
// never reachable with umask-derived permissions.
func Listen(address string) (net.Listener, error) {
	// Same directory, so the rename stays on one filesystem; short names keep
	// the staging path within the sun_path limit.
	staging, err := os.MkdirTemp(filepath.Dir(address), ".pe")
	if err != nil {
		return nil, fmt.Errorf("could not create executor socket staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	stagingPath := filepath.Join(staging, "s")
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: stagingPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	lis.SetUnlinkOnClose(false)

	if err := chmodSocket(stagingPath, socketMode); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("could not restrict executor socket permissions: %w", err)
	}
	// The rename keeps the inode, so this identifies the published socket.
	info, err := os.Lstat(stagingPath)
	if err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("could not stat executor socket: %w", err)
	}
	if err := os.Rename(stagingPath, address); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("could not move executor socket into place: %w", err)
	}
	return &unixListener{UnixListener: lis, path: address, info: info}, nil
}

// unixListener reports and unlinks the final socket path rather than the
// staging path the socket was bound to.
type unixListener struct {
	*net.UnixListener
	path      string
	info      os.FileInfo
	closeOnce sync.Once
}

func (l *unixListener) Addr() net.Addr {
	return &net.UnixAddr{Name: l.path, Net: "unix"}
}

func (l *unixListener) Close() error {
	err := l.UnixListener.Close()
	l.closeOnce.Do(func() {
		// Leave the path alone if another listener has replaced the socket.
		if current, statErr := os.Lstat(l.path); statErr == nil && os.SameFile(current, l.info) {
			_ = os.Remove(l.path)
		}
	})
	return err
}

// Dial connects to the executor's socket.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "unix", address)
}
