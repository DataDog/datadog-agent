// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
)

// Open provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Open(path string, policy common.SymlinkPolicy) (*os.File, error) {
	if policy != common.FollowSymlinks {
		return nil, fmt.Errorf("privileged-logs client: invalid SymlinkPolicy %d; must be FollowSymlinks on non-Linux platforms", policy)
	}

	return os.Open(path)
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
