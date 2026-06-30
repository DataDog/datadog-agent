// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

func TestParseMatchRequestValidatesStrictShape(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "unknown top level field",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"extra":true}`,
			wantError: "request contains unknown field",
		},
		{
			name:      "unknown target field",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres","extra":true}}`,
			wantError: "target contains unknown field",
		},
		{
			name:      "credential-like top level field is unknown",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"password":"secret-value"}`,
			wantError: "request contains unknown field",
		},
		{
			name:      "credential-like target field is unknown",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres","username":"alice"}}`,
			wantError: "target contains unknown field",
		},
		{
			name:      "non-integer port",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432.1,"dbname":"postgres"}}`,
			wantError: "target.port must be an integer",
		},
		{
			name:      "string port",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":"5432","dbname":"postgres"}}`,
			wantError: "target.port must be an integer",
		},
		{
			name:      "missing dbname",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432}}`,
			wantError: "target.dbname is required",
		},
		{
			name:      "malformed JSON",
			body:      `{"integration":"postgres","target":`,
			wantError: "malformed JSON request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, err := parseMatchRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
			assert.NotContains(t, err.Error(), "secret-value")
			assert.NotContains(t, err.Error(), "alice")
		})
	}
}

func TestParseMatchRequestNormalizesTargetHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(
		`{"integration":"postgres","target":{"host":" LocalHost. ","port":5432,"dbname":"Postgres"}}`,
	))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	parsed, err := parseMatchRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "postgres", parsed.Integration)
	assert.Equal(t, remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "Postgres"}, parsed.Target)
}

func TestParseMatchRequestRejectsInvalidIntegration(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(
		`{"integration":"my-sql","target":{"host":"localhost","port":3306,"dbname":"mysql"}}`,
	))
	req.Header.Set("Content-Type", "application/json")

	_, err := parseMatchRequest(req)
	require.Error(t, err)
	assert.Equal(t, "integration contains invalid characters", err.Error())
}

func TestRemoteQueryMatchHandlerDisabled(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: false, collector: fakeCollector{}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"bridge_disabled"`)
}

func TestRemoteQueryMatchHandlerExactMatch(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: LOCALHOST.\nport: 5432\ndbname: postgres\nusername: alice\npassword: secret-value\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\npassword: other-secret\n"},
		fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: mysql-secret\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"ok"`)
	assert.Contains(t, body, `"matched_count":1`)
	assert.Contains(t, body, `"integration":"postgres"`)
	assert.Contains(t, body, `"loader":"python"`)
	assert.Contains(t, body, `"config_provider":"file"`)
	assert.NotContains(t, body, "alice")
	assert.NotContains(t, body, "secret-value")
	assert.NotContains(t, body, "other-secret")
	assert.NotContains(t, body, "mysql-secret")
	assert.NotContains(t, body, "InstanceConfig")
}

func TestRemoteQueryMatchHandlerNoMatch(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"other"}}`)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"target_not_found"`)
	assert.Contains(t, body, `"matched_count":0`)
	assert.NotContains(t, body, "secret-value")
	assert.NotContains(t, body, "other")
}

func TestRemoteQueryMatchHandlerAmbiguousMatch(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"ambiguous_target"`)
	assert.Contains(t, body, `"matched_count":2`)
	assert.NotContains(t, body, "secret-one")
	assert.NotContains(t, body, "secret-two")
}

func TestRemoteQueryMatchHandlerUnknownTargetFieldDoesNotEchoValue(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres","dsn":"postgres://secret-value@example/db"}}`)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"invalid_request"`)
	assert.Contains(t, body, "target contains unknown field")
	assert.NotContains(t, body, "postgres://secret-value@example/db")
	assert.NotContains(t, body, "secret-value")
}

func TestRemoteQueryMatchHandlerRejectsInvalidIntegration(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{}}

	recorder := callMatchHandler(handler, `{"integration":"my-sql","target":{"host":"localhost","port":3306,"dbname":"mysql"}}`)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
	assert.Contains(t, recorder.Body.String(), "integration contains invalid characters")
	assert.NotContains(t, recorder.Body.String(), "mysql")
}

