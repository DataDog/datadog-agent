// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package egressimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// mockStore satisfies storedef.Component; captures the registered observer for assertions.
type mockStore struct {
	mu       sync.Mutex
	observer storedef.IssueObserver
}

func (m *mockStore) RegisterObserver(obs storedef.IssueObserver) {
	m.mu.Lock()
	m.observer = obs
	m.mu.Unlock()
}

func (m *mockStore) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) { return 0, nil }
func (m *mockStore) ReportIssue(_ *healthplatformpayload.Issue) error             { return nil }
func (m *mockStore) ResolveIssue(_ string)                                        {}
func (m *mockStore) ResolveAllIssues()                                            {}
func (m *mockStore) GetIssue(_ string) *healthplatformpayload.Issue               { return nil }
func (m *mockStore) GetActiveIssueIDsByIssueName(_ string) []string               { return nil }

// mockForwarder records Send calls.
type mockForwarder struct {
	mu        sync.Mutex
	reports   []*healthplatformpayload.HealthReport
	sendErr   error
	sendCount atomic.Int32
}

func (m *mockForwarder) Send(_ context.Context, report *healthplatformpayload.HealthReport) error {
	m.sendCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.reports = append(m.reports, report)
	return nil
}

var _ forwarderdef.Component = (*mockForwarder)(nil)

func newTestEgress(t *testing.T, interval time.Duration, fwd *mockForwarder) *egress {
	t.Helper()
	return &egress{
		log:         logmock.New(t),
		interval:    interval,
		hostname:    "test-host",
		agentFlavor: "agent",
		forwarder:   fwd,
		activeCh:    make(chan *healthplatformpayload.Issue, issueChSize),
		resolvedCh:  make(chan *healthplatformpayload.Issue, issueChSize),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

func TestTickSendsActiveIssues(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)
	e.activeCh <- &healthplatformpayload.Issue{Id: "issue-1", Title: "Test"}

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	require.Len(t, fwd.reports, 1)
	assert.Contains(t, fwd.reports[0].Issues, "issue-1")
	assert.Equal(t, "test-host", fwd.reports[0].Host.Hostname)
	assert.Equal(t, eventType, fwd.reports[0].EventType)
}

func TestTickSkipsWhenEmpty(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.tick()

	assert.Equal(t, int32(0), fwd.sendCount.Load())
}

func TestTickLogsOnForwarderError(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, fwd)
	e.activeCh <- &healthplatformpayload.Issue{Id: "issue-1"}

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
}

func TestLifecycleStartStop(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, 50*time.Millisecond, fwd)

	require.NoError(t, e.start(context.Background()))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, e.stop(context.Background()))
}

func TestTickFiresOnInterval(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, 30*time.Millisecond, fwd)

	require.NoError(t, e.start(context.Background()))

	// Keep activeCh populated so ticks don't skip.
	go func() {
		issue := &healthplatformpayload.Issue{Id: "issue-1"}
		for {
			select {
			case e.activeCh <- issue:
			case <-e.stopCh:
				return
			}
		}
	}()

	require.Eventually(t, func() bool {
		return fwd.sendCount.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond, "expected at least 2 ticks")
	require.NoError(t, e.stop(context.Background()))
}

func TestErrorThenRecovery(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, 20*time.Millisecond, fwd)

	require.NoError(t, e.start(context.Background()))

	go func() {
		issue := &healthplatformpayload.Issue{Id: "issue-1"}
		for {
			select {
			case e.activeCh <- issue:
			case <-e.stopCh:
				return
			}
		}
	}()

	require.Eventually(t, func() bool { return fwd.sendCount.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)

	fwd.mu.Lock()
	fwd.sendErr = nil
	fwd.mu.Unlock()

	require.Eventually(t, func() bool {
		fwd.mu.Lock()
		defer fwd.mu.Unlock()
		return len(fwd.reports) >= 1
	}, 2*time.Second, 5*time.Millisecond, "expected successful send after error recovery")

	require.NoError(t, e.stop(context.Background()))
}

func TestBuildReport(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	report := e.buildReport(map[string]*healthplatformpayload.Issue{"a": {Id: "a"}, "b": {Id: "b"}})

	assert.Equal(t, eventType, report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, "agent", report.Service)
	assert.Len(t, report.Issues, 2)
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

// TestResolvedIssueSentOnce verifies that a resolved issue is sent and removed
// from resolvedCh; activeCh is also drained each tick.
func TestResolvedIssueSentOnce(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.resolvedCh <- &healthplatformpayload.Issue{
		Id: "r-issue",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	}

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
	fwd.mu.Lock()
	assert.Contains(t, fwd.reports[0].Issues, "r-issue")
	fwd.mu.Unlock()

	assert.Empty(t, e.resolvedCh, "resolvedCh must be empty after successful send")

	// second tick: both channels empty — skip
	e.tick()
	assert.Equal(t, int32(1), fwd.sendCount.Load())
}

// TestResolvedReturnedOnSendFailure verifies resolvedCh items are put back on failure.
func TestResolvedReturnedOnSendFailure(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, fwd)

	e.resolvedCh <- &healthplatformpayload.Issue{Id: "fail-issue"}
	e.tick()

	assert.Len(t, e.resolvedCh, 1, "resolved issue must be returned after failed send")
}

// TestActiveDrainedNotReturned verifies active issues are drained each tick and
// not put back (re-populated by the next check run).
func TestActiveDrainedNotReturned(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, fwd)

	e.activeCh <- &healthplatformpayload.Issue{Id: "active-issue"}
	e.tick()

	assert.Empty(t, e.activeCh, "active issues are not returned on failure — next check run repopulates")
}

// TestObserverWiresChannels verifies that the observer registered in New routes
// reported and resolved issues into the correct channels.
func TestObserverWiresChannels(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	store.RegisterObserver(storedef.IssueObserver{
		OnIssueReported: func(issue *healthplatformpayload.Issue) {
			select {
			case e.activeCh <- issue:
			default:
			}
		},
		OnIssueResolved: func(resolved *healthplatformpayload.Issue) {
			select {
			case e.resolvedCh <- resolved:
			default:
			}
		},
	})

	store.mu.Lock()
	store.observer.OnIssueReported(&healthplatformpayload.Issue{Id: "active"})
	store.observer.OnIssueResolved(&healthplatformpayload.Issue{Id: "resolved"})
	store.mu.Unlock()

	assert.Len(t, e.activeCh, 1)
	assert.Len(t, e.resolvedCh, 1)
}
