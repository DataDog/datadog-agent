// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"errors"
	"os"
)

// Open provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Open(path string) (*os.File, error) {
	return os.Open(path)
}

// OpenNoFollow is not supported on non-Linux platforms. Symlink rejection is
// only meaningful for process_log-discovered paths, and process_log discovery
// (based on /proc/<pid>/fd) is Linux-only, so there is no caller on macOS or
// Windows. Return ErrUnsupported rather than silently falling back to a plain
// open, which would give callers the false impression that symlink-swap
// protection is active.
func OpenNoFollow(path string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}

// OpenPrivileged is not supported on non-Linux platforms.
func OpenPrivileged(_, _ string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}

// OpenPrivilegedNoFollow is not supported on non-Linux platforms.
func OpenPrivilegedNoFollow(_, _ string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
