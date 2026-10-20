// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package systemd

import (
	"context"
	"fmt"
	"net"
	"os"
)

func (c *Client) dialJournalControl(ctx context.Context, path string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := c.lstatTargetPath(path)
	if err != nil {
		return nil, fmt.Errorf("inspect journal control socket: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("journal control endpoint is not a Unix socket")
	}
	return nil, fmt.Errorf("journal control connections require Linux")
}
