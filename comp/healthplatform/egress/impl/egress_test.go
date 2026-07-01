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

// mockStore satisfies storedef.Component; captures the registered observer and
// returns configurable active issues from GetAllIssues.
type mockStore struct {
	mu       sync.Mutex
	observer storedef.IssuesObserver
	issues   map[string]*healthplatformpayload.Issue
}

func (m *mockStore) RegisterIssuesObserver(obs storedef.IssuesObserver) {
	m.mu.Lock()
	m.observer = obs
	m.mu.Unlock()
}

func (m *mockStore) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.issues), m.issues
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
	return &egress{
		log:         logmock.New(t),
		interval:    interval,
		hostname:    "test-host",
		agentFlavor: "agent",
		store:       store,
		forwarder:   fwd,
		resolvedCh:  make(chan *healthplatformpayload.Issue, resolvedChBuf),
		resolved:    make(map[string]*healthplatformpayload.Issue),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

func newEmptyStore() *mockStore {
	return &mockStore{issues: make(map[string]*healthplatformpayload.Issue)}
}

func TestTickSendsActiveIssues(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"issue-1": {Id: "issue-1", Title: "Test"},
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
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, newEmptyStore(), fwd)

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
	fwd := &mockForwarder{}
	e := newTestEgress(t, 50*time.Millisecond, newEmptyStore(), fwd)

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
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, newEmptyStore(), fwd)

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
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, newEmptyStore(), fwd)
	e.resolved["r-issue"] = &healthplatformpayload.Issue{
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
	assert.Empty(t, e.resolved, "resolved map must be cleared after successful send")

	e.tick()
	assert.Equal(t, int32(1), fwd.sendCount.Load(), "second tick must skip: no active or resolved issues")
}

// TestResolvedStaysOnSendFailure verifies resolved tombstones are retained when send fails.
func TestResolvedStaysOnSendFailure(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, newEmptyStore(), fwd)
	e.resolved["fail-issue"] = &healthplatformpayload.Issue{Id: "fail-issue"}

	e.tick()

	assert.Contains(t, e.resolved, "fail-issue", "resolved map must be retained after failed send")
}

// TestActiveWinsOverResolvedOnRecurrence verifies that an ACTIVE entry takes precedence over
// a stale resolved tombstone for the same ID (issue recurred after being resolved).
func TestActiveWinsOverResolvedOnRecurrence(t *testing.T) {
	store := &mockStore{issues: map[string]*healthplatformpayload.Issue{
		"i:1": {Id: "i:1", PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE,
		}},
	}}
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)
	e.resolved["i:1"] = &healthplatformpayload.Issue{
		Id: "i:1",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	}

	e.tick()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	require.Len(t, fwd.reports, 1)
	sent := fwd.reports[0].Issues["i:1"]
	require.NotNil(t, sent)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE, sent.PersistedIssue.GetState(),
		"active entry must win over stale resolved tombstone on recurrence")
}

// TestObserverWiresResolvedCh verifies that RegisterIssuesObserver wires resolvedCh into egress.
func TestObserverWiresResolvedCh(t *testing.T) {
	store := newEmptyStore()
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, store, fwd)

	store.RegisterIssuesObserver(storedef.IssuesObserver{
		ResolvedCh: e.resolvedCh,
	})

	store.mu.Lock()
	store.observer.ResolvedCh <- &healthplatformpayload.Issue{Id: "resolved"}
	store.mu.Unlock()

	assert.Len(t, e.resolvedCh, 1)
}
