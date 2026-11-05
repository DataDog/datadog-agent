// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// The sessionless-v1 contract is an explicit opt-in second shape of the result delivery
// and of the final upload receipt. These tests pin it: the discriminated strict wire
// parse, the typed validation and experiment caps, the byte shape forwarded to the
// integration (session shape unchanged, no dummy uploadId), and the final receipt
// guard that refuses anything but the exact frozen receipt without ever rewriting it.

const (
	sessionlessObjectID1 = "0b9d2f4e-6a1c-4d3b-9f7e-1a2b3c4d5e6f"
	sessionlessObjectID2 = "7c1e8a90-52d4-4b6f-8e3a-9d0c1b2a3f45"

	sessionlessDeliveryJSON = `{"protocol":"sessionless-v1","runId":"crawl-proof","taskId":"task-proof","artifactVersion":1,"baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake","scanSensitiveData":true,"limits":{"maxFileBytes":16777216,"maxResultBytes":1073741824,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":128,"timeoutMs":30000}}`
)

// sessionlessDeliveryObject returns the valid sessionless delivery with one top-level or
// "limits.<field>" key overridden (value nil deletes it).
func sessionlessDeliveryObject(t *testing.T, path string, value interface{}, deleteKey bool) string {
	t.Helper()
	var delivery map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(sessionlessDeliveryJSON), &delivery))
	target := delivery
	key := path
	if nested, ok := strings.CutPrefix(path, "limits."); ok {
		target = delivery["limits"].(map[string]interface{})
		key = nested
	}
	if deleteKey {
		delete(target, key)
	} else {
		target[key] = value
	}
	encoded, err := json.Marshal(delivery)
	require.NoError(t, err)
	return string(encoded)
}

func sessionlessTestDelivery() *RemoteQueryResultDelivery {
	scan := true
	return &RemoteQueryResultDelivery{
		Protocol:          RemoteQueryResultDeliveryProtocolSessionlessV1,
		RunID:             "crawl-proof",
		TaskID:            "task-proof",
		ArtifactVersion:   RemoteQueryArtifactVersion,
		BaseURL:           "https://dd.datad0g.com/api/unstable/its-agent-intake",
		ScanSensitiveData: &scan,
		Limits: &RemoteQueryUploadLimits{
			MaxFileBytes:   16 << 20,
			MaxResultBytes: 1 << 30,
			MaxRowBytes:    16 << 20,
			MaxColumns:     1024,
			MaxSchemaBytes: 1 << 20,
			MaxPages:       128,
			TimeoutMs:      30000,
		},
	}
}

func sessionlessReceiptJSON(objectIDs ...string) string {
	quoted := make([]string, len(objectIDs))
	for i, id := range objectIDs {
		quoted[i] = `"` + id + `"`
	}
	return fmt.Sprintf(`{"protocol":"sessionless-v1","objectIds":[%s],"pageCount":%d,"totalRows":42,"totalBytes":4096}`, strings.Join(quoted, ","), len(objectIDs))
}

func sessionlessFinalEvent(receiptJSON string) check.RemoteQueryStreamEvent {
	return check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":` + receiptJSON + `}`}
}

func parseSessionlessWire(t *testing.T, deliveryJSON string) (RemoteQueryExecuteRequest, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(executeRequestBody(deliveryJSON)))
	req.Header.Set("Content-Type", "application/json")
	return parseExecuteRequest(req)
}

func TestParseExecuteRequestAcceptsSessionlessDelivery(t *testing.T) {
	parsed, err := parseSessionlessWire(t, sessionlessDeliveryJSON)
	require.NoError(t, err)
	require.NotNil(t, parsed.ResultDelivery)
	assert.Equal(t, RemoteQueryResultDeliveryProtocolSessionlessV1, parsed.ResultDelivery.Protocol)
	assert.Equal(t, "crawl-proof", parsed.ResultDelivery.RunID)
	assert.Empty(t, parsed.ResultDelivery.UploadID)
	require.NotNil(t, parsed.ResultDelivery.ScanSensitiveData)
	assert.True(t, *parsed.ResultDelivery.ScanSensitiveData)
	assert.Equal(t, int64(16<<20), parsed.ResultDelivery.Limits.MaxFileBytes)

	// An explicit false is a present value, not an omission.
	parsed, err = parseSessionlessWire(t, sessionlessDeliveryObject(t, "scanSensitiveData", false, false))
	require.NoError(t, err)
	require.NotNil(t, parsed.ResultDelivery.ScanSensitiveData)
	assert.False(t, *parsed.ResultDelivery.ScanSensitiveData)
}

