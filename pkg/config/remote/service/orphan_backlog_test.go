// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/go-tuf/data"
)

// orphanBacklogStats tracks the state of the simulated tracer fleet during
// TestOrphanedRequestBacklog. inFlight is the number of ClientGetConfigs
// calls that have been invoked but have not yet returned -- this is the
// server-side backlog we care about, and it is tracked independently of
// whether the simulated "client" has already given up waiting for it.
type orphanBacklogStats struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	completed   atomic.Int64
	abandoned   atomic.Int64
}

func (s *orphanBacklogStats) recordInFlightDelta(delta int64) {
	v := s.inFlight.Add(delta)
	for {
		max := s.maxInFlight.Load()
		if v <= max || s.maxInFlight.CompareAndSwap(max, v) {
			return
		}
	}
}

// simulateTimeoutRetryClient mimics dd-trace-go's remote-config poller under
// contention: fire a request, and if it doesn't complete within clientTimeout,
// stop waiting on it and immediately fire a new one. Crucially, the original
// call is NOT cancelled from the caller's perspective in a way that stops the
// underlying goroutine -- it keeps running against the real ClientGetConfigs
// call, exactly like the orphaned goroutines described in JULY-9.md Session 3.
func simulateTimeoutRetryClient(
	service *CoreAgentService,
	req *pbgo.ClientGetConfigsRequest,
	clientTimeout time.Duration,
	stop <-chan struct{},
	stats *orphanBacklogStats,
	inner *sync.WaitGroup,
) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		reqCtx, cancel := context.WithTimeout(context.Background(), clientTimeout)
		waitDone := make(chan struct{})

		stats.recordInFlightDelta(1)
		inner.Go(func() {
			defer stats.recordInFlightDelta(-1)
			defer close(waitDone)
			_, _ = service.ClientGetConfigs(reqCtx, req)
		})

		select {
		case <-waitDone:
			stats.completed.Add(1)
		case <-reqCtx.Done():
			// The simulated tracer gives up here, exactly like dd-trace-go's
			// 10s HTTP timeout. The goroutine above keeps running regardless
			// -- pre-A4 it will run the full (slow) handler body; post-A4 it
			// should notice ctx.Err() and return almost immediately once it
			// acquires s.mu.
			stats.abandoned.Add(1)
		case <-stop:
			cancel()
			return
		}
		cancel()
	}
}

// TestOrphanedRequestBacklog reproduces, at the unit-test level, the
// mechanism documented in JULY-9.md Session 1-3: many clients that time out
// and immediately retry without the server ever reaping their abandoned
// request. It measures the server-side in-flight ClientGetConfigs backlog
// over a sustained run.
//
// Run manually (not part of the default suite -- it's a load test, not a
// correctness test) via:
//
//	dda inv test --targets=./pkg/config/remote/service --test-run-name=TestOrphanedRequestBacklog \
//	  --extra-args="-run TestOrphanedRequestBacklog -v"
func TestOrphanedRequestBacklog(t *testing.T) {
	const (
		numClients      = 10
		clientTimeout   = 30 * time.Millisecond
		serverWorkDelay = 50 * time.Millisecond
		testDuration    = 300 * time.Millisecond
		sampleInterval  = 5 * time.Millisecond
		// Bound on how long we wait for the backlog built up during the
		// active phase to fully drain afterwards, so the test can't hang
		// forever pre-A4 (each queued request costs a full serverWorkDelay
		// pre-fix, vs. microseconds post-fix).
		drainTimeout = 30 * time.Second
	)

	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	mockClock := clock.NewMock()
	svc := newTestService(t, api, uptaneClient, mockClock)

	// hasUpdate=true (DirectorTargets=1 != the request's TargetsVersion=0) so
	// the handler takes the expensive path through Targets()/TargetFiles()/
	// TargetsMeta() -- the same path identified as the hot path in
	// PROJECT.md Session 4. The artificial sleep on Targets() simulates a
	// contended/slow critical section while s.mu is held.
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{DirectorTargets: 1}, nil)
	uptaneClient.On("Targets").Return(data.TargetFiles{}, nil).Run(func(_ mock.Arguments) {
		time.Sleep(serverWorkDelay)
	})
	uptaneClient.On("TargetFiles", mock.Anything).Return(map[string][]byte{}, nil)
	uptaneClient.On("TargetsMeta").Return([]byte{}, nil)

	// One distinct client ID per simulated tracer, matching real-world
	// fan-out. Each must be pre-seeded via clients.seen() so ClientGetConfigs
	// doesn't take the new-client "bypass" path (service.go:967), which waits
	// on a poll-loop goroutine we never start in this test and would hang.
	reqs := make([]*pbgo.ClientGetConfigsRequest, numClients)
	for i := range reqs {
		reqs[i] = &pbgo.ClientGetConfigsRequest{
			Client: &pbgo.Client{
				Id:          fmt.Sprintf("orphan-test-client-%d", i),
				IsAgent:     true,
				ClientAgent: &pbgo.ClientAgent{},
				Products:    []string{"AGENT_CONFIG"},
				State:       &pbgo.ClientState{RootVersion: 1},
			},
		}
		svc.clients.seen(reqs[i].Client)
	}

	stats := &orphanBacklogStats{}
	stop := make(chan struct{})
	var outer sync.WaitGroup // the per-client retry loops
	var inner sync.WaitGroup // the actual (possibly orphaned) ClientGetConfigs calls
	for i := range numClients {
		req := reqs[i]
		outer.Go(func() {
			simulateTimeoutRetryClient(svc, req, clientTimeout, stop, stats, &inner)
		})
	}

	// Sample in-flight count throughout the run to see whether it grows
	// unboundedly (pre-A4) or stabilizes (post-A4).
	deadline := time.Now().Add(testDuration)
	var samples []int64
	for time.Now().Before(deadline) {
		samples = append(samples, stats.inFlight.Load())
		time.Sleep(sampleInterval)
	}

	// Stop new arrivals, then wait for the backlog built up during the
	// active phase to fully drain (bounded by drainTimeout so a pre-A4 run
	// with a deep backlog can't hang the test suite indefinitely).
	close(stop)
	outer.Wait()
	drained := make(chan struct{})
	go func() {
		inner.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(drainTimeout):
		t.Logf("WARNING: backlog did not fully drain within %s; reporting partial results", drainTimeout)
	}

	// Report -- this test intentionally does not assert a specific bound; run
	// it before and after the A4 fix and compare max in-flight / completed /
	// abandoned by hand (see JULY-9.md for the recorded numbers).
	t.Logf("numClients=%d clientTimeout=%s serverWorkDelay=%s duration=%s",
		numClients, clientTimeout, serverWorkDelay, testDuration)
	t.Logf("completed=%d abandoned=%d maxInFlight=%d finalInFlight=%d",
		stats.completed.Load(), stats.abandoned.Load(), stats.maxInFlight.Load(), stats.inFlight.Load())
	// Print a coarse timeline so growth-vs-plateau is visible without a
	// separate plotting step.
	step := len(samples) / 10
	if step == 0 {
		step = 1
	}
	for i := 0; i < len(samples); i += step {
		t.Logf("t=%-6s inFlight=%d", time.Duration(i)*sampleInterval, samples[i])
	}
}
