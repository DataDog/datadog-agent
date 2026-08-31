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

const (
	testRunID    = "run-01k"
	testTaskID   = "task-01k"
	testUploadID = "upload-01k"
	testBaseURL  = "https://dd.datad0g.com/api/unstable/its-agent-intake"
	testToken    = "scoped-upload-token"
)

func resultDeliveryInputs() map[string]interface{} {
	return map[string]interface{}{
		"runId":           testRunID,
		"taskId":          testTaskID,
		"artifactVersion": 1,
		"uploadId":        testUploadID,
		"baseUrl":         testBaseURL,
		"token":           testToken,
		"partBytes":       8388608,
		"limits": map[string]interface{}{
			"maxFileBytes":   33554432,
			"maxResultBytes": 10737418240,
			"maxRowBytes":    33554432,
			"maxColumns":     1024,
			"maxSchemaBytes": 1048576,
			"maxPages":       128,
			"timeoutMs":      30000,
		},
	}
}

func metadataEvent(sequence uint64) *pb.RemoteQueryExecuteChunk {
	return &pb.RemoteQueryExecuteChunk{
		ChunkIndex: int32(sequence),
		Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: sequence, Event: &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{
			Operation:   remoteQueryOperationProduceJSONPages,
			Integration: "postgres",
			Attributes:  map[string]string{"status": "STARTED", "includeSchema": "true"},
		}}},
	}
}

func finalEvent(sequence uint64, receipt *pb.RemoteQueryUploadReceipt, attributes map[string]string) *pb.RemoteQueryExecuteChunk {
	return &pb.RemoteQueryExecuteChunk{
		ChunkIndex: int32(sequence),
		Event: &pb.RemoteQueryExecuteStreamEvent{Sequence: sequence, Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{
			Status:        "SUCCEEDED",
			UploadReceipt: receipt,
			Attributes:    attributes,
		}}},
	}
}

func validReceipt() *pb.RemoteQueryUploadReceipt {
	return &pb.RemoteQueryUploadReceipt{
		UploadId:   testUploadID,
		PageCount:  3,
		TotalRows:  123456,
		TotalBytes: 987654,
	}
}

func finalMarker(sequence uint64) *pb.RemoteQueryExecuteChunk {
	return &pb.RemoteQueryExecuteChunk{ChunkIndex: int32(sequence), Final: true}
}

func TestExecuteActionUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		metadataEvent(0),
		finalEvent(1, validReceipt(), map[string]string{"agent_total_stream_ms": "12.345", "stats.rowsEmitted": "123456"}),
		finalMarker(2),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":          "SELECT city, country FROM cities ORDER BY city",
		"includeSchema":  true,
		"resultDelivery": resultDeliveryInputs(),
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "postgres", client.request.GetIntegration())
	assert.Equal(t, "localhost", client.request.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.request.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.request.GetTarget().GetDbname())
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", client.request.GetQuery())
	assert.True(t, client.request.GetIncludeSchema())

	// The AgentSecure request carries only the typed paged-JSON contract fields: no
	// operation, format, or COPY-era field. The fixed operation is emitted by the
	// Agent's native request mapping after the request crosses the boundary.
	delivery := client.request.GetResultDelivery()
	require.NotNil(t, delivery)
	assert.Equal(t, testRunID, delivery.GetRunId())
	assert.Equal(t, testTaskID, delivery.GetTaskId())
	assert.Equal(t, int32(1), delivery.GetArtifactVersion())
	assert.Equal(t, testUploadID, delivery.GetUploadId())
	assert.Equal(t, testBaseURL, delivery.GetBaseUrl())
	assert.Equal(t, testToken, delivery.GetToken())
	assert.Equal(t, int64(8388608), delivery.GetPartBytes())
	require.NotNil(t, delivery.GetLimits())
	assert.Equal(t, int64(33554432), delivery.GetLimits().GetMaxFileBytes())
	assert.Equal(t, int64(10737418240), delivery.GetLimits().GetMaxResultBytes())
	assert.Equal(t, int64(33554432), delivery.GetLimits().GetMaxRowBytes())
	assert.Equal(t, int64(1024), delivery.GetLimits().GetMaxColumns())
	assert.Equal(t, int64(1048576), delivery.GetLimits().GetMaxSchemaBytes())
	assert.Equal(t, int64(128), delivery.GetLimits().GetMaxPages())
	assert.Equal(t, int64(30000), delivery.GetLimits().GetTimeoutMs())

	// The scoped upload token and base URL are forwarded inside the delivery handle;
	// the private credential tokens never reach the AgentSecure request.
	requestEvidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.Contains(t, string(requestEvidence), testToken)
	assert.Contains(t, string(requestEvidence), testBaseURL)
	assert.NotContains(t, string(requestEvidence), "secret-value")

	out, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SUCCEEDED", out["status"])
	receipt, ok := out["uploadReceipt"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, map[string]interface{}{
		"uploadId":   testUploadID,
		"pageCount":  int64(3),
		"totalRows":  int64(123456),
		"totalBytes": int64(987654),
	}, receipt)
	attributes, ok := out["attributes"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "STARTED", attributes["status"])
	assert.Equal(t, "true", attributes["includeSchema"])
	assert.Equal(t, "12.345", attributes["agent_total_stream_ms"])
	assert.Equal(t, "123456", attributes["stats.rowsEmitted"])
	assertNoBulkDataFields(t, out)

	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), testToken)
}

