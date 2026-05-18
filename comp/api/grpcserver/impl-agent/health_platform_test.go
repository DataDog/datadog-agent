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
	"google.golang.org/protobuf/types/known/anypb"

	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func serverWithStore(store healthplatformstore.Component) *serverSecure {
	return &serverSecure{healthPlatformStore: option.New(store)}
}

// TestReportHealthIssue_StoresIssue verifies that a valid packed Issue is forwarded to the store.
func TestReportHealthIssue_StoresIssue(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	srv := serverWithStore(storeMock)

	issue := &healthplatformpayload.Issue{Id: "test-issue", Title: "Test Issue", Severity: "high"}
	packed, err := anypb.New(issue)
	require.NoError(t, err)

	_, err = srv.ReportHealthIssue(context.Background(), packed)
	require.NoError(t, err)

	got := storeMock.GetIssue("test-issue")
	require.NotNil(t, got)
	assert.Equal(t, "Test Issue", got.Title)
	assert.Equal(t, "high", got.Severity)
}

// TestReportHealthIssue_StoreUnavailable verifies Unavailable when the store option is empty.
func TestReportHealthIssue_StoreUnavailable(t *testing.T) {
	srv := &serverSecure{healthPlatformStore: option.None[healthplatformstore.Component]()}

	packed, err := anypb.New(&healthplatformpayload.Issue{Id: "x"})
	require.NoError(t, err)

	_, err = srv.ReportHealthIssue(context.Background(), packed)
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

// TestReportHealthIssue_WrongPayloadType verifies InvalidArgument when Any contains a non-Issue proto.
func TestReportHealthIssue_WrongPayloadType(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	wrongPayload := &anypb.Any{
		TypeUrl: "type.googleapis.com/unknown.Type",
		Value:   []byte("not-a-valid-proto"),
	}

	_, err := srv.ReportHealthIssue(context.Background(), wrongPayload)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestReportHealthIssue_EmptyIssueID verifies InvalidArgument when the Issue has no id.
func TestReportHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	packed, err := anypb.New(&healthplatformpayload.Issue{Title: "no id"})
	require.NoError(t, err)

	_, err = srv.ReportHealthIssue(context.Background(), packed)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestResolveHealthIssue_ClearsIssue verifies that an active issue is removed after resolution.
func TestResolveHealthIssue_ClearsIssue(t *testing.T) {
	storeMock := healthplatformmock.Mock(t)
	srv := serverWithStore(storeMock)

	require.NoError(t, storeMock.AcceptIssue(&healthplatformpayload.Issue{Id: "to-resolve", Title: "active"}))
	require.NotNil(t, storeMock.GetIssue("to-resolve"))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.HealthIssueResolve{IssueId: "to-resolve"})
	require.NoError(t, err)
	assert.Nil(t, storeMock.GetIssue("to-resolve"))
}

// TestResolveHealthIssue_StoreUnavailable verifies Unavailable when the store option is empty.
func TestResolveHealthIssue_StoreUnavailable(t *testing.T) {
	srv := &serverSecure{healthPlatformStore: option.None[healthplatformstore.Component]()}

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.HealthIssueResolve{IssueId: "x"})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

// TestResolveHealthIssue_EmptyIssueID verifies InvalidArgument when no issue_id is provided.
func TestResolveHealthIssue_EmptyIssueID(t *testing.T) {
	srv := serverWithStore(healthplatformmock.Mock(t))

	_, err := srv.ResolveHealthIssue(context.Background(), &pb.HealthIssueResolve{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
