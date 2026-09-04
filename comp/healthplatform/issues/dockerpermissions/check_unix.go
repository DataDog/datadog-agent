// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package dockerpermissions

import (
	"errors"
	"net"
	"os"
	"time"
)

// checkSocketPermission reports whether the Unix socket at path exists and,
// if so, whether dialing it failed specifically with a permission-denied
// error. A busy socket, connection refused, or timeout is not a permission
// problem and reports permissionDenied=false.
func checkSocketPermission(path string, timeout time.Duration) (exists bool, permissionDenied bool) {
	f, err := os.Stat(path)
	if err != nil || f.Mode()&os.ModeSocket == 0 {
		return false, false
	}

	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return true, errors.Is(err, os.ErrPermission)
	}

	conn.Close()
	return true, false
}
