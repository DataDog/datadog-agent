// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package persistentcache

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func replaceFile(sourcePath, targetPath string) error {
	if err := os.Rename(sourcePath, targetPath); err != nil {
		return err
	}
	// Persist the directory entry as well as the synchronized file contents.
	dir, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && (runtime.GOOS != "aix" || (!errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.EBADF))) {
		return err
	}
	return nil
}
