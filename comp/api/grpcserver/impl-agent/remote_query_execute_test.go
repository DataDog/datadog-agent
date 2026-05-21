// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestRemoteQueryExecuteResponseFromJSONMapsStructuredRows(t *testing.T) {
	resp, err := remoteQueryExecuteResponseFromJSON(`{"status":"SUCCEEDED","columns":[{"name":"value","type":"integer"}],"rows":[{"value":1}],"stats":{"elapsed_ms":2},"truncated":true}`)

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", resp.GetStatus())
	require.Len(t, resp.GetColumns(), 1)
	assert.Equal(t, "value", resp.GetColumns()[0].AsMap()["name"])
	require.Len(t, resp.GetRows(), 1)
	assert.Equal(t, float64(1), resp.GetRows()[0].AsMap()["value"])
	assert.True(t, resp.GetTruncated())
	assert.Equal(t, float64(2), resp.GetStats().AsMap()["elapsed_ms"])
}

func TestRemoteQueryExecuteReturnsSanitizedUnavailableWhenServiceMissing(t *testing.T) {
	resp, err := (&serverSecure{}).RemoteQueryExecute(context.Background(), &pb.RemoteQueryExecuteRequest{})

	require.NoError(t, err)
	assert.Equal(t, "executor_unavailable", resp.GetStatus())
	require.NotNil(t, resp.GetError())
	assert.Equal(t, "executor_unavailable", resp.GetError().GetCode())
}

func TestRemoteQueryExecuteStreamReturnsSanitizedUnavailableWhenServiceMissing(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{}).RemoteQueryExecuteStream(&pb.RemoteQueryExecuteRequest{}, stream)

	require.NoError(t, err)
	require.Len(t, stream.chunks, 1)
	assert.True(t, stream.chunks[0].GetFinal())
	assert.JSONEq(t, `{"status":"executor_unavailable","error":{"code":"executor_unavailable","message":"remote query executor is unavailable"}}`, string(stream.chunks[0].GetResponseJsonChunk()))
}

func TestRemoteQueryExecuteStreamJSONChunksResponse(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	responseJSON := `{"status":"SUCCEEDED","rows":[{"payload":"` + string(make([]byte, remoteQueryExecuteStreamChunkSize+1)) + `"}]}`

	err := remoteQueryExecuteStreamJSON(responseJSON, stream)

	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.False(t, stream.chunks[0].GetFinal())
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	assert.True(t, stream.chunks[1].GetFinal())
	assert.Equal(t, len(responseJSON), len(stream.chunks[0].GetResponseJsonChunk())+len(stream.chunks[1].GetResponseJsonChunk()))
}

type captureRemoteQueryExecuteStreamServer struct {
	grpc.ServerStream
	chunks []*pb.RemoteQueryExecuteChunk
}

func (s *captureRemoteQueryExecuteStreamServer) Send(chunk *pb.RemoteQueryExecuteChunk) error {
	s.chunks = append(s.chunks, chunk)
	return nil
}
