// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

func testRangeConfig(id, cidr string) rangeConfig {
	return rangeConfig{
		AutodiscoveryID: id,
		Namespace:       "default",
		CIDR:            cidr,
		IntervalSec:     3600,
		Probes: probeParams{SNMP: &snmpParams{
			Port:          161,
			Timeout:       2 * time.Second,
			Retries:       1,
			CredentialIDs: []string{"cred-a"},
		}},
	}
}

func testSchedulerStore() *stubCredentialStore {
	s := v2cStore()
	s.creds["cred-a"] = s.creds["cred-1"]
	return s
}

func newTestScheduler(t *testing.T, scanner *recordingScanner, workers int64) (*scheduler, *recordingReporter, *stubCredentialStore) {
	t.Helper()
	reporter := &recordingReporter{}
	store := testSchedulerStore()
	sw := newTestSweeper(t, scanner, reporter, newMemCursorStore(), workers)

	s := newScheduler(sw, logmock.New(t), schedulerOptions{
		Workers:      workers,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
		Credentials:  store,
	})
	return s, reporter, store
}

func TestSchedulerSweepsOnAdd(t *testing.T) {
	scanner := answerAll()
	s, reporter, _ := newTestScheduler(t, scanner, 10)

	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	assert.Equal(t, 1, s.count())

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond,
		"a newly configured range is swept immediately, not at the next interval")

	require.Eventually(t, func() bool {
		reporter.mu.Lock()
		defer reporter.mu.Unlock()
		return len(reporter.runs) == 2
	}, 5*time.Second, 10*time.Millisecond)
}

func TestSchedulerKeepsARangeWhoseProbeCannotBeResolved(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.Probes.SNMP.CredentialIDs = []string{"cred-missing"}

	require.NoError(t, s.set(cfg), "a missing credential no longer rejects the range")
	assert.Equal(t, 1, s.count(), "the range keeps its schedule and self-heals when the credential arrives")
	require.Never(t, func() bool { return len(scanner.recorded()) > 0 }, 200*time.Millisecond, 10*time.Millisecond,
		"a cycle with no usable probe sweeps nothing")
}

func TestSchedulerResolvesEveryProbeEachCycle(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.Probes.Ping = &pingprobe.Options{Count: 1, Interval: time.Second, Timeout: time.Second}
	require.NoError(t, s.set(cfg))

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)
	opts := scanner.recorded()[0].Options
	assert.NotNil(t, opts.Ping)
	assert.NotNil(t, opts.SNMP)
}

func TestSchedulerDropsOnlyTheProbeItCannotResolve(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.Probes.Ping = &pingprobe.Options{Count: 1, Interval: time.Second, Timeout: time.Second}
	cfg.Probes.SNMP.CredentialIDs = []string{"cred-missing"}
	require.NoError(t, s.set(cfg))

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)
	opts := scanner.recorded()[0].Options
	assert.NotNil(t, opts.Ping, "one broken probe does not stop the others")
	assert.Nil(t, opts.SNMP)
}