// TestParseExecuteRequestRejectsMalformedSessionlessDelivery pins the strict
// discriminated parse: unknown fields, mixed shapes, wrong types and missing protocol
// fields all fail before any value is trusted.
func TestParseExecuteRequestRejectsMalformedSessionlessDelivery(t *testing.T) {
	tests := []struct {
		name      string
		delivery  string
		wantError string
	}{
		{name: "unknown field", delivery: sessionlessDeliveryObject(t, "bucketName", "b", false), wantError: "resultDelivery contains unknown field"},
		{name: "trusted scanning stamp", delivery: sessionlessDeliveryObject(t, "sts-scanned", true, false), wantError: "resultDelivery contains unknown field"},
		{name: "unknown limits field", delivery: sessionlessDeliveryObject(t, "limits.extra", 1, false), wantError: "limits contains unknown field"},
		{name: "uploadId alongside protocol", delivery: sessionlessDeliveryObject(t, "uploadId", "upload-proof", false), wantError: "result_delivery.uploadId must not be set for protocol sessionless-v1"},
		{name: "empty uploadId alongside protocol", delivery: sessionlessDeliveryObject(t, "uploadId", "", false), wantError: "result_delivery.uploadId must not be set for protocol sessionless-v1"},
		{name: "null uploadId alongside protocol", delivery: sessionlessDeliveryObject(t, "uploadId", nil, false), wantError: "result_delivery.uploadId must not be set for protocol sessionless-v1"},
		{name: "scan flag missing", delivery: sessionlessDeliveryObject(t, "scanSensitiveData", nil, true), wantError: "result_delivery.scanSensitiveData is required"},
		{name: "scan flag null", delivery: sessionlessDeliveryObject(t, "scanSensitiveData", nil, false), wantError: "resultDelivery.scanSensitiveData must be a boolean"},
		{name: "scan flag string", delivery: sessionlessDeliveryObject(t, "scanSensitiveData", "true", false), wantError: "resultDelivery.scanSensitiveData must be a boolean"},
		{name: "scan flag number", delivery: sessionlessDeliveryObject(t, "scanSensitiveData", 1, false), wantError: "resultDelivery.scanSensitiveData must be a boolean"},
		{name: "unknown protocol", delivery: sessionlessDeliveryObject(t, "protocol", "sessionless-v2", false), wantError: "result_delivery.protocol must be omitted or sessionless-v1"},
		{name: "empty protocol", delivery: sessionlessDeliveryObject(t, "protocol", "", false), wantError: "result_delivery.protocol must be omitted or sessionless-v1"},
		{name: "null protocol", delivery: sessionlessDeliveryObject(t, "protocol", nil, false), wantError: "resultDelivery.protocol must be a string"},
		{name: "numeric protocol", delivery: sessionlessDeliveryObject(t, "protocol", 1, false), wantError: "resultDelivery.protocol must be a string"},
		{name: "missing runId", delivery: sessionlessDeliveryObject(t, "runId", nil, true), wantError: "result_delivery.runId is required"},
		{name: "missing taskId", delivery: sessionlessDeliveryObject(t, "taskId", nil, true), wantError: "result_delivery.taskId is required"},
		{name: "missing artifactVersion", delivery: sessionlessDeliveryObject(t, "artifactVersion", nil, true), wantError: "result_delivery.artifactVersion is required"},
		{name: "missing baseUrl", delivery: sessionlessDeliveryObject(t, "baseUrl", nil, true), wantError: "result_delivery.baseUrl is required"},
		{name: "missing limits", delivery: sessionlessDeliveryObject(t, "limits", nil, true), wantError: "result_delivery.limits is required"},
		{name: "missing maxPages", delivery: sessionlessDeliveryObject(t, "limits.maxPages", nil, true), wantError: "result_delivery.limits.maxPages is required"},
		{name: "string limit", delivery: sessionlessDeliveryObject(t, "limits.maxPages", "128", false), wantError: "resultDelivery.limits.maxPages must be an integer"},
		{name: "page cap above experiment cap", delivery: sessionlessDeliveryObject(t, "limits.maxFileBytes", (16<<20)+1, false), wantError: "result_delivery.limits.maxFileBytes must not exceed 16777216 for protocol sessionless-v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSessionlessWire(t, tt.delivery)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
		})
	}
}