func TestRemoteQueryMatchHandlerRejectsInvalidContentType(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{}}
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`))
	req.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()

	handler.handle(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "content-type must be application/json")
}

func callMatchHandler(handler *remoteQueryMatchHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.handle(recorder, req)
	return recorder
}

type fakeCollector struct {
	checks []check.Check
}

func (f fakeCollector) GetChecks() []check.Check { return f.checks }

type fakeCheck struct {
	name     string
	loader   string
	provider string
	instance string
}

func (f fakeCheck) Run() error { return nil }
func (f fakeCheck) Stop()      {}
func (f fakeCheck) Cancel()    {}
func (f fakeCheck) String() string {
	return f.name
}
func (f fakeCheck) Loader() string { return f.loader }
func (f fakeCheck) Configure(sender.SenderManager, uint64, integration.Data, integration.Data, string, string) error {
	return nil
}
func (f fakeCheck) Interval() time.Duration                    { return 0 }
func (f fakeCheck) ID() checkid.ID                             { return checkid.ID(f.name) }
func (f fakeCheck) GetWarnings() []error                       { return nil }
func (f fakeCheck) GetSenderStats() (stats.SenderStats, error) { return stats.SenderStats{}, nil }
func (f fakeCheck) Version() string                            { return "" }
func (f fakeCheck) ConfigSource() string                       { return "" }
func (f fakeCheck) ConfigProvider() string                     { return f.provider }
func (f fakeCheck) IsTelemetryEnabled() bool                   { return false }
func (f fakeCheck) InitConfig() string                         { return "" }
func (f fakeCheck) InstanceConfig() string                     { return f.instance }
func (f fakeCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}
func (f fakeCheck) IsHASupported() bool { return false }

func TestParseExecuteRequestValidatesStrictShape(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "unknown top level field",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","extra":true}`,
			wantError: "request contains unknown field",
		},
		{
			name:      "credential-like top level field is unknown",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","token":"secret-value"}`,
			wantError: "request contains unknown field",
		},
		{
			name:      "unknown target field",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres","extra":true},"query":"SELECT 1 AS value"}`,
			wantError: "target contains unknown field",
		},
		{
			name:      "credential-like target field is unknown",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres","password":"secret-value"},"query":"SELECT 1 AS value"}`,
			wantError: "target contains unknown field",
		},
		{
			name:      "non-exact query",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value;"}`,
			wantError: "query is not allowed",
		},
		{
			name:      "unknown limits field",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":10,"maxBytes":1048576,"timeoutMs":5000,"extra":true}}`,
			wantError: "limits contains unknown field",
		},
		{
			name:      "credential-like limits field is unknown",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":10,"maxBytes":1048576,"timeoutMs":5000,"password":"secret-value"}}`,
			wantError: "limits contains unknown field",
		},
		{
			name:      "string maxRows",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":"10","maxBytes":1048576,"timeoutMs":5000}}`,
			wantError: "limits.maxRows must be an integer",
		},
		{
			name:      "zero timeout",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":10,"maxBytes":1048576,"timeoutMs":0}}`,
			wantError: "limits.timeoutMs must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, _, err := parseExecuteRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestParseExecuteRequestNormalizesAndMarshalsCopyStreamExecutorJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":" LocalHost. ","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","copyLimits":{"chunkBytes":1024,"maxBytes":1048576,"maxRowBytes":1048576,"timeoutMs":5000}}`,
	))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	parsed, requestJSON, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "postgres", parsed.Integration)
	assert.Equal(t, "copy_stream", parsed.Operation)
	assert.Equal(t, "csv", parsed.Format)
	assert.Equal(t, remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, parsed.Target)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","format":"csv","limits":{"chunkBytes":1024,"maxBytes":1048576,"maxRowBytes":1048576,"timeoutMs":5000}}`, requestJSON)
	assert.NotContains(t, requestJSON, "integration")
}

func TestParseExecuteRequestRejectsInvalidIntegration(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"my-sql","target":{"host":"localhost","port":3306,"dbname":"mysql"},"query":"SELECT 1 AS value"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	_, _, err := parseExecuteRequest(req)
	require.Error(t, err)
	assert.Equal(t, "integration contains invalid characters", err.Error())
}

func TestParseExecuteRequestRejectsOmittedOperation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	_, _, err := parseExecuteRequest(req)
	require.Error(t, err)
	assert.Equal(t, "operation must be copy_stream", err.Error())
}

