// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package systemd

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

const journalControlFDDir = "/proc/self/fd"

func (c *Client) dialJournalControl(ctx context.Context, path string) (net.Conn, error) {
	socket, err := c.openJournalControlSocket(path)
	if err != nil {
		return nil, err
	}
	defer socket.Close()
	return dialPinnedJournalControl(ctx, int(socket.Fd()))
}

func (c *Client) openJournalControlSocket(path string) (*os.File, error) {
	socket, err := c.openTargetFileFlags(path, unix.O_PATH|unix.O_NOFOLLOW)
	if err != nil {
		return nil, fmt.Errorf("inspect journal control socket: %w", err)
	}

	info, err := socket.Stat()
	if err != nil {
		_ = socket.Close()
		return nil, fmt.Errorf("inspect journal control socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		_ = socket.Close()
		return nil, fmt.Errorf("journal control endpoint is not a Unix socket")
	}
	return socket, nil
}

func dialPinnedJournalControl(ctx context.Context, fd int) (net.Conn, error) {
	endpoint := filepath.Join(journalControlFDDir, strconv.Itoa(fd))
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", endpoint)
	if err != nil {
		return nil, contextIOError(ctx, "connect to pinned journal control socket", err)
	}
	return conn, nil
}
