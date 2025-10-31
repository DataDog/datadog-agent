// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// copyDirectory copies a directory from source to target.
// It preserves the directory structure and file permissions.
func copyDirectory(ctx context.Context, sourcePath, targetPath string) error {
	cmd := telemetry.CommandContext(ctx, "cp", "-a", sourcePath, targetPath)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to copy directory: %w", err)
	}
	existingStablePath := filepath.Join(sourcePath, "managed", "datadog-agent", "stable")
	// 1. Check if stable is a symlink or not
	info, err := os.Lstat(existingStablePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// If it's not a symlink, we can return early
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	// 2. Recursively delete targetPath/
	// RemoveAll removes symlinks but not the content they point to as it uses os.Remove first
	err = os.RemoveAll(filepath.Join(targetPath, "managed"))
	if err != nil {
		return fmt.Errorf("failed to remove managed directory: %w", err)
	}
	return nil
}
