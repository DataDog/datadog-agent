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
)

// Open provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Open(path string) (*os.File, error) {
	return os.Open(path)
}

// OpenNoFollow is not supported on non-Linux platforms.
func OpenNoFollow(path string) (*os.File, error) {
	return nil, fmt.Errorf("privileged-logs client: no-follow open is not supported on non-Linux platforms: %s", path)
}

// OpenPrivileged is not supported on non-Linux platforms.
func OpenPrivileged(_, _ string) (*os.File, error) {
	return nil, os.ErrUnsupported
}

// OpenPrivilegedNoFollow is not supported on non-Linux platforms.
func OpenPrivilegedNoFollow(_, _ string) (*os.File, error) {
	return nil, os.ErrUnsupported
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
