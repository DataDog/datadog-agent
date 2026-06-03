// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
)

// Open provides a fallback for non-Linux platforms where the privileged logs module is not available.
// RejectSymlinks degrades to a plain open here: the process_log provider only produces
// sources from /proc/<pid>/fd, which does not exist outside Linux, so symlink-rejection
// is Linux-only by nature and there is no attack surface to defend on other platforms.
func Open(path string, _ common.SymlinkPolicy) (*os.File, error) {
	return os.Open(path)
}

// OpenPrivileged is not supported on non-Linux platforms.
func OpenPrivileged(_ string, _ string, _ common.SymlinkPolicy) (*os.File, error) {
	return nil, os.ErrUnsupported
}

// OpenNoFollow provides a fallback for non-Linux platforms.
// On non-Linux platforms this is equivalent to Open with FollowSymlinks.
func OpenNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
