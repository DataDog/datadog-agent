// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package socket provides method to check if socket path is available.
package socket

import (
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// IsAvailable returns named pipe availability
// as on Windows, sockets do not exist
func IsAvailable(path string, timeout time.Duration) (bool, bool) {
	if !checkExists(path) {
		return false, false
	}

	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return true, false
	}

	if conn != nil {
		conn.Close()
	}

	return true, true
}

func checkExists(path string) bool {
	// On Windows there's not easy way to check if a path is a named pipe
	_, err := os.Stat(path)
	return err == nil
}
