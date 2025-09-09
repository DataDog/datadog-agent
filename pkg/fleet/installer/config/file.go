// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func isSameFile(file1, file2 string) bool {
	stat1, err := os.Stat(file1)
	if err != nil {
		return false
	}
	stat2, err := os.Stat(file2)
	if err != nil {
		return false
	}
	return os.SameFile(stat1, stat2)
}

// replaceConfigDirectory replaces the contents of two directories.
func replaceConfigDirectory(oldDir, newDir string) (err error) {
	backupPath := filepath.Clean(oldDir) + ".bak"
	err = os.Rename(oldDir, backupPath)
	if err != nil {
		return fmt.Errorf("could not rename old directory: %w", err)
	}
	defer func() {
		if err != nil {
			rollbackErr := os.Rename(backupPath, oldDir)
			if rollbackErr != nil {
				err = fmt.Errorf("%w, rollback error: %w", err, rollbackErr)
			}
		}
	}()
	err = os.Rename(newDir, oldDir)
	if err != nil {
		return fmt.Errorf("could not rename new directory: %w", err)
	}
	err = os.RemoveAll(backupPath)
	if err != nil {
		return fmt.Errorf("could not remove old directory: %w", err)
	}
	return nil
}
