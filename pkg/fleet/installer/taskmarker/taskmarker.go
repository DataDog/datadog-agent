// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taskmarker lets the daemon tell a detached installer process that it has
// recorded the result of a remote task, so that process can safely restart the
// services hosting the daemon.
package taskmarker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const markerPrefix = "task-recorded-"

// PollInterval is how often Wait checks for the marker.
var PollInterval = 500 * time.Millisecond

var runDir = func() string { return paths.RunPath }

// Path returns the marker file for taskID.
func Path(taskID string) string {
	return filepath.Join(runDir(), markerPrefix+taskID)
}

// Write records that the daemon has persisted the result of taskID.
func Write(taskID string) error {
	if taskID == "" {
		return errors.New("empty task id")
	}
	if err := os.MkdirAll(runDir(), 0755); err != nil {
		return fmt.Errorf("could not create run directory: %w", err)
	}
	return os.WriteFile(Path(taskID), nil, 0644)
}

// Wait blocks until the marker for taskID exists, ctx is done, or timeout elapses, and
// removes the marker once seen. It reports whether the marker was seen.
func Wait(ctx context.Context, taskID string, timeout time.Duration) bool {
	if taskID == "" {
		return false
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(Path(taskID)); err == nil {
			_ = os.Remove(Path(taskID))
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

// RemoveStale deletes markers older than maxAge, left behind when no process waited for them.
func RemoveStale(maxAge time.Duration) error {
	entries, err := os.ReadDir(runDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), markerPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < maxAge {
			continue
		}
		if err := os.Remove(filepath.Join(runDir(), entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
