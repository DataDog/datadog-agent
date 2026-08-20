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

// Synthetic producer proof for the M3 Agent role after the Go upload relay was removed.
//
// The Agent no longer owns an HTTP upload transport. In POC_PUBLIC_CHUNKED_UPLOAD mode it is a
// control-plane forwarder only: it carries resultDelivery (including baseUrl and token) through
// to the integration request JSON, exposes the org API key and POC application key to the
// integration via datadog_agent.get_config, and passes the downstream emit callback straight
// through without intercepting bulk data events. The integration uploads bounded COPY chunks
// directly to its-agent-intake over HTTP; only metadata/final/error events come back through
// the stream.
//
// These tests drive ExecuteStream with a fake stream runner that captures the requestJSON the
// integration receives and replays deterministic events through emit. They prove:
//
//   1. The requestJSON forwarded to the integration carries resultDelivery.baseUrl and
//      resultDelivery.token (the credentials the integration needs to upload directly).
//   2. The requestJSON does NOT carry the org API key or POC application key: the integration
//      reads those from Agent config via get_config, so they never appear on the request wire.
//   3. The Agent passes emit straight through: the events the integration emits surface
//      downstream byte-for-byte, including any data event payloads. The Agent does not buffer,
//      re-upload, or suppress bulk bytes.
//
// The 243021 reference (task contract id) is carried as the deterministic upload session id so
// the forwarded handle is stable end-to-end.

const (
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
		Mode:        RemoteQueryResultDeliveryModeChunkedUpload,
		UploadID:    syntheticForwardUploadID,
		BaseURL:     syntheticForwardBaseURL,
		Token:       syntheticForwardToken,
		ChunkBytes:  1 << 20,  // 1 MiB
		MaxBytes:    32 << 20, // 32 MiB
		Format:      "csv",
		Compression: "none",
	}
}

// TestExecuteStreamForwardsResultDeliveryCredentialsToIntegration proves the Agent forwards
// baseUrl and token to the integration and keeps the API/application key off the request wire.
func TestExecuteStreamForwardsResultDeliveryCredentialsToIntegration(t *testing.T) {
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`},
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"mode":"POC_PUBLIC_CHUNKED_UPLOAD","uploadId":"upload-243021","bucketName":"rq-bucket","manifestPath":"its-agent-intake/poc/upload-243021/manifest.json","totalBytes":33554432,"totalRows":0,"chunkCount":32,"sha256":"aggregate-sha"}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryCopyStreamExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"},
		"SELECT repeat('x', 33554432) AS payload",
		"csv",
		&RemoteQueryExecuteCopyLimits{ChunkBytes: 1 << 20, MaxBytes: 32 << 20, MaxRowBytes: 32 << 20, TimeoutMs: 30000},
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

	// The integration receives the intake base URL and scoped upload token so it can upload
	// directly. The org API key and POC application key are NOT on the request wire: the
	// integration reads them from Agent config via datadog_agent.get_config.
	assert.Contains(t, runner.streamSeen, `"baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake"`)
	assert.Contains(t, runner.streamSeen, `"token":"scoped-upload-token-243021"`)
	assert.Contains(t, runner.streamSeen, `"uploadId":"upload-243021"`)
	assert.NotContains(t, runner.streamSeen, "api_key")
	assert.NotContains(t, runner.streamSeen, "application_key")
	assert.NotContains(t, runner.streamSeen, "app_key")

	// The integration's metadata and final receipt surface downstream unmodified.
	require.Len(t, seen, 2)
	assert.Equal(t, "metadata", seen[0].Type)
	assert.Equal(t, "final", seen[1].Type)
	assert.Contains(t, seen[1].MetadataJSON, `"uploadId":"upload-243021"`)
	assert.Contains(t, seen[1].MetadataJSON, `"manifestPath":"its-agent-intake/poc/upload-243021/manifest.json"`)
}

// TestExecuteStreamPassesDataEventsStraightThrough proves the Agent does not intercept bulk
// data events in upload mode: there is no relay, so a data event the integration emits arrives
// downstream with its payload intact. (In production the integration uploads bulk bytes over
// HTTP and emits only metadata/final/error; this test confirms the Agent gets out of the way
// if the integration does emit a data event.)
func TestExecuteStreamPassesDataEventsStraightThrough(t *testing.T) {
	payload := make([]byte, 1<<20) // 1 MiB
	for i := range payload {
		payload[i] = byte(i)
	}
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`},
		{Type: "data", MetadataJSON: `{"sequence":0,"offset":0,"bytes":1048576}`, Payload: payload},
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-243021","chunkCount":1,"totalBytes":1048576}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryCopyStreamExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"},
		"SELECT repeat('x', 1048576) AS payload",
		"csv",
		&RemoteQueryExecuteCopyLimits{ChunkBytes: 1 << 20, MaxBytes: 32 << 20, MaxRowBytes: 32 << 20, TimeoutMs: 30000},
		syntheticForwardDelivery(),
	)
	require.NoError(t, err)

	var seen []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		seen = append(seen, event)
		return nil
	})

	require.Nil(t, result.Error)

	// The data event arrives downstream with its full payload intact: the Agent did not
	// buffer, re-upload, or suppress it. There is no Go relay in the upload path.
	require.Len(t, seen, 3)
	assert.Equal(t, "data", seen[1].Type)
	require.NotNil(t, seen[1].Payload)
	assert.Equal(t, payload, seen[1].Payload)
	assert.Equal(t, 1<<20, len(seen[1].Payload))
}

// TestExecuteStreamOmittedResultDeliveryLeavesInlinePathUnchanged proves that without
// resultDelivery the Agent behaves exactly as the inline streaming path: no upload handle is
// forwarded and data events pass straight through.
func TestExecuteStreamOmittedResultDeliveryLeavesInlinePathUnchanged(t *testing.T) {
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`},
		{Type: "data", MetadataJSON: `{"sequence":0,"offset":0,"bytes":3}`, Payload: []byte{0x00, 0xff, 0x80}},
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryCopyStreamExecuteRequest(
		"postgres",
		RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"},
		"SELECT 1 AS value",
		"csv",
		&RemoteQueryExecuteCopyLimits{ChunkBytes: 4, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000},
		nil,
	)
	require.NoError(t, err)

	var seen []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		seen = append(seen, event)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, runner.events, seen)
	assert.NotContains(t, runner.streamSeen, "resultDelivery")
	assert.NotContains(t, runner.streamSeen, "baseUrl")
	assert.NotContains(t, runner.streamSeen, "token")
}
