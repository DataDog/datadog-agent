// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// TestBlockedPipeCausesLivenessFailure proves that when the check runner's
// pendingChecksChan is full (all workers busy with slow checks), the
// collector-queue scheduler goroutine blocks on its send to checksPipe and
// cannot drain its health channel, causing the liveness health check to fail.
//
// This is the mechanism by which a degraded cluster agent (slow/unavailable
// endpoint check responses) can cascade into node agent liveness failures:
//
//  1. Cluster agent returns 502/timeout for endpoint check configs
//  2. Endpoint checks take ~10s each to timeout
//  3. All check runner workers are busy waiting on these timeouts
//  4. pendingChecksChan (unbuffered) blocks because no worker is available
//  5. Scheduler's blocking send at job.go:207 hangs
//  6. Health channel drain at job.go:213-217 never executes
//  7. After ~30s, health system marks collector-queue-* as unhealthy
//  8. Kubernetes liveness probe fails → pod is killed
func TestBlockedPipeCausesLivenessFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping: requires ~45s to observe health system timeout")
	}

	t.Run("blocked_pipe_causes_unhealthy", func(t *testing.T) {
		// Create an unbuffered channel that nobody reads from.
		// This simulates all check runner workers being saturated with slow checks.
		blockedPipe := make(chan check.Check)

		s := NewScheduler(blockedPipe)

		// Enter a check BEFORE Run() so it's placed in a bucket without spawning
		// a one-time enqueue goroutine. The 1-second interval means the bucket
		// ticker will attempt to send this check within 1 second of starting.
		slowCheck := &TestCheck{intl: 1 * time.Second}
		err := s.Enter(slowCheck)
		require.NoError(t, err)

		s.Run()
		defer s.Stop()

		// The scheduler's process() goroutine will:
		//   1. Receive a bucket tick after ~1 second
		//   2. Attempt to send slowCheck to blockedPipe (job.go:207)
		//   3. Block indefinitely — nobody is reading from blockedPipe
		//   4. Never reach the health channel drain (job.go:213-217)
		//
		// The health system pings every 15 seconds into a buffer-2 channel.
		// After ~30 seconds without the scheduler draining it, the buffer fills
		// and the next ping marks the component as unhealthy.
		assert.Eventually(t, func() bool {
			status := health.GetLive()
			for _, name := range status.Unhealthy {
				if name == "collector-queue-1s" {
					return true
				}
			}
			return false
		}, 45*time.Second, 1*time.Second,
			"collector-queue-1s should become unhealthy when checksPipe is blocked")
	})

	t.Run("consumed_pipe_stays_healthy", func(t *testing.T) {
		// Control test: when the pipe is consumed, the queue stays healthy.
		consumedPipe := make(chan check.Check)
		stopConsumer := make(chan bool)
		go consume(consumedPipe, stopConsumer)

		s := NewScheduler(consumedPipe)

		normalCheck := &TestCheck{intl: 10 * time.Second}
		err := s.Enter(normalCheck)
		require.NoError(t, err)

		s.Run()
		defer s.Stop()
		defer func() { stopConsumer <- true }()

		// Wait for the health system to run at least 2 ping cycles (~30s)
		// so the component has had a chance to be marked healthy.
		assert.Eventually(t, func() bool {
			status := health.GetLive()
			for _, name := range status.Healthy {
				if name == "collector-queue-10s" {
					return true
				}
			}
			return false
		}, 45*time.Second, 1*time.Second,
			"collector-queue-10s should be healthy when checksPipe is consumed")

		// Verify it's NOT in the unhealthy list
		status := health.GetLive()
		for _, name := range status.Unhealthy {
			if strings.HasPrefix(name, "collector-queue-10s") {
				t.Errorf("collector-queue-10s should not be unhealthy when pipe is consumed")
			}
		}
	})
}
