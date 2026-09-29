// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/devtracing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	testRunID    = "run-01k"
	testTaskID   = "task-01k"
	testUploadID = "upload-01k"
	testBaseURL  = "https://dd.datad0g.com/api/unstable/its-agent-intake"
)

// TestMain keeps the real development tracer out of every unit test. Development
// tracing is enabled by default (the development endpoint constant is non-empty),
// and letting a test start the real tracer would replace whichever tracer owns the
// process — the mocktracer in the development-span tests — with a live flush loop
// pointed at the development endpoint. The span path itself stays under test
// through the mocktracer; only the process-wide tracer start is stubbed out.
func TestMain(m *testing.M) {
	devtracing.DisableTracerStartForTest()
	os.Exit(m.Run())
}

// setDevelopmentTracing flips the development-tracing gate for one test and
// restores the production value on cleanup.
func setDevelopmentTracing(t *testing.T, enabled bool) func() {
	t.Helper()
	return devtracing.SetEnabledForTest(enabled)
}

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
// target: no query, no result delivery, the execute stream is never opened, and the
// private credentials never reach the AgentSecure request.
func TestExecuteActionResolveOnlyUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status: "matched",
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
	// The matched resolve output carries exactly the status: there is no
	// resolve-time binding to revalidate on execute.
	assert.Equal(t, map[string]interface{}{
		"status": "matched",
	}, out)
}

// TestExecuteActionResolveOnlyAcceptsDatabaseInstanceTarget proves the
// managed-instance selector maps through the resolve request like execute's target
// mapping.
func TestExecuteActionResolveOnlyAcceptsDatabaseInstanceTarget(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status: "matched",
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
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":          "SELECT city, country FROM cities ORDER BY city",
		"resultDelivery": resultDeliveryInputs(),
		"resolveOnly":    false,
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", client.request.GetQuery())
	assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
}

// TestExecuteActionResolveOnlyRejectsExecuteOnlyFieldsBeforeRPC proves the resolveOnly
// mode is side-effect-free by construction: a resolve-mode input carrying any
// execute-only field — query, includeSchema, or resultDelivery — never creates the
// bridge client. Rejection is presence-based: the empty-string, explicit-false, and
// null variants are contract violations too, mirroring the retired standalone
// resolve action's DisallowUnknownFields structural guarantee.
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
// mirroring the status, and matched carries the status only.
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

