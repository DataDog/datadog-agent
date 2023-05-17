// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package util

import (
	"os"
	"path/filepath"
	"strconv"
)

// GetRootNSPID returns the current PID from the root namespace
func GetRootNSPID() (int, error) {
	pidPath := filepath.Join(GetProcRoot(), "self")
	pidStr, err := os.Readlink(pidPath)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(pidStr)
}
