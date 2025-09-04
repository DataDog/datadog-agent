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
