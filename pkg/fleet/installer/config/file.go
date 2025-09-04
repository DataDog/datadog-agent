// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// copyDirectory copies a directory from source to target.
// It preserves the directory structure and file permissions.
func copyDirectory(sourcePath, targetPath string) error {
	return filepath.Walk(sourcePath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}

		if path == sourcePath {
			// Skip root
			return nil
		}

		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		targetFilePath := filepath.Join(targetPath, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetFilePath, info.Mode())
		}

		return copyFileWithPermissions(path, targetFilePath, info)
	})
}

// swapConfigDirectories swaps the contents of two directories.
func swapConfigDirectories(oldDir, newDir string) (err error) {
	err = os.Rename(oldDir, filepath.Join(oldDir, ".bak"))
	if err != nil {
		return fmt.Errorf("could not rename old directory: %w", err)
	}
	defer func() {
		if err != nil {
			rollbackErr := os.Rename(filepath.Join(oldDir, ".bak"), oldDir)
			if rollbackErr != nil {
				err = fmt.Errorf("%w, rollback error: %w", err, rollbackErr)
			}
		}
	}()
	err = os.Rename(newDir, oldDir)
	if err != nil {
		return fmt.Errorf("could not rename new directory: %w", err)
	}
	err = os.RemoveAll(filepath.Join(oldDir, ".bak"))
	if err != nil {
		return fmt.Errorf("could not remove old directory: %w", err)
	}
	return nil
}
