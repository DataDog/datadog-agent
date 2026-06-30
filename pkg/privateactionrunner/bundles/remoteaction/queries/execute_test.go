// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"context"
	"encoding/json"
	"io"
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
	assert.Equal(t, "SUCCEEDED", out["status"])
	assert.Equal(t, "Beautiful city of lights,France\nNew York,USA\n", out["data"])
}

func TestExecuteActionPreservesCopyStreamEvents(t *testing.T) {
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
	assert.Equal(t, "Beautiful city of lights,France\nNew York,USA\n", output.(map[string]interface{})["data"])
	assert.Equal(t, []byte("Beautiful city of lights,France\nNew York,USA\n"), output.(map[string]interface{})["data_bytes"])
	assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
}

func TestExecuteActionPreservesBinaryCopyStreamPayload(t *testing.T) {
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
	assert.Equal(t, []byte{0x00, 0xff, 0x80}, out["data_bytes"])
	assert.NotContains(t, out, "data")
	assert.Equal(t, "SUCCEEDED", out["status"])
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
