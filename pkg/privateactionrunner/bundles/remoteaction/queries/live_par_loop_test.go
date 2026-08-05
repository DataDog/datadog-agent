// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build remoteaction_queries_live && !windows

package com_datadoghq_remoteaction_queries_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/util/sets"

	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	com_datadoghq_remoteaction_queries "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/queries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteQueriesActionRunsThroughLivePARLoopAndFakeintake(t *testing.T) {
	t.Setenv(app.InternalSkipTaskVerificationEnvVar, "true")

	row, err := structpb.NewStruct(map[string]interface{}{"value": 1})
	require.NoError(t, err)
	bridgeRequests := make(chan *pb.RemoteQueryExecuteRequest, 1)
	restoreFactory := com_datadoghq_remoteaction_queries.SetBridgeClientFactoryForTest(func() (com_datadoghq_remoteaction_queries.BridgeClient, error) {
		return &captureAgentSecureClient{
			requests: bridgeRequests,
			response: &pb.RemoteQueryExecuteResponse{Status: "SUCCEEDED", Rows: []*structpb.Struct{row}},
		}, nil
	})
	defer restoreFactory()

	fakeintake, _ := fakeintakeserver.InitialiseForTests(t)
	defer func() { require.NoError(t, fakeintake.Stop()) }()
	fakeintakeClient := fakeintakeclient.NewClient(fakeintake.URL())
	require.NoError(t, fakeintakeClient.FlushPAR())

	cfg := newLivePARTestConfig(t, fakeintake.URL())
	keysManager := taskverifier.NewKeyManager(nil)
	verifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	workflowRunner, err := runners.NewWorkflowRunner(cfg, keysManager, verifier, opms.NewClient(cfg), nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, workflowRunner.Start(ctx))
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		require.NoError(t, workflowRunner.Stop(stopCtx))
	}()

	taskID := "remoteaction-queries-live-par-local-proof"
	require.NoError(t, fakeintakeClient.EnqueuePARTask(taskID, com_datadoghq_remoteaction_queries.BundleID+"."+com_datadoghq_remoteaction_queries.ExecuteActionName, map[string]interface{}{
		"integration": "postgres",
		"target": map[string]interface{}{
			"host":   "localhost",
			"port":   5432,
			"dbname": "postgres",
		},
		"operation": "copy_stream",
		"format":    "csv",
		"query":     "SELECT 1 AS value",
		"copyLimits": map[string]interface{}{
			"chunkBytes":  1024,
			"maxBytes":    1024,
			"maxRowBytes": 1024,
			"timeoutMs":   1000,
		},
	}))

	result, err := fakeintakeClient.GetPARTaskResult(taskID, 10*time.Second)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "SUCCEEDED", result.Outputs["status"])
	require.Contains(t, result.Outputs, "events")

	select {
	case req := <-bridgeRequests:
		requestEvidence, err := json.Marshal(req)
		require.NoError(t, err)
		require.NotContains(t, string(requestEvidence), "password")
		require.NotContains(t, string(requestEvidence), "token")
		require.NotContains(t, string(requestEvidence), "secret")
		assert.Equal(t, "postgres", req.GetIntegration())
		assert.Equal(t, "copy_stream", req.GetOperation())
		assert.Equal(t, "csv", req.GetFormat())
		assert.Equal(t, "SELECT 1 AS value", req.GetQuery())
		assert.Equal(t, "localhost", req.GetTarget().GetHost())
		assert.Equal(t, int32(5432), req.GetTarget().GetPort())
		assert.Equal(t, "postgres", req.GetTarget().GetDbname())
	case <-time.After(2 * time.Second):
		require.FailNow(t, "remote query action did not call the AgentSecure client")
	}

	dequeueCalls, err := fakeintakeClient.GetPARDequeueCount()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dequeueCalls, 1)
}

func newLivePARTestConfig(t *testing.T, fakeintakeURL string) *parconfig.Config {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	taskTimeoutSeconds := int32(5)

	return &parconfig.Config{
		ActionsAllowlist: map[string]sets.Set[string]{
			com_datadoghq_remoteaction_queries.BundleID: sets.New[string](com_datadoghq_remoteaction_queries.ExecuteActionName),
		},
		DDHost:             fakeintakeURL,
		DDApiHost:          "unused.local",
		DatadogSite:        "local",
		OrgId:              123456,
		PrivateKey:         privateKey,
		RunnerId:           "remoteaction-queries-live-par-local-proof-runner",
		Urn:                "urn:dd:apps:on-prem-runner:us1:123456:remoteaction-queries-live-par-local-proof-runner",
		LoopInterval:       10 * time.Millisecond,
		MinBackoff:         10 * time.Millisecond,
		MaxBackoff:         50 * time.Millisecond,
		WaitBeforeRetry:    50 * time.Millisecond,
		MaxAttempts:        3,
		OpmsRequestTimeout: 1000,
		RunnerPoolSize:     1,
		HeartbeatInterval:  time.Hour,
		TaskTimeoutSeconds: &taskTimeoutSeconds,
		MetricsClient:      &statsd.NoOpClient{},
		Tags:               []observability.Tag{},
	}
}

type captureAgentSecureClient struct {
	requests chan<- *pb.RemoteQueryExecuteRequest
	response *pb.RemoteQueryExecuteResponse
}

func (c *captureAgentSecureClient) RemoteQueryExecute(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error) {
	c.requests <- req
	return c.response, nil
}

func (c *captureAgentSecureClient) RemoteQueryExecuteStream(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error) {
	c.requests <- req
	return &captureRemoteQueryExecuteStream{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{Operation: "copy_stream", Integration: "postgres", Format: "csv"}}}, ChunkIndex: 0},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 1, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: []byte("1\n"), Offset: 0, Bytes: 2}}}, ChunkIndex: 1},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 2, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: c.response.GetStatus(), BytesEmitted: 2, ChunksEmitted: 1}}}, ChunkIndex: 2},
		{ChunkIndex: 3, Final: true},
	}}, nil
}

type captureRemoteQueryExecuteStream struct {
	grpc.ClientStream
	chunks []*pb.RemoteQueryExecuteChunk
}

func (s *captureRemoteQueryExecuteStream) Recv() (*pb.RemoteQueryExecuteChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}