// TestExecuteActionResolveOnlyPassesThroughStatusOpaquely proves the bundle maps
// resolve responses opaquely: a matched response and an unknown status pass through
// unchanged so the dispatcher classifies them (resolution_error for the unknown
// case) instead of the bundle collapsing them into a transport failure.
func TestExecuteActionResolveOnlyPassesThroughStatusOpaquely(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
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
// output is bounded by construction: with no inline result-byte path the AP
// artifact stays tiny even for multi-page runs at the maximum limits, so the
// pinned ceiling covers the output shape.
func TestRemoteQueryExecuteOutputStaysUnderActionPlatformLimit(t *testing.T) {
	const actionPlatformOutputLimitBytes = 15 * 1024 * 1024
	stream := &captureRemoteQueryExecuteStream{chunks: []*pb.RemoteQueryExecuteChunk{
		metadataEvent(0),
		finalEvent(1, &pb.RemoteQueryUploadReceipt{
			UploadId:   "upload-01k",
			PageCount:  128,
			TotalRows:  1099511627776,
			TotalBytes: 10737418240,
		}, map[string]string{"stats.rowsEmitted": "1099511627776", "agent_total_stream_ms": "12.345"}),
		finalMarker(2),
	}}

	output, err := remoteQueryExecuteOutputFromStream(stream, testUploadID)
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", output["status"])
	assert.Len(t, output, 2, "output keys are exactly status and uploadReceipt")
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

// activeActionRunContext mirrors the private-action-runner task executor: the
// action.run span is started from the task-supplied trace and parent IDs, so the
// context the action runs under carries the action.run span's own identity. It
// returns the context together with the identity the mini-tracer would propagate
// to a child process (telemetry.EnvFromContext): the trace ID is the task's
// trace, and the parent is the action.run span's own ID.
func activeActionRunContext(t *testing.T, taskTraceID, taskParentSpanID uint64) (context.Context, uint64, uint64) {
	t.Helper()
	ctx := telemetry.WithService(context.Background(), observability.ParService)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, taskTraceID, taskParentSpanID)
	t.Cleanup(func() { span.Finish(nil) })

	propagatedTraceID, propagatedParentID := miniTracerEnvTraceIdentity(t, ctx)
	require.Equal(t, taskTraceID, propagatedTraceID, "the action.run context must propagate the task's trace")
	return ctx, propagatedTraceID, propagatedParentID
}

// miniTracerEnvTraceIdentity reads the trace identity the mini-tracer propagates
// for ctx through the raw environment contract the bundle consumes — the
// DATADOG_TRACE_ID and DATADOG_PARENT_ID entries of telemetry.EnvFromContext,
// pinned here by their literal names so a contract drift on either side fails
// the test.
func miniTracerEnvTraceIdentity(t *testing.T, ctx context.Context) (traceID, parentID uint64) {
	t.Helper()
	var haveTraceID, haveParentID bool
	for _, entry := range telemetry.EnvFromContext(ctx) {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "DATADOG_TRACE_ID":
			id, err := strconv.ParseUint(value, 10, 64)
			require.NoError(t, err)
			traceID, haveTraceID = id, true
		case "DATADOG_PARENT_ID":
			id, err := strconv.ParseUint(value, 10, 64)
			require.NoError(t, err)
			parentID, haveParentID = id, true
		}
	}
	require.True(t, haveTraceID && haveParentID, "the action.run context must propagate an active trace")
	return traceID, parentID
}

// executeInputsForTraceContextTests is the minimal valid execute-mode input.
func executeInputsForTraceContextTests() map[string]interface{} {
	return map[string]interface{}{
		"integration":    "postgres",
		"target":         map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query":          "SELECT 1 AS value",
		"resultDelivery": resultDeliveryInputs(),
	}
}

// TestExecuteActionAttachesActiveActionRunTraceContext proves the execute dispatch
// carries the runner's active action.run trace through the AgentSecure request:
// the trace ID is the task's trace, the parent ID is the action.run span's own
// ID — not the task-supplied parent — and the sampling priority is the mini-tracer's
// effective keep priority. The identity is read through the mini-tracer's
// environment contract (EnvFromContext). The typed field also survives a binary
// proto round-trip, so the generated bindings carry it on the real gRPC boundary.
func TestExecuteActionAttachesActiveActionRunTraceContext(t *testing.T) {
	// Development tracing disabled: the propagation-only path, where the request
	// carries the runner's own identity.
	defer setDevelopmentTracing(t, false)()

	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	const taskTraceID = uint64(1234567890123456789)
	const taskParentSpanID = uint64(200)
	ctx, _, activeParentID := activeActionRunContext(t, taskTraceID, taskParentSpanID)

	_, err := action.Run(ctx, taskWithInputs(executeInputsForTraceContextTests()), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	traceContext := client.request.GetTraceContext()
	require.NotNil(t, traceContext, "the AgentSecure execute request must carry the active action.run trace context")
	assert.Equal(t, taskTraceID, traceContext.GetTraceId())
	assert.Equal(t, activeParentID, traceContext.GetParentId())
	assert.NotEqual(t, taskParentSpanID, traceContext.GetParentId(), "the parent ID must be the action.run span, not its parent")
	assert.Equal(t, int32(2), traceContext.GetSamplingPriority())

	// Generated-binding contract surface: the typed field serializes on the
	// binary gRPC wire and decodes back to the same request.
	encoded, err := proto.Marshal(client.request)
	require.NoError(t, err)
	decoded := &pb.RemoteQueryExecuteRequest{}
	require.NoError(t, proto.Unmarshal(encoded, decoded))
	assert.True(t, proto.Equal(client.request, decoded))
}

// TestExecuteActionOmitsTraceContextWithoutActiveTrace proves trace propagation is
// fail-open: a background context and a dropped trace (priority <= 0) leave the
// optional field absent, and the request executes exactly as before.
func TestExecuteActionOmitsTraceContextWithoutActiveTrace(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "background context", ctx: context.Background()},
		{name: "dropped trace", ctx: func() context.Context {
			ctx := telemetry.WithSamplingPriority(context.Background(), 0)
			_, ctx = telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, 1234, 200)
			return ctx
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
				finalEvent(0, validReceipt(), nil),
				finalMarker(1),
			}}
			action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

			output, err := action.Run(tc.ctx, taskWithInputs(executeInputsForTraceContextTests()), nil)

			require.NoError(t, err)
			require.NotNil(t, client.request)
			assert.Nil(t, client.request.GetTraceContext(), "no active trace means no traceContext field")
			assert.Equal(t, "SUCCEEDED", output.(map[string]interface{})["status"])
		})
	}
}

