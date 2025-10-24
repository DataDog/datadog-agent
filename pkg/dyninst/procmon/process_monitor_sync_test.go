// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	containerutils "github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestProcessMonitorSyncBlockingBehaviour(t *testing.T) {
	t.Run("close unblocks waiting syncs", func(t *testing.T) {
		root := t.TempDir()
		// It needs to be +3 because the one that is currently being analyzed is
		// not in the queue yet, and then the one we would block on adding is
		// also not in the queue yet, so we'll be able to add n+2 and then block
		// on the n+3rd.
		populateProcfs(t, root, queueBackpressureLimit+3)

		pm, analyze := newProcessMonitorForTest(t, root)

		sync1 := startSync(pm)
		assertSyncBlocked(t, sync1)
		// The first sync is now holding the Sync lock and has issued an
		// analysis request.
		_ = waitForAnalysisRequest(t, analyze)

		sync2 := startSync(pm)
		// The second sync should block because Sync serializes access while the
		// queue backpressure prevents the first sync from completing.
		assertSyncBlocked(t, sync2)
		assertSyncBlocked(t, sync1)

		// Closing the monitor flips isClosed and wakes any waiters, so both syncs
		// should now unwind cleanly.
		pm.Close()

		require.NoError(t, waitForSync(t, sync1))
		require.NoError(t, waitForSync(t, sync2))
	})

	t.Run("first sync finishing unblocks next", func(t *testing.T) {
		root := t.TempDir()
		populateProcfs(t, root, queueBackpressureLimit+3)

		pm, analyze := newProcessMonitorForTest(t, root)

		sync1 := startSync(pm)
		assertSyncBlocked(t, sync1)
		firstPID := waitForAnalysisRequest(t, analyze)

		sync2 := startSync(pm)
		// With sync1 still mid-flight, sync2 must block until the queue drains.
		assertSyncBlocked(t, sync2)

		pending := []uint32{firstPID}
		for sync1 != nil {
			for len(pending) > 0 {
				// Complete queued analyses one at a time; sync1 should eventually
				// finish once its outstanding requests are acknowledged.
				pid := pending[0]
				pending = pending[:0]
				deliverAnalysisResult(pm, pid)
			}

			select {
			case pid := <-analyze:
				pending = append(pending, pid)
				require.Len(t, pending, 1)
			case err := <-sync1:
				require.NoError(t, err)
				sync1 = nil
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout while draining analysis queue")
			}
		}

		// With the queue empty and all the processes marked as interesting,
		// sync2 should now complete successfully and immediately.
		require.NoError(t, waitForSync(t, sync2))
	})
}

func newProcessMonitorForTest(
	t *testing.T, procfsRoot string,
) (*ProcessMonitor, <-chan uint32) {
	t.Helper()
	pm, analyze := newProcessMonitor(
		testHandler{}, procfsRoot, testResolver{}, noopAnalyzer{},
	)
	t.Cleanup(pm.Close)
	return pm, analyze
}

func populateProcfs(t *testing.T, root string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		pid := 10000 + i
		dir := filepath.Join(root, strconv.Itoa(pid))
		require.NoError(t, os.Mkdir(dir, 0o755))
	}
}

func startSync(pm *ProcessMonitor) <-chan error {
	done := make(chan error, 1)
	go func() { done <- pm.Sync() }()
	return done
}

func waitForAnalysisRequest(t *testing.T, ch <-chan uint32) uint32 {
	t.Helper()
	select {
	case pid, ok := <-ch:
		require.True(t, ok, "analysis request channel closed unexpectedly")
		return pid
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for analysis request")
		return 0
	}
}

func assertSyncBlocked(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		t.Fatalf("expected sync to block, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func waitForSync(t *testing.T, ch <-chan error) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for sync")
		return nil
	}
}

func deliverAnalysisResult(pm *ProcessMonitor, pid uint32) {
	handleEvent(pm, (*state).handleAnalysisResult, analysisResult{
		pid:             pid,
		processAnalysis: processAnalysis{interesting: true},
	})
}

type testHandler struct{}

func (testHandler) HandleUpdate(ProcessesUpdate) {}

type testResolver struct{}

func (testResolver) GetContainerContext(uint32) (containerutils.ContainerID, model.CGroupContext, string, error) {
	return "", model.CGroupContext{}, "", nil
}

type noopAnalyzer struct{}

func (noopAnalyzer) checkFileKeyCache(process.FileKey) (bool, bool) {
	return false, false
}
func (noopAnalyzer) isInteresting(*os.File, process.FileKey) (bool, error) {
	return true, nil
}
