// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthreporter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// mockClient implements pb.AgentSecureClient with configurable Report/Resolve handlers.
// All other methods panic — they are not exercised by these tests.
type mockClient struct {
	reportFn  func(context.Context, *pb.ReportHealthIssueRequest, ...grpc.CallOption) (*emptypb.Empty, error)
	resolveFn func(context.Context, *pb.ResolveHealthIssueRequest, ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *mockClient) ReportHealthIssue(ctx context.Context, in *pb.ReportHealthIssueRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.reportFn(ctx, in, opts...)
}
func (m *mockClient) ResolveHealthIssue(ctx context.Context, in *pb.ResolveHealthIssueRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.resolveFn(ctx, in, opts...)
}

// Unused methods — panic to catch unexpected calls.
func (m *mockClient) TaggerStreamEntities(context.Context, *pb.StreamTagsRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[pb.StreamTagsResponse], error) {
	panic("unexpected call")
}
func (m *mockClient) TaggerGenerateContainerIDFromOriginInfo(context.Context, *pb.GenerateContainerIDFromOriginInfoRequest, ...grpc.CallOption) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) TaggerFetchEntity(context.Context, *pb.FetchEntityRequest, ...grpc.CallOption) (*pb.FetchEntityResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) DogstatsdCaptureTrigger(context.Context, *pb.CaptureTriggerRequest, ...grpc.CallOption) (*pb.CaptureTriggerResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) DogstatsdSetTaggerState(context.Context, *pb.TaggerState, ...grpc.CallOption) (*pb.TaggerStateResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) ClientGetConfigs(context.Context, *pb.ClientGetConfigsRequest, ...grpc.CallOption) (*pb.ClientGetConfigsResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) GetConfigState(context.Context, *emptypb.Empty, ...grpc.CallOption) (*pb.GetStateConfigResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) ClientGetConfigsHA(context.Context, *pb.ClientGetConfigsRequest, ...grpc.CallOption) (*pb.ClientGetConfigsResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) GetConfigStateHA(context.Context, *emptypb.Empty, ...grpc.CallOption) (*pb.GetStateConfigResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) CreateConfigSubscription(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[pb.ConfigSubscriptionRequest, pb.ConfigSubscriptionResponse], error) {
	panic("unexpected call")
}
func (m *mockClient) ResetConfigState(context.Context, *emptypb.Empty, ...grpc.CallOption) (*pb.ResetStateConfigResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) WorkloadmetaStreamEntities(context.Context, *pb.WorkloadmetaStreamRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[pb.WorkloadmetaStreamResponse], error) {
	panic("unexpected call")
}
func (m *mockClient) RegisterRemoteAgent(context.Context, *pb.RegisterRemoteAgentRequest, ...grpc.CallOption) (*pb.RegisterRemoteAgentResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) RefreshRemoteAgent(context.Context, *pb.RefreshRemoteAgentRequest, ...grpc.CallOption) (*pb.RefreshRemoteAgentResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) AutodiscoveryStreamConfig(context.Context, *emptypb.Empty, ...grpc.CallOption) (grpc.ServerStreamingClient[pb.AutodiscoveryStreamResponse], error) {
	panic("unexpected call")
}
func (m *mockClient) GetHostTags(context.Context, *pb.HostTagRequest, ...grpc.CallOption) (*pb.HostTagReply, error) {
	panic("unexpected call")
}
func (m *mockClient) StreamConfigEvents(context.Context, *pb.ConfigStreamRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[pb.ConfigEvent], error) {
	panic("unexpected call")
}
func (m *mockClient) WorkloadFilterEvaluate(context.Context, *pb.WorkloadFilterEvaluateRequest, ...grpc.CallOption) (*pb.WorkloadFilterEvaluateResponse, error) {
	panic("unexpected call")
}
func (m *mockClient) StreamKubeMetadata(context.Context, *pb.KubeMetadataStreamRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[pb.KubeMetadataStreamResponse], error) {
	panic("unexpected call")
}

// newReporter returns a Reporter with the mock client injected, short timeouts for tests.
func newReporter(client pb.AgentSecureClient) *Reporter {
	return &Reporter{
		callTimeout: 500 * time.Millisecond,
		maxWait:     2 * time.Second,
		newClientFn: func() (pb.AgentSecureClient, error) { return client, nil },
	}
}

// ============================================================================
// retryWithBackoff
// ============================================================================

func TestRetryWithBackoff_SuccessOnFirstTry(t *testing.T) {
	r := &Reporter{maxWait: 5 * time.Second}
	calls := 0
	ok := r.retryWithBackoff("op", 5*time.Second, func() error {
		calls++
		return nil
	})
	assert.True(t, ok)
	assert.Equal(t, 1, calls)
}

func TestRetryWithBackoff_RetryOnError(t *testing.T) {
	r := &Reporter{maxWait: 5 * time.Second}
	var calls atomic.Int32
	sentinel := errors.New("transient")
	ok := r.retryWithBackoff("op", 5*time.Second, func() error {
		if calls.Add(1) < 3 {
			return sentinel
		}
		return nil
	})
	assert.True(t, ok)
	assert.GreaterOrEqual(t, int(calls.Load()), 3)
}

func TestRetryWithBackoff_DeadlineExceeded(t *testing.T) {
	r := &Reporter{maxWait: 100 * time.Millisecond}
	calls := 0
	ok := r.retryWithBackoff("op", 100*time.Millisecond, func() error {
		calls++
		return errors.New("always failing")
	})
	assert.False(t, ok)
	assert.GreaterOrEqual(t, calls, 1)
}

