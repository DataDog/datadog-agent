// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"context"
	"fmt"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	issueregistrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

func (r *stubRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent { return nil }

func (r *stubRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData { return nil }

// stubIssueRegistry is a minimal issueregistrydef.Component for testing the report path.
type stubIssueRegistry struct {
	templates map[string]issues.Template
}

func (r *stubIssueRegistry) GetTemplate(issueName string) (issues.Template, bool) {
	tmpl, ok := r.templates[issueName]
	return tmpl, ok
}

func (r *stubIssueRegistry) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (r *stubIssueRegistry) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return nil
}

// stubTemplate is a minimal issues.Template for testing.
type stubTemplate struct {
	issueName string
}

func (t *stubTemplate) IssueName() string { return t.issueName }

func (t *stubTemplate) BuildIssue(ctx map[string]string) (*healthplatformpayload.Issue, error) {
	if ctx["fail"] == "true" {
		return nil, fmt.Errorf("build failed")
	}
	return &healthplatformpayload.Issue{
		IssueName: t.issueName,
		Title:     "Built: " + ctx["title"],
		Source:    "template-default-source",
		Tags:      []string{"template-tag"},
	}, nil
}

func serverWithStore(store healthplatformstore.Component) *serverSecure {
	return &serverSecure{healthPlatformStore: option.New(store)}
}

func serverWithStoreAndRegistry(store healthplatformstore.Component, reg remoteagentregistry.Component) *serverSecure {
	return &serverSecure{
		healthPlatformStore: option.New(store),
		remoteAgentRegistry: reg,
	}
}

func serverWithAll(store healthplatformstore.Component, reg remoteagentregistry.Component, issueReg issueregistrydef.Component) *serverSecure {
	return &serverSecure{
		healthPlatformStore: option.New(store),
		remoteAgentRegistry: reg,
		issueRegistry:       option.New(issueReg),
	}
}

func packIssue(t *testing.T, issue *healthplatformpayload.Issue) *anypb.Any {
	t.Helper()
	packed, err := anypb.New(issue)
	require.NoError(t, err)
	return packed
}

func fullIssuePayload(packed *anypb.Any) *pb.ReportHealthIssueRequest_FullIssue {
	return &pb.ReportHealthIssueRequest_FullIssue{FullIssue: packed}
}

// ── ReportHealthIssue / full_issue path ─────────────────────────────────────

func TestReportHealthIssue_StoresIssue(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	srv := serverWithStore(storeMock)

	issue := &healthplatformpayload.Issue{Id: "test-issue", IssueName: "test-issue", Title: "Test Issue", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH}
	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Payload: fullIssuePayload(packIssue(t, issue))})
	require.NoError(t, err)

	got := storeMock.GetIssue("test-issue")
	require.NotNil(t, got)
	assert.Equal(t, "Test Issue", got.Title)
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH, got.Severity)
}

func TestReportHealthIssue_StoreUnavailable(t *testing.T) {
	srv := &serverSecure{healthPlatformStore: option.None[healthplatformstore.Component]()}

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Payload: fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{Id: "x"}))})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestReportHealthIssue_WrongPayloadType(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	wrongPayload := &anypb.Any{TypeUrl: "type.googleapis.com/unknown.Type", Value: []byte("not-a-valid-proto")}
	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Payload: fullIssuePayload(wrongPayload)})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Payload: fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{IssueName: "has-name-no-id"}))})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_EmptyIssueName(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{Payload: fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{Id: "has-id-no-name"}))})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_NoPayload(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_ValidSession(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	reg := &stubRegistry{validSessions: map[string]bool{"sess-123": true}}
	srv := serverWithStoreAndRegistry(storeMock, reg)

	issue := &healthplatformpayload.Issue{Id: "adp-issue", IssueName: "adp-issue", Title: "ADP issue"}
	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "sess-123",
		Payload:              fullIssuePayload(packIssue(t, issue)),
	})
	require.NoError(t, err)
	assert.NotNil(t, storeMock.GetIssue("adp-issue"))
}

func TestReportHealthIssue_InvalidSession(t *testing.T) {
	reg := &stubRegistry{validSessions: map[string]bool{}}
	srv := serverWithStoreAndRegistry(healthplatformmock.Mock(t), reg)

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "stale-session",
		Payload:              fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{Id: "x", IssueName: "x"})),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestReportHealthIssue_SessionWithoutRegistry(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		RemoteAgentSessionId: "some-session",
		Payload:              fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{Id: "x"})),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestReportHealthIssue_NoSessionSubAgent(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	srv := serverWithStore(storeMock)

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: fullIssuePayload(packIssue(t, &healthplatformpayload.Issue{Id: "sysprobe-issue", IssueName: "sysprobe-issue"})),
	})
	require.NoError(t, err)
	assert.NotNil(t, storeMock.GetIssue("sysprobe-issue"))
}

