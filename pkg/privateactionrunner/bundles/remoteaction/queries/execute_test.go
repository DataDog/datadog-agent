// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestExecuteActionUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{Operation: "copy_stream", Integration: "postgres", Format: "csv"}}}, ChunkIndex: 0},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 1, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: []byte("Beautiful city of lights,France\nNew York,USA\n"), Offset: 0, Bytes: 42}}}, ChunkIndex: 1},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 2, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: "SUCCEEDED", BytesEmitted: 42, ChunksEmitted: 1}}}, ChunkIndex: 2},
		{ChunkIndex: 3, Final: true},
	}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      "csv",
		"target": map[string]interface{}{
			"host":   "localhost",
			"port":   5432,
			"dbname": "postgres",
		},
		"query": "SELECT city, country FROM cities ORDER BY city",
		"copyLimits": map[string]interface{}{
			"chunkBytes":  1024,
			"maxBytes":    1024,
			"maxRowBytes": 1024,
			"timeoutMs":   1000,
		},
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "postgres", client.request.GetIntegration())
	assert.Equal(t, "copy_stream", client.request.GetOperation())
	assert.Equal(t, "csv", client.request.GetFormat())
	assert.Equal(t, "localhost", client.request.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.request.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.request.GetTarget().GetDbname())
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", client.request.GetQuery())
	assert.Equal(t, int32(1024), client.request.GetCopyLimits().GetChunkBytes())
	requestEvidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "secret-value")
	out, ok := output.(map[string]interface{})
	require.True(t, ok)
	expectedData := "Beautiful city of lights,France\nNew York,USA\n"
	assert.Equal(t, "SUCCEEDED", out["status"])
	assert.Equal(t, "csv", out["format"])
	assert.Equal(t, "utf8", out["encoding"])
	assert.Equal(t, len(expectedData), out["bytes"])
	assert.Equal(t, expectedData, out["data"])
	assertNoPayloadDuplicateFields(t, out)
	assert.NotContains(t, out, "data_base64")
}

func TestExecuteActionAcceptsDatabaseInstanceTarget(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: "SUCCEEDED"}}}, ChunkIndex: 0},
		{ChunkIndex: 1, Final: true},
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      "csv",
		"target":      map[string]interface{}{"database_instance": "Rq-Proof-A1-DB1"},
		"query":       "SELECT city, country FROM cities ORDER BY city",
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "Rq-Proof-A1-DB1", client.request.GetTarget().GetDatabaseInstance())
	assert.Empty(t, client.request.GetTarget().GetHost())
	assert.Zero(t, client.request.GetTarget().GetPort())
	assert.Empty(t, client.request.GetTarget().GetDbname())
	assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
}

func TestExecuteActionRejectsMixedAndPartialTargetSelectorsBeforeRPC(t *testing.T) {
	tests := []struct {
		name   string
		target map[string]interface{}
	}{
		{name: "mixed", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": "localhost", "port": 5432, "dbname": "postgres"}},
		{name: "mixed empty host", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": ""}},
		{name: "mixed empty dbname", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": ""}},
		{name: "mixed null host", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": nil}},
		{name: "mixed port", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "port": 5432}},
		{name: "database instance surrounding whitespace", target: map[string]interface{}{"database_instance": " rq-proof-a1-db1 "}},
		{name: "partial tuple", target: map[string]interface{}{"host": "localhost", "dbname": "postgres"}},
		{name: "unknown credential field", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "password": "secret-value"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewExecuteAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for invalid target")
				return nil, nil
			})

			_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
				"integration": "postgres",
				"operation":   "copy_stream",
				"target":      tt.target,
				"query":       "SELECT city, country FROM cities ORDER BY city",
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestExecuteActionReturnsCompactCSVOutputWithoutPayloadEvents(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{Operation: "copy_stream", Format: "csv"}}}, ChunkIndex: 0},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 1, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: []byte("Beautiful city of lights,France\n"), Offset: 0, Bytes: 32}}}, ChunkIndex: 1},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 2, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: []byte("New York,USA\n"), Offset: 32, Bytes: 13}}}, ChunkIndex: 2},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 3, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: "SUCCEEDED", BytesEmitted: 45}}}, ChunkIndex: 3},
		{ChunkIndex: 4, Final: true},
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      "csv",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":       "SELECT city, country FROM cities ORDER BY city",
		"copyLimits":  map[string]interface{}{"chunkBytes": 32, "maxBytes": 1024, "maxRowBytes": 1024, "timeoutMs": 1000},
	}), nil)

	require.NoError(t, err)
	assert.Equal(t, "copy_stream", client.request.GetOperation())
	assert.Equal(t, "csv", client.request.GetFormat())
	assert.Equal(t, int32(32), client.request.GetCopyLimits().GetChunkBytes())

	expectedData := "Beautiful city of lights,France\nNew York,USA\n"
	out := output.(map[string]interface{})
	assert.Equal(t, "SUCCEEDED", out["status"])
	assert.Equal(t, "csv", out["format"])
	assert.Equal(t, "utf8", out["encoding"])
	assert.Equal(t, len(expectedData), out["bytes"])
	assert.Equal(t, expectedData, out["data"])
	assert.Equal(t, map[string]interface{}{"payload_bytes": len(expectedData), "chunks_received": 2}, out["stream_summary"])
	assertNoPayloadDuplicateFields(t, out)
	assert.NotContains(t, out, "data_base64")
}

