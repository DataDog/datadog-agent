// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package garbagecollect contains shared fleet installer cleanup logic.
package garbagecollect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Run removes unused packages and old temporary files.
func Run(ctx context.Context, repositories *repository.Repositories, rootTmpDir string) error {
	err := repositories.Cleanup(ctx)
	if err != nil {
		return fmt.Errorf("could not cleanup packages: %w", err)
	}
	err = cleanupTmpDirectory(rootTmpDir)
	if err != nil {
		return fmt.Errorf("could not cleanup tmp directory: %w", err)
	}
	return nil
}

// cleanupTmpDirectory removes files and directories in rootTmpDir that are older than 24 hours.
func cleanupTmpDirectory(rootTmpDir string) error {
	if _, err := os.Stat(rootTmpDir); os.IsNotExist(err) {
		return nil
	}

	cutoffTime := time.Now().Add(-24 * time.Hour)
	entries, err := os.ReadDir(rootTmpDir)
	if err != nil {
		return fmt.Errorf("could not read tmp directory: %w", err)
	}

	var cleanupErrors []string
	for _, entry := range entries {
		entryPath := filepath.Join(rootTmpDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			log.Warnf("Could not get info for %s: %v", entryPath, err)
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			log.Debugf("Removing old tmp file/directory: %s (modified: %v)", entryPath, info.ModTime())

			err := os.RemoveAll(entryPath)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("failed to remove %s: %v", entryPath, err))
				log.Warnf("Could not remove old tmp file/directory %s: %v", entryPath, err)
			} else {
				log.Debugf("Successfully removed old tmp file/directory: %s", entryPath)
			}
		}
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("tmp directory cleanup completed with errors: %s", strings.Join(cleanupErrors, "; "))
	}

	return nil
}
