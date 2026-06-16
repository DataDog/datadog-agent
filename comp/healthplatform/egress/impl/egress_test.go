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
	issues   map[string]*healthplatformpayload.Issue
	observer storedef.IssueObserver
}

func (m *mockStore) RegisterObserver(obs storedef.IssueObserver) {
	m.mu.Lock()
	m.observer = obs
	m.mu.Unlock()
}

func (m *mockStore) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.issues) == 0 {
		return 0, nil
	}
	cp := make(map[string]*healthplatformpayload.Issue, len(m.issues))
	for k, v := range m.issues {
		cp[k] = v
	}
	return len(cp), cp
}

func (m *mockStore) ReportIssue(_ *healthplatformpayload.Issue) error { return nil }
func (m *mockStore) ResolveIssue(_ string)                            {}
func (m *mockStore) ResolveAllIssues()                                {}
func (m *mockStore) GetIssue(_ string) *healthplatformpayload.Issue   { return nil }
func (m *mockStore) GetActiveIssueIDsByIssueName(_ string) []string   { return nil }

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

func newTestEgress(t *testing.T, interval time.Duration, store *mockStore, fwd *mockForwarder) *egress {
	t.Helper()
	e := &egress{
		log:         logmock.New(t),
		interval:    interval,
		hostname:    "test-host",
		agentFlavor: "agent",
		store:       store,
		forwarder:   fwd,
		resolvedCh:  make(chan *healthplatformpayload.Issue, resolvedChSize),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	store.RegisterObserver(storedef.IssueObserver{
		OnIssueResolved: func(resolved *healthplatformpayload.Issue) {
			e.resolvedCh <- resolved
		},
	})
	return e
}

func TestTickSendsActiveIssues(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"issue-1": {Id: "issue-1", Title: "Test", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH},
	}}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

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
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

	e.tick()

	assert.Equal(t, int32(0), fwd.sendCount.Load())
}

func TestTickLogsOnForwarderError(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"issue-1": {Id: "issue-1"},
	}}
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, store, fwd)

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
}

func TestLifecycleStartStop(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, 50*time.Millisecond, store, fwd)

	require.NoError(t, e.start(context.Background()))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, e.stop(context.Background()))
}

func TestTickFiresOnInterval(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"issue-1": {Id: "issue-1"},
	}}
	fwd := &mockForwarder{}
	e := newTestEgress(t, 30*time.Millisecond, store, fwd)

	require.NoError(t, e.start(context.Background()))
	require.Eventually(t, func() bool {
		return fwd.sendCount.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond, "expected at least 2 ticks")
	require.NoError(t, e.stop(context.Background()))
}

func TestErrorThenRecovery(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"issue-1": {Id: "issue-1"},
	}}
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, 20*time.Millisecond, store, fwd)

	require.NoError(t, e.start(context.Background()))
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
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

	report := e.buildReport(map[string]*healthplatformpayload.Issue{"a": {Id: "a"}, "b": {Id: "b"}})

	assert.Equal(t, eventType, report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, "agent", report.Service)
	assert.Len(t, report.Issues, 2)
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

// TestTombstoneChannelForwardedOnce verifies that a tombstone written to the
// channel is sent once and consumed; the channel is empty after success.
func TestTombstoneChannelForwardedOnce(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

	// Simulate the store writing a tombstone.
	e.resolvedCh <- &healthplatformpayload.Issue{
		Id: "r-issue",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	}

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
	fwd.mu.Lock()
	require.Len(t, fwd.reports, 1)
	assert.Contains(t, fwd.reports[0].Issues, "r-issue")
	fwd.mu.Unlock()

	// channel must be empty after a successful send
	assert.Empty(t, e.resolvedCh)

	// second tick: no active issues, no tombstones — nothing to send
	e.tick()
	assert.Equal(t, int32(1), fwd.sendCount.Load())
}

// TestTombstoneReturnedOnSendFailure verifies that tombstones are put back into
// the channel when the send fails, so they are retried on the next tick.
func TestTombstoneReturnedOnSendFailure(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, store, fwd)

	e.resolvedCh <- &healthplatformpayload.Issue{Id: "fail-issue"}
	e.tick() // send fails

	assert.Len(t, e.resolvedCh, 1, "tombstone must be returned to channel after failed send")
}

// TestObserverWiredToTombstoneChannel verifies that the observer registered by
// the egress routes resolved issues into the tombstone channel.
func TestObserverWiredToTombstoneChannel(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

	store.mu.Lock()
	require.NotNil(t, store.observer.OnIssueResolved)
	store.observer.OnIssueResolved(&healthplatformpayload.Issue{Id: "from-store"})
	store.mu.Unlock()

	assert.Len(t, e.resolvedCh, 1)
}
