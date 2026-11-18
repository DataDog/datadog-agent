// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskLedgerAcceptsEachTaskOnce(t *testing.T) {
	ledger := newTaskLedger("", nil)
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, ledger.accept("task-1", expiresAt))
	require.ErrorContains(t, ledger.accept("task-1", expiresAt), errTaskAlreadyAccepted.Error())
	require.NoError(t, ledger.accept("task-2", expiresAt))
}

func TestTaskLedgerRefusesEmptyTaskID(t *testing.T) {
	ledger := newTaskLedger("", nil)

	require.ErrorContains(t, ledger.accept("", time.Now().Add(time.Hour)), "task id is missing")
}

func TestTaskLedgerPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, newTaskLedger(dir, nil).accept("task/../1", expiresAt))

	require.ErrorContains(t, newTaskLedger(dir, nil).accept("task/../1", expiresAt), errTaskAlreadyAccepted.Error())
	require.NoError(t, newTaskLedger(dir, nil).accept("task/../2", expiresAt))
}

func TestTaskLedgerAcceptsTaskOnceAcrossConcurrentInstances(t *testing.T) {
	// Each instance prunes on its first accept, racing the other instances' writes;
	// repeat with fresh instances to hit that window.
	for round := 0; round < 50; round++ {
		dir := t.TempDir()
		expiresAt := time.Now().Add(time.Hour)
		ledgers := make([]*taskLedger, 4)
		for i := range ledgers {
			ledgers[i] = newTaskLedger(dir, nil)
		}

		var accepted atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(ledger *taskLedger) {
				defer wg.Done()
				if ledger.accept("task-1", expiresAt) == nil {
					accepted.Add(1)
				}
			}(ledgers[i%len(ledgers)])
		}
		wg.Wait()

		require.Equal(t, int32(1), accepted.Load(), "round %d", round)
	}
}

func TestTaskLedgerPrunesOnlyStalePendingMarkers(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	clk := func() time.Time { return now }
	pending := filepath.Join(dir, pendingMarkerPrefix+"1")
	require.NoError(t, os.WriteFile(pending, nil, 0o600))
	require.NoError(t, os.Chtimes(pending, now, now))

	require.NoError(t, newTaskLedger(dir, clk).accept("task-1", now.Add(time.Hour)))
	assert.FileExists(t, pending)

	now = now.Add(taskLedgerPruneInterval)
	require.NoError(t, newTaskLedger(dir, clk).accept("task-2", now.Add(time.Hour)))
	assert.NoFileExists(t, pending)
}

func TestTaskLedgerPrunesExpiredEntries(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	clk := func() time.Time { return now }
	ledger := newTaskLedger(dir, clk)

	require.NoError(t, ledger.accept("short-lived", now.Add(time.Minute)))
	require.NoError(t, ledger.accept("long-lived", now.Add(time.Hour)))

	now = now.Add(taskLedgerPruneInterval)
	require.NoError(t, ledger.accept("trigger-prune", now.Add(time.Hour)))

	_, err := os.Stat(filepath.Join(dir, markerName("short-lived")))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, markerName("long-lived")))
	assert.NoError(t, err)
	require.NoError(t, ledger.accept("short-lived", now.Add(time.Minute)))
	require.ErrorContains(t, ledger.accept("long-lived", now.Add(time.Hour)), errTaskAlreadyAccepted.Error())
	require.ErrorContains(t, newTaskLedger(dir, clk).accept("long-lived", now.Add(time.Hour)), errTaskAlreadyAccepted.Error())
}

func TestTaskLedgerFallsBackToMemoryWhenDirIsUnusable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	ledger := newTaskLedger(file, nil)

	assert.Empty(t, ledger.dir)
	expiresAt := time.Now().Add(time.Hour)
	require.NoError(t, ledger.accept("task-1", expiresAt))
	require.ErrorContains(t, ledger.accept("task-1", expiresAt), errTaskAlreadyAccepted.Error())
}