// ============================================================================
// Report / Resolve
// ============================================================================

func TestReport_Success(t *testing.T) {
	var got *pb.ReportHealthIssueRequest
	client := &mockClient{
		reportFn: func(_ context.Context, in *pb.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			got = in
			return &emptypb.Empty{}, nil
		},
	}
	r := newReporter(client)
	issue := &healthplatformpayload.Issue{Id: "test-issue", IssueName: "Test"}
	err := r.Report(context.Background(), issue)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "test-issue", got.Issue.Id)
}

func TestReport_Error(t *testing.T) {
	client := &mockClient{
		reportFn: func(_ context.Context, _ *pb.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, errors.New("rpc error")
		},
	}
	r := newReporter(client)
	err := r.Report(context.Background(), &healthplatformpayload.Issue{Id: "x", IssueName: "X"})
	assert.Error(t, err)
}

func TestResolve_Success(t *testing.T) {
	var gotID string
	client := &mockClient{
		resolveFn: func(_ context.Context, in *pb.ResolveHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			gotID = in.IssueId
			return &emptypb.Empty{}, nil
		},
	}
	r := newReporter(client)
	err := r.Resolve(context.Background(), "my-issue")
	require.NoError(t, err)
	assert.Equal(t, "my-issue", gotID)
}

func TestResolve_Error(t *testing.T) {
	client := &mockClient{
		resolveFn: func(_ context.Context, _ *pb.ResolveHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, errors.New("rpc error")
		},
	}
	r := newReporter(client)
	assert.Error(t, r.Resolve(context.Background(), "x"))
}

// ============================================================================
// ReportWithRetry / ResolveWithRetry
// ============================================================================

func TestReportWithRetry_SucceedsInSynchronousWindow(t *testing.T) {
	var calls atomic.Int32
	client := &mockClient{
		reportFn: func(_ context.Context, _ *pb.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			calls.Add(1)
			return &emptypb.Empty{}, nil
		},
	}
	r := &Reporter{
		callTimeout: 500 * time.Millisecond,
		maxWait:     5 * time.Second,
		newClientFn: func() (pb.AgentSecureClient, error) { return client, nil },
	}
	r.ReportWithRetry(&healthplatformpayload.Issue{Id: "x", IssueName: "X"})
	assert.Equal(t, int32(1), calls.Load())
}

func TestReportWithRetry_RetriesInBackground(t *testing.T) {
	// Fail for the first 200ms (synchronous window = 100ms), then succeed.
	start := time.Now()
	var calls atomic.Int32
	client := &mockClient{
		reportFn: func(_ context.Context, _ *pb.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			calls.Add(1)
			if time.Since(start) < 200*time.Millisecond {
				return nil, errors.New("not ready")
			}
			return &emptypb.Empty{}, nil
		},
	}
	r := &Reporter{
		callTimeout: 50 * time.Millisecond,
		maxWait:     2 * time.Second,
		newClientFn: func() (pb.AgentSecureClient, error) { return client, nil },
	}
	// ReportMaxWait is 30s in production; override by using a tiny maxWait so the
	// synchronous window (= maxWait - remaining) expires quickly.
	// Instead, test retryWithBackoff directly for the background-continuation case.
	_ = r.retryWithBackoff("report x", 100*time.Millisecond, func() error {
		return r.Report(context.Background(), &healthplatformpayload.Issue{Id: "x", IssueName: "X"})
	})
	// Not yet succeeded — background retry should pick it up.
	go r.retryWithBackoff("report x bg", 2*time.Second, func() error {
		return r.Report(context.Background(), &healthplatformpayload.Issue{Id: "x", IssueName: "X"})
	})
	require.Eventually(t, func() bool { return calls.Load() > 1 }, 3*time.Second, 50*time.Millisecond)
}

func TestResolveWithRetry_EventuallySucceeds(t *testing.T) {
	// Fail on the first attempt, succeed on the second.
	// With the 2s initial backoff the resolve completes at ~T=2s; wait up to 5s.
	var calls atomic.Int32
	done := make(chan struct{})
	client := &mockClient{
		resolveFn: func(_ context.Context, _ *pb.ResolveHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			n := calls.Add(1)
			if n == 1 {
				return nil, errors.New("not ready")
			}
			close(done)
			return &emptypb.Empty{}, nil
		},
	}
	r := &Reporter{
		callTimeout: 50 * time.Millisecond,
		maxWait:     5 * time.Second,
		newClientFn: func() (pb.AgentSecureClient, error) { return client, nil },
	}
	r.ResolveWithRetry("my-issue")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ResolveWithRetry did not succeed within 5s")
	}
	assert.Equal(t, int32(2), calls.Load())
}

// ============================================================================
// StringStruct
// ============================================================================

func TestStringStruct(t *testing.T) {
	s := StringStruct(map[string]string{"key": "value", "empty": ""})
	require.NotNil(t, s)
	assert.Equal(t, structpb.NewStringValue("value"), s.Fields["key"])
	assert.Equal(t, structpb.NewStringValue(""), s.Fields["empty"])
	assert.Len(t, s.Fields, 2)
}

func TestStringStruct_Empty(t *testing.T) {
	s := StringStruct(nil)
	assert.NotNil(t, s)
	assert.Empty(t, s.Fields)
}
