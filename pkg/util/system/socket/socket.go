// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package socket provides method to check if socket path is available.
package socket

import (
	"errors"
	"net"
	"os"
	"time"
)

// IsAvailable returns if a socket at path is available
// first boolean returns if socket path exists
// second boolean returns if socket is reachable
func IsAvailable(path string, timeout time.Duration) (bool, bool) {
	if !checkExists(path) {
		return false, false
	}

	// Assuming socket file exists (bind() done)
	// -> but we don't have permission: permission denied
	// -> but no process associated to socket anymore: connection refused
	// -> but process did not call listen(): connection refused
	// -> but process does not call accept(): no error
	// We'll consider socket available in all cases except if permission is denied
	// as if a path exists and we do have access, it's likely that a process will re-use it later.
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil && errors.Is(err, os.ErrPermission) {
		return true, false
	}

	if conn != nil {
		conn.Close()
	}

	return true, true
}

func checkExists(path string) bool {
	f, err := os.Stat(path)
	if err != nil {
		return false
	}

	if f.Mode()&os.ModeSocket != 0 {
		return true
	}

	return false
}
