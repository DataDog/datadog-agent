// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package interp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// normalizeOSError rewrites Windows-specific error messages to their
// POSIX equivalents so that shell output is consistent across platforms.
// On non-Windows systems it is a no-op.
func normalizeOSError(err error) error {
	if runtime.GOOS != "windows" || err == nil {
		return err
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		pathErr.Path = filepath.ToSlash(pathErr.Path)
		pathErr.Err = normalizeInnerError(pathErr.Err)
		return pathErr
	}
	return err
}

// normalizeInnerError maps Windows syscall errors to their POSIX equivalents.
func normalizeInnerError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return syscall.ENOENT // "no such file or directory"
	}
	// Windows returns ERROR_INVALID_FUNCTION (errno 1) / "Incorrect function"
	// when attempting to read a directory as a file.
	if errors.Is(err, syscall.Errno(1)) {
		return errors.New("is a directory")
	}
	return err
}