func TestSchedulerRejectsOversizedRange(t *testing.T) {
	s, _, _ := newTestScheduler(t, answerAll(), 10)
	s.start(context.Background())
	defer s.stop()

	err := s.set(testRangeConfig("ad-1", "10.0.0.0/12"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerSetBeforeStartIsRejected(t *testing.T) {
	s, _, _ := newTestScheduler(t, answerAll(), 10)

	err := s.set(testRangeConfig("ad-1", "10.0.0.0/24"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerRemoveStopsTheRange(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)

	s.remove("ad-1")
	assert.Equal(t, 0, s.count())

	// Removing a range that was never scheduled is a no-op.
	s.remove("never-existed")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerReplacesRangeOnUpdate(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.1.0/24")))
	assert.Equal(t, 1, s.count(), "the same autodiscovery ID replaces its range rather than adding one")

	require.Eventually(t, func() bool {
		for _, call := range scanner.recorded() {
			if len(call.Targets) > 0 && call.Targets[0] == "10.0.1.0" {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
}

func TestSchedulerWorkerShare(t *testing.T) {
	s, _, _ := newTestScheduler(t, answerAll(), 10)

	s.ranges["a"] = &scheduledRange{}
	assert.Equal(t, int64(10), s.workerShare(), "one range gets the whole budget")

	s.ranges["b"] = &scheduledRange{}
	s.ranges["c"] = &scheduledRange{}
	assert.Equal(t, int64(3), s.workerShare())

	for _, id := range []string{"d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		s.ranges[id] = &scheduledRange{}
	}
	assert.Equal(t, int64(1), s.workerShare(), "the share never drops below one worker")
}

func TestSchedulerWorkerShareNeverExceedsTheSweeperBudget(t *testing.T) {
	sw := newTestSweeper(t, answerAll(), &recordingReporter{}, newMemCursorStore(), 4)
	s := newScheduler(sw, logmock.New(t), schedulerOptions{Workers: 64, MaxAddresses: 65536})

	assert.Equal(t, int64(4), s.workerShare())
}

func TestSchedulerStopIsIdempotentAndDrains(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)
	s.start(context.Background())

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(scanner.recorded()) >= 1 }, 5*time.Second, 10*time.Millisecond)

	s.stop()
	s.stop()
	assert.Equal(t, 0, s.count())
}

func TestSchedulerResolvesTheProbesPerCycle(t *testing.T) {
	scanner := answerAll()
	s, _, store := newTestScheduler(t, scanner, 10)
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.GreaterOrEqual(t, store.loadCount(), 1,
		"credentials are re-read each cycle so a Fleet rotation lands without a restart")
}

func TestSchedulerFloorsIntervalsBelowTheMinimum(t *testing.T) {
	scanner := answerAll()
	s, _, _ := newTestScheduler(t, scanner, 10)

	// The ticker is built inside the range goroutine, so a non-positive
	// interval would panic there and take the agent down with it.
	intervals := make(chan time.Duration, 4)
	s.newTicker = func(d time.Duration) (<-chan time.Time, func()) {
		intervals <- d
		ticker := time.NewTicker(d)
		return ticker.C, ticker.Stop
	}

	s.start(context.Background())
	defer s.stop()

	floor := time.Duration(minIntervalSec) * time.Second

	zero := testRangeConfig("ad-zero", "10.0.0.0/24")
	zero.IntervalSec = 0
	require.NoError(t, s.set(zero))
	assert.Equal(t, floor, <-intervals, "a zero interval ticks at the floor instead of panicking")

	negative := testRangeConfig("ad-negative", "10.0.1.0/24")
	negative.IntervalSec = -5
	require.NoError(t, s.set(negative))
	assert.Equal(t, floor, <-intervals, "a negative interval ticks at the floor instead of panicking")

	// parseRange clamps upstream, but a config reaching the scheduler by
	// another route must not out-tick the floor either.
	tooFast := testRangeConfig("ad-too-fast", "10.0.2.0/24")
	tooFast.IntervalSec = 1
	require.NoError(t, s.set(tooFast))
	assert.Equal(t, floor, <-intervals, "an interval below the floor is raised to it")

	require.Eventually(t, func() bool { return len(scanner.recorded()) == 3 }, 5*time.Second, 10*time.Millisecond,
		"every range is still swept once immediately")
}

// blockingScanner holds every scan until release is closed, and records how
// many scans were in flight at once.
type blockingScanner struct {
	recordingScanner
	entered     chan struct{}
	release     chan struct{}
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

func newBlockingScanner() *blockingScanner {
	c := &blockingScanner{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
	}
	answers := answerAll()
	c.respond = func(call scanCall) ([]probe.Result, error) {
		n := c.inFlight.Add(1)
		for {
			seen := c.maxInFlight.Load()
			if n <= seen || c.maxInFlight.CompareAndSwap(seen, n) {
				break
			}
		}
		c.entered <- struct{}{}
		<-c.release
		c.inFlight.Add(-1)
		return answers.respond(call)
	}
	return c
}

func (c *blockingScanner) sawTarget(ip string) bool {
	for _, call := range c.recorded() {
		if len(call.Targets) > 0 && call.Targets[0] == ip {
			return true
		}
	}
	return false
}

func TestSchedulerDoesNotOverlapCyclesForOneRange(t *testing.T) {
	scanner := newBlockingScanner()
	// A share equal to the budget would let the global semaphore serialise the
	// cycles on its own, so the cycle chain would go unexercised.
	sw := newTestSweeper(t, &scanner.recordingScanner, &recordingReporter{}, newMemCursorStore(), 10)
	s := newScheduler(sw, logmock.New(t), schedulerOptions{
		Workers:      1,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
		Credentials:  testSchedulerStore(),
	})
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	<-scanner.entered

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.1.0/24")))
	require.Never(t, func() bool { return scanner.sawTarget("10.0.1.0") }, 200*time.Millisecond, 10*time.Millisecond,
		"the replacement cycle waits for the cancelled one to unwind")

	// A third replacement, while the first cycle is stuck and the second waits.
	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.2.0/24")))
	require.Never(t, func() bool { return scanner.sawTarget("10.0.2.0") }, 200*time.Millisecond, 10*time.Millisecond,
		"the newest cycle waits for the whole chain ahead of it, not just its immediate predecessor")

	close(scanner.release)
	require.Eventually(t, func() bool { return scanner.sawTarget("10.0.2.0") }, 5*time.Second, 10*time.Millisecond)
	assert.False(t, scanner.sawTarget("10.0.1.0"), "a cycle cancelled before its turn never probes")
	assert.Equal(t, int32(1), scanner.maxInFlight.Load(), "one range never has two cycles in flight")
}

func TestSchedulerRemoveDuringACycleDrainsOnStop(t *testing.T) {
	scanner := newBlockingScanner()
	s, _, _ := newTestScheduler(t, &scanner.recordingScanner, 10)
	s.start(context.Background())

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	<-scanner.entered

	s.remove("ad-1")
	assert.Equal(t, 0, s.count())

	close(scanner.release)
	// stop returns only once the cancelled cycle has finished unwinding.
	s.stop()
	assert.Equal(t, 0, s.count())
}
