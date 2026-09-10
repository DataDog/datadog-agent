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
)

// testFingerprint is the opaque resolve-time match fingerprint carried between the
// resolve and execute modes.
const testFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func resultDeliveryInputs() map[string]interface{} {
	return map[string]interface{}{
		"runId":           testRunID,
		"taskId":          testTaskID,
		"artifactVersion": 1,
		"uploadId":        testUploadID,
		"baseUrl":         testBaseURL,
		"limits": map[string]interface{}{
			"maxFileBytes":   33554432,
			"maxResultBytes": 107374182400, // 100 GiB, the backend-owned result cap.
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
	require.NotNil(t, delivery.GetLimits())
	assert.Equal(t, int64(33554432), delivery.GetLimits().GetMaxFileBytes())
	assert.Equal(t, int64(107374182400), delivery.GetLimits().GetMaxResultBytes())
	assert.Equal(t, int64(33554432), delivery.GetLimits().GetMaxRowBytes())
	assert.Equal(t, int64(1024), delivery.GetLimits().GetMaxColumns())
	assert.Equal(t, int64(1048576), delivery.GetLimits().GetMaxSchemaBytes())
	assert.Equal(t, int64(128), delivery.GetLimits().GetMaxPages())
	assert.Equal(t, int64(30000), delivery.GetLimits().GetTimeoutMs())

	// The delivery handle carries the base URL; the session has no upload token, so no
	// token key appears anywhere in the AgentSecure request, and the private credential
	// tokens never reach it either.
	requestEvidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "token")
	assert.Contains(t, string(requestEvidence), testBaseURL)
	assert.NotContains(t, string(requestEvidence), "secret-value")

	out, ok := output.(map[string]interface{})
	require.True(t, ok)
	// The AP action output matches the strict AP metadata schema exactly:
	// status and uploadReceipt and nothing else. Progress metadata and agent
	// timing stay on the internal AgentSecure stream events.
	assert.Equal(t, map[string]interface{}{
		"status": "SUCCEEDED",
		"uploadReceipt": map[string]interface{}{
			"uploadId":   testUploadID,
			"pageCount":  int64(3),
			"totalRows":  int64(123456),
			"totalBytes": int64(987654),
		},
	}, out)
	assertNoBulkDataFields(t, out)

	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "token")
	assert.NotContains(t, string(encoded), "STARTED")
	assert.NotContains(t, string(encoded), "stats.rowsEmitted")
	assert.NotContains(t, string(encoded), "agent_total_stream_ms")
}

// TestExecuteActionDropsStaleUploadTokenFromInputs proves the session-id contract on
// the AP action input: the backend resultDelivery carries no token, and a stale
// producer still including one is tolerated (the input decoding is not strict) but
// the value never reaches the AgentSecure request.
func TestExecuteActionDropsStaleUploadTokenFromInputs(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	staleDelivery := resultDeliveryInputs()
	staleDelivery["token"] = "stale-upload-token"

	_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":          "SELECT city, country FROM cities ORDER BY city",
		"resultDelivery": staleDelivery,
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	require.NotNil(t, client.request.GetResultDelivery())
	evidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(evidence), "stale-upload-token")
	assert.NotContains(t, string(evidence), "token")
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

// TestExecuteActionRejectsDatabaseInstanceWithDbnameTargetBeforeRPC proves the
// managed-instance selector carries no database override: dbname alongside
// database_instance would select a check identity and then override its
// configured database, so the input is rejected before the bridge client is ever
// created, in both dispatch modes.
func TestExecuteActionRejectsDatabaseInstanceWithDbnameTargetBeforeRPC(t *testing.T) {
	for _, mode := range []string{"execute", "resolveOnly"} {
		t.Run(mode, func(t *testing.T) {
			action := NewExecuteAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for a database_instance + dbname target")
				return nil, nil
			})

			inputs := map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"database_instance": "Rq-Proof-A1-DB1", "dbname": "rq_requested_db"},
			}
			var err error
			if mode == "resolveOnly" {
				_, err = action.Run(context.Background(), resolveOnlyTaskWithInputs(inputs), nil)
			} else {
				inputs["query"] = "SELECT city, country FROM cities ORDER BY city"
				inputs["resultDelivery"] = resultDeliveryInputs()
				_, err = action.Run(context.Background(), taskWithInputs(inputs), nil)
			}

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
		})
	}
}