// ── ReportHealthIssue / report path ─────────────────────────────────────────

func TestReportHealthIssue_Report_BuildsViaTemplate(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	issueReg := &stubIssueRegistry{
		templates: map[string]issues.Template{
			"my_issue": &stubTemplate{issueName: "my_issue"},
		},
	}
	srv := &serverSecure{
		healthPlatformStore: option.New[healthplatformstore.Component](storeMock),
		issueRegistry:       option.New[issueregistrydef.Component](issueReg),
	}

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{
			Report: &pb.RemoteIssueReport{
				IssueId:   "my-issue-id",
				IssueName: "my_issue",
				Context:   map[string]string{"title": "Hello"},
				Source:    "system-probe",
				Tags:      []string{"extra-tag"},
			},
		},
	})
	require.NoError(t, err)

	got := storeMock.GetIssue("my-issue-id")
	require.NotNil(t, got)
	assert.Equal(t, "my-issue-id", got.Id)
	assert.Equal(t, "my_issue", got.IssueName)
	assert.Equal(t, "Built: Hello", got.Title)
	assert.Equal(t, "system-probe", got.Source)
	assert.Contains(t, got.Tags, "template-tag")
	assert.Contains(t, got.Tags, "extra-tag")
}

func TestReportHealthIssue_Report_RegistryUnavailable(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t)) // no issueRegistry

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{
			Report: &pb.RemoteIssueReport{IssueId: "x", IssueName: "my_issue"},
		},
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestReportHealthIssue_Report_UnknownIssueName(t *testing.T) {
	issueReg := &stubIssueRegistry{templates: map[string]issues.Template{}}
	srv := &serverSecure{
		healthPlatformStore: option.New[healthplatformstore.Component](healthplatformmock.Mock(t)),
		issueRegistry:       option.New[issueregistrydef.Component](issueReg),
	}

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{
			Report: &pb.RemoteIssueReport{IssueId: "x", IssueName: "unknown_issue"},
		},
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestReportHealthIssue_Report_EmptyIssueID(t *testing.T) {
	issueReg := &stubIssueRegistry{templates: map[string]issues.Template{
		"my_issue": &stubTemplate{issueName: "my_issue"},
	}}
	srv := &serverSecure{
		healthPlatformStore: option.New[healthplatformstore.Component](healthplatformmock.Mock(t)),
		issueRegistry:       option.New[issueregistrydef.Component](issueReg),
	}

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{
			Report: &pb.RemoteIssueReport{IssueName: "my_issue"},
		},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestReportHealthIssue_Report_BuildIssueError(t *testing.T) {
	issueReg := &stubIssueRegistry{templates: map[string]issues.Template{
		"my_issue": &stubTemplate{issueName: "my_issue"},
	}}
	srv := &serverSecure{
		healthPlatformStore: option.New[healthplatformstore.Component](healthplatformmock.Mock(t)),
		issueRegistry:       option.New[issueregistrydef.Component](issueReg),
	}

	_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{
			Report: &pb.RemoteIssueReport{IssueId: "x", IssueName: "my_issue", Context: map[string]string{"fail": "true"}},
		},
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// ── ResolveHealthIssue ───────────────────────────────────────────────────────

func TestResolveHealthIssue_ClearsIssue(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	srv := serverWithStore(storeMock)

	require.NoError(t, storeMock.ReportIssue(&healthplatformpayload.Issue{Id: "to-resolve", IssueName: "to-resolve", Title: "active"}))
	require.NotNil(t, storeMock.GetIssue("to-resolve"))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{IssueId: "to-resolve"})
	require.NoError(t, err)
	assert.Nil(t, storeMock.GetIssue("to-resolve"))
}

func TestResolveHealthIssue_StoreUnavailable(t *testing.T) {
	srv := &serverSecure{healthPlatformStore: option.None[healthplatformstore.Component]()}

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{IssueId: "x"})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestResolveHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestResolveHealthIssue_ValidSession(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
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

func TestResolveHealthIssue_InvalidSession(t *testing.T) {
	reg := &stubRegistry{validSessions: map[string]bool{}}
	srv := serverWithStoreAndRegistry(healthplatformmock.Mock(t), reg)

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{
		RemoteAgentSessionId: "stale",
		IssueId:              "x",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
