// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package auditorimpl

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func replaceRegistryFile(sourcePath, targetPath string) error {
	if err := os.Rename(sourcePath, targetPath); err != nil {
		return err
	}

	// Persist the directory entry changed by the rename. The temporary file is
	// already synchronized, but its new name is not durable until its parent
	// directory is synchronized as well.
	directory, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return err
	}
	defer directory.Close()

	if err := directory.Sync(); err != nil {
		// AIX does not support fsync on directory descriptors: the fsync and
		// fsync_range subroutines only accept regular files, so the call fails
		// with EINVAL (Go's syscall.Fsync uses fsync_range on AIX) or EBADF.
		// Treat the directory sync as best-effort on AIX — JFS2's metadata
		// journal keeps the rename atomic across crashes — and surface the
		// error on every other platform.
		if runtime.GOOS != "aix" || (!errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.EBADF)) {
			return err
		}
	}
	return nil
}