func TestExecuteActionRejectsMixedAndPartialTargetSelectorsBeforeRPC(t *testing.T) {
	tests := []struct {
		name   string
		target map[string]interface{}
	}{
		{name: "mixed", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": "localhost", "port": 5432, "dbname": "postgres"}},
		{name: "mixed empty host", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": ""}},
		{name: "database instance with dbname", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": "rq_requested_db"}},
		{name: "mixed dbname with empty host", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": "rq_requested_db", "host": ""}},
		{name: "mixed dbname with port", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": "rq_requested_db", "port": 5432}},
		{name: "database instance with empty dbname", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": ""}},
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
			"uploadId": testUploadID, "baseUrl": testBaseURL,
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
	// The error object carries exactly code and message: the AP metadata
	// ExecutionError schema is strict, and progress metadata stays on the
	// internal stream.
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"status": "target_not_found",
		"error": map[string]interface{}{
			"code":    "target_not_found",
			"message": "no matching integration check found",
		},
	}, output)
}

// TestExecuteActionForwardsMatchFingerprint proves the optional resolve-time
// fingerprint crosses the AgentSecure request boundary when the AP execute input
// carries one.
func TestExecuteActionForwardsMatchFingerprint(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":      "postgres",
		"target":           map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":            "SELECT city, country FROM cities ORDER BY city",
		"resultDelivery":   resultDeliveryInputs(),
		"matchFingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", client.request.GetMatchFingerprint())
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

// TestBundleRegistersExecuteActionOnly proves the bundle registers the single
// execute action: the standalone resolve action is retired and resolution is the
// execute action's resolveOnly mode.
func TestBundleRegistersExecuteActionOnly(t *testing.T) {
	bundle := NewRemoteQueriesBundle()

	require.NotNil(t, bundle.GetAction(ExecuteActionName))
	assert.Nil(t, bundle.GetAction("resolve"))
}

// TestExecuteActionResolveOnlyUsesCredentialFreeAgentSecureRequestShape proves the
// resolveOnly mode routes to the resolve RPC with exactly the integration and
// target: no query, no result delivery, no fingerprint echo, the execute stream is
// never opened, and the private credentials never reach the AgentSecure request.
func TestExecuteActionResolveOnlyUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status:           "matched",
		MatchFingerprint: testFingerprint,
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	require.NotNil(t, client.resolveRequest)
	assert.Equal(t, "postgres", client.resolveRequest.GetIntegration())
	assert.Equal(t, "localhost", client.resolveRequest.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.resolveRequest.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.resolveRequest.GetTarget().GetDbname())
	assert.Empty(t, client.resolveRequest.GetTarget().GetDatabaseInstance())

	// The streaming execute RPC is never opened in resolve mode.
	assert.Nil(t, client.request)

	requestEvidence, err := json.Marshal(client.resolveRequest)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "secret-value")

	out, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, map[string]interface{}{
		"status":           "matched",
		"matchFingerprint": testFingerprint,
	}, out)
}

// TestExecuteActionResolveOnlyAcceptsDatabaseInstanceTarget proves the
// managed-instance selector maps through the resolve request like execute's target
// mapping.
func TestExecuteActionResolveOnlyAcceptsDatabaseInstanceTarget(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status:           "matched",
		MatchFingerprint: testFingerprint,
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"database_instance": "Rq-Proof-A1-DB1"},
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.resolveRequest)
	assert.Equal(t, "Rq-Proof-A1-DB1", client.resolveRequest.GetTarget().GetDatabaseInstance())
	assert.Empty(t, client.resolveRequest.GetTarget().GetHost())
}

