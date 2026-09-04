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

// checkSocketPermission reports whether the named pipe at path exists and,
// if so, whether dialing it failed specifically with an access-denied
// error. A busy pipe or no listener yet is not a permission problem and
// reports permissionDenied=false.
func checkSocketPermission(path string, timeout time.Duration) (exists bool, permissionDenied bool) {
	if _, err := os.Stat(path); err != nil {
		return false, false
	}

	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return true, errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}

	conn.Close()
	return true, false
}
