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

func TestSchedulerTriggerSweepsWithoutWaitingForTheInterval(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	// The range sweeps once on add, then is not due again for an hour.
	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/26")))
	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, s.triggerNow("ad-1"))

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 2 }, 5*time.Second, 10*time.Millisecond,
		"a triggered range sweeps now rather than at its next scheduled run")
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
	s.start(context.Background())
	defer s.stop()

	// The first cycle resumes, so it scans only the last two chunks.
	require.NoError(t, s.set(cfg))
	require.Eventually(t, func() bool { return len(scanner.recorded()) == 2 }, 5*time.Second, 10*time.Millisecond)
	require.False(t, scannedFirstAddress(scanner), "a resumed cycle skips the chunks already scanned")

	require.NoError(t, s.triggerNow("ad-1"))

	require.Eventually(t, func() bool { return scannedFirstAddress(scanner) }, 5*time.Second, 10*time.Millisecond,
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
	s.start(context.Background())
	defer s.stop()

	err := s.triggerNow("ad-missing")

	require.ErrorIs(t, err, errUnknownRange)
	assert.Contains(t, err.Error(), "ad-missing")
}

func TestSchedulerTriggerRejectsARangeThatIsAlreadySweeping(t *testing.T) {
	scanner := newBlockingScanner()
	s, _, _ := newTestScheduler(t, &scanner.recordingScanner, 10)
	s.start(context.Background())
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
