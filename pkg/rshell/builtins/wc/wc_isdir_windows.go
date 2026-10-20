// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package wc

import (
	"errors"
	"syscall"
)

// errnoERROR_INVALID_FUNCTION is the Windows errno for ERROR_INVALID_FUNCTION.
// Go's syscall package does not export this constant, so we define it here.
const errnoERROR_INVALID_FUNCTION = syscall.Errno(1)

// isErrIsDir reports whether err wraps the Windows equivalent of EISDIR.
// On Windows, reading a directory handle returns ERROR_INVALID_FUNCTION (errno 1).
func isErrIsDir(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == errnoERROR_INVALID_FUNCTION
	}
	return false
}
