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
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
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

func TestParseMatchRequestAllowsDatabaseInstanceTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(
		`{"integration":"postgres","target":{"database_instance":"Rq-Proof-A1-DB1"}}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, err := parseMatchRequest(req)
	require.NoError(t, err)
	assert.Equal(t, remoteQueryTarget{DatabaseInstance: "Rq-Proof-A1-DB1"}, parsed.Target)
}

func TestParseMatchRequestRejectsMixedAndPartialTargetSelectors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "mixed database instance and tuple",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":"localhost","port":5432,"dbname":"postgres"}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "mixed database instance and empty host field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":""}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "mixed database instance and empty dbname field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":""}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "mixed database instance and null host field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":null}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "mixed database instance and port field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","port":5432}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "database instance must be non-empty",
			body:      `{"integration":"postgres","target":{"database_instance":""}}`,
			wantError: "target.database_instance is required",
		},
		{
			name:      "database instance rejects surrounding whitespace",
			body:      `{"integration":"postgres","target":{"database_instance":" rq-proof-a1-db1 "}}`,
			wantError: "target.database_instance must not contain surrounding whitespace",
		},
		{
			name:      "partial tuple",
			body:      `{"integration":"postgres","target":{"host":"localhost","dbname":"postgres"}}`,
			wantError: "target.port is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, err := parseMatchRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
		})
	}
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

func TestRemoteQueryMatchHandlerDatabaseInstanceMatch(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-value\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a2-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: other-secret\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1"}}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"ok"`)
	assert.Contains(t, body, `"matched_count":1`)
	assert.NotContains(t, body, "secret-value")
	assert.NotContains(t, body, "other-secret")
}

func TestRemoteQueryMatchHandlerDatabaseInstanceFailClosed(t *testing.T) {
	t.Run("unsupported template is not guessed", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ndatabase_identifier:\n  template: $resolved_hostname\npassword: secret-value\n"},
		}}}

		recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"database_instance":"localhost"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"target_not_found"`)
		assert.NotContains(t, recorder.Body.String(), "secret-value")
	})

	t.Run("ambiguous", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:duplicate\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-one\n"},
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\ntags:\n  - rq_database_instance:duplicate\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-two\n"},
		}}}

		recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"database_instance":"duplicate"}}`)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"ambiguous_target"`)
		assert.Contains(t, recorder.Body.String(), `"matched_count":2`)
		assert.NotContains(t, recorder.Body.String(), "secret-one")
		assert.NotContains(t, recorder.Body.String(), "secret-two")
	})
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
			name:      "empty query",
			body:      `{"integration":"postgres","operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":""}`,
			wantError: "query is required",
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

func TestParseExecuteRequestAllowsNonProofQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT * FROM arbitrary_table"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, requestJSON, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "SELECT * FROM arbitrary_table", parsed.Query)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT * FROM arbitrary_table","format":"csv"}`, requestJSON)
}

func TestParseExecuteRequestAllowsDatabaseInstanceTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"Rq-Proof-A1-DB1"},"query":"SELECT * FROM arbitrary_table"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, requestJSON, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, remoteQueryTarget{DatabaseInstance: "Rq-Proof-A1-DB1"}, parsed.Target)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"database_instance":"Rq-Proof-A1-DB1"},"query":"SELECT * FROM arbitrary_table","format":"csv"}`, requestJSON)
}

func TestParseExecuteRequestRejectsMixedDatabaseInstanceTargetSelectors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "non-empty tuple fields",
			body: `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"rq-proof-a1-db1","host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT * FROM arbitrary_table"}`,
		},
		{
			name: "empty host field",
			body: `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"rq-proof-a1-db1","host":""},"query":"SELECT * FROM arbitrary_table"}`,
		},
		{
			name: "empty dbname field",
			body: `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"rq-proof-a1-db1","dbname":""},"query":"SELECT * FROM arbitrary_table"}`,
		},
		{
			name: "null host field",
			body: `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"rq-proof-a1-db1","host":null},"query":"SELECT * FROM arbitrary_table"}`,
		},
		{
			name: "port field",
			body: `{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"database_instance":"rq-proof-a1-db1","port":5432},"query":"SELECT * FROM arbitrary_table"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, _, err := parseExecuteRequest(req)
			require.Error(t, err)
			assert.Equal(t, "target must specify exactly one selector mode", err.Error())
		})
	}
}

func TestParseExecuteRequestRejectsInvalidFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"json","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	_, _, err := parseExecuteRequest(req)
	require.Error(t, err)
	assert.Equal(t, "format must be csv or binary", err.Error())
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

