// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	discoverytypes "github.com/DataDog/datadog-agent/pkg/networkdevices/discovery"
)

// cycleWaits signals once per loop iteration, when the scheduler starts waiting
// for the next one. The sweep has finished and the range is idle by then, which
// recording a scan call does not tell us: the scanner records on entry.
// Install it before the scheduler starts.
func cycleWaits(s *scheduler) <-chan struct{} {
	waits := make(chan struct{}, 8)
	s.newTimer = func(d time.Duration) (<-chan time.Time, func()) {
		waits <- struct{}{}
		timer := time.NewTimer(d)
		return timer.C, func() { timer.Stop() }
	}
	return waits
}

func awaitCycle(t *testing.T, waits <-chan struct{}) {
	t.Helper()
	select {
	case <-waits:
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduler never finished a cycle")
	}
}

func TestSchedulerTriggerSweepsWithoutWaitingForTheInterval(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	waits := cycleWaits(s)
	s.start(t.Context())
	defer s.stop()

	// The range sweeps once on add, then is not due again for an hour.
	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/26")))
	awaitCycle(t, waits)
	require.Len(t, scanner.recorded(), 1)

	require.NoError(t, s.triggerNow("ad-1"))
	awaitCycle(t, waits)

	assert.Len(t, scanner.recorded(), 2,
		"a triggered range sweeps now rather than at its next scheduled run")
}

func TestSchedulerTriggerSweepsThroughAnUnexpiredCursor(t *testing.T) {
	scanner := answerAll()
	store := testSchedulerStore()
	cursors := newMemCursorStore()
	sw := newTestSweeper(t, scanner, &recordingReporter{}, cursors, 10)

	// A cursor whose cycle completed moments ago: dueIn would defer the sweep
	// for the whole interval, and clearing it is no longer what makes it due.
	cfg := testRangeConfig("ad-1", "10.0.0.0/26")
	opts, _ := cfg.Probes.resolve(store)
	cursors.Save("ad-1", cursorState{
		RunID:        "run-completed",
		NextChunk:    1,
		Scanned:      64,
		StartedAtMs:  1700000000000,
		EndedAtMs:    1700000000000,
		ConfigDigest: rangeDigest(cfg, opts.Fingerprints()),
	})

	s := newScheduler(sw, logmock.New(t), schedulerOptions{
		Workers:      10,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
		Credentials:  store,
	})
	waits := cycleWaits(s)
	s.start(t.Context())
	defer s.stop()

	require.NoError(t, s.set(cfg))
	awaitCycle(t, waits)
	require.Empty(t, scanner.recorded(), "the range is not due for another hour")

	require.NoError(t, s.triggerNow("ad-1"))
	awaitCycle(t, waits)

	assert.Len(t, scanner.recorded(), 1,
		"a requested sweep runs even though the cursor says the range is not due")
}

func TestSchedulerTriggerRescansTheChunksAResumeWouldSkip(t *testing.T) {
	scanner := answerAll()
	store := testSchedulerStore()
	cursors := newMemCursorStore()
	sw := newTestSweeper(t, scanner, &recordingReporter{}, cursors, 10)

	// A /24 is four chunks, so a cursor can sit half way through one.
	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	opts, _ := cfg.Probes.resolve(store)
	cursors.Save("ad-1", cursorState{
		RunID:        "run-interrupted",
		NextChunk:    2,
		Scanned:      128,
		StartedAtMs:  1700000000000,
		ConfigDigest: rangeDigest(cfg, opts.Fingerprints()),
	})

	s := newScheduler(sw, logmock.New(t), schedulerOptions{
		Workers:      10,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
		Credentials:  store,
	})
	waits := cycleWaits(s)
	s.start(t.Context())
	defer s.stop()

	// The first cycle resumes, so it scans only the last two chunks.
	require.NoError(t, s.set(cfg))
	awaitCycle(t, waits)
	require.Len(t, scanner.recorded(), 2)
	require.False(t, scannedFirstAddress(scanner), "a resumed cycle skips the chunks already scanned")

	require.NoError(t, s.triggerNow("ad-1"))
	awaitCycle(t, waits)

	assert.True(t, scannedFirstAddress(scanner),
		"the triggered sweep starts from the first chunk instead of resuming")
}

// scannedFirstAddress reports whether any scan covered the range's first address.
func scannedFirstAddress(scanner *recordingScanner) bool {
	for _, call := range scanner.recorded() {
		for _, target := range call.Targets {
			if target == "10.0.0.0" {
				return true
			}
		}
	}
	return false
}

func TestSchedulerTriggerRejectsAnUnknownRange(t *testing.T) {
	s, _, _ := newTestScheduler(t, answerAll(), 10)
	s.start(t.Context())
	defer s.stop()

	err := s.triggerNow("ad-missing")

	require.ErrorIs(t, err, errUnknownRange)
	assert.Contains(t, err.Error(), "ad-missing")
}

func TestSchedulerTriggerRejectsARangeThatIsAlreadySweeping(t *testing.T) {
	scanner := newBlockingScanner()
	s, _, _ := newTestScheduler(t, &scanner.recordingScanner, 10)
	s.start(t.Context())
	defer s.stop()
	defer close(scanner.release)

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/26")))
	<-scanner.entered

	require.ErrorIs(t, s.triggerNow("ad-1"), errAlreadySweeping,
		"the caller is told to retry rather than queueing a second sweep")
}

func TestTriggerRejectsAnEmptyAutodiscoveryID(t *testing.T) {
	d := &ndmDiscovery{enabled: true}

	require.ErrorIs(t, d.Trigger(""), discoverytypes.ErrInvalidRequest)
}

func TestTriggerRejectsADisabledComponent(t *testing.T) {
	d := &ndmDiscovery{}

	require.ErrorIs(t, d.Trigger("ad-1"), errDisabled)
}
