// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package auditorimpl

import (
	"os"
	"path/filepath"
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
	return directory.Sync()
}
