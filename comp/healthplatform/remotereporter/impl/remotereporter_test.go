// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotereporterimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	mock_core "github.com/DataDog/datadog-agent/pkg/proto/pbgo/mocks/core"
)

func testIssue(id, title string) *healthplatformpayload.Issue {
	return &healthplatformpayload.Issue{Id: id, IssueName: "Test Issue", Title: title}
}

func expectReport(client *mock_core.MockAgentSecureClient, id string, err error) *gomock.Call {
	return client.EXPECT().ReportHealthIssue(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *pbcore.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			if req.GetIssue().GetId() != id {
				return nil, errors.New("unexpected issue id " + req.GetIssue().GetId())
			}
			return &emptypb.Empty{}, err
		})
}

func expectResolve(client *mock_core.MockAgentSecureClient, id string, err error) *gomock.Call {
	return client.EXPECT().ResolveHealthIssue(gomock.Any(), &pbcore.ResolveHealthIssueRequest{IssueId: id}).Return(&emptypb.Empty{}, err)
}

func TestReportIssueValidation(t *testing.T) {
	r := newReporter(mock_core.NewMockAgentSecureClient(gomock.NewController(t)), logmock.New(t))

	require.Error(t, r.ReportIssue(nil))
	require.Error(t, r.ReportIssue(&healthplatformpayload.Issue{IssueName: "Test Issue"}))
	require.Error(t, r.ReportIssue(&healthplatformpayload.Issue{Id: "id"}))
	// Rejected issues are not sent: the mock fails on any call.
	require.NoError(t, r.flush(context.Background()))
}

func TestFlushDeliversAndDeduplicates(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))
	ctx := context.Background()

	expectReport(client, "a", nil).Times(1)
	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	require.NoError(t, r.flush(ctx))

	// Same content as the acknowledged issue: nothing to send.
	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	require.NoError(t, r.flush(ctx))

	// Changed content is sent again.
	expectReport(client, "a", nil).Times(1)
	require.NoError(t, r.ReportIssue(testIssue("a", "new title")))
	require.NoError(t, r.flush(ctx))

	expectResolve(client, "a", nil).Times(1)
	r.ResolveIssue("a")
	require.NoError(t, r.flush(ctx))

	// Already resolved: nothing to send.
	r.ResolveIssue("a")
	require.NoError(t, r.flush(ctx))
}

func TestResolveUnknownIssueIsSentOnce(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))

	// The core agent may still hold the issue from a previous run of this process.
	expectResolve(client, "stale", nil).Times(1)
	r.ResolveIssue("stale")
	r.ResolveIssue("stale")
	require.NoError(t, r.flush(context.Background()))

	r.ResolveIssue("stale")
	require.NoError(t, r.flush(context.Background()))
}

func TestFlushKeepsPendingOnFailure(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))
	ctx := context.Background()

	expectReport(client, "a", errors.New("unavailable")).Times(1)
	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	require.Error(t, r.flush(ctx))

	expectReport(client, "a", nil).Times(1)
	require.NoError(t, r.flush(ctx))
	require.NoError(t, r.flush(ctx))
}

func TestLatestStateWins(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))

	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	r.ResolveIssue("a")

	expectResolve(client, "a", nil).Times(1)
	require.NoError(t, r.flush(context.Background()))
}

func TestRequeueActive(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))
	ctx := context.Background()

	expectReport(client, "active", nil).Times(2)
	expectResolve(client, "resolved", nil).Times(1)
	require.NoError(t, r.ReportIssue(testIssue("active", "title")))
	r.ResolveIssue("resolved")
	require.NoError(t, r.flush(ctx))

	// Only the active issue is sent again.
	r.requeueActive()
	require.NoError(t, r.flush(ctx))
}

func TestRunDeliversInBackground(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))

	delivered := make(chan struct{})
	expectReport(client, "a", nil).Times(1).Do(func(context.Context, *pbcore.ReportHealthIssueRequest, ...grpc.CallOption) {
		close(delivered)
	})
	r.start()
	defer r.stop(context.Background())

	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "issue was not delivered")
	}
}

func TestStopFlushesPending(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))

	// The first attempt fails, so the issue is still pending when stopping,
	// well before the background retry is due.
	firstAttempt := make(chan struct{})
	gomock.InOrder(
		expectReport(client, "a", errors.New("unavailable")).Do(func(context.Context, *pbcore.ReportHealthIssueRequest, ...grpc.CallOption) {
			close(firstAttempt)
		}),
		expectReport(client, "a", nil),
	)
	r.start()

	require.NoError(t, r.ReportIssue(testIssue("a", "title")))
	select {
	case <-firstAttempt:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "issue was not sent")
	}
	r.stop(context.Background())
}

func TestStopIsBounded(t *testing.T) {
	client := mock_core.NewMockAgentSecureClient(gomock.NewController(t))
	r := newReporter(client, logmock.New(t))

	// An unresponsive core agent: every call blocks until its context is done.
	client.EXPECT().ReportHealthIssue(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *pbcore.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}).AnyTimes()
	r.start()
	require.NoError(t, r.ReportIssue(testIssue("a", "title")))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r.stop(ctx)
	assert.Less(t, time.Since(start), 2*time.Second)
}