func TestParseExecuteRequestAllowsFixtureTableProofQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT city, country FROM cities ORDER BY city"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, requestJSON, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, remoteQueryFixtureTableProofQuery, parsed.Query)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT city, country FROM cities ORDER BY city","format":"csv"}`, requestJSON)
	assert.NotContains(t, requestJSON, "integration")
}

func TestNewRemoteQueryExecuteRequestRejectsInlineMode(t *testing.T) {
	_, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: " LocalHost. ", Port: 5432, DBName: "postgres"}, remoteQueryFixtureTableProofQuery, &RemoteQueryExecuteLimits{MaxRows: 2, MaxBytes: 1024, TimeoutMs: 1000})
	require.Error(t, err)
	assert.EqualError(t, err, "operation must be copy_stream")
}

func TestRemoteQueryExecuteHandlerDisabled(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: false, collector: fakeCollector{}}

	recorder := callExecuteHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"bridge_disabled"`)
}

func TestRemoteQueryExecuteHandlerRejectsInlineHTTPExecution(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: &fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}}}}}}

	recorder := callExecuteHandler(handler, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"LOCALHOST.","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "streaming executor")
	assert.NotContains(t, recorder.Body.String(), "secret-value")
}

func TestRemoteQueryExecuteServiceCopyStreamDispatch(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		events: []check.RemoteQueryStreamEvent{
			{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`},
			{Type: "data", MetadataJSON: `{"sequence":0,"offset":0,"bytes":3}`, Payload: []byte{0x00, 0xff, 0x80}},
			{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`},
		},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true)
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"}, "SELECT city, country FROM cities ORDER BY city", "csv", &RemoteQueryExecuteCopyLimits{ChunkBytes: 4, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000})
	require.NoError(t, err)

	var events []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(req, func(event check.RemoteQueryStreamEvent) error {
		events = append(events, event)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, runner.events, events)
	assert.Equal(t, 1, runner.streamCalls)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT city, country FROM cities ORDER BY city","format":"csv","limits":{"chunkBytes":4,"maxBytes":1024,"maxRowBytes":1024,"timeoutMs":1000}}`, runner.streamSeen)
	assert.NotContains(t, runner.streamSeen, "integration")
}

func TestRemoteQueryExecuteHandlerRejectsInvalidIntegration(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{}}

	recorder := callExecuteHandler(handler, `{"integration":"my-sql","target":{"host":"localhost","port":3306,"dbname":"mysql"},"query":"SELECT 1 AS value"}`)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
	assert.Contains(t, recorder.Body.String(), "integration contains invalid characters")
	assert.NotContains(t, recorder.Body.String(), "mysql")
}

func TestRemoteQueryExecuteHandlerNoMatchAndAmbiguous(t *testing.T) {
	t.Run("no match", func(t *testing.T) {
		handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		}}}

		recorder := callExecuteHandler(handler, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"other"},"query":"SELECT 1 AS value"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
		assert.NotContains(t, recorder.Body.String(), "secret-value")
		assert.NotContains(t, recorder.Body.String(), "other")
	})

	t.Run("ambiguous", func(t *testing.T) {
		handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}},
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}},
		}}}

		recorder := callExecuteHandler(handler, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
		assert.NotContains(t, recorder.Body.String(), "secret-one")
		assert.NotContains(t, recorder.Body.String(), "secret-two")
	})
}

func TestRemoteQueryExecuteHandlerUnsupportedAndRunnerErrorAreSanitized(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
		}}}

		recorder := callExecuteHandler(handler, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
		assert.NotContains(t, recorder.Body.String(), "secret-value")
	})

	t.Run("runner error", func(t *testing.T) {
		handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}, err: assert.AnError},
		}}}

		recorder := callExecuteHandler(handler, `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
		assert.NotContains(t, recorder.Body.String(), "secret-value")
		assert.NotContains(t, recorder.Body.String(), assert.AnError.Error())
	})
}

func callExecuteHandler(handler *remoteQueryExecuteHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.handle(recorder, req)
	return recorder
}

type fakeWrappedCheck struct {
	check.Check
}

func (f fakeWrappedCheck) Unwrap() check.Check {
	return f.Check
}

type fakeRunnerCheck struct {
	fakeCheck
	err error
}

type fakeStreamRunnerCheck struct {
	fakeRunnerCheck
	events      []check.RemoteQueryStreamEvent
	streamSeen  string
	streamCalls int
}

func (f *fakeStreamRunnerCheck) RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error {
	if integration != "postgres" {
		return assert.AnError
	}
	f.streamCalls++
	f.streamSeen = requestJSON
	for _, event := range f.events {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}
