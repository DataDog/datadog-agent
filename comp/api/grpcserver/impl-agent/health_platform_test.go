// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"context"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// stubRegistry is a minimal remoteagentregistry.Component for testing session validation.
type stubRegistry struct {
	validSessions map[string]bool
}

func (r *stubRegistry) RegisterRemoteAgent(_ *remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "", 0, nil
}

func (r *stubRegistry) RefreshRemoteAgent(sessionID string) bool {
	return r.validSessions[sessionID]
}

func (r *stubRegistry) ReportRemoteAgentEvent(_ string, _ []remoteagentregistry.RemoteAgentEvent) error {
	return nil
}

func (r *stubRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent { return nil }

func (r *stubRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData { return nil }

func serverWithStore(store healthplatformstore.Component) *serverSecure {
	return &serverSecure{healthPlatformStore: store}
}

func serverWithStoreAndRegistry(store healthplatformstore.Component, reg remoteagentregistry.Component) *serverSecure {
	return &serverSecure{
		healthPlatformStore: store,
		remoteAgentRegistry: reg,
	}
}

// ── ReportHealthIssue ────────────────────────────────────────────────────────

func TestReportHealthIssue_StoresIssue(t *testing.T) {
	storeMock := healthplatformmock.New(t)
	srv := serverWithStore(storeMock)

	issue := &healthplatformpayload.Issue{Id: "test-issue", IssueName: "test-issue", Title: "Test Issue", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH}
	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Issue: issue})
	require.NoError(t, err)

	got := storeMock.GetIssue("test-issue")
	require.NotNil(t, got)
	assert.Equal(t, "Test Issue", got.Title)
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH, got.Severity)
}

func TestReportHealthIssue_NilIssue(t *testing.T) {
	srv := serverWithStore(healthplatformmock.New(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.New(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Issue: &healthplatformpayload.Issue{Title: "no id"}})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_EmptyIssueName(t *testing.T) {
	srv := serverWithStore(healthplatformmock.New(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Issue: &healthplatformpayload.Issue{Id: "has-id-no-name"}})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestReportHealthIssue_ValidSession verifies that a registered remote agent with a
// valid session ID can report issues successfully.
func TestReportHealthIssue_ValidSession(t *testing.T) {
	storeMock := healthplatformmock.New(t)
	reg := &stubRegistry{validSessions: map[string]bool{"sess-123": true}}
	srv := serverWithStoreAndRegistry(storeMock, reg)

	issue := &healthplatformpayload.Issue{Id: "adp-issue", IssueName: "adp-issue", Title: "ADP issue"}
	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "sess-123",
		Issue:                issue,
	})
	require.NoError(t, err)
	assert.NotNil(t, storeMock.GetIssue("adp-issue"))
}

// TestReportHealthIssue_InvalidSession verifies that a stale or unknown session ID
// is rejected with UNAUTHENTICATED.
func TestReportHealthIssue_InvalidSession(t *testing.T) {
	reg := &stubRegistry{validSessions: map[string]bool{}}
	srv := serverWithStoreAndRegistry(healthplatformmock.New(t), reg)

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "stale-session",
		Issue:                &healthplatformpayload.Issue{Id: "x", IssueName: "x"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// TestReportHealthIssue_SessionWithoutRegistry verifies that supplying a session ID
// when the registry is not wired returns Unavailable.
func TestReportHealthIssue_SessionWithoutRegistry(t *testing.T) {
	srv := serverWithStore(healthplatformmock.New(t)) // no registry

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "some-session",
		Issue:                &healthplatformpayload.Issue{Id: "x"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

// TestReportHealthIssue_NoSessionSubAgent verifies that sub-agents (no session ID)
// can report issues without a registry.
func TestReportHealthIssue_NoSessionSubAgent(t *testing.T) {
	storeMock := healthplatformmock.New(t)
	srv := serverWithStore(storeMock) // no registry — fine for sub-agents

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Issue: &healthplatformpayload.Issue{Id: "sysprobe-issue", IssueName: "sysprobe-issue"},
	})
	require.NoError(t, err)
	assert.NotNil(t, storeMock.GetIssue("sysprobe-issue"))
}

// ── ResolveHealthIssue ───────────────────────────────────────────────────────

func TestResolveHealthIssue_ClearsIssue(t *testing.T) {
	storeMock := healthplatformmock.New(t)
	srv := serverWithStore(storeMock)

	require.NoError(t, storeMock.ReportIssue(&healthplatformpayload.Issue{Id: "to-resolve", IssueName: "to-resolve", Title: "active"}))
	require.NotNil(t, storeMock.GetIssue("to-resolve"))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{IssueId: "to-resolve"})
	require.NoError(t, err)
	assert.Nil(t, storeMock.GetIssue("to-resolve"))
}

func TestResolveHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.New(t))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestResolveHealthIssue_ValidSession verifies that a registered remote agent with a
// valid session can resolve its own issues.
func TestResolveHealthIssue_ValidSession(t *testing.T) {
	storeMock := healthplatformmock.New(t)
	reg := &stubRegistry{validSessions: map[string]bool{"sess-abc": true}}
	srv := serverWithStoreAndRegistry(storeMock, reg)

	require.NoError(t, storeMock.ReportIssue(&healthplatformpayload.Issue{Id: "adp-resolved", IssueName: "adp-resolved"}))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{
		RemoteAgentSessionId: "sess-abc",
		IssueId:              "adp-resolved",
	})
	require.NoError(t, err)
	assert.Nil(t, storeMock.GetIssue("adp-resolved"))
}

// TestResolveHealthIssue_InvalidSession verifies that a stale session is rejected.
func TestResolveHealthIssue_InvalidSession(t *testing.T) {
	reg := &stubRegistry{validSessions: map[string]bool{}}
	srv := serverWithStoreAndRegistry(healthplatformmock.New(t), reg)

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{
		RemoteAgentSessionId: "stale",
		IssueId:              "x",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
