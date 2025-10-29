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
	// 1. Eval stable symlink in sourcePath
	stableSymlinkPath, err := filepath.EvalSymlinks(
		filepath.Join(sourcePath, "managed", "datadog-agent", "stable"),
	)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to eval stable symlink: %w", err)
	}
	// 2. Get the version from the stable symlink
	stablePath := filepath.Base(stableSymlinkPath)
	// 3. Delete stable and experiment symlinks in targetPath
	err = os.Remove(filepath.Join(targetPath, "managed", "datadog-agent", "stable"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stable symlink: %w", err)
	}
	err = os.Remove(filepath.Join(targetPath, "managed", "datadog-agent", "experiment"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove experiment symlink: %w", err)
	}
	// 4. Rename targetPath/managed/datadog-agent/<version> to targetPath/managed/datadog-agent/stable
	err = os.Rename(
		filepath.Join(targetPath, "managed", "datadog-agent", stablePath),
		filepath.Join(targetPath, "managed", "datadog-agent", "stable"),
	)
	if err != nil {
		return fmt.Errorf("failed to rename stable symlink: %w", err)
	}
	return nil
}
