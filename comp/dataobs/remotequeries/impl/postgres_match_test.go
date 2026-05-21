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

	"github.com/DataDog/datadog-agent/comp/collector/collector"
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
			body:      `{"target":{"host":"localhost","port":5432,"dbname":"postgres"},"extra":true}`,
			wantError: "request contains unknown field",
		},
		{
			name:      "unknown target field",
			body:      `{"target":{"host":"localhost","port":5432,"dbname":"postgres","extra":true}}`,
			wantError: "target contains unknown field",
		},
		{
			name:      "credential-shaped top level field",
			body:      `{"target":{"host":"localhost","port":5432,"dbname":"postgres"},"password":"secret-value"}`,
			wantError: "request contains disallowed credential-shaped field",
		},
		{
			name:      "credential-shaped target field",
			body:      `{"target":{"host":"localhost","port":5432,"dbname":"postgres","username":"alice"}}`,
			wantError: "request contains disallowed credential-shaped field",
		},
		{
			name:      "non-integer port",
			body:      `{"target":{"host":"localhost","port":5432.1,"dbname":"postgres"}}`,
			wantError: "target.port must be an integer",
		},
		{
			name:      "string port",
			body:      `{"target":{"host":"localhost","port":"5432","dbname":"postgres"}}`,
			wantError: "target.port must be an integer",
		},
		{
			name:      "missing dbname",
			body:      `{"target":{"host":"localhost","port":5432}}`,
			wantError: "target.dbname is required",
		},
		{
			name:      "malformed JSON",
			body:      `{"target":`,
			wantError: "malformed JSON request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, PostgresMatchEndpointPath, strings.NewReader(tt.body))
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
	req := httptest.NewRequest(http.MethodPost, PostgresMatchEndpointPath, strings.NewReader(
		`{"target":{"host":" LocalHost. ","port":5432,"dbname":"Postgres"}}`,
	))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	target, err := parseMatchRequest(req)
	require.NoError(t, err)
	assert.Equal(t, postgresTarget{Host: "localhost", Port: 5432, DBName: "Postgres"}, target)
}

func TestPostgresMatchHandlerDisabled(t *testing.T) {
	handler := &postgresMatchHandler{enabled: false, collector: fakeCollector{}}

	recorder := callMatchHandler(handler, `{"target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"bridge_disabled"`)
}

func TestPostgresMatchHandlerExactMatch(t *testing.T) {
	handler := &postgresMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: LOCALHOST.\nport: 5432\ndbname: postgres\nusername: alice\npassword: secret-value\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\npassword: other-secret\n"},
		fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: mysql-secret\n"},
	}}}

	recorder := callMatchHandler(handler, `{"target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

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

func TestPostgresMatchHandlerNoMatch(t *testing.T) {
	handler := &postgresMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
	}}}

	recorder := callMatchHandler(handler, `{"target":{"host":"localhost","port":5432,"dbname":"other"}}`)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"target_not_found"`)
	assert.Contains(t, body, `"matched_count":0`)
	assert.NotContains(t, body, "secret-value")
	assert.NotContains(t, body, "other")
}

func TestPostgresMatchHandlerAmbiguousMatch(t *testing.T) {
	handler := &postgresMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"},
	}}}

	recorder := callMatchHandler(handler, `{"target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"ambiguous_target"`)
	assert.Contains(t, body, `"matched_count":2`)
	assert.NotContains(t, body, "secret-one")
	assert.NotContains(t, body, "secret-two")
}

func TestPostgresMatchHandlerCredentialRequestDoesNotEchoValue(t *testing.T) {
	handler := &postgresMatchHandler{enabled: true, collector: fakeCollector{}}

	recorder := callMatchHandler(handler, `{"target":{"host":"localhost","port":5432,"dbname":"postgres","dsn":"postgres://secret-value@example/db"}}`)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"invalid_request"`)
	assert.Contains(t, body, "credential-shaped field")
	assert.NotContains(t, body, "postgres://secret-value@example/db")
	assert.NotContains(t, body, "secret-value")
}

func TestPostgresMatchHandlerRejectsInvalidContentType(t *testing.T) {
	handler := &postgresMatchHandler{enabled: true, collector: fakeCollector{}}
	req := httptest.NewRequest(http.MethodPost, PostgresMatchEndpointPath, strings.NewReader(`{"target":{"host":"localhost","port":5432,"dbname":"postgres"}}`))
	req.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()

	handler.handle(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "content-type must be application/json")
}

func callMatchHandler(handler *postgresMatchHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, PostgresMatchEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.handle(recorder, req)
	return recorder
}

type fakeCollector struct {
	checks []check.Check
}

func (f fakeCollector) RunCheck(inner check.Check) (checkid.ID, error) { return inner.ID(), nil }
func (f fakeCollector) StopCheck(checkid.ID) error                     { return nil }
func (f fakeCollector) MapOverChecks(cb func([]check.Info))            {}
func (f fakeCollector) GetChecks() []check.Check                       { return f.checks }
func (f fakeCollector) ReloadAllCheckInstances(string, []check.Check) ([]checkid.ID, error) {
	return nil, nil
}
func (f fakeCollector) AddEventReceiver(collector.EventReceiver) {}

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
