// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// CheckSocketAvailable returns named pipe availability
// as on Windows, sockets do not exist
func CheckSocketAvailable(path string, timeout time.Duration) (bool, bool) {
	if !checkSocketExists(path) {
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

func checkSocketExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return false
	}

	// On Windows there's not easy to check if a path is a named pipe
	return true
}