// TestParseExecuteRequestRejectsScanFlagOnSessionBackedDelivery proves the default shape
// cannot carry a sessionless-only field, and keeps its exact shape otherwise.
func TestParseExecuteRequestRejectsScanFlagOnSessionBackedDelivery(t *testing.T) {
	for _, value := range []interface{}{true, false, nil} {
		_, err := parseSessionlessWire(t, deliveryJSONObject(t, "scanSensitiveData", value))
		require.Error(t, err, "value %v", value)
		if value == nil {
			assert.EqualError(t, err, "resultDelivery.scanSensitiveData must be a boolean")
		} else {
			assert.EqualError(t, err, "result_delivery.scanSensitiveData requires result_delivery.protocol")
		}
	}

	parsed, err := parseSessionlessWire(t, validDeliveryJSON)
	require.NoError(t, err)
	assert.Empty(t, parsed.ResultDelivery.Protocol)
	assert.Nil(t, parsed.ResultDelivery.ScanSensitiveData)
	assert.Equal(t, "upload-proof", parsed.ResultDelivery.UploadID)
}

func TestNewRemoteQueryExecuteRequestValidatesSessionlessDelivery(t *testing.T) {
	target := RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}
	trueValue := true

	_, err := NewRemoteQueryExecuteRequest("postgres", target, "SELECT 1", false, sessionlessTestDelivery())
	require.NoError(t, err)

	tests := []struct {
		name    string
		mutate  func(*RemoteQueryResultDelivery)
		wantErr string
	}{
		{name: "unknown protocol", mutate: func(d *RemoteQueryResultDelivery) { d.Protocol = "sessionless-v2" }, wantErr: "result_delivery.protocol must be omitted or sessionless-v1"},
		{name: "uploadId set", mutate: func(d *RemoteQueryResultDelivery) { d.UploadID = "upload-proof" }, wantErr: "result_delivery.uploadId must not be set for protocol sessionless-v1"},
		{name: "scan flag unset", mutate: func(d *RemoteQueryResultDelivery) { d.ScanSensitiveData = nil }, wantErr: "result_delivery.scanSensitiveData is required"},
		{name: "scan flag on session shape", mutate: func(d *RemoteQueryResultDelivery) {
			d.Protocol = ""
			d.UploadID = "upload-proof"
			d.ScanSensitiveData = &trueValue
		}, wantErr: "result_delivery.scanSensitiveData requires result_delivery.protocol"},
		{name: "missing runId", mutate: func(d *RemoteQueryResultDelivery) { d.RunID = "" }, wantErr: "result_delivery.runId is required"},
		{name: "artifact version drift", mutate: func(d *RemoteQueryResultDelivery) { d.ArtifactVersion = 2 }, wantErr: "result_delivery.artifactVersion must be 1"},
		{name: "missing baseUrl", mutate: func(d *RemoteQueryResultDelivery) { d.BaseURL = "" }, wantErr: "result_delivery.baseUrl is required"},
		{name: "maxFileBytes above experiment cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxFileBytes = (16 << 20) + 1 }, wantErr: "result_delivery.limits.maxFileBytes must not exceed 16777216 for protocol sessionless-v1"},
		{name: "maxResultBytes above experiment cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxResultBytes = (1 << 30) + 1 }, wantErr: "result_delivery.limits.maxResultBytes must not exceed 1073741824 for protocol sessionless-v1"},
		{name: "maxPages above experiment cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxPages = 129 }, wantErr: "result_delivery.limits.maxPages must not exceed 128 for protocol sessionless-v1"},
		{name: "maxColumns above experiment cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxColumns = 1025 }, wantErr: "result_delivery.limits.maxColumns must not exceed 1024 for protocol sessionless-v1"},
		{name: "maxSchemaBytes above experiment cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxSchemaBytes = (1 << 20) + 1 }, wantErr: "result_delivery.limits.maxSchemaBytes must not exceed 1048576 for protocol sessionless-v1"},
		{name: "maxRowBytes above page cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxRowBytes = (16 << 20) + 1 }, wantErr: "result_delivery.limits.maxRowBytes must not exceed maxFileBytes"},
		{name: "smaller worker limits stay valid but zero does not", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxPages = 0 }, wantErr: "result_delivery.limits.maxPages must be at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delivery := sessionlessTestDelivery()
			tt.mutate(delivery)
			_, err := NewRemoteQueryExecuteRequest("postgres", target, "SELECT 1", false, delivery)
			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
		})
	}

	t.Run("smaller worker limits accepted", func(t *testing.T) {
		delivery := sessionlessTestDelivery()
		delivery.Limits = &RemoteQueryUploadLimits{MaxFileBytes: 1 << 20, MaxResultBytes: 4 << 20, MaxRowBytes: 1 << 10, MaxColumns: 8, MaxSchemaBytes: 4096, MaxPages: 4, TimeoutMs: 1000}
		_, err := NewRemoteQueryExecuteRequest("postgres", target, "SELECT 1", false, delivery)
		require.NoError(t, err)
	})
}

// TestMarshalExecuteRequestSessionlessDeliveryShape pins the exact forwarded delivery:
// sessionless carries protocol and the scan flag (explicit false survives) and no
// uploadId, while the session-backed JSON is unchanged — no protocol or scan keys.
func TestMarshalExecuteRequestSessionlessDeliveryShape(t *testing.T) {
	deliveryOf := func(t *testing.T, delivery *RemoteQueryResultDelivery) map[string]interface{} {
		t.Helper()
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1", false, delivery)
		require.NoError(t, err)
		requestJSON, err := marshalExecuteRequest(req.internal())
		require.NoError(t, err)
		var wire struct {
			ResultDelivery map[string]interface{} `json:"resultDelivery"`
		}
		require.NoError(t, json.Unmarshal([]byte(requestJSON), &wire))
		return wire.ResultDelivery
	}

	t.Run("sessionless scan true", func(t *testing.T) {
		got := deliveryOf(t, sessionlessTestDelivery())
		assert.Equal(t, map[string]interface{}{
			"protocol":          "sessionless-v1",
			"runId":             "crawl-proof",
			"taskId":            "task-proof",
			"artifactVersion":   float64(1),
			"baseUrl":           "https://dd.datad0g.com/api/unstable/its-agent-intake",
			"scanSensitiveData": true,
			"limits":            map[string]interface{}{"maxFileBytes": float64(16 << 20), "maxResultBytes": float64(1 << 30), "maxRowBytes": float64(16 << 20), "maxColumns": float64(1024), "maxSchemaBytes": float64(1 << 20), "maxPages": float64(128), "timeoutMs": float64(30000)},
		}, got)
		assert.NotContains(t, got, "uploadId")
	})

	t.Run("sessionless explicit false is emitted", func(t *testing.T) {
		delivery := sessionlessTestDelivery()
		scan := false
		delivery.ScanSensitiveData = &scan
		got := deliveryOf(t, delivery)
		assert.Equal(t, false, got["scanSensitiveData"])
		assert.NotContains(t, got, "uploadId")
	})

	t.Run("session-backed shape unchanged", func(t *testing.T) {
		got := deliveryOf(t, pagedTestDelivery())
		assert.NotContains(t, got, "protocol")
		assert.NotContains(t, got, "scanSensitiveData")
		assert.Equal(t, "upload-proof", got["uploadId"])
	})
}

func sessionlessExecuteRequest(t *testing.T, includeSchema bool) RemoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT 1 AS value", includeSchema, sessionlessTestDelivery())
	require.NoError(t, err)
	return req
}

func newSessionlessRunner(events ...check.RemoteQueryStreamEvent) (*fakeStreamRunnerCheck, fakeCollector) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		events:          events,
	}
	return runner, fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}
}