// TestExecuteActionResolveOnlyIsUnchangedByTraceContext proves the resolveOnly mode
// never gains trace propagation: the resolve request is side-effect-free Agent-local
// matching with no integration execution and no upload, and its credential-free
// shape stays exactly the integration and target.
func TestExecuteActionResolveOnlyIsUnchangedByTraceContext(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status: "matched",
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	ctx, _, _ := activeActionRunContext(t, 1234567890123456789, 200)

	output, err := action.Run(ctx, resolveOnlyTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.resolveRequest)
	assert.Equal(t, "postgres", client.resolveRequest.GetIntegration())
	assert.Equal(t, "localhost", client.resolveRequest.GetTarget().GetHost())
	assert.Nil(t, client.request, "the streaming execute RPC is never opened in resolve mode")
	assert.Equal(t, "matched", output.(map[string]interface{})["status"])
}

// TestExecuteActionIgnoresTaskSuppliedTraceContext proves the propagated trace
// context is the runner's own active action.run identity, never task data: a
// stale or hostile AP input carrying a traceContext object is tolerated by the
// tolerant input decode but never reaches the AgentSecure request when the
// action has no active trace.
func TestExecuteActionIgnoresTaskSuppliedTraceContext(t *testing.T) {
	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	inputs := executeInputsForTraceContextTests()
	inputs["traceContext"] = map[string]interface{}{
		"traceId":          "1111111111111111111",
		"parentId":         "2222222222222222222",
		"samplingPriority": 2,
	}

	_, err := action.Run(context.Background(), taskWithInputs(inputs), nil)

	require.NoError(t, err)
	require.NotNil(t, client.request)
	assert.Nil(t, client.request.GetTraceContext(), "trace context is read from the action context only, never from task inputs")
	evidence, err := json.Marshal(client.request)
	require.NoError(t, err)
	assert.NotContains(t, string(evidence), "1111111111111111111")
}