// TestExecuteActionExplicitFalseResolveOnlyRunsExecute proves an explicit
// resolveOnly: false is the default execute mode: the resolve-mode forbidden-field
// contract does not apply and the dispatch runs the streaming execute path with the
// full execute input.
func TestExecuteActionExplicitFalseResolveOnlyRunsExecute(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), taskWithInputs(map[string]interface{}{
		"integration":      "postgres",
		"target":           map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":            "SELECT city, country FROM cities ORDER BY city",
		"resultDelivery":   resultDeliveryInputs(),
		"matchFingerprint": testFingerprint,
		"resolveOnly":      false,
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", client.request.GetQuery())
	assert.Equal(t, testFingerprint, client.request.GetMatchFingerprint())
	assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
}

// TestExecuteActionResolveOnlyRejectsExecuteOnlyFieldsBeforeRPC proves the resolveOnly
// mode is side-effect-free by construction: a resolve-mode input carrying any
// execute-only field — query, includeSchema, resultDelivery, or even a fingerprint —
// never creates the bridge client. Rejection is presence-based: the empty-string,
// explicit-false, and null variants are contract violations too, mirroring the
// retired standalone resolve action's DisallowUnknownFields structural guarantee.
func TestExecuteActionResolveOnlyRejectsExecuteOnlyFieldsBeforeRPC(t *testing.T) {
	executeOnlyInputs := []struct {
		name  string
		extra map[string]interface{}
	}{
		{
			name:  "query",
			extra: map[string]interface{}{"query": "SELECT secret FROM private_table"},
		},
		{
			name:  "query empty but present",
			extra: map[string]interface{}{"query": ""},
		},
		{
			name:  "includeSchema",
			extra: map[string]interface{}{"includeSchema": true},
		},
		{
			name:  "includeSchema explicit false",
			extra: map[string]interface{}{"includeSchema": false},
		},
		{
			name:  "resultDelivery",
			extra: map[string]interface{}{"resultDelivery": resultDeliveryInputs()},
		},
		{
			name:  "resultDelivery null but present",
			extra: map[string]interface{}{"resultDelivery": nil},
		},
		{
			name:  "matchFingerprint",
			extra: map[string]interface{}{"matchFingerprint": testFingerprint},
		},
		{
			name:  "matchFingerprint empty but present",
			extra: map[string]interface{}{"matchFingerprint": ""},
		},
	}

	for _, tt := range executeOnlyInputs {
		t.Run(tt.name, func(t *testing.T) {
			action := NewExecuteAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for a resolveOnly input with execute-only fields")
				return nil, nil
			})

			inputs := map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			}
			for key, value := range tt.extra {
				inputs[key] = value
			}

			_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(inputs), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "SELECT secret")
			assert.NotContains(t, err.Error(), testFingerprint)
		})
	}
}

// TestExecuteActionResolveOnlyRejectsInvalidTargetBeforeRPC mirrors the execute-mode
// target validation: malformed selectors never create the bridge client.
func TestExecuteActionResolveOnlyRejectsInvalidTargetBeforeRPC(t *testing.T) {
	tests := []struct {
		name   string
		target map[string]interface{}
	}{
		{name: "mixed selectors", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": "localhost", "port": 5432, "dbname": "postgres"}},
		{name: "database instance with dbname", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "dbname": "rq_requested_db"}},
		{name: "partial tuple", target: map[string]interface{}{"host": "localhost", "dbname": "postgres"}},
		{name: "whitespace instance", target: map[string]interface{}{"database_instance": " rq-proof-a1-db1 "}},
		{name: "unknown credential field", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "password": "secret-value"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewExecuteAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for an invalid resolveOnly target")
				return nil, nil
			})

			_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
				"integration": "postgres",
				"target":      tt.target,
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

// TestExecuteActionResolveOnlyPropagatesStructuredOutcomes proves the typed resolve
// response maps to the AP output envelope: non-matched statuses carry the error object
// mirroring the status, matched carries the fingerprint.
func TestExecuteActionResolveOnlyPropagatesStructuredOutcomes(t *testing.T) {
	t.Run("target not found", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "target_not_found",
			ErrorCode:    "target_not_found",
			ErrorMessage: "no matching integration check found",
		}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "target_not_found",
			"error": map[string]interface{}{
				"code":    "target_not_found",
				"message": "no matching integration check found",
			},
		}, output)
	})

	t.Run("ambiguous", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "ambiguous_target",
			ErrorCode:    "ambiguous_target",
			ErrorMessage: "multiple matching integration checks found",
		}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"database_instance": "duplicate"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "ambiguous_target",
			"error": map[string]interface{}{
				"code":    "ambiguous_target",
				"message": "multiple matching integration checks found",
			},
		}, output)
	})

	t.Run("resolution error", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "resolution_error",
			ErrorCode:    "resolution_error",
			ErrorMessage: "remote queries resolve bridge is disabled",
		}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "resolution_error",
			"error": map[string]interface{}{
				"code":    "resolution_error",
				"message": "remote queries resolve bridge is disabled",
			},
		}, output)
	})
}

// TestExecuteActionResolveOnlyPassesThroughWellFormedContractViolations proves the
// bundle maps resolve responses opaquely: a matched response without a fingerprint
// and an unknown status pass through unchanged so the dispatcher classifies them
// (resolution_error) instead of the bundle collapsing them into a transport failure.
func TestExecuteActionResolveOnlyPassesThroughWellFormedContractViolations(t *testing.T) {
	t.Run("matched without fingerprint", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{Status: "matched"}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"status": "matched"}, output)
	})

	t.Run("unknown status", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{Status: "something_unexpected"}}
		action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"status": "something_unexpected"}, output)
	})
}

