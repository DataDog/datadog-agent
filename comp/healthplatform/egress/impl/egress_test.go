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

// mockStore satisfies storedef.Component for egress unit tests.
type mockStore struct {
	mu  sync.Mutex
	cbs storedef.EgressCallbacks
}

func (m *mockStore) SetEgressCallbacks(cbs storedef.EgressCallbacks) {
	m.mu.Lock()
	m.cbs = cbs
	m.mu.Unlock()
}

func (m *mockStore) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	return 0, nil
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

func newTestEgress(t *testing.T, interval time.Duration, fwd *mockForwarder) *egress {
	t.Helper()
	return &egress{
		log:         logmock.New(t),
		interval:    interval,
		hostname:    "test-host",
		agentFlavor: "agent",
		forwarder:   fwd,
		active:      make(map[string]*healthplatformpayload.Issue),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

func TestTickSendsReport(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)
	e.active["issue-1"] = &healthplatformpayload.Issue{Id: "issue-1", Title: "Test", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH}

	e.tick()

	assert.Equal(t, int32(1), fwd.sendCount.Load())
	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	require.Len(t, fwd.reports, 1)
	assert.Contains(t, fwd.reports[0].Issues, "issue-1")
	assert.Equal(t, "test-host", fwd.reports[0].Host.Hostname)
	assert.Equal(t, eventType, fwd.reports[0].EventType)
}

func TestTickSkipsEmptyQueues(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.tick()

	assert.Equal(t, int32(0), fwd.sendCount.Load())
}

func TestTickLogsOnForwarderError(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, fwd)
	e.active["issue-1"] = &healthplatformpayload.Issue{Id: "issue-1"}

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
	e.active["issue-1"] = &healthplatformpayload.Issue{Id: "issue-1"}

	require.NoError(t, e.start(context.Background()))

	require.Eventually(t, func() bool {
		return fwd.sendCount.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond, "expected at least 2 ticks")

	require.NoError(t, e.stop(context.Background()))
}

func TestErrorThenRecovery(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, 20*time.Millisecond, fwd)
	e.active["issue-1"] = &healthplatformpayload.Issue{Id: "issue-1"}

	require.NoError(t, e.start(context.Background()))

	require.Eventually(t, func() bool {
		return fwd.sendCount.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond)

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

	issues := map[string]*healthplatformpayload.Issue{
		"a": {Id: "a"},
		"b": {Id: "b"},
	}
	report := e.buildReport(issues)

	assert.Equal(t, eventType, report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, "agent", report.Service)
	assert.Len(t, report.Issues, 2)
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

func TestOnReportIssueFeedsTickPayload(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.onReportIssue(&healthplatformpayload.Issue{Id: "cb-issue"})

	e.tick()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	require.Len(t, fwd.reports, 1)
	assert.Contains(t, fwd.reports[0].Issues, "cb-issue")
}

func TestOnResolveIssueForwardedOnce(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.onReportIssue(&healthplatformpayload.Issue{Id: "r-issue", IssueName: "t"})
	e.onResolveIssue(&healthplatformpayload.Issue{
		Id:        "r-issue",
		IssueName: "t",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	})

	// First tick: tombstone forwarded.
	e.tick()
	assert.Equal(t, int32(1), fwd.sendCount.Load())
	fwd.mu.Lock()
	require.Len(t, fwd.reports, 1)
	iss := fwd.reports[0].Issues["r-issue"]
	require.NotNil(t, iss)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED, iss.PersistedIssue.GetState())
	fwd.mu.Unlock()

	e.pendingMu.Lock()
	assert.Empty(t, e.toSendOnce, "toSendOnce must be trimmed after a successful send")
	e.pendingMu.Unlock()

	// second tick: active is empty and toSendOnce was trimmed — nothing to send
	e.tick()
	assert.Equal(t, int32(1), fwd.sendCount.Load(), "tombstone must not be re-sent")
}

func TestResolvedTombstoneKeptOnSendFailure(t *testing.T) {
	fwd := &mockForwarder{sendErr: assert.AnError}
	e := newTestEgress(t, time.Minute, fwd)

	e.onResolveIssue(&healthplatformpayload.Issue{
		Id: "fail-issue",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	})

	e.tick() // send fails

	e.pendingMu.Lock()
	assert.Len(t, e.toSendOnce, 1, "toSendOnce must not be trimmed after a failed send")
	e.pendingMu.Unlock()
}

// TestTombstoneAddedDuringSendIsRetained verifies the slice-trim invariant:
// tombstones appended to toSendOnce after the snapshot but before the trim survive.
func TestTombstoneAddedDuringSendIsRetained(t *testing.T) {
	fwd := &mockForwarder{}
	e := newTestEgress(t, time.Minute, fwd)

	e.onResolveIssue(&healthplatformpayload.Issue{Id: "first"})
	e.tick() // snapshots len=1, trims 1

	e.onResolveIssue(&healthplatformpayload.Issue{Id: "second"})

	e.pendingMu.Lock()
	assert.Len(t, e.toSendOnce, 1, "second tombstone must survive the trim")
	assert.Equal(t, "second", e.toSendOnce[0].Id)
	e.pendingMu.Unlock()
}

func TestCallbacksRegisteredWithStore(t *testing.T) {
	store := &mockStore{}
	fwd := &mockForwarder{}

	e := newTestEgress(t, time.Minute, fwd)
	store.SetEgressCallbacks(storedef.EgressCallbacks{
		OnReportIssue:  e.onReportIssue,
		OnResolveIssue: e.onResolveIssue,
	})

	store.mu.Lock()
	require.NotNil(t, store.cbs.OnReportIssue)
	require.NotNil(t, store.cbs.OnResolveIssue)
	store.mu.Unlock()

	store.cbs.OnReportIssue(&healthplatformpayload.Issue{Id: "wired-issue"})

	e.pendingMu.Lock()
	_, ok := e.active["wired-issue"]
	e.pendingMu.Unlock()
	assert.True(t, ok)
}