func TestExecuteActionAcceptsDatabaseInstanceTarget(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"database_instance": "Rq-Proof-A1-DB1"},
		"query":          "SELECT city, country FROM cities ORDER BY city",
		"resultDelivery": resultDeliveryInputs(),
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
				"integration":    "postgres",
				"target":         tt.target,
				"query":          "SELECT city, country FROM cities ORDER BY city",
				"resultDelivery": resultDeliveryInputs(),
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

// TestExecuteActionRejectsMissingResultDeliveryBeforeRPC proves a run cannot dispatch
// without the backend-injected upload handle: there is no inline fallback path.
func TestExecuteActionRejectsMissingResultDeliveryBeforeRPC(t *testing.T) {
	tests := []struct {
		name           string
		resultDelivery map[string]interface{}
	}{
		{name: "missing delivery", resultDelivery: nil},
		{name: "missing limits", resultDelivery: map[string]interface{}{
			"runId": testRunID, "taskId": testTaskID, "artifactVersion": 1,
			"uploadId": testUploadID, "baseUrl": testBaseURL, "token": testToken, "partBytes": 8388608,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewExecuteAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created without resultDelivery")
				return nil, nil
			})

			inputs := map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
				"query":       "SELECT city, country FROM cities ORDER BY city",
			}
			if tt.resultDelivery != nil {
				inputs["resultDelivery"] = tt.resultDelivery
			}

			_, err := action.Run(context.Background(), taskWithInputs(inputs), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
		})
	}
}

func TestExecuteActionFailsClosedWhenFinalReceiptIsMissingOrMismatched(t *testing.T) {
	t.Run("missing receipt", func(t *testing.T) {
		client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
			finalEvent(0, nil, nil),
			finalMarker(1),
		}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
			"integration":    "postgres",
			"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			"query":          "SELECT city, country FROM cities ORDER BY city",
			"resultDelivery": resultDeliveryInputs(),
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Contains(t, parErr.Message, "missing upload receipt")
	})

	t.Run("receipt uploadId mismatch", func(t *testing.T) {
		mismatched := validReceipt()
		mismatched.UploadId = "upload-other"
		client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
			finalEvent(0, mismatched, nil),
			finalMarker(1),
		}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
			"integration":    "postgres",
			"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			"query":          "SELECT city, country FROM cities ORDER BY city",
			"resultDelivery": resultDeliveryInputs(),
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Contains(t, parErr.Message, "uploadId does not match")
	})
}

