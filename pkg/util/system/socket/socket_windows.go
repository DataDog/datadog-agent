// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package socket provides method to check if socket path is available.
package socket

import (
	"errors"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// IsAvailable returns named pipe availability, as on Windows sockets do not exist.
// The second return value is the dial error, if any.
func IsAvailable(path string, timeout time.Duration) (bool, error) {
	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return true, err
	}

	conn.Close()

	return true, nil
}
