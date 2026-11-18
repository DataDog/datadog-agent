// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	remotequeriesimpl "github.com/DataDog/datadog-agent/comp/remotequeries/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestRemoteQueryExecuteStreamReturnsSanitizedUnavailableWhenServiceMissing(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{}).RemoteQueryExecuteStream(&pb.RemoteQueryExecuteRequest{}, stream)

	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, "executor_unavailable", stream.chunks[0].GetEvent().GetError().GetCode())
	assert.True(t, stream.chunks[1].GetFinal())
}

func TestRemoteQueryExecuteStreamRejectsInvalidRequestWithTerminalErrorEvent(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{remoteQueries: remoteQueriesServiceForTest(t)}).RemoteQueryExecuteStream(
		&pb.RemoteQueryExecuteRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
			Query:       "SELECT 1 AS value",
		}, stream)

	// A request without the backend-injected delivery fails closed as a terminal
	// error event; the final chunk still terminates the stream.
	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.Equal(t, remotequeriesimpl.RemoteQueryStatusInvalidRequest, stream.chunks[0].GetEvent().GetError().GetCode())
	assert.Equal(t, "result_delivery is required", stream.chunks[0].GetEvent().GetError().GetMessage())
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	assert.True(t, stream.chunks[1].GetFinal())
}

func remoteQueriesServiceForTest(t *testing.T) *remotequeriesimpl.RemoteQueryExecuteService {
	t.Helper()
	return remotequeriesimpl.NewRemoteQueryExecuteService(nil, true, nil)
}

func validRemoteQueryResultDeliveryProto() *pb.RemoteQueryResultDelivery {
	return &pb.RemoteQueryResultDelivery{
		RunId:    "run-proof",
		TaskId:   "task-proof",
		UploadId: "upload-proof",
		Limits: &pb.RemoteQueryUploadLimits{
			MaxFileBytes:   128 << 20,
			MaxResultBytes: 100 << 30,
			MaxRowBytes:    16 << 20,
			MaxColumns:     1024,
			MaxSchemaBytes: 1 << 20,
			MaxPages:       128,
			TimeoutMs:      30000,
		},
	}
}

func TestRemoteQueryExecuteRequestFromProtoPreservesPagedJSONContract(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "LOCALHOST.", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		IncludeSchema:  true,
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})

	require.NoError(t, err)
	assert.Equal(t, "postgres", req.Integration)
	assert.Equal(t, "localhost", req.Target.Host)
	assert.Equal(t, 5432, req.Target.Port)
	assert.Equal(t, "postgres", req.Target.DBName)
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", req.Query)
	assert.True(t, req.IncludeSchema)
	require.NotNil(t, req.ResultDelivery)
	assert.Equal(t, "run-proof", req.ResultDelivery.RunID)
	assert.Equal(t, "task-proof", req.ResultDelivery.TaskID)
	assert.Equal(t, "upload-proof", req.ResultDelivery.UploadID)
	require.NotNil(t, req.ResultDelivery.Limits)
	assert.Equal(t, &remotequeriesimpl.RemoteQueryUploadLimits{
		MaxFileBytes:   128 << 20,
		MaxResultBytes: 100 << 30,
		MaxRowBytes:    16 << 20,
		MaxColumns:     1024,
		MaxSchemaBytes: 1 << 20,
		MaxPages:       128,
		TimeoutMs:      30000,
	}, req.ResultDelivery.Limits)
}

func TestRemoteQueryExecuteRequestFromProtoPreservesDatabaseInstanceTarget(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{DatabaseInstance: "rq-proof-a1-db1"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})

	require.NoError(t, err)
	assert.Equal(t, "rq-proof-a1-db1", req.Target.DatabaseInstance)
	assert.Empty(t, req.Target.Host)
	assert.Zero(t, req.Target.Port)
	assert.Empty(t, req.Target.DBName)
}

// TestRemoteQueryExecuteRequestFromProtoPreserves100GiBInt64Fidelity proves the 100 GiB
// result cap survives the AgentSecure proto boundary and the typed request without loss.
// The limit fields are int64 so the backend-owned 100 GiB cap is representable without
// overflow, and a value one byte above the cap fails closed rather than truncating.
func TestRemoteQueryExecuteRequestFromProtoPreserves100GiBInt64Fidelity(t *testing.T) {
	const hundredGiB = int64(100) << 30
	const hundredGiBPlusOne = hundredGiB + 1

	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})
	require.NoError(t, err)
	require.NotNil(t, req.ResultDelivery.Limits)
	assert.Equal(t, hundredGiB, req.ResultDelivery.Limits.MaxResultBytes)

	overflowProto := validRemoteQueryResultDeliveryProto()
	overflowProto.Limits.MaxResultBytes = hundredGiBPlusOne
	_, err = remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: overflowProto,
	})
	require.Error(t, err)
	assert.EqualError(t, err, "result_delivery.limits.maxResultBytes must not exceed 107374182400")
}