func TestParseExecuteRequestAllowsMatrixIdentityProofQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(
		`{"integration":"postgres","operation":"copy_stream","format":"csv","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT current_database() AS current_db, expected_agent_hostname, expected_postgres_host, expected_postgres_port, expected_dbname, marker FROM remote_query_identity"}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, requestJSON, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, remoteQueryMatrixIdentityProofQuery, parsed.Query)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT current_database() AS current_db, expected_agent_hostname, expected_postgres_host, expected_postgres_port, expected_dbname, marker FROM remote_query_identity","format":"csv"}`, requestJSON)
	assert.NotContains(t, requestJSON, "integration")
}

func TestNewRemoteQueryExecuteRequestRejectsInlineMode(t *testing.T) {
	_, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: " LocalHost. ", Port: 5432, DBName: "postgres"}, remoteQueryFixtureTableProofQuery, &RemoteQueryExecuteLimits{MaxRows: 2, MaxBytes: 1024, TimeoutMs: 1000})
	require.Error(t, err)
	assert.EqualError(t, err, "operation must be copy_stream")
}

func TestRemoteQueriesQueryAllowlistEnabledConfigDefault(t *testing.T) {
	t.Run("missing key defaults enabled", func(t *testing.T) {
		cfg := configcomp.NewMock(t)

		assert.True(t, RemoteQueriesQueryAllowlistEnabled(cfg))
	})

	t.Run("explicit true enables", func(t *testing.T) {
		cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{RemoteQueriesEnableQueryAllowlistConfig: true})

		assert.True(t, RemoteQueriesQueryAllowlistEnabled(cfg))
	})

	t.Run("explicit false disables", func(t *testing.T) {
		cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{RemoteQueriesEnableQueryAllowlistConfig: false})

		assert.False(t, RemoteQueriesQueryAllowlistEnabled(cfg))
	})
}

func TestNewRemoteQueryCopyStreamExecuteRequestValidation(t *testing.T) {
	target := RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}

	t.Run("allows non proof query", func(t *testing.T) {
		req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", target, "SELECT * FROM arbitrary_table", "csv", nil)
		require.NoError(t, err)
		assert.Equal(t, "SELECT * FROM arbitrary_table", req.Query)
	})

	t.Run("empty query", func(t *testing.T) {
		_, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", target, "", "csv", nil)
		require.Error(t, err)
		assert.EqualError(t, err, "query is required")
	})

	t.Run("bad operation", func(t *testing.T) {
		_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, &RemoteQueryExecuteLimits{MaxRows: 2, MaxBytes: 1024, TimeoutMs: 1000})
		require.Error(t, err)
		assert.EqualError(t, err, "operation must be copy_stream")
	})

	t.Run("bad target", func(t *testing.T) {
		_, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "", Port: 5432, DBName: "postgres"}, remoteQueryFixtureTableProofQuery, "csv", nil)
		require.Error(t, err)
		assert.EqualError(t, err, "target.host is required")
	})

	t.Run("bad database instance target", func(t *testing.T) {
		_, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{DatabaseInstance: " rq-proof-a1-db1 "}, remoteQueryFixtureTableProofQuery, "csv", nil)
		require.Error(t, err)
		assert.EqualError(t, err, "target.database_instance must not contain surrounding whitespace")
	})

	t.Run("bad format", func(t *testing.T) {
		_, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, "json", nil)
		require.Error(t, err)
		assert.EqualError(t, err, "format must be csv or binary")
	})

	t.Run("bad limits", func(t *testing.T) {
		_, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, "csv", &RemoteQueryExecuteCopyLimits{ChunkBytes: 0, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000})
		require.Error(t, err)
		assert.EqualError(t, err, "copyLimits.chunkBytes must be at least 1")
	})
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
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true)
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

func TestRemoteQueryExecuteServiceCopyStreamDispatchesDatabaseInstanceTarget(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-value\n"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false)
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{DatabaseInstance: "rq-proof-a1-db1"}, "SELECT * FROM arbitrary_table", "csv", nil)
	require.NoError(t, err)

	result := service.ExecuteStream(req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.JSONEq(t, `{"operation":"copy_stream","target":{"database_instance":"rq-proof-a1-db1"},"query":"SELECT * FROM arbitrary_table","format":"csv"}`, runner.streamSeen)
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

func TestRemoteQueryExecuteServiceRejectsNonAllowlistedQueryByDefault(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true)
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT * FROM arbitrary_table", "csv", nil)
	require.NoError(t, err)

	result := service.ExecuteStream(req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
	assert.Equal(t, statusInvalidRequest, result.Error.Code)
	assert.Equal(t, "query is not allowed", result.Error.Message)
	assert.Equal(t, 0, runner.streamCalls)
}

func TestRemoteQueryExecuteServiceAllowsNonAllowlistedQueryWhenAllowlistDisabled(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false)
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT * FROM arbitrary_table", "csv", nil)
	require.NoError(t, err)

	result := service.ExecuteStream(req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, "SELECT * FROM arbitrary_table")
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
