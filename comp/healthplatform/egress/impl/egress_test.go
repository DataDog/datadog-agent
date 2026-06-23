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
	forwardermock "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

func newTestEgress(t *testing.T, interval time.Duration, store storedef.Component, fwd forwarderdef.Component) *egress {
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

func TestTickSendsActiveIssues(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1", Title: "Test"}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := newTestEgress(t, time.Minute, store, fwd)

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
	e := newTestEgress(t, time.Minute, storemock.New(t), fwd)

	e.tick()

	assert.False(t, called)
}

func TestTickLogsOnForwarderError(t *testing.T) {
	var callCount int32
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		atomic.AddInt32(&callCount, 1)
		return assert.AnError
	}))
	e := newTestEgress(t, time.Minute, store, fwd)

	e.tick()

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
}

func TestLifecycleStartStop(t *testing.T) {
	e := newTestEgress(t, 50*time.Millisecond, storemock.New(t), forwardermock.New(t))

	require.NoError(t, e.start(context.Background()))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, e.stop(context.Background()))
}

func TestTickFiresOnInterval(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var callCount int32
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}))
	e := newTestEgress(t, 30*time.Millisecond, store, fwd)

	require.NoError(t, e.start(context.Background()))
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 2
	}, 2*time.Second, 10*time.Millisecond, "expected at least 2 ticks")
	require.NoError(t, e.stop(context.Background()))
}

func TestErrorThenRecovery(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	var (
		mu          sync.Mutex
		sendErr     error = assert.AnError
		callCount   int32
		successSent []*healthplatformpayload.HealthReport
	)
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		atomic.AddInt32(&callCount, 1)
		mu.Lock()
		defer mu.Unlock()
		if sendErr != nil {
			return sendErr
		}
		successSent = append(successSent, r)
		return nil
	}))
	e := newTestEgress(t, 20*time.Millisecond, store, fwd)

	require.NoError(t, e.start(context.Background()))
	require.Eventually(t, func() bool { return atomic.LoadInt32(&callCount) >= 1 }, 2*time.Second, 5*time.Millisecond)

	mu.Lock()
	sendErr = nil
	mu.Unlock()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(successSent) >= 1
	}, 2*time.Second, 5*time.Millisecond, "expected successful send after error recovery")

	require.NoError(t, e.stop(context.Background()))
}

func TestBuildReport(t *testing.T) {
	e := newTestEgress(t, time.Minute, storemock.New(t), forwardermock.New(t))

	report := e.buildReport(map[string]*healthplatformpayload.Issue{"a": {Id: "a"}, "b": {Id: "b"}})

	assert.Equal(t, eventType, report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, "agent", report.Service)
	assert.Len(t, report.Issues, 2)
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

func TestResolvedIssueSentOnce(t *testing.T) {
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := newTestEgress(t, time.Minute, storemock.New(t), fwd)
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

func TestResolvedStaysOnSendFailure(t *testing.T) {
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		return assert.AnError
	}))
	e := newTestEgress(t, time.Minute, storemock.New(t), fwd)
	e.resolved["fail-issue"] = &healthplatformpayload.Issue{Id: "fail-issue"}

	e.tick()

	assert.Contains(t, e.resolved, "fail-issue", "resolved map must be retained after failed send")
}

func TestActiveWinsOverResolvedOnRecurrence(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{
		Id: "i:1",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_NEW,
		},
	}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := newTestEgress(t, time.Minute, store, fwd)
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
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_NEW, sent.PersistedIssue.GetState(),
		"active NEW entry must win over stale resolved tombstone on recurrence")
}

func TestObserverWiresResolvedCh(t *testing.T) {
	store := storemock.New(t)
	e := newTestEgress(t, time.Minute, store, forwardermock.New(t))

	store.RegisterIssuesObserver(storedef.IssuesObserver{
		ResolvedCh: e.resolvedCh,
	})

	store.Observer().ResolvedCh <- &healthplatformpayload.Issue{Id: "resolved"}

	assert.Len(t, e.resolvedCh, 1)
}