type captureRemoteQueryExecuteStreamServer struct {
	grpc.ServerStream
	ctx     context.Context
	chunks  []*pb.RemoteQueryExecuteChunk
	sendErr error
}

func (s *captureRemoteQueryExecuteStreamServer) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *captureRemoteQueryExecuteStreamServer) Send(chunk *pb.RemoteQueryExecuteChunk) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.chunks = append(s.chunks, chunk)
	return nil
}

// TestRemoteQueryExecuteRequestFromProtoPreservesTraceContext proves the optional
// trace-continuation metadata crosses the AgentSecure proto boundary with its exact
// values: the active trace ID, the action.run span's own ID as the parent, and the
// propagated sampling priority — here a positive keep priority, the only domain the
// bridge forwards.
func TestRemoteQueryExecuteRequestFromProtoPreservesTraceContext(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT 1 AS value",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
		TraceContext: &pb.RemoteQueryTraceContext{
			TraceId:          1234567890123456789,
			ParentId:         9876543210987654321,
			SamplingPriority: 2,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, req.TraceContext)
	assert.Equal(t, uint64(1234567890123456789), req.TraceContext.TraceID)
	assert.Equal(t, uint64(9876543210987654321), req.TraceContext.SpanID)
	assert.Equal(t, 2, req.TraceContext.SamplingPriority)
}

// TestRemoteQueryExecuteRequestFromProtoDropsInvalidTraceContext proves invalid
// trace context is dropped fail-open, never fatal: a nil message, a zero trace or
// parent ID, or any non-keep sampling priority — including the tracer's drop
// priorities, which the integration's strict carrier validation would reject —
// leaves the typed request with no trace context, and the request itself stays
// valid so execution is unchanged.
func TestRemoteQueryExecuteRequestFromProtoDropsInvalidTraceContext(t *testing.T) {
	tests := []struct {
		name         string
		traceContext *pb.RemoteQueryTraceContext
	}{
		{name: "absent", traceContext: nil},
		{name: "zero trace id", traceContext: &pb.RemoteQueryTraceContext{TraceId: 0, ParentId: 2, SamplingPriority: 2}},
		{name: "zero parent id", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 0, SamplingPriority: 2}},
		{name: "priority above keep domain", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: 3}},
		{name: "priority below keep domain", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: -2}},
		{name: "auto drop priority", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: 0}},
		{name: "user drop priority", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
				Integration:    "postgres",
				Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
				Query:          "SELECT 1 AS value",
				ResultDelivery: validRemoteQueryResultDeliveryProto(),
				TraceContext:   tt.traceContext,
			})

			require.NoError(t, err, "invalid trace context must never fail the request")
			assert.Nil(t, req.TraceContext)
		})
	}
}

// detachedRunCheck matches the requested target and blocks its execute run until
// release is closed.
type detachedRunCheck struct {
	*resolveFakeCheck
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (c *detachedRunCheck) RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error {
	if strings.Contains(requestJSON, `"operation":"resolve_target"`) {
		return c.resolveFakeCheck.RunRemoteQueryStream(integration, requestJSON, emit)
	}
	defer close(c.finished)
	close(c.started)
	<-c.release
	return emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
}

// The stream carries one final SUCCEEDED event as soon as the run has started and
// closes while the run is still going; the run then ends without touching the stream.
func TestRemoteQueryExecuteStreamAnswersSucceededWhileRunContinues(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.com")
	runner := &detachedRunCheck{
		resolveFakeCheck: &resolveFakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\n", resolveEvents: resolveMatchedEvents()},
		started:          make(chan struct{}),
		release:          make(chan struct{}),
		finished:         make(chan struct{}),
	}
	server := &serverSecure{remoteQueries: remotequeriesimpl.NewRemoteQueryExecuteService(resolveFakeCollector{checks: []check.Check{runner}}, true, cfg)}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &captureRemoteQueryExecuteStreamServer{ctx: ctx}

	returned := make(chan error, 1)
	go func() {
		returned <- server.RemoteQueryExecuteStream(&pb.RemoteQueryExecuteRequest{
			Integration:    "postgres",
			Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
			Query:          "SELECT 1 AS value",
			ResultDelivery: validRemoteQueryResultDeliveryProto(),
		}, stream)
	}()
	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the stream waited for the run")
	}
	cancel()

	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.Equal(t, "SUCCEEDED", stream.chunks[0].GetEvent().GetFinal().GetStatus())
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	assert.True(t, stream.chunks[1].GetFinal())

	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not start")
	}
	close(runner.release)
	select {
	case <-runner.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not finish after the stream closed")
	}
	assert.Len(t, stream.chunks, 2)
}
