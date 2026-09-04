// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package dockerpermissions

import (
	"net"
	"os"
	"time"
)

// checkSocketPermission returns nil if the Unix socket at path is reachable
// (or absent). Otherwise it returns the error from stat-ing or dialing the
// socket; callers can test errors.Is(err, os.ErrPermission) to distinguish a
// genuine permission problem from a busy socket, connection refused, or
// timeout, none of which are permission issues.
func checkSocketPermission(path string, timeout time.Duration) error {
	f, err := os.Stat(path)
	if err != nil {
		return nil // socket doesn't exist, not a permission problem
	}
	if f.Mode()&os.ModeSocket == 0 {
		return nil
	}

	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return err
	}

	conn.Close()
	return nil
}