// TestExecuteStreamForwardsSessionlessDeliveryAndReceiptUnchanged proves the Agent hands
// the integration the discriminated delivery and forwards a valid final receipt
// byte-for-byte: it validates, it never rewrites or adds a scanning claim.
func TestExecuteStreamForwardsSessionlessDeliveryAndReceiptUnchanged(t *testing.T) {
	events := []check.RemoteQueryStreamEvent{
		{Type: "metadata", MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages"}`},
		sessionlessFinalEvent(sessionlessReceiptJSON(sessionlessObjectID1, sessionlessObjectID2)),
	}
	runner, collector := newSessionlessRunner(events...)
	service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))

	var forwarded []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), sessionlessExecuteRequest(t, false), func(event check.RemoteQueryStreamEvent) error {
		forwarded = append(forwarded, event)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, events, forwarded)
	assert.JSONEq(t, `{
		"operation": "produce_json_pages",
		"target": {"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query": "SELECT 1 AS value",
		"includeSchema": false,
		"resultDelivery": {
			"protocol": "sessionless-v1",
			"runId": "crawl-proof",
			"taskId": "task-proof",
			"artifactVersion": 1,
			"baseUrl": "https://dd.datad0g.com/api/unstable/its-agent-intake",
			"scanSensitiveData": true,
			"limits": {"maxFileBytes": 16777216, "maxResultBytes": 1073741824, "maxRowBytes": 16777216, "maxColumns": 1024, "maxSchemaBytes": 1048576, "maxPages": 128, "timeoutMs": 30000}
		}
	}`, runner.streamSeen)
	assert.NotContains(t, runner.streamSeen, "uploadId")
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