func TestExecuteActionRejectsStreamProtocolViolations(t *testing.T) {
	tests := []struct {
		name   string
		chunks []*pb.RemoteQueryExecuteChunk
	}{
		{
			name: "chunk index mismatch",
			chunks: []*pb.RemoteQueryExecuteChunk{
				metadataEvent(0),
				metadataEvent(5),
				finalMarker(6),
			},
		},
		{
			name: "chunk after final",
			chunks: []*pb.RemoteQueryExecuteChunk{
				finalEvent(0, validReceipt(), nil),
				finalMarker(1),
				metadataEvent(2),
			},
		},
		{
			name: "missing final chunk",
			chunks: []*pb.RemoteQueryExecuteChunk{
				metadataEvent(0),
			},
		},
		{
			name: "missing typed event",
			chunks: []*pb.RemoteQueryExecuteChunk{
				{ChunkIndex: 0},
				finalMarker(1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &captureBridgeClient{chunks: tt.chunks}
			action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

			_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
				"integration":    "postgres",
				"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
				"query":          "SELECT city, country FROM cities ORDER BY city",
				"resultDelivery": resultDeliveryInputs(),
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "remote query AgentSecure streaming RPC response was invalid", parErr.ExternalMessage)
		})
	}
}

func TestExecuteActionPreservesSanitizedBridgeErrorBody(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		{ChunkIndex: 0, Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code: "target_not_found", Message: "no matching integration check found", Retryable: false,
			Attributes: map[string]string{"stats.elapsedMs": "3"},
		}}}},
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) {
		return client, nil
	})

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":          "SELECT 1 AS value",
		"resultDelivery": resultDeliveryInputs(),
	}), nil)

	// Terminal errors propagate through the AP output envelope without a receipt.
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"status": "target_not_found",
		"error": map[string]interface{}{
			"code":      "target_not_found",
			"message":   "no matching integration check found",
			"retryable": false,
		},
		"attributes": map[string]interface{}{"stats.elapsedMs": "3"},
	}, output)
}

func TestExecuteActionSanitizesInputExtractionErrors(t *testing.T) {
	action := NewExecuteAction(func() (BridgeClient, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, nil
	})

	_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"query":          "SELECT secret FROM private_table",
		"resultDelivery": resultDeliveryInputs(),
		"bad":            make(chan struct{}),
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "invalid remote query action inputs", parErr.Message)
	assert.Equal(t, "invalid remote query action inputs", parErr.ExternalMessage)
	assert.NotContains(t, err.Error(), "secret-db")
	assert.NotContains(t, err.Error(), "SELECT secret")
}

// TestRemoteQueryExecuteOutputStaysUnderActionPlatformLimit proves the receipt-only
// output is bounded by construction: with no inline result-byte path the AP artifact
// stays tiny even for multi-page runs.
func TestRemoteQueryExecuteOutputStaysUnderActionPlatformLimit(t *testing.T) {
	const actionPlatformOutputLimitBytes = 15 * 1024 * 1024
	stream := &captureRemoteQueryExecuteStream{chunks: []*pb.RemoteQueryExecuteChunk{
		metadataEvent(0),
		finalEvent(1, &pb.RemoteQueryUploadReceipt{
			UploadId:   "upload-01k",
			PageCount:  128,
			TotalRows:  1099511627776,
			TotalBytes: 10737418240,
		}, nil),
		finalMarker(2),
	}}

	output, err := remoteQueryExecuteOutputFromStream(stream, testUploadID)
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", output["status"])
	assertNoBulkDataFields(t, output)

	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	assert.Less(t, len(encoded), actionPlatformOutputLimitBytes)
}

func assertNoBulkDataFields(t *testing.T, out map[string]interface{}) {
	t.Helper()
	assert.NotContains(t, out, "events")
	assert.NotContains(t, out, "payload")
	assert.NotContains(t, out, "data")
	assert.NotContains(t, out, "data_base64")
	assert.NotContains(t, out, "data_bytes")
	assert.NotContains(t, out, "csv")
	assert.NotContains(t, out, "columns")
	assert.NotContains(t, out, "rows")
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
