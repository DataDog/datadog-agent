// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package egressimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	forwardermock "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

// newTestEgress constructs the real egress type directly (bypassing New) so
// tests exercise the actual tick()/buildReport() logic, with the store and
// forwarder mocks standing in for the two real dependencies.
func newTestEgress(t *testing.T, store storedef.Component, forwarder forwarderdef.Component) *egress {
	t.Helper()
	e := &egress{
		log:         logmock.New(t),
		interval:    time.Minute,
		hostname:    "test-host",
		agentFlavor: "agent",
		store:       store,
		forwarder:   forwarder,
		resolvedCh:  make(chan *healthplatformpayload.Issue, resolvedChBuf),
		resolved:    make(map[string]*healthplatformpayload.Issue),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	store.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: e.resolvedCh})
	return e
}

func TestTickSendsActiveIssues(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1", Title: "Test"}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) (int, error) {
		reports = append(reports, r)
		return 42, nil
	}))
	e := newTestEgress(t, store, fwd)

	e.tick()

	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "issue-1")
	assert.Equal(t, "test-host", reports[0].Host.Hostname)
	assert.Equal(t, eventType, reports[0].EventType)
}

func TestTickSkipsWhenEmpty(t *testing.T) {
	var called bool
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		called = true
		return 0, nil
	}))
	e := newTestEgress(t, storemock.New(t), fwd)

	e.tick()

	assert.False(t, called)
}

func TestTickLogsOnForwarderError(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var called bool
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		called = true
		return 0, assert.AnError
	}))
	e := newTestEgress(t, store, fwd)

	// tick() only logs a forwarder error internally; it has no return value.
	// TestResolvedStaysOnSendFailure below covers the observable consequence.
	e.tick()

	assert.True(t, called, "forwarder.Send must still be attempted")
}

func TestLifecycleStartStop(t *testing.T) {
	e := newTestEgress(t, storemock.New(t), forwardermock.New(t))
	e.interval = 50 * time.Millisecond

	require.NoError(t, e.start(context.Background()))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, e.stop(context.Background()))
}

// TestTickFiresOnInterval verifies run()'s ticker loop actually repeats, not
// just fires once — a property bundle_test.go's Eventually(count > 0) checks
// don't cover, since they're satisfied by a single tick.
func TestTickFiresOnInterval(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var sendCount atomic.Int32
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		sendCount.Add(1)
		return 0, nil
	}))
	e := newTestEgress(t, store, fwd)
	e.interval = 30 * time.Millisecond

	require.NoError(t, e.start(context.Background()))
	require.Eventually(t, func() bool {
		return sendCount.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond, "expected at least 2 ticks")
	require.NoError(t, e.stop(context.Background()))
}

// TestErrorThenRecovery verifies a failing tick does not kill run()'s loop:
// once the forwarder recovers, the next tick still fires and succeeds.
func TestErrorThenRecovery(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var attempts atomic.Int32
	var erroring atomic.Bool
	erroring.Store(true)
	var successes atomic.Int32
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		attempts.Add(1)
		if erroring.Load() {
			return 0, assert.AnError
		}
		successes.Add(1)
		return 0, nil
	}))
	e := newTestEgress(t, store, fwd)
	e.interval = 20 * time.Millisecond

	require.NoError(t, e.start(context.Background()))
	require.Eventually(t, func() bool { return attempts.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)

	erroring.Store(false)

	require.Eventually(t, func() bool { return successes.Load() >= 1 }, 2*time.Second, 5*time.Millisecond,
		"expected successful send after error recovery")

	require.NoError(t, e.stop(context.Background()))
}

func TestBuildReport(t *testing.T) {
	e := newTestEgress(t, storemock.New(t), forwardermock.New(t))

	report := e.buildReport(map[string]*healthplatformpayload.Issue{"a": {Id: "a"}, "b": {Id: "b"}})

	assert.Equal(t, eventType, report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, "agent", report.Service)
	assert.Len(t, report.Issues, 2)
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

// TestResolvedIssueSentOnce verifies that resolved tombstones are cleared after a successful send.
func TestResolvedIssueSentOnce(t *testing.T) {
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) (int, error) {
		reports = append(reports, r)
		return 0, nil
	}))
	e := newTestEgress(t, storemock.New(t), fwd)
	e.resolved["r-issue"] = &healthplatformpayload.Issue{
		Id: "r-issue",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	}

	e.tick()

	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "r-issue")
	assert.Empty(t, e.resolved, "resolved map must be cleared after successful send")

	e.tick()
	assert.Len(t, reports, 1, "second tick must skip: no active or resolved issues")
}