// TestExecuteStreamSessionlessKeepsSitePinnedBaseURL proves the sessionless shape does not
// loosen the Agent-side base URL check that guards the org API keys.
func TestExecuteStreamSessionlessKeepsSitePinnedBaseURL(t *testing.T) {
	runner, collector := newSessionlessRunner(sessionlessFinalEvent(sessionlessReceiptJSON(sessionlessObjectID1)))
	service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))
	delivery := sessionlessTestDelivery()
	delivery.BaseURL = "https://evil.example.com/api/unstable/its-agent-intake"
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1", true, delivery)
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Equal(t, statusInvalidRequest, result.Error.Code)
	assert.Contains(t, result.Error.Message, "must be on the Agent's site")
	assert.Zero(t, runner.executeCalls)
}

// TestExecuteStreamRejectsHandAssembledMixedDelivery proves a typed request that skipped
// the constructor still cannot forward a mixed or unknown shape.
func TestExecuteStreamRejectsHandAssembledMixedDelivery(t *testing.T) {
	scan := true
	for name, mutate := range map[string]func(*RemoteQueryResultDelivery){
		"sessionless with uploadId":    func(d *RemoteQueryResultDelivery) { d.UploadID = "upload-proof" },
		"sessionless without scan":     func(d *RemoteQueryResultDelivery) { d.ScanSensitiveData = nil },
		"session shape with scan flag": func(d *RemoteQueryResultDelivery) { d.Protocol = ""; d.UploadID = "u"; d.ScanSensitiveData = &scan },
		"unknown protocol":             func(d *RemoteQueryResultDelivery) { d.Protocol = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			runner, collector := newSessionlessRunner(sessionlessFinalEvent(sessionlessReceiptJSON(sessionlessObjectID1)))
			service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))
			req := sessionlessExecuteRequest(t, true)
			delivery := *req.ResultDelivery
			limits := *delivery.Limits
			delivery.Limits = &limits
			mutate(&delivery)
			req.ResultDelivery = &delivery

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.NotNil(t, result.Error)
			assert.Equal(t, statusInvalidRequest, result.Error.Code)
			assert.Zero(t, runner.executeCalls)
			assert.Zero(t, runner.resolveCalls)
		})
	}
}

// TestExecuteStreamRefusesInvalidSessionlessFinalReceipt proves an invalid, missing or
// session-shaped final receipt is never forwarded and fails the run closed with a
// sanitized message that does not echo receipt content.
func TestExecuteStreamRefusesInvalidSessionlessFinalReceipt(t *testing.T) {
	tests := []struct {
		name          string
		includeSchema bool
		event         check.RemoteQueryStreamEvent
		wantMessage   string
	}{
		{name: "session shaped receipt", includeSchema: true, event: check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}, wantMessage: "unknown field"},
		{name: "missing receipt", includeSchema: true, event: check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}, wantMessage: "missing upload_receipt"},
		{name: "empty metadata", includeSchema: true, event: check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: ``}, wantMessage: "metadata is malformed"},
		{name: "empty list with requested schema", includeSchema: true, event: sessionlessFinalEvent(sessionlessReceiptJSON()), wantMessage: "requested schema"},
		{name: "duplicate reference", includeSchema: true, event: sessionlessFinalEvent(sessionlessReceiptJSON(sessionlessObjectID1, sessionlessObjectID1)), wantMessage: "must be unique"},
		{name: "bad reference", includeSchema: true, event: sessionlessFinalEvent(sessionlessReceiptJSON(sessionlessObjectID1, "not-a-uuid")), wantMessage: "must be UUIDs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, collector := newSessionlessRunner(tt.event)
			service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))
			var forwarded []check.RemoteQueryStreamEvent

			result := service.ExecuteStream(context.Background(), sessionlessExecuteRequest(t, tt.includeSchema), func(event check.RemoteQueryStreamEvent) error {
				forwarded = append(forwarded, event)
				return nil
			})

			require.NotNil(t, result.Error)
			assert.Equal(t, http.StatusBadGateway, result.HTTPStatus)
			assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
			assert.Contains(t, result.Error.Message, tt.wantMessage)
			assert.NotContains(t, result.Error.Message, "upload-proof")
			assert.NotContains(t, result.Error.Message, "not-a-uuid")
			assert.Empty(t, forwarded, "an invalid receipt must never be forwarded")
			assert.Equal(t, 1, runner.executeCalls)
		})
	}
}

