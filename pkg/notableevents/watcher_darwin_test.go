// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"errors"
	"fmt"
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

// TestDarwinWatcherEnqueuesAllRootsAfterDrop verifies a dropped notification
// triggers full reconciliation and reports the typed drop error the collector
// counts apart from other watcher failures.
func TestDarwinWatcherEnqueuesAllRootsAfterDrop(t *testing.T) {
	watcher := newTestDirectoryWatcher()

	watcher.handleEvent("/user-reports/ExampleApp.ips", fseventsDroppedFlags)

	assert.Len(t, watcher.pending, 2)
	assert.Contains(t, watcher.pending, "/system-reports")
	assert.Contains(t, watcher.pending, "/user-reports")

	require.Len(t, watcher.errors, 1, "a drop must be reported so it can be counted")
	var dropped *fseventsDroppedError
	require.ErrorAs(t, <-watcher.errors, &dropped)
	assert.Equal(t, fseventsDroppedFlags, dropped.flags)
}

// TestDarwinWatcherOrdinaryEventReconcilesOnlyItsRoot verifies a normal
// notification is not classified as a drop.
func TestDarwinWatcherOrdinaryEventReconcilesOnlyItsRoot(t *testing.T) {
	watcher := newTestDirectoryWatcher()

	watcher.handleEvent("/user-reports/ExampleApp.ips", 0)

	assert.Equal(t, map[string]struct{}{"/user-reports": {}}, watcher.pending)
	assert.Empty(t, watcher.errors)
}

// newTestDirectoryWatcher builds a watcher with no native stream attached. The
// error channel holds more than any test expects because reportError discards
// on a full channel, which would hide an unexpected extra error behind a
// passing length assertion.
func newTestDirectoryWatcher() *darwinDirectoryWatcher {
	return &darwinDirectoryWatcher{
		watched: map[string]struct{}{
			"/system-reports": {},
			"/user-reports":   {},
		},
		pending: make(map[string]struct{}),
		wake:    make(chan struct{}, 1),
		errors:  make(chan error, 2),
	}
}

// TestFSEventsDroppedErrorIsDistinguishable verifies drops stay identifiable
// once wrapped, so the collector can count them apart from other failures.
func TestFSEventsDroppedErrorIsDistinguishable(t *testing.T) {
	err := fmt.Errorf("watcher: %w", &fseventsDroppedError{flags: fseventsDroppedFlags})

	var dropped *fseventsDroppedError
	require.ErrorAs(t, err, &dropped)
	assert.Equal(t, fseventsDroppedFlags, dropped.flags)
	assert.Contains(t, err.Error(), "FSEvents dropped events")
	assert.NotErrorAs(t, errors.New("unrelated watcher failure"), &dropped)
}
