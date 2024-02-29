// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tools contains tooling required by the updater.
package tools

import (
	"github.com/shirou/gopsutil/v3/disk"
)

// CheckAvailableDiskSpace checks if the given path has enough free space to store the required bytes
// This will check the underlying partition of the given path.
func CheckAvailableDiskSpace(path string, requiredBytes uint64) (bool, error) {
	s, err := disk.Usage(path)
	if err != nil {
		return false, err
	}
	return s.Free >= requiredBytes, nil
}
