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
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

func testRangeConfig(id, cidr string) rangeConfig {
	return rangeConfig{
		AutodiscoveryID: id,
		Namespace:       "default",
		CIDR:            cidr,
		CredentialIDs:   []string{"cred-a"},
		IntervalSec:     3600,
		SNMPOptions:     &connectivity.SNMPOptions{Port: 161, TimeoutMs: 2000, Retries: 1},
	}
}

func newTestScheduler(t *testing.T, checker connectivityChecker, workers int64) (*scheduler, *recordingReporter) {
	t.Helper()
	reporter := &recordingReporter{}
	sw := newTestSweeper(t, checker, reporter, newMemCursorStore(), workers)

	creds := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"cred-a": {ID: "cred-a", Version: "2c", Community: "public"},
	}}

	s := newScheduler(sw, creds, logmock.New(t), schedulerOptions{
		Workers:      workers,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
	})
	return s, reporter
}

func TestSchedulerSweepsOnAdd(t *testing.T) {
	checker := answerAll()
	s, reporter := newTestScheduler(t, checker, 10)

	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	assert.Equal(t, 1, s.count())

	require.Eventually(t, func() bool { return len(checker.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond,
		"a newly configured range is swept immediately, not at the next interval")

	require.Eventually(t, func() bool {
		reporter.mu.Lock()
		defer reporter.mu.Unlock()
		return len(reporter.runs) == 2
	}, 5*time.Second, 10*time.Millisecond)
}

func TestSchedulerRejectsBadConfig(t *testing.T) {
	s, _ := newTestScheduler(t, answerAll(), 10)
	s.start(context.Background())
	defer s.stop()

	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.CredentialIDs = []string{"missing"}
	err := s.set(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
	assert.Equal(t, 0, s.count(), "a range whose credentials are unavailable is not scheduled")
}

func TestSchedulerRejectsOversizedRange(t *testing.T) {
	s, _ := newTestScheduler(t, answerAll(), 10)
	s.start(context.Background())
	defer s.stop()

	err := s.set(testRangeConfig("ad-1", "10.0.0.0/12"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerSetBeforeStartIsRejected(t *testing.T) {
	s, _ := newTestScheduler(t, answerAll(), 10)

	err := s.set(testRangeConfig("ad-1", "10.0.0.0/24"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerRemoveStopsTheRange(t *testing.T) {
	checker := answerAll()
	s, _ := newTestScheduler(t, checker, 10)
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(checker.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)

	s.remove("ad-1")
	assert.Equal(t, 0, s.count())

	// Removing a range that was never scheduled is a no-op.
	s.remove("never-existed")
	assert.Equal(t, 0, s.count())
}

func TestSchedulerReplacesRangeOnUpdate(t *testing.T) {
	checker := answerAll()
	s, _ := newTestScheduler(t, checker, 10)
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(checker.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.1.0/24")))
	assert.Equal(t, 1, s.count(), "the same autodiscovery ID replaces its range rather than adding one")

	require.Eventually(t, func() bool {
		for _, req := range checker.recorded() {
			if len(req.Targets) > 0 && req.Targets[0] == "10.0.1.0" {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
}

func TestSchedulerWorkerShare(t *testing.T) {
	s, _ := newTestScheduler(t, answerAll(), 10)

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
	// The sweeper's semaphore only holds 4 units, so a larger share could
	// never be acquired. The scheduler must not hand out more than the sweeper
	// can actually grant.
	sw := newTestSweeper(t, answerAll(), &recordingReporter{}, newMemCursorStore(), 4)
	s := newScheduler(sw, &stubCredentialStore{}, logmock.New(t), schedulerOptions{Workers: 64, MaxAddresses: 65536})

	assert.Equal(t, int64(4), s.workerShare())
}

func TestSchedulerStopIsIdempotentAndDrains(t *testing.T) {
	checker := answerAll()
	s, _ := newTestScheduler(t, checker, 10)
	s.start(context.Background())

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(checker.recorded()) >= 1 }, 5*time.Second, 10*time.Millisecond)

	s.stop()
	s.stop()
	assert.Equal(t, 0, s.count())
}

func TestSchedulerReloadsCredentialsPerCycle(t *testing.T) {
	checker := answerAll()
	sw := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 10)

	creds := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"cred-a": {ID: "cred-a", Version: "2c", Community: "public"},
	}}
	s := newScheduler(sw, creds, logmock.New(t), schedulerOptions{
		Workers:      10,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
	})
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	require.Eventually(t, func() bool { return len(checker.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.GreaterOrEqual(t, creds.reloads(), 1,
		"credentials are re-read each cycle so a Fleet rotation lands without a restart")
}

func TestSchedulerPingToggle(t *testing.T) {
	checker := answerAll()
	s, _ := newTestScheduler(t, checker, 10)
	s.setPingEnabled(true)
	s.start(context.Background())
	defer s.stop()

	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.PingOptions = &connectivity.PingOptions{Count: 1, IntervalMs: 1000, TimeoutMs: 1000}
	require.NoError(t, s.set(cfg))

	require.Eventually(t, func() bool { return len(checker.recorded()) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{connectivity.CheckPing, connectivity.CheckSNMP}, checker.recorded()[0].Checks)
}

func TestSchedulerFloorsANonPositiveInterval(t *testing.T) {
	checker := answerAll()
	s, _ := newTestScheduler(t, checker, 10)

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

	require.Eventually(t, func() bool { return len(checker.recorded()) == 2 }, 5*time.Second, 10*time.Millisecond,
		"both ranges are still swept once immediately")
}

// blockingChecker holds every probe until release is closed, and records how
// many probes were in flight at once.
type blockingChecker struct {
	fakeChecker
	entered     chan struct{}
	release     chan struct{}
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

func newBlockingChecker() *blockingChecker {
	c := &blockingChecker{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
	}
	answers := answerAll()
	c.respond = func(req connectivity.Request) (connectivity.Result, error) {
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
		return answers.respond(req)
	}
	return c
}

func (c *blockingChecker) sawTarget(ip string) bool {
	for _, req := range c.recorded() {
		if len(req.Targets) > 0 && req.Targets[0] == ip {
			return true
		}
	}
	return false
}

func TestSchedulerDoesNotOverlapCyclesForOneRange(t *testing.T) {
	checker := newBlockingChecker()
	// The worker share is deliberately smaller than the sweeper's budget. With
	// a share equal to the budget the global semaphore alone would serialise
	// the cycles and the test would pass without exercising the cycle chain.
	sw := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 10)
	creds := &stubCredentialStore{creds: map[string]connectivity.SNMPCredential{
		"cred-a": {ID: "cred-a", Version: "2c", Community: "public"},
	}}
	s := newScheduler(sw, creds, logmock.New(t), schedulerOptions{
		Workers:      1,
		MaxAddresses: 65536,
		Defaults:     rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536},
	})
	s.start(context.Background())
	defer s.stop()

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	<-checker.entered

	// The first cycle is stuck inside its probe. Replacing the range must not
	// start a second cycle for the same autodiscovery ID alongside it: the two
	// would interleave their cursor writes.
	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.1.0/24")))
	require.Never(t, func() bool { return checker.sawTarget("10.0.1.0") }, 200*time.Millisecond, 10*time.Millisecond,
		"the replacement cycle waits for the cancelled one to unwind")

	// A third replacement, while the first cycle is still stuck in its probe
	// and the second is still waiting its turn. The chain has to stay ordered
	// at every depth: the newest cycle must not jump ahead of the oldest one
	// just because the cycle it directly replaces was cancelled mid-wait.
	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.2.0/24")))
	require.Never(t, func() bool { return checker.sawTarget("10.0.2.0") }, 200*time.Millisecond, 10*time.Millisecond,
		"the newest cycle waits for the whole chain ahead of it, not just its immediate predecessor")

	close(checker.release)
	require.Eventually(t, func() bool { return checker.sawTarget("10.0.2.0") }, 5*time.Second, 10*time.Millisecond)
	assert.False(t, checker.sawTarget("10.0.1.0"), "a cycle cancelled before its turn never probes")
	assert.Equal(t, int32(1), checker.maxInFlight.Load(), "one range never has two cycles in flight")
}

func TestSchedulerRemoveDuringACycleDrainsOnStop(t *testing.T) {
	checker := newBlockingChecker()
	s, _ := newTestScheduler(t, checker, 10)
	s.start(context.Background())

	require.NoError(t, s.set(testRangeConfig("ad-1", "10.0.0.0/24")))
	<-checker.entered

	s.remove("ad-1")
	assert.Equal(t, 0, s.count())

	close(checker.release)
	// stop returns only once the cancelled cycle has finished unwinding.
	s.stop()
	assert.Equal(t, 0, s.count())
}
