// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taskmarker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRunPath(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), "run")
	previousDir, previousPoll := runDir, PollInterval
	runDir = func() string { return dir }
	PollInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		runDir = previousDir
		PollInterval = previousPoll
	})
	return dir
}

func TestWaitReturnsOnceMarkerIsWritten(t *testing.T) {
	withRunPath(t)
	written := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		written <- Write("task-1")
	}()

	assert.True(t, Wait(context.Background(), "task-1", 5*time.Second))
	require.NoError(t, <-written)
	_, err := os.Stat(Path("task-1"))
	assert.True(t, os.IsNotExist(err), "Wait should remove the marker it consumed")
}

func TestWaitSeesMarkerWrittenBeforehand(t *testing.T) {
	withRunPath(t)
	require.NoError(t, Write("task-1"))

	assert.True(t, Wait(context.Background(), "task-1", time.Second))
}

func TestWaitTimesOutWithoutMarker(t *testing.T) {
	withRunPath(t)

	start := time.Now()
	assert.False(t, Wait(context.Background(), "task-1", 50*time.Millisecond))
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestWaitIgnoresOtherTasks(t *testing.T) {
	withRunPath(t)
	require.NoError(t, Write("task-2"))

	assert.False(t, Wait(context.Background(), "task-1", 50*time.Millisecond))
	_, err := os.Stat(Path("task-2"))
	assert.NoError(t, err, "another task's marker must be left alone")
}

func TestWaitStopsOnContextCancel(t *testing.T) {
	withRunPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	start := time.Now()
	assert.False(t, Wait(ctx, "task-1", time.Minute))
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestWaitWithoutTaskIDReturnsImmediately(t *testing.T) {
	withRunPath(t)

	start := time.Now()
	assert.False(t, Wait(context.Background(), "", time.Minute))
	assert.Less(t, time.Since(start), time.Second)
}

func TestWriteRejectsEmptyTaskID(t *testing.T) {
	withRunPath(t)
	assert.Error(t, Write(""))
}

func TestRemoveStaleKeepsRecentMarkersAndOtherFiles(t *testing.T) {
	runPath := withRunPath(t)
	require.NoError(t, Write("old"))
	require.NoError(t, Write("recent"))
	require.NoError(t, os.WriteFile(filepath.Join(runPath, "installer_tasks.db"), nil, 0644))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(Path("old"), old, old))

	require.NoError(t, RemoveStale(time.Hour))

	_, err := os.Stat(Path("old"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(Path("recent"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(runPath, "installer_tasks.db"))
	assert.NoError(t, err)
}

func TestRemoveStaleWithoutRunDirectory(t *testing.T) {
	withRunPath(t)
	assert.NoError(t, RemoveStale(time.Hour))
}
