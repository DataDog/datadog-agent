// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// Synthetic producer proof for the M3 Agent role in the paged-JSON contract.
//
// The Agent is a control-plane forwarder only: it carries the backend-injected upload
// instructions (including baseUrl and token) through to the integration request JSON,
// and the org API key and POC application key are read by the integration via
// datadog_agent.get_config, so they never appear on the request wire. The integration
// uploads bounded JSON page files directly to its-agent-intake over HTTP; only
// metadata/final/error events come back through the stream, and the only result
// accounting that crosses the AgentSecure boundary is the compact run receipt.
//
// These tests drive ExecuteStream with a fake stream runner that captures the
// requestJSON the integration receives and replays deterministic events through emit.
// They prove:
//
//  1. The requestJSON forwarded to the integration emits the fixed operation
//     produce_json_pages and carries the target, the query, the explicit includeSchema
//     flag, and the full resultDelivery: runId, taskId, artifactVersion, uploadId,
//     baseUrl, token, partBytes, and the seven nested limits including timeoutMs.
//  2. The requestJSON does NOT carry the org API key or POC application key and carries
//     no CSV/COPY-era fields: there is no format, no copyLimits, and no inline path.
//  3. The Agent passes emit straight through: the events the integration emits surface
//     downstream byte-for-byte, including the final compact run receipt.
//
// The 243021 reference (task contract id) is carried through the run/task/upload ids
// so the forwarded handle is stable end-to-end.

const (
	syntheticForwardRunID    = "run-243021"
	syntheticForwardTaskID   = "task-243021"
	syntheticForwardUploadID = "upload-243021"
	syntheticForwardBaseURL  = "https://dd.datad0g.com/api/unstable/its-agent-intake"
	syntheticForwardToken    = "scoped-upload-token-243021"
)

func newSyntheticForwardRunner(events []check.RemoteQueryStreamEvent) *fakeStreamRunnerCheck {
	return &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		events:          events,
	}
}

func syntheticForwardDelivery() *RemoteQueryResultDelivery {
	return &RemoteQueryResultDelivery{
		RunID:           syntheticForwardRunID,
		TaskID:          syntheticForwardTaskID,
		ArtifactVersion: RemoteQueryArtifactVersion,
		UploadID:        syntheticForwardUploadID,
		BaseURL:         syntheticForwardBaseURL,
		Token:           syntheticForwardToken,
		PartBytes:       1 << 20, // 1 MiB
		Limits: &RemoteQueryUploadLimits{
			MaxFileBytes:   32 << 20, // 32 MiB page cap
			MaxResultBytes: 32 << 20, // 32 MiB total cap
			MaxRowBytes:    32 << 20,
			MaxColumns:     1024,
			MaxSchemaBytes: 1 << 20,
			MaxPages:       128,
			TimeoutMs:      30000,
		},
	}
}

// TestExecuteStreamForwardsPagedJSONContractToIntegration proves the Agent forwards the
// complete backend-injected contract to the integration and keeps the API/application
// keys off the request wire.
func TestExecuteStreamForwardsPagedJSONContractToIntegration(t *testing.T) {
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "metadata", MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages","includeSchema":true}`},
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-243021","pageCount":3,"totalRows":123456,"totalBytes":987654}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"},
		"SELECT repeat('x', 33554432) AS payload",
		true,
		syntheticForwardDelivery(),
	)
	require.NoError(t, err)

	var seen []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		seen = append(seen, event)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)

	// The integration receives the fixed operation, the explicit schema flag, the
	// normalized target, and the full upload handle including the intake base URL and
	// scoped upload token so it can upload page files directly. The nested limits carry
	// the backend-owned effective values including timeoutMs.
	assert.JSONEq(t, `{
		"operation": "produce_json_pages",
		"target": {"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query": "SELECT repeat('x', 33554432) AS payload",
		"includeSchema": true,
		"resultDelivery": {
			"runId": "run-243021",
			"taskId": "task-243021",
			"artifactVersion": 1,
			"uploadId": "upload-243021",
			"baseUrl": "https://dd.datad0g.com/api/unstable/its-agent-intake",
			"token": "scoped-upload-token-243021",
			"partBytes": 1048576,
			"limits": {
				"maxFileBytes": 33554432,
				"maxResultBytes": 33554432,
				"maxRowBytes": 33554432,
				"maxColumns": 1024,
				"maxSchemaBytes": 1048576,
				"maxPages": 128,
				"timeoutMs": 30000
			}
		}
	}`, runner.streamSeen)

	// The org API key and POC application key are NOT on the request wire: the
	// integration reads them from Agent config via datadog_agent.get_config. No
	// CSV/COPY-era field survives either: there is no format, compression, mode, or
	// copyLimits anywhere in the forwarded request.
	assert.NotContains(t, runner.streamSeen, "api_key")
	assert.NotContains(t, runner.streamSeen, "application_key")
	assert.NotContains(t, runner.streamSeen, "app_key")
	assert.NotContains(t, runner.streamSeen, "format")
	assert.NotContains(t, runner.streamSeen, "compression")
	assert.NotContains(t, runner.streamSeen, "mode")
	assert.NotContains(t, runner.streamSeen, "copy")
	assert.NotContains(t, runner.streamSeen, "secret-value")
	assert.NotContains(t, runner.streamSeen, "integration")

	// The integration's metadata and final receipt surface downstream unmodified.
	require.Len(t, seen, 2)
	assert.Equal(t, "metadata", seen[0].Type)
	assert.Equal(t, "final", seen[1].Type)
	assert.JSONEq(t, `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-243021","pageCount":3,"totalRows":123456,"totalBytes":987654}}`, seen[1].MetadataJSON)
}

// TestExecuteStreamForwardsOmittedSchemaExplicitly proves the includeSchema flag is
// forwarded explicitly even when disabled: omitted or false both serialize as an
// explicit false on the integration request so the schema switch is never implicit.
func TestExecuteStreamForwardsOmittedSchemaExplicitly(t *testing.T) {
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-243021","pageCount":0,"totalRows":0,"totalBytes":0}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT city, country FROM cities ORDER BY city",
		false,
		syntheticForwardDelivery(),
	)
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Contains(t, runner.streamSeen, `"includeSchema":false`)
}

// TestExecuteStreamOmittedResultDeliveryIsRejected proves there is no inline fallback:
// without the backend-injected upload handle the request never reaches the integration.
func TestExecuteStreamOmittedResultDeliveryIsRejected(t *testing.T) {
	runner := newSyntheticForwardRunner(nil)
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)

	_, err := NewRemoteQueryExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT 1 AS value",
		false,
		nil,
	)
	require.Error(t, err)
	assert.EqualError(t, err, "result_delivery is required")

	// A hand-assembled request without a delivery fails the same way and never runs
	// the matched check.
	result := service.ExecuteStream(context.Background(), RemoteQueryExecuteRequest{
		Integration: "postgres",
		Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		Query:       "SELECT 1 AS value",
	}, func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Equal(t, statusInvalidRequest, result.Error.Code)
	assert.Equal(t, "result_delivery is required", result.Error.Message)
	assert.Equal(t, 0, runner.streamCalls)
}
