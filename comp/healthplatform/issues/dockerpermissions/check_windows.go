// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package dockerpermissions

import (
	"errors"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// checkSocketPermission returns nil if the named pipe at path is reachable
// (or absent). Otherwise it returns the error from stat-ing or dialing the
// pipe; callers can test errors.Is(err, os.ErrPermission) to distinguish a
// genuine access-denied error from a busy pipe or no listener yet, neither
// of which are permission issues.
//
// A restrictive pipe ACL can make os.Stat itself fail with
// ERROR_ACCESS_DENIED, so that error must be checked here rather than
// treated as "pipe doesn't exist" -- otherwise the primary Windows failure
// mode this check exists to catch would never be reported.
func checkSocketPermission(path string, timeout time.Duration) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		return nil // pipe doesn't exist, not a permission problem
	}

	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return err
	}

	conn.Close()
	return nil
}
