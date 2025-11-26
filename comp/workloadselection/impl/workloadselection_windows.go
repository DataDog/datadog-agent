// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package workloadselectionimpl

import (
	"os"
	"strings"
)

// isCompilePolicyBinaryAvailable checks if the compile policy binary is available
// on Windows systems
func (c *workloadselectionComponent) isCompilePolicyBinaryAvailable() bool {
	compilePath := getCompilePolicyBinaryPath()
	info, err := os.Stat(compilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			c.log.Warnf("failed to stat APM workload selection compile policy binary: %v", err)
		}
		return false
	}
	// On Windows, executable nature is determined by extension, not permission bits
	// Just check if it's a regular file with .exe extension
	return info.Mode().IsRegular() && strings.HasSuffix(strings.ToLower(compilePath), ".exe")
}
