// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package egressimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	egressmock "github.com/DataDog/datadog-agent/comp/healthplatform/egress/mock"
	forwardermock "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/mock"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

func TestTickSendsActiveIssues(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1", Title: "Test"}))
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := egressmock.New(t, store, fwd)

	require.NoError(t, e.Tick(context.Background()))

	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "issue-1")
}

func TestTickSkipsWhenEmpty(t *testing.T) {
	var called bool
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		called = true
		return nil
	}))
	e := egressmock.New(t, storemock.New(t), fwd)

	require.NoError(t, e.Tick(context.Background()))

	assert.False(t, called)
}

func TestTickReturnsForwarderError(t *testing.T) {
	store := storemock.New(t, storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}))
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		return assert.AnError
	}))
	e := egressmock.New(t, store, fwd)

	assert.ErrorIs(t, e.Tick(context.Background()), assert.AnError)
}

func TestResolvedIssueSentOnce(t *testing.T) {
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := egressmock.New(t, storemock.New(t), fwd)
	e.AddResolved(&healthplatformpayload.Issue{
		Id: "r-issue",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	})

	require.NoError(t, e.Tick(context.Background()))

	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "r-issue")
	assert.Empty(t, e.Resolved(), "resolved map must be cleared after successful send")

	require.NoError(t, e.Tick(context.Background()))
	assert.Len(t, reports, 1, "second tick must skip: no active or resolved issues")
}

func TestResolvedStaysOnSendFailure(t *testing.T) {
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, _ *healthplatformpayload.HealthReport) error {
		return assert.AnError
	}))
	e := egressmock.New(t, storemock.New(t), fwd)
	e.AddResolved(&healthplatformpayload.Issue{Id: "fail-issue"})

	_ = e.Tick(context.Background())

	assert.Contains(t, e.Resolved(), "fail-issue", "resolved map must be retained after failed send")
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
	e := egressmock.New(t, store, fwd)
	e.AddResolved(&healthplatformpayload.Issue{
		Id: "i:1",
		PersistedIssue: &healthplatformpayload.PersistedIssue{
			State: healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED,
		},
	})

	require.NoError(t, e.Tick(context.Background()))

	require.Len(t, reports, 1)
	sent := reports[0].Issues["i:1"]
	require.NotNil(t, sent)
	assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_NEW, sent.PersistedIssue.GetState(),
		"active NEW entry must win over stale resolved tombstone on recurrence")
}

func TestObserverReceivesResolvedFromStore(t *testing.T) {
	store := storemock.New(t,
		storemock.WithIssue(&healthplatformpayload.Issue{Id: "issue-1"}),
	)
	var reports []*healthplatformpayload.HealthReport
	fwd := forwardermock.New(t, forwardermock.WithSendFunc(func(_ context.Context, r *healthplatformpayload.HealthReport) error {
		reports = append(reports, r)
		return nil
	}))
	e := egressmock.New(t, store, fwd)

	// First tick: issue-1 is active.
	require.NoError(t, e.Tick(context.Background()))
	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].Issues, "issue-1")

	// Store resolves the issue — triggers the observer channel registered by New().
	store.ResolveIssue("issue-1")

	// Second tick: issue-1 now appears as a resolved tombstone.
	require.NoError(t, e.Tick(context.Background()))
	require.Len(t, reports, 2)
	assert.Contains(t, reports[1].Issues, "issue-1")
}