// TestExecuteActionDevelopmentSpanWrapsExecution proves the development tracing
// span: with the development endpoint configured, the execution is wrapped in
// exactly one remote_queries.agent_execute span that continues the runner's
// trace (the task's trace ID, the action.run span as parent, the propagated
// sampling priority), is tagged with the run's identity and terminal status
// only, and becomes the parent the AgentSecure request propagates so the
// integration's spans nest beneath it.
func TestExecuteActionDevelopmentSpanWrapsExecution(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	const taskTraceID = uint64(1234567890123456789)
	const taskParentSpanID = uint64(200)
	ctx, activeTraceID, activeParentID := activeActionRunContext(t, taskTraceID, taskParentSpanID)

	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(ctx, taskWithInputs(executeInputsForTraceContextTests()), nil)

	require.NoError(t, err)
	spans := mt.FinishedSpans()
	require.Len(t, spans, 1, "development tracing must record exactly the agent_execute span")
	span := spans[0]
	assert.Equal(t, "remote_queries.agent_execute", span.OperationName())

	// Parenting: the span continues the runner's trace as a child of the active
	// action.run span, inheriting its sampling priority.
	assert.Equal(t, activeTraceID, span.TraceID())
	assert.Equal(t, activeParentID, span.ParentID())
	assert.Equal(t, float64(2), span.Tag("_sampling_priority_v1"))

	// Tags: the run's identity, the integration, and the terminal status only.
	assert.Equal(t, testRunID, span.Tag("run_id"))
	assert.Equal(t, testTaskID, span.Tag("task_id"))
	assert.Equal(t, "postgres", span.Tag("integration"))
	assert.Equal(t, "SUCCEEDED", span.Tag("status"))

	// The span never carries the query text, the target, the upload session, or
	// any credential or token.
	spanEvidence, err := json.Marshal(span.Tags())
	require.NoError(t, err)
	assert.NotContains(t, string(spanEvidence), "SELECT")
	assert.NotContains(t, string(spanEvidence), "localhost")
	assert.NotContains(t, string(spanEvidence), testUploadID)
	assert.NotContains(t, string(spanEvidence), testBaseURL)

	// The request's trace context is the span's own identity, so the integration's
	// spans nest beneath the agent_execute span rather than beside it.
	traceContext := client.request.GetTraceContext()
	require.NotNil(t, traceContext, "the development span's identity must become the propagated trace context")
	assert.Equal(t, span.TraceID(), traceContext.GetTraceId())
	assert.Equal(t, span.SpanID(), traceContext.GetParentId())
	assert.NotEqual(t, activeParentID, traceContext.GetParentId(), "the propagated parent must be the agent_execute span, not the action.run span")
	assert.Equal(t, int32(2), traceContext.GetSamplingPriority())
}

// TestExecuteActionDevelopmentSpanContinuesPropagatedSamplingPriority proves the
// development span inherits a runner-propagated sampling priority (here user
// keep, 1) instead of falling back to the mini-tracer's flush default, and that
// the propagated request context carries the same priority.
func TestExecuteActionDevelopmentSpanContinuesPropagatedSamplingPriority(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	ctx := telemetry.WithSamplingPriority(context.Background(), 1)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, 1234567890123456789, 200)
	t.Cleanup(func() { span.Finish(nil) })

	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(ctx, taskWithInputs(executeInputsForTraceContextTests()), nil)

	require.NoError(t, err)
	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, float64(1), spans[0].Tag("_sampling_priority_v1"))
	traceContext := client.request.GetTraceContext()
	require.NotNil(t, traceContext)
	assert.Equal(t, int32(1), traceContext.GetSamplingPriority())
}

// TestExecuteActionDevelopmentTracingDisabledCreatesNoSpan proves the disabled
// development switch (the endpoint constant empty): no span is created at all,
// and the AgentSecure request carries the runner's own identity exactly as
// before development tracing existed.
func TestExecuteActionDevelopmentTracingDisabledCreatesNoSpan(t *testing.T) {
	defer setDevelopmentTracing(t, false)()

	mt := mocktracer.Start()
	defer mt.Stop()

	const taskTraceID = uint64(1234567890123456789)
	ctx, activeTraceID, activeParentID := activeActionRunContext(t, taskTraceID, 200)

	client := &captureBridgeClient{chunks: []*pb.RemoteQueryExecuteChunk{
		finalEvent(0, validReceipt(), nil),
		finalMarker(1),
	}}
	action := NewExecuteAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(ctx, taskWithInputs(executeInputsForTraceContextTests()), nil)

	require.NoError(t, err)
	assert.Empty(t, mt.FinishedSpans(), "the disabled development tracer must not create spans")
	traceContext := client.request.GetTraceContext()
	require.NotNil(t, traceContext)
	assert.Equal(t, activeTraceID, traceContext.GetTraceId())
	assert.Equal(t, activeParentID, traceContext.GetParentId())
	assert.Equal(t, int32(2), traceContext.GetSamplingPriority())
}
