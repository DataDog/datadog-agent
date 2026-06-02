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
	switch policy {
	case common.FollowSymlinks, common.RejectSymlinks:
		return os.Open(path)
	default:
		return nil, fmt.Errorf("privileged-logs client: invalid SymlinkPolicy %d; must be FollowSymlinks or RejectSymlinks", policy)
	}
}

// OpenPrivileged is not supported on non-Linux platforms.
func OpenPrivileged(_ string, _ string, _ common.SymlinkPolicy) (*os.File, error) {
	return nil, os.ErrUnsupported
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
