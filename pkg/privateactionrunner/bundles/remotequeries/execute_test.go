// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteActionUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	franceRow, err := structpb.NewStruct(map[string]interface{}{"city": "Beautiful city of lights", "country": "France"})
	require.NoError(t, err)
	usaRow, err := structpb.NewStruct(map[string]interface{}{"city": "New York", "country": "USA"})
	require.NoError(t, err)
	client := &captureBridgeClient{response: &pb.RemoteQueryExecuteResponse{Status: "SUCCEEDED", Rows: []*structpb.Struct{franceRow, usaRow}}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target": map[string]interface{}{
			"host":   "localhost",
			"port":   5432,
			"dbname": "postgres",
		},
		"query": "SELECT city, country FROM cities ORDER BY city",
		"limits": map[string]interface{}{
			"maxRows":   2,
			"maxBytes":  1024,
			"timeoutMs": 1000,
		},
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "postgres", client.request.GetIntegration())
	assert.Equal(t, "localhost", client.request.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.request.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.request.GetTarget().GetDbname())
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", client.request.GetQuery())
	assert.Equal(t, int32(2), client.request.GetLimits().GetMaxRows())
	requestEvidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "secret-value")
	assert.Equal(t, map[string]interface{}{
		"status": "SUCCEEDED",
		"rows": []interface{}{
			map[string]interface{}{"city": "Beautiful city of lights", "country": "France"},
			map[string]interface{}{"city": "New York", "country": "USA"},
		},
	}, output)
}

func TestExecuteActionPreservesCopyStreamEvents(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{ResponseJsonChunk: []byte(`{"type":"metadata","status":"STARTED","operation":"copy_stream"}`), ChunkIndex: 0},
		{ResponseJsonChunk: []byte(`{"type":"data","sequence":0,"data":"Beautiful city of lights,France\n","bytes":32}`), ChunkIndex: 1},
		{ResponseJsonChunk: []byte(`{"type":"data","sequence":1,"data":"New York,USA\n","bytes":13}`), ChunkIndex: 2},
		{ResponseJsonChunk: []byte(`{"type":"final","status":"SUCCEEDED","stats":{"bytesEmitted":45}}`), ChunkIndex: 3},
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
	assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
}

func TestExecuteActionPreservesSanitizedBridgeErrorBody(t *testing.T) {
	client := &captureBridgeClient{
		response: &pb.RemoteQueryExecuteResponse{Status: "target_not_found", Error: &pb.RemoteQueryExecuteError{Code: "target_not_found", Message: "no matching integration check found"}},
	}
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
	request  *pb.RemoteQueryExecuteRequest
	response *pb.RemoteQueryExecuteResponse
	chunks   []*pb.RemoteQueryExecuteChunk
	err      error
}

func (c *captureBridgeClient) RemoteQueryExecute(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error) {
	c.request = req
	return c.response, c.err
}

func (c *captureBridgeClient) RemoteQueryExecuteStream(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error) {
	c.request = req
	if c.err != nil {
		return nil, c.err
	}
	if c.chunks != nil {
		return &captureRemoteQueryExecuteStream{chunks: c.chunks}, nil
	}
	responseJSON, err := json.Marshal(remoteQueryExecuteOutputFromProtoForTest(c.response))
	if err != nil {
		return nil, err
	}
	return &captureRemoteQueryExecuteStream{chunks: []*pb.RemoteQueryExecuteChunk{{ResponseJsonChunk: responseJSON, Final: true}}}, nil
}

func remoteQueryExecuteOutputFromProtoForTest(resp *pb.RemoteQueryExecuteResponse) map[string]interface{} {
	output := map[string]interface{}{"status": resp.GetStatus()}
	if resp.GetError() != nil {
		output["error"] = map[string]interface{}{"code": resp.GetError().GetCode(), "message": resp.GetError().GetMessage()}
	}
	if len(resp.GetRows()) > 0 {
		rows := make([]interface{}, 0, len(resp.GetRows()))
		for _, row := range resp.GetRows() {
			rows = append(rows, row.AsMap())
		}
		output["rows"] = rows
	}
	return output
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