// TestResolvedStaysOnSendFailure verifies resolved tombstones are retained when send fails.
func TestResolvedStaysOnSendFailure(t *testing.T) {
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		return 0, assert.AnError
	}))
	e := newTestEgress(t, storemock.New(t), fwd)
	e.resolved["fail-issue"] = &healthplatformpayload.Issue{Id: "fail-issue"}

	e.tick()

	assert.Contains(t, e.resolved, "fail-issue", "resolved map must be retained after failed send")
}

// TestActiveWinsOverResolvedOnRecurrence verifies that an active entry takes precedence over
// a stale resolved tombstone for the same ID (issue recurred after being resolved).
func TestActiveWinsOverResolvedOnRecurrence(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{
		Id: "i:1",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE,
		},
	}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) (int, error) {
		reports = append(reports, r)
		return 0, nil
	}))
	e := newTestEgress(t, store, fwd)
	e.resolved["i:1"] = &healthplatformpayload.Issue{
		Id: "i:1",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	}

	e.tick()

	require.Len(t, reports, 1)
	sent := reports[0].Issues["i:1"]
	require.NotNil(t, sent)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE, sent.PersistedIssue.GetState(),
		"active entry must win over stale resolved tombstone on recurrence")
}

// TestObserverReceivesResolvedFromStore verifies the full store -> resolvedCh -> tick() path:
// resolving an issue in the store must surface as a tombstone on the next tick.
func TestObserverReceivesResolvedFromStore(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) (int, error) {
		reports = append(reports, r)
		return 0, nil
	}))
	e := newTestEgress(t, store, fwd)

	// First tick: issue-1 is active.
	e.tick()
	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "issue-1")

	// Store resolves the issue — flows into e.resolvedCh via the observer
	// registered in newTestEgress, exactly as it would through the real store.
	// tick() drains resolvedCh itself, so no manual drain is needed here.
	store.ResolveIssue("issue-1")

	// Second tick: issue-1 now appears as a resolved tombstone.
	e.tick()
	require.Len(t, reports, 2)
	sent := reports[1].Issues["issue-1"]
	require.NotNil(t, sent)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED, sent.PersistedIssue.GetState(),
		"issue resolved via store.ResolveIssue must be forwarded as a resolved tombstone")
}

// TestStatusInitial verifies a fresh egress reports healthy with zero counters
// before any tick has run.
func TestStatusInitial(t *testing.T) {
	e := newTestEgress(t, storemock.New(t), forwardermock.New(t))

	s := e.Status()

	assert.True(t, s.Healthy)
	assert.True(t, s.LastAttemptAt.IsZero())
	assert.True(t, s.LastSuccessAt.IsZero())
	assert.NoError(t, s.LastError)
	assert.Zero(t, s.IssuesSentTotal)
	assert.Zero(t, s.BytesSentTotal)
	assert.Zero(t, s.SendErrorsTotal)
}

// TestStatusAfterSuccessfulSend verifies Status reflects a successful tick:
// healthy, LastSuccessAt set, and issues/bytes counters incremented.
func TestStatusAfterSuccessfulSend(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		return 123, nil
	}))
	e := newTestEgress(t, store, fwd)

	e.tick()
	s := e.Status()

	assert.True(t, s.Healthy)
	assert.False(t, s.LastAttemptAt.IsZero())
	assert.False(t, s.LastSuccessAt.IsZero())
	assert.NoError(t, s.LastError)
	assert.EqualValues(t, 1, s.IssuesSentTotal)
	assert.EqualValues(t, 123, s.BytesSentTotal)
	assert.Zero(t, s.SendErrorsTotal)
}

// TestStatusAfterFailedSend verifies Status reflects a failed tick: unhealthy,
// LastError set, and the send-errors counter incremented without touching the
// success counters.
func TestStatusAfterFailedSend(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		return 0, assert.AnError
	}))
	e := newTestEgress(t, store, fwd)

	e.tick()
	s := e.Status()

	assert.False(t, s.Healthy)
	assert.False(t, s.LastAttemptAt.IsZero())
	assert.True(t, s.LastSuccessAt.IsZero())
	assert.Equal(t, assert.AnError, s.LastError)
	assert.Zero(t, s.IssuesSentTotal)
	assert.Zero(t, s.BytesSentTotal)
	assert.EqualValues(t, 1, s.SendErrorsTotal)
}