func TestExecuteActionReturnsCompactBase64ForBinaryCopyStreamPayload(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: []byte{0x00, 0xff, 0x80}, Offset: 0, Bytes: 3}}}, ChunkIndex: 0},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 1, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: "SUCCEEDED", BytesEmitted: 3, ChunksEmitted: 1}}}, ChunkIndex: 1},
		{ChunkIndex: 2, Final: true},
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"operation":   "copy_stream",
		"format":      "binary",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":       "SELECT decode('00ff80', 'hex') AS payload",
		"copyLimits":  map[string]interface{}{"chunkBytes": 32, "maxBytes": 1024, "maxRowBytes": 1024, "timeoutMs": 1000},
	}), nil)

	require.NoError(t, err)
	out := output.(map[string]interface{})
	assert.Equal(t, "SUCCEEDED", out["status"])
	assert.Equal(t, "binary", out["format"])
	assert.Equal(t, "base64", out["encoding"])
	assert.Equal(t, 3, out["bytes"])
	assert.Equal(t, "AP+A", out["data_base64"])
	assert.Equal(t, map[string]interface{}{"payload_bytes": 3, "chunks_received": 1}, out["stream_summary"])
	assertNoPayloadDuplicateFields(t, out)
	assert.NotContains(t, out, "data")
}

func TestRemoteQueryExecuteOutputForFiveMiBCSVStaysUnderActionPlatformLimit(t *testing.T) {
	const actionPlatformOutputLimitBytes = 15 * 1024 * 1024
	payload := strings.Repeat("x", 5*1024*1024+1)
	payloadBytes := []byte(payload)
	stream := &captureRemoteQueryExecuteStream{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{Operation: "copy_stream", Format: "csv"}}}, ChunkIndex: 0},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 1, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: payloadBytes[:2*1024*1024], Offset: 0, Bytes: 2 * 1024 * 1024}}}, ChunkIndex: 1},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 2, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: payloadBytes[2*1024*1024 : 4*1024*1024], Offset: 2 * 1024 * 1024, Bytes: 2 * 1024 * 1024}}}, ChunkIndex: 2},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 3, Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{Payload: payloadBytes[4*1024*1024:], Offset: 4 * 1024 * 1024, Bytes: uint64(len(payloadBytes) - 4*1024*1024)}}}, ChunkIndex: 3},
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 4, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: "SUCCEEDED", BytesEmitted: uint64(len(payloadBytes)), ChunksEmitted: 3}}}, ChunkIndex: 4},
		{ChunkIndex: 5, Final: true},
	}}

	output, err := remoteQueryExecuteOutputFromStream(stream, "csv")
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", output["status"])
	assert.Equal(t, "csv", output["format"])
	assert.Equal(t, "utf8", output["encoding"])
	assert.Equal(t, len(payloadBytes), output["bytes"])
	data, ok := output["data"].(string)
	require.True(t, ok)
	assert.Len(t, data, len(payloadBytes))
	assertNoPayloadDuplicateFields(t, output)
	assert.NotContains(t, output, "data_base64")

	encodedOutput, err := json.Marshal(output)
	require.NoError(t, err)
	assert.Less(t, len(encodedOutput), actionPlatformOutputLimitBytes)
}

func TestExecuteActionPreservesSanitizedBridgeErrorBody(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: 0, Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{Code: "target_not_found", Message: "no matching integration check found"}}}, ChunkIndex: 0},
		{ChunkIndex: 1, Final: true},
	}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":       "SELECT 1 AS value",
	}), nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"status": "target_not_found",
		"error": map[string]interface{}{
			"code":    "target_not_found",
			"message": "no matching integration check found",
		},
	}, output)
}

func TestExecuteActionSanitizesInputExtractionErrors(t *testing.T) {
	action := NewExecuteAction(func() (BridgeClient, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, nil
	})

	_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":       "SELECT secret FROM private_table",
		"bad":         make(chan struct{}),
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "invalid remote query action inputs", parErr.Message)
	assert.Equal(t, "invalid remote query action inputs", parErr.ExternalMessage)
	assert.NotContains(t, err.Error(), "secret-db")
	assert.NotContains(t, err.Error(), "SELECT secret")
}

func assertNoPayloadDuplicateFields(t *testing.T, out map[string]interface{}) {
	t.Helper()
	assert.NotContains(t, out, "events")
	assert.NotContains(t, out, "payload")
	assert.NotContains(t, out, "data_bytes")
	_, hasData := out["data"]
	_, hasBase64Data := out["data_base64"]
	assert.False(t, hasData && hasBase64Data, "output must not contain both data and data_base64")
}

func taskWithInputs(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: BundleID,
		Name:     ExecuteActionName,
		Inputs:   inputs,
	}
	return task
}

type captureBridgeClient struct {
	request *pb.RemoteQueryExecuteRequest
	chunks  []*pb.RemoteQueryExecuteChunk
	err     error
}

func (c *captureBridgeClient) RemoteQueryExecute(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error) {
	c.request = req
	return nil, c.err
}

func (c *captureBridgeClient) RemoteQueryExecuteStream(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error) {
	c.request = req
	if c.err != nil {
		return nil, c.err
	}
	return &captureRemoteQueryExecuteStream{chunks: c.chunks}, nil
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