func TestExecuteStreamSessionlessRequiresFinalEvent(t *testing.T) {
	_, collector := newSessionlessRunner(check.RemoteQueryStreamEvent{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`})
	service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))

	result := service.ExecuteStream(context.Background(), sessionlessExecuteRequest(t, false), func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Message, "without a final event")
}

// TestExecuteStreamSessionBackedFinalReceiptIsNotInspected proves the default shape keeps
// forwarding its stream untouched: no receipt validation is added to it.
func TestExecuteStreamSessionBackedFinalReceiptIsNotInspected(t *testing.T) {
	event := check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":2,"totalBytes":18}}`}
	_, collector := newSessionlessRunner(event)
	service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))
	var forwarded []check.RemoteQueryStreamEvent

	result := service.ExecuteStream(context.Background(), executeTestRequest(t), func(e check.RemoteQueryStreamEvent) error {
		forwarded = append(forwarded, e)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, []check.RemoteQueryStreamEvent{event}, forwarded)
}

func TestValidateSessionlessUploadReceipt(t *testing.T) {
	manyIDs := make([]string, 129)
	for i := range manyIDs {
		manyIDs[i] = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
	}
	maxIDs := manyIDs[:128]
	valid := sessionlessReceiptJSON(sessionlessObjectID1, sessionlessObjectID2)
	limits := &RemoteQueryUploadLimits{MaxPages: 2, MaxResultBytes: 4096}

	t.Run("valid ordered receipt", func(t *testing.T) {
		ids, err := ValidateSessionlessUploadReceipt([]byte(valid), true, limits)
		require.NoError(t, err)
		assert.Equal(t, []string{sessionlessObjectID1, sessionlessObjectID2}, ids)
	})
	t.Run("exactly 128 references", func(t *testing.T) {
		ids, err := ValidateSessionlessUploadReceipt([]byte(sessionlessReceiptJSON(maxIDs...)), true, nil)
		require.NoError(t, err)
		assert.Len(t, ids, 128)
	})
	t.Run("empty without schema", func(t *testing.T) {
		ids, err := ValidateSessionlessUploadReceipt([]byte(`{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`), false, nil)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	tests := []struct {
		name          string
		receipt       string
		includeSchema bool
		limits        *RemoteQueryUploadLimits
		wantErr       string
	}{
		{name: "not an object", receipt: `[]`, wantErr: "must be an object"},
		{name: "null", receipt: `null`, wantErr: "must be an object"},
		{name: "unknown field", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0,"sts-scanned":true}`, wantErr: "unknown field"},
		{name: "session uploadId", receipt: `{"protocol":"sessionless-v1","uploadId":"u","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "unknown field"},
		{name: "missing protocol", receipt: `{"objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "protocol must be sessionless-v1"},
		{name: "wrong protocol", receipt: `{"protocol":"sessionless-v2","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "protocol must be sessionless-v1"},
		{name: "null protocol", receipt: `{"protocol":null,"objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "protocol must be sessionless-v1"},
		{name: "missing objectIds", receipt: `{"protocol":"sessionless-v1","pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "objectIds is required"},
		{name: "null objectIds", receipt: `{"protocol":"sessionless-v1","objectIds":null,"pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "objectIds is required"},
		{name: "objectIds not a list", receipt: `{"protocol":"sessionless-v1","objectIds":"x","pageCount":0,"totalRows":0,"totalBytes":0}`, wantErr: "objectIds has the wrong type"},
		{name: "non-string reference", receipt: `{"protocol":"sessionless-v1","objectIds":[1],"pageCount":1,"totalRows":0,"totalBytes":0}`, wantErr: "objectIds has the wrong type"},
		{name: "null reference", receipt: `{"protocol":"sessionless-v1","objectIds":[null],"pageCount":1,"totalRows":0,"totalBytes":0}`, wantErr: "must be UUIDs"},
		{name: "missing pageCount", receipt: `{"protocol":"sessionless-v1","objectIds":[],"totalRows":0,"totalBytes":0}`, wantErr: "pageCount, totalRows and totalBytes are required"},
		{name: "missing totalRows", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalBytes":0}`, wantErr: "pageCount, totalRows and totalBytes are required"},
		{name: "missing totalBytes", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0}`, wantErr: "pageCount, totalRows and totalBytes are required"},
		{name: "string count", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":"0","totalRows":0,"totalBytes":0}`, wantErr: "pageCount has the wrong type"},
		{name: "float count", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":1.5,"totalBytes":0}`, wantErr: "totalRows has the wrong type"},
		{name: "boolean count", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":true}`, wantErr: "totalBytes has the wrong type"},
		{name: "negative rows", receipt: strings.Replace(valid, `"totalRows":42`, `"totalRows":-1`, 1), wantErr: "must not be negative"},
		{name: "negative bytes", receipt: strings.Replace(valid, `"totalBytes":4096`, `"totalBytes":-1`, 1), wantErr: "must not be negative"},
		{name: "bad uuid", receipt: sessionlessReceiptJSON(sessionlessObjectID1, "not-a-uuid"), wantErr: "must be UUIDs"},
		{name: "uuid without hyphens", receipt: sessionlessReceiptJSON(strings.ReplaceAll(sessionlessObjectID1, "-", "")), wantErr: "must be UUIDs"},
		{name: "path traversal reference", receipt: sessionlessReceiptJSON("../" + sessionlessObjectID1), wantErr: "must be UUIDs"},
		{name: "duplicate reference", receipt: sessionlessReceiptJSON(sessionlessObjectID1, sessionlessObjectID1), wantErr: "must be unique"},
		{name: "uppercase uuid is not normalized", receipt: sessionlessReceiptJSON(strings.ToUpper(sessionlessObjectID1)), wantErr: "must be UUIDs"},
		{name: "mixed-case uuid", receipt: sessionlessReceiptJSON(sessionlessObjectID1[:8] + strings.ToUpper(sessionlessObjectID1[8:])), wantErr: "must be UUIDs"},
		{name: "case alias of protocol", receipt: strings.Replace(valid, `"protocol"`, `"Protocol"`, 1), wantErr: "field names must be spelled exactly"},
		{name: "case alias of objectIds", receipt: strings.Replace(valid, `"objectIds"`, `"objectIDs"`, 1), wantErr: "field names must be spelled exactly"},
		{name: "case alias of pageCount", receipt: strings.Replace(valid, `"pageCount"`, `"pagecount"`, 1), wantErr: "field names must be spelled exactly"},
		{name: "duplicate protocol", receipt: `{"protocol":"sessionless-v1",` + valid[1:], wantErr: "contains a duplicate field"},
		{name: "duplicate objectIds", receipt: `{"objectIds":[],` + valid[1:], wantErr: "contains a duplicate field"},
		{name: "duplicate pageCount", receipt: `{"pageCount":2,` + valid[1:], wantErr: "contains a duplicate field"},
		{name: "duplicate totalRows", receipt: `{"totalRows":42,` + valid[1:], wantErr: "contains a duplicate field"},
		{name: "duplicate totalBytes", receipt: `{"totalBytes":4096,` + valid[1:], wantErr: "contains a duplicate field"},
		{name: "129 references", receipt: sessionlessReceiptJSON(manyIDs...), wantErr: "more than 128 objects"},
		{name: "page count mismatch", receipt: strings.Replace(valid, `"pageCount":2`, `"pageCount":1`, 1), wantErr: "pageCount must equal"},
		{name: "empty list with page count", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":1,"totalRows":0,"totalBytes":0}`, wantErr: "pageCount must equal"},
		{name: "empty list with rows", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":1,"totalBytes":0}`, wantErr: "zero rows and bytes"},
		{name: "empty list with bytes", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":1}`, wantErr: "zero rows and bytes"},
		{name: "empty list with requested schema", receipt: `{"protocol":"sessionless-v1","objectIds":[],"pageCount":0,"totalRows":0,"totalBytes":0}`, includeSchema: true, wantErr: "requested schema"},
		{name: "oversized receipt", receipt: valid[:len(valid)-1] + `,"padding":"` + strings.Repeat("x", 16384) + `"}`, wantErr: "must not exceed 16384 bytes"},
		{name: "more pages than the delivery allows", receipt: valid, limits: &RemoteQueryUploadLimits{MaxPages: 1, MaxResultBytes: 4096}, wantErr: "maxPages limit"},
		{name: "more bytes than the delivery allows", receipt: valid, limits: &RemoteQueryUploadLimits{MaxPages: 2, MaxResultBytes: 4095}, wantErr: "maxResultBytes limit"},
		{name: "trailing value", receipt: valid + ` {}`, wantErr: "is malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := ValidateSessionlessUploadReceipt([]byte(tt.receipt), tt.includeSchema, tt.limits)
			require.Error(t, err)
			assert.Nil(t, ids)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), sessionlessObjectID1)
		})
	}
}

// withDeliveryMember prepends one raw JSON member to the valid sessionless delivery, so a
// duplicate or case-aliased spelling can be expressed (a Go map cannot hold one).
func withDeliveryMember(member string) string {
	return `{` + member + `,` + sessionlessDeliveryJSON[1:]
}

// TestParseExecuteRequestRejectsAliasedOrDuplicatedDeliveryFields proves the exact-name
// guard on the new resultDelivery and limits objects: a case alias or a duplicate of a
// discriminating or limit field is rejected with a sanitized error, so it can never
// decode as a value yet read as absent and downgrade into the session shape.
func TestParseExecuteRequestRejectsAliasedOrDuplicatedDeliveryFields(t *testing.T) {
	const aliased = "resultDelivery field names must be spelled exactly"
	const duplicate = "resultDelivery contains a duplicate field"
	tests := []struct {
		name      string
		delivery  string
		wantError string
	}{
		{name: "protocol case alias replaces protocol", delivery: strings.Replace(sessionlessDeliveryJSON, `"protocol"`, `"Protocol"`, 1), wantError: aliased},
		{name: "protocol upper alias beside protocol", delivery: withDeliveryMember(`"PROTOCOL":"sessionless-v1"`), wantError: aliased},
		{name: "protocol alias on session shape", delivery: strings.Replace(validDeliveryJSON, `{"runId"`, `{"Protocol":"sessionless-v1","runId"`, 1), wantError: aliased},
		{name: "uploadId case alias beside protocol", delivery: withDeliveryMember(`"uploadID":"upload-proof"`), wantError: aliased},
		{name: "uploadId case alias on session shape", delivery: strings.Replace(validDeliveryJSON, `"uploadId"`, `"UploadId"`, 1), wantError: aliased},
		{name: "scan flag case alias", delivery: strings.Replace(sessionlessDeliveryJSON, `"scanSensitiveData"`, `"scansensitivedata"`, 1), wantError: aliased},
		{name: "runId case alias", delivery: strings.Replace(sessionlessDeliveryJSON, `"runId"`, `"runid"`, 1), wantError: aliased},
		{name: "duplicate protocol", delivery: withDeliveryMember(`"protocol":"sessionless-v1"`), wantError: duplicate},
		{name: "duplicate protocol with different value", delivery: withDeliveryMember(`"protocol":"sessionless-v2"`), wantError: duplicate},
		{name: "duplicate scan flag", delivery: withDeliveryMember(`"scanSensitiveData":false`), wantError: duplicate},
		{name: "duplicate uploadId", delivery: strings.Replace(validDeliveryJSON, `{"runId"`, `{"uploadId":"other","runId"`, 1), wantError: duplicate},
		{name: "duplicate runId", delivery: withDeliveryMember(`"runId":"other"`), wantError: duplicate},
		{name: "limits case alias", delivery: strings.Replace(sessionlessDeliveryJSON, `"maxPages"`, `"MaxPages"`, 1), wantError: "resultDelivery.limits field names must be spelled exactly"},
		{name: "limits duplicate", delivery: strings.Replace(sessionlessDeliveryJSON, `"limits":{`, `"limits":{"maxPages":1,`, 1), wantError: "resultDelivery.limits contains a duplicate field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSessionlessWire(t, tt.delivery)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
			assert.NotContains(t, err.Error(), "upload-proof")
		})
	}
}

// TestParseExecuteRequestKeepsCanonicalDeliveriesValid proves the guard leaves ordinary
// valid input alone: the canonical sessionless shape with either scan value and the
// existing session shape still parse.
func TestParseExecuteRequestKeepsCanonicalDeliveriesValid(t *testing.T) {
	for _, scan := range []bool{true, false} {
		parsed, err := parseSessionlessWire(t, sessionlessDeliveryObject(t, "scanSensitiveData", scan, false))
		require.NoError(t, err)
		assert.Equal(t, RemoteQueryResultDeliveryProtocolSessionlessV1, parsed.ResultDelivery.Protocol)
		assert.Equal(t, scan, *parsed.ResultDelivery.ScanSensitiveData)
	}
	parsed, err := parseSessionlessWire(t, validDeliveryJSON)
	require.NoError(t, err)
	assert.Empty(t, parsed.ResultDelivery.Protocol)
	assert.Equal(t, "upload-proof", parsed.ResultDelivery.UploadID)
}
