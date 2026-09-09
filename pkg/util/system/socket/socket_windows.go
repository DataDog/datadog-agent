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

// IsAvailable returns named pipe availability, as on Windows sockets do not exist.
// The second return value is the dial error, if any.
func IsAvailable(path string, timeout time.Duration) (bool, error) {
	if !checkExists(path) {
		return false, nil
	}

	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return true, err
	}

	conn.Close()

	return true, nil
}

func checkExists(path string) bool {
	// On Windows there's not easy way to check if a path is a named pipe
	_, err := os.Stat(path)
	return err == nil
}
