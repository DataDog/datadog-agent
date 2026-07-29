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

// drainResolved replicates the channel-drain case that run()'s background
// select loop performs, so tick() can be driven deterministically in tests
// without starting the real ticker goroutine.
func drainResolved(e *egress) {
	for {
		select {
		case issue := <-e.resolvedCh:
			e.resolved[issue.Id] = issue
		default:
			return
		}
	}
}

func TestTickSendsActiveIssues(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1", Title: "Test"}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		called = true
		return nil
	}))
	e := newTestEgress(t, storemock.New(t), fwd)

	e.tick()

	assert.False(t, called)
}

func TestTickLogsOnForwarderError(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var called bool
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		called = true
		return assert.AnError
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		sendCount.Add(1)
		return nil
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		attempts.Add(1)
		if erroring.Load() {
			return assert.AnError
		}
		successes.Add(1)
		return nil
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		return assert.AnError
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
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
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := newTestEgress(t, store, fwd)

	// First tick: issue-1 is active.
	e.tick()
	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "issue-1")

	// Store resolves the issue — flows into e.resolvedCh via the observer
	// registered in newTestEgress, exactly as it would through the real store.
	store.ResolveIssue("issue-1")
	drainResolved(e)

	// Second tick: issue-1 now appears as a resolved tombstone.
	e.tick()
	require.Len(t, reports, 2)
	sent := reports[1].Issues["issue-1"]
	require.NotNil(t, sent)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED, sent.PersistedIssue.GetState(),
		"issue resolved via store.ResolveIssue must be forwarded as a resolved tombstone")
}
