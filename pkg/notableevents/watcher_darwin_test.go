// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDarwinWatcherMapsDescendantEvents verifies changed descendants reconcile their watch root.
func TestDarwinWatcherMapsDescendantEvents(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "Users", "alice", "Library", "Logs", "DiagnosticReports")
	watcher := &darwinDirectoryWatcher{watched: map[string]struct{}{root: {}}}

	mapped, found := watcher.watchedDirectoryForPath(filepath.Join(root, "ExampleApp.ips"))
	require.True(t, found)
	assert.Equal(t, root, mapped)
	_, found = watcher.watchedDirectoryForPath(root + "-other")
	assert.False(t, found)
}

// TestDarwinWatcherCoalescesPendingDirectories verifies duplicate root notifications are collapsed.
func TestDarwinWatcherCoalescesPendingDirectories(t *testing.T) {
	watcher := &darwinDirectoryWatcher{
		pending: make(map[string]struct{}),
		wake:    make(chan struct{}, 1),
	}
	watcher.enqueue("/reports")
	watcher.enqueue("/reports")

	path, found := watcher.popPending()
	require.True(t, found)
	assert.Equal(t, "/reports", path)
	_, found = watcher.popPending()
	assert.False(t, found)
	assert.Len(t, watcher.wake, 1)
}

// TestDarwinWatcherEnqueuesAllRootsAfterDrop verifies dropped events trigger full reconciliation.
func TestDarwinWatcherEnqueuesAllRootsAfterDrop(t *testing.T) {
	watcher := &darwinDirectoryWatcher{
		watched: map[string]struct{}{
			"/system-reports": {},
			"/user-reports":   {},
		},
		pending: make(map[string]struct{}),
		wake:    make(chan struct{}, 1),
	}
	watcher.enqueueAllWatched()

	assert.Len(t, watcher.pending, 2)
	assert.Contains(t, watcher.pending, "/system-reports")
	assert.Contains(t, watcher.pending, "/user-reports")
}
