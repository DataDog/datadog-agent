// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// remoteQueryTestConfig returns a mock Agent config for the shared test service.
func remoteQueryTestConfig(t testing.TB) config.Component {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datad0g.com")
	return cfg
}

// The session_info and upload receipt objects are arbitrary JSON the Agent only
// transports. These tests pin that the transport preserves every nested key, array, null
// and unknown future field exactly, checks nothing but "one bounded JSON object", and
// therefore cannot reintroduce field enumeration or shape discrimination.

// sessionInfoShapes are session_info objects of the two currently defined consumer shapes
// plus values no consumer would accept: the transport must forward all of them unchanged.
var sessionInfoShapes = map[string]string{
	"sessionless shape":    `{"runId":"crawl-1","taskId":"task-1","baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake","scanSensitiveData":true,"limits":{"maxFileBytes":16777216,"maxResultBytes":1073741824,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":128,"timeoutMs":30000}}`,
	"session-backed shape": `{"runId":"run-1","taskId":"task-1","uploadId":"upload-1","baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake","limits":{"maxFileBytes":33554432,"maxResultBytes":10737418240,"maxRowBytes":33554432,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":128,"timeoutMs":30000}}`,
	"future fields":        `{"runId":"r","future":{"deep":[1,null,{"k":[true,false,"v"]},[]],"empty":{}},"nullField":null,"unicode":"\u00e9"}`,
	"semantically invalid": `{"baseUrl":"http://169.254.169.254/","protocol":"made-up","artifactVersion":99,"uploadId":"has/slash","limits":"not-an-object","scanSensitiveData":"yes"}`,
	"empty object":         `{}`,
}

func sessionInfoFromRequestJSON(t *testing.T, requestJSON string) json.RawMessage {
	t.Helper()
	var request map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(requestJSON), &request))
	sessionInfo, ok := request["session_info"]
	require.True(t, ok, "integration request must carry top-level session_info")
	return sessionInfo
}

func TestExecuteStreamForwardsArbitrarySessionInfoVerbatim(t *testing.T) {
	for name, sessionInfo := range sessionInfoShapes {
		t.Run(name, func(t *testing.T) {
			runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
				{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"pageCount":0}}`},
			})
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, remoteQueryTestConfig(t))
			req, err := NewRemoteQueryExecuteRequest("postgres",
				RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
				"SELECT 1 AS value", false, json.RawMessage(sessionInfo))
			require.NoError(t, err)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.Nil(t, result.Error)
			assert.Equal(t, 1, runner.executeCalls)
			forwarded := sessionInfoFromRequestJSON(t, runner.streamSeen)
			assert.JSONEq(t, sessionInfo, string(forwarded))
			// The request carries session_info and no retired typed delivery key.
			assert.NotContains(t, runner.streamSeen, "resultDelivery")
		})
	}
}

// TestExecuteStreamDoesNotInterpretSessionInfo proves the Agent adds, removes and
// defaults nothing in session_info: no artifactVersion or protocol is injected, and a
// base URL off the Agent's site is the integration's to reject, not the transport's.
func TestExecuteStreamDoesNotInterpretSessionInfo(t *testing.T) {
	runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, remoteQueryTestConfig(t))
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT 1 AS value", false, json.RawMessage(`{"baseUrl":"https://evil.example.com/"}`))
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.JSONEq(t, `{"baseUrl":"https://evil.example.com/"}`, string(sessionInfoFromRequestJSON(t, runner.streamSeen)))
	assert.NotContains(t, runner.streamSeen, "artifactVersion")
	assert.NotContains(t, runner.streamSeen, "protocol")
}

func TestExecuteStreamRejectsHandAssembledNonObjectSessionInfo(t *testing.T) {
	for name, sessionInfo := range map[string]json.RawMessage{
		"missing": nil,
		"null":    json.RawMessage(`null`),
		"array":   json.RawMessage(`[{"a":1}]`),
		"string":  json.RawMessage(`"x"`),
		"oversized": json.RawMessage(`{"a":"` +
			strings.Repeat("x", RemoteQuerySessionInfoMaxBytes) + `"}`),
	} {
		t.Run(name, func(t *testing.T) {
			runner := newSyntheticForwardRunner(nil)
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, remoteQueryTestConfig(t))

			result := service.ExecuteStream(context.Background(), RemoteQueryExecuteRequest{
				Integration: "postgres",
				Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
				Query:       "SELECT 1 AS value",
				SessionInfo: sessionInfo,
			}, func(check.RemoteQueryStreamEvent) error { return nil })

			require.NotNil(t, result.Error)
			assert.Equal(t, statusInvalidRequest, result.Error.Code)
			assert.Equal(t, 0, runner.streamCalls)
		})
	}
}

// TestExecuteStreamForwardsArbitraryFinalMetadataUnchanged proves the Agent's executor
// neither validates nor rewrites the integration's final event: the receipt object,
// whatever its shape, surfaces downstream byte-for-byte.
func TestExecuteStreamForwardsArbitraryFinalMetadataUnchanged(t *testing.T) {
	receipts := []string{
		`{"objectIds":["0b9d2f4e-6a1c-4d3b-9f7e-1a2b3c4d5e6f"],"pageCount":1,"totalRows":2,"totalBytes":18}`,
		`{"uploadId":"upload-1","pageCount":1,"totalRows":2,"totalBytes":18}`,
		`{"anything":[1,null,{"future":{"k":[]}}],"n":null}`,
	}
	for _, receipt := range receipts {
		finalMetadata := `{"status":"SUCCEEDED","upload_receipt":` + receipt + `}`
		runner := newSyntheticForwardRunner([]check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: finalMetadata}})
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, remoteQueryTestConfig(t))
		req, err := NewRemoteQueryExecuteRequest("postgres",
			RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
			"SELECT 1 AS value", false, syntheticForwardSessionInfo())
		require.NoError(t, err)

		var seen []check.RemoteQueryStreamEvent
		result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
			seen = append(seen, event)
			return nil
		})

		require.Nil(t, result.Error)
		require.Len(t, seen, 1)
		assert.Equal(t, finalMetadata, seen[0].MetadataJSON)
	}
}

func TestValidateRemoteQueryJSONObject(t *testing.T) {
	const max = 64
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "empty object", data: `{}`},
		{name: "nested arrays nulls", data: `{"a":[1,null,{"b":[]}],"c":null}`},
		{name: "surrounding whitespace", data: " \n{\"a\":1}\t"},
		{name: "empty", data: ``, wantErr: "field is required"},
		{name: "blank", data: "  ", wantErr: "field is required"},
		{name: "null", data: `null`, wantErr: "field must be a JSON object"},
		{name: "array", data: `[{}]`, wantErr: "field must be a JSON object"},
		{name: "number", data: `1`, wantErr: "field must be a JSON object"},
		{name: "truncated", data: `{"a":`, wantErr: "field must be a JSON object"},
		{name: "two values", data: `{}{}`, wantErr: "field must be a JSON object"},
		{name: "trailing garbage", data: `{} x`, wantErr: "field must be a JSON object"},
		{name: "oversized", data: `{"a":"` + strings.Repeat("x", max) + `"}`, wantErr: fmt.Sprintf("field must not exceed %d bytes", max)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateRemoteQueryJSONObject("field", []byte(tt.data), max)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(tt.data), string(got))
		})
	}
}