// TestExecuteActionResolveOnlyFailsClosedOnMalformedResponses proves structurally
// invalid responses — a nil response or a missing status — surface as PAR action
// errors with sanitized display messages.
func TestExecuteActionResolveOnlyFailsClosedOnMalformedResponses(t *testing.T) {
	tests := []struct {
		name         string
		resp         *pb.RemoteQueryResolveResponse
		wantExternal string
	}{
		{name: "nil response", resp: nil, wantExternal: "remote query AgentSecure resolve RPC response was invalid"},
		{name: "missing status", resp: &pb.RemoteQueryResolveResponse{}, wantExternal: "remote query AgentSecure resolve RPC response was invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &captureBridgeClient{resolveResp: tt.resp}
			action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

			_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, tt.wantExternal, parErr.ExternalMessage)
		})
	}
}

// TestExecuteActionResolveOnlySurfacesTransportFailure proves an AgentSecure transport
// failure on the resolve RPC surfaces as a sanitized PAR action error.
func TestExecuteActionResolveOnlySurfacesTransportFailure(t *testing.T) {
	client := &captureBridgeClient{err: assert.AnError}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "remote query AgentSecure resolve RPC failed", parErr.ExternalMessage)
}

// TestExecuteActionResolveOnlyRequiresBridgeClient exercises the shared bridge client
// guards through the resolveOnly mode.
func TestExecuteActionResolveOnlyRequiresBridgeClient(t *testing.T) {
	t.Run("missing factory", func(t *testing.T) {
		var action *ExecuteAction

		_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action requires an Agent IPC client", parErr.Message)
	})

	t.Run("client creation failure", func(t *testing.T) {
		action := NewExecuteAction(func() (BridgeClient, error) { return nil, assert.AnError })

		_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action could not create an Agent IPC client", parErr.ExternalMessage)
	})

	t.Run("nil client", func(t *testing.T) {
		action := NewExecuteAction(func() (BridgeClient, error) { return nil, nil })

		_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action requires an AgentSecure client", parErr.Message)
	})
}

// TestExecuteActionResolveOnlySanitizesInputExtractionErrors proves a malformed
// resolve-mode task input fails closed before the bridge client with a constant,
// sanitized error.
func TestExecuteActionResolveOnlySanitizesInputExtractionErrors(t *testing.T) {
	action := NewExecuteAction(func() (BridgeClient, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, nil
	})

	_, err := action.Run(context.Background(), resolveOnlyTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"bad":         make(chan struct{}),
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "invalid remote query action inputs", parErr.Message)
	assert.Equal(t, "invalid remote query action inputs", parErr.ExternalMessage)
	assert.NotContains(t, err.Error(), "secret-db")
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
	// The AP metadata ExecuteOutputs schema is additionalProperties:false with
	// exactly status, error, and uploadReceipt.
	assert.NotContains(t, out, "attributes")
	assert.NotContains(t, out, "stream_timing")
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

// resolveOnlyTaskWithInputs builds an execute task dispatching the resolveOnly mode.
// The action name stays execute — resolution is a mode of the execute action, not a
// separate action.
func resolveOnlyTaskWithInputs(inputs map[string]interface{}) *types.Task {
	inputs["resolveOnly"] = true
	return taskWithInputs(inputs)
}

type captureBridgeClient struct {
	request        *pb.RemoteQueryExecuteRequest
	chunks         []*pb.RemoteQueryExecuteChunk
	resolveRequest *pb.RemoteQueryResolveRequest
	resolveResp    *pb.RemoteQueryResolveResponse
	err            error
}

func (c *captureBridgeClient) RemoteQueryExecuteStream(_ context.Context, req *pb.RemoteQueryExecuteRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error) {
	c.request = req
	if c.err != nil {
		return nil, c.err
	}
	return &captureRemoteQueryExecuteStream{chunks: c.chunks}, nil
}

func (c *captureBridgeClient) RemoteQueryResolve(_ context.Context, req *pb.RemoteQueryResolveRequest, _ ...grpc.CallOption) (*pb.RemoteQueryResolveResponse, error) {
	c.resolveRequest = req
	if c.err != nil {
		return nil, c.err
	}
	return c.resolveResp, nil
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