// TestStatusStaysHealthyDuringIdlePeriodAfterSuccess verifies that once an
// issue resolves and the store goes empty, repeated skipped ticks (the normal
// steady state) keep Status healthy, rather than going stale after
// 2*interval because no further send ever touches LastSuccessAt.
func TestStatusStaysHealthyDuringIdlePeriodAfterSuccess(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		return 10, nil
	}))
	e := newTestEgress(t, store, fwd)
	e.interval = time.Minute

	e.tick()
	require.True(t, e.Status().Healthy)

	// The issue resolves and is sent once as a tombstone, then the store is
	// empty and every subsequent tick takes the skip path. tick() drains
	// resolvedCh itself, so no manual drain is needed here.
	store.ResolveIssue("issue-1")
	e.tick()
	require.Empty(t, e.resolved)

	// Simulate several more ticker cycles with nothing to report each time:
	// each skip tick still refreshes lastAttemptAt, so Status stays healthy
	// no matter how long the idle period lasts.
	for i := 0; i < 5; i++ {
		e.tick()
	}

	s := e.Status()
	assert.True(t, s.Healthy, "idle egress with no errors must stay healthy across sustained quiet ticks")
	assert.NoError(t, s.LastError)
}

// TestStatusRetriesResolvedTombstoneAfterFailure verifies the fix for the
// select race between run()'s ticker and resolvedCh cases: even when a tick
// fires right as an issue resolves -- before run()'s select loop has drained
// the tombstone into e.resolved -- tick() drains resolvedCh itself first, so
// the tombstone is retried rather than mistaken for "nothing to report".
// Status only recovers once that retry actually succeeds.
func TestStatusRetriesResolvedTombstoneAfterFailure(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var erroring atomic.Bool
	erroring.Store(true)
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		if erroring.Load() {
			return 0, assert.AnError
		}
		return 0, nil
	}))
	e := newTestEgress(t, store, fwd)
	e.interval = time.Minute

	e.tick()
	require.False(t, e.Status().Healthy, "failed send must report unhealthy")

	// The issue resolves in the store; its tombstone is left sitting in
	// resolvedCh, exactly as if run()'s ticker case had won the select race
	// against the resolvedCh case.
	store.ResolveIssue("issue-1")

	erroring.Store(false)
	e.tick()

	s := e.Status()
	assert.True(t, s.Healthy, "a successful retry of the resolved tombstone must clear the stale error")
	assert.NoError(t, s.LastError)
	assert.EqualValues(t, 1, s.SendErrorsTotal, "cumulative error count must be preserved across recovery")
}

// TestStatusStaysUnhealthyWhenPersistentFailureOutlivesResolvedIssue verifies
// the follow-up review fix on #56278: a systemic send failure (e.g. bad
// credentials) must stay flagged even after the issue that first triggered it
// resolves on its own. Resolving the issue only queues its tombstone for
// retry -- it does not, by itself, prove the send pipeline works again, so
// the local issue queue draining to zero must not be read as "healthy".
func TestStatusStaysUnhealthyWhenPersistentFailureOutlivesResolvedIssue(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		return 0, assert.AnError
	}))
	e := newTestEgress(t, store, fwd)
	e.interval = time.Minute

	e.tick()
	require.False(t, e.Status().Healthy, "failed send must report unhealthy")

	// The issue resolves, but the forwarder keeps failing for an unrelated,
	// persistent reason.
	store.ResolveIssue("issue-1")

	for i := 0; i < 5; i++ {
		e.tick()
	}

	s := e.Status()
	assert.False(t, s.Healthy, "a persistent send failure must stay flagged even once the originating issue resolves")
	assert.Error(t, s.LastError)
	assert.Contains(t, e.resolved, "issue-1", "the unresolved tombstone must remain queued for retry")
}

// TestStatusGoesUnhealthyWhenTicksStop verifies the staleness check in
// Status still catches a genuinely stalled tick loop: if lastAttemptAt
// hasn't been refreshed for over 2*interval, Status must report unhealthy
// even though the last recorded attempt didn't error.
func TestStatusGoesUnhealthyWhenTicksStop(t *testing.T) {
	e := newTestEgress(t, storemock.New(t), forwardermock.New(t))
	e.interval = time.Minute
	e.lastAttemptAt = time.Now().Add(-3 * e.interval)

	assert.False(t, e.Status().Healthy, "stale lastAttemptAt beyond 2*interval must report unhealthy")
}

// TestStatusRecoversAfterErrorThenSuccess verifies a successful tick clears
// the unhealthy state left by a prior failed tick, while cumulative error
// counters are preserved.
func TestStatusRecoversAfterErrorThenSuccess(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var erroring atomic.Bool
	erroring.Store(true)
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) (int, error) {
		if erroring.Load() {
			return 0, assert.AnError
		}
		return 10, nil
	}))
	e := newTestEgress(t, store, fwd)

	e.tick()
	require.False(t, e.Status().Healthy)

	erroring.Store(false)
	e.tick()
	s := e.Status()

	assert.True(t, s.Healthy)
	assert.NoError(t, s.LastError)
	assert.EqualValues(t, 1, s.SendErrorsTotal, "cumulative error count must be preserved across recovery")
	assert.EqualValues(t, 1, s.IssuesSentTotal)
	assert.EqualValues(t, 10, s.BytesSentTotal)
}
