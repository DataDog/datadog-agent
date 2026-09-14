// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package socket provides method to check if socket path is available.
package socket

import (
	"net"
	"os"
	"time"
)

// IsAvailable reports whether a socket at path exists. The second return
// value is nil if reachable, or the dial error otherwise (permission denied,
// connection refused, or any other reachability failure).
func IsAvailable(path string, timeout time.Duration) (bool, error) {
	if !checkExists(path) {
		return false, nil
	}

	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return true, err
	}

	conn.Close()
	return true, nil
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
