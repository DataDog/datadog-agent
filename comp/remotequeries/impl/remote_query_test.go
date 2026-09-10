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
	"sync"
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

// TestParseMatchRequestAllowsDatabaseInstanceWithDbnameTarget proves the managed
// instance + database selector mode: database_instance selects the check and dbname
// is the requested logical execution database carried alongside, never an endpoint
// selector.
func TestParseMatchRequestAllowsDatabaseInstanceWithDbnameTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryMatchEndpointPath, strings.NewReader(
		`{"integration":"postgres","target":{"database_instance":"Rq-Proof-A1-DB1","dbname":"rq_requested_db"}}`,
	))
	req.Header.Set("Content-Type", "application/json")

	parsed, err := parseMatchRequest(req)
	require.NoError(t, err)
	assert.Equal(t, remoteQueryTarget{DatabaseInstance: "Rq-Proof-A1-DB1", DBName: "rq_requested_db"}, parsed.Target)
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
			name:      "database instance with empty dbname field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":""}}`,
			wantError: "target.dbname is required",
		},
		{
			name:      "database instance with dbname and host is still mixed",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":"rq_requested_db","host":"localhost"}}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "database instance with dbname and port is still mixed",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":"rq_requested_db","port":5432}}`,
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

// TestRemoteQueryMatchHandlerNoMatch proves the not-found outcome on the selected
// tier: a request naming no loaded endpoint is target_not_found. A request whose
// host+port match but whose dbname differs is an endpoint candidate, not a miss —
// the tiered-selection tests below cover that path.
func TestRemoteQueryMatchHandlerNoMatch(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5433,"dbname":"other"}}`)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"status":"target_not_found"`)
	assert.Contains(t, body, `"matched_count":0`)
	assert.NotContains(t, body, "secret-value")
	assert.NotContains(t, body, "other")
}

// TestRemoteQueryMatchHandlerPostgresTieredTupleSelection proves the two-tier
// Postgres tuple selection and its match_kind marker: the exact configured tuple is
// the match set when present (tier 1), and only when tier 1 is empty do the host+port
// endpoint candidates apply (tier 2), where the requested dbname is resolved
// dynamically on the integration at execute time.
func TestRemoteQueryMatchHandlerPostgresTieredTupleSelection(t *testing.T) {
	// Three checks on one endpoint: the exact configured tuple, a check whose
	// configured dbname differs, and an autodiscovery-style check with no
	// configured dbname. Distinct config providers identify which check matched.
	exact := fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: db.example.com\nport: 5432\ndbname: requested_db\npassword: secret-exact\n"}
	differentDB := fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: db.example.com\nport: 5432\ndbname: other_db\npassword: secret-different\n"}
	noConfiguredDB := fakeCheck{name: "postgres", loader: "python", provider: "ad", instance: "host: db.example.com\nport: 5432\npassword: secret-ad\n"}
	requestedTarget := `{"integration":"postgres","target":{"host":"db.example.com","port":5432,"dbname":"requested_db"}}`

	t.Run("exact configured tuple wins over endpoint candidates", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{differentDB, noConfiguredDB, exact}}}

		recorder := callMatchHandler(handler, requestedTarget)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ok"`)
		assert.Contains(t, body, `"matched_count":1`)
		assert.Contains(t, body, `"config_provider":"file"`)
		assert.Contains(t, body, `"match_kind":"exact"`)
		assert.NotContains(t, body, `"match_kind":"endpoint-candidate"`)
		assert.NotContains(t, body, "kube")
		assert.NotContains(t, body, "secret-exact")
		assert.NotContains(t, body, "secret-different")
		assert.NotContains(t, body, "secret-ad")
	})

	t.Run("endpoint candidate without configured dbname matches when no exact tuple exists", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{noConfiguredDB}}}

		recorder := callMatchHandler(handler, requestedTarget)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ok"`)
		assert.Contains(t, body, `"matched_count":1`)
		assert.Contains(t, body, `"config_provider":"ad"`)
		assert.Contains(t, body, `"match_kind":"endpoint-candidate"`)
		assert.NotContains(t, body, "secret-ad")
	})

	t.Run("endpoint candidate with different configured dbname matches when no exact tuple exists", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{differentDB}}}

		recorder := callMatchHandler(handler, requestedTarget)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ok"`)
		assert.Contains(t, body, `"matched_count":1`)
		assert.Contains(t, body, `"match_kind":"endpoint-candidate"`)
		assert.NotContains(t, body, "secret-different")
	})

	t.Run("multiple endpoint candidates are ambiguous only when no exact tuple exists", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{differentDB, noConfiguredDB}}}

		recorder := callMatchHandler(handler, requestedTarget)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ambiguous_target"`)
		assert.Contains(t, body, `"matched_count":2`)
		assert.NotContains(t, body, "secret-different")
		assert.NotContains(t, body, "secret-ad")
	})

	t.Run("no endpoint match is target not found", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{differentDB, noConfiguredDB}}}

		recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"other.example.com","port":5432,"dbname":"requested_db"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"target_not_found"`)
	})
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
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":""}`,
			wantError: "query is required",
		},
		{
			name:      "missing result delivery",
			body:      `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`,
			wantError: "result_delivery is required",
		},
		{
			name:      "unknown result delivery field",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "extra", true)),
			wantError: "resultDelivery contains unknown field",
		},
		{
			// The session-id contract has no per-session upload token: a stale result
			// delivery still carrying one is rejected fail-closed instead of tolerated.
			name:      "stale upload token in result delivery is an unknown field",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "token", "scoped-upload-token")),
			wantError: "resultDelivery contains unknown field",
		},
		{
			name:      "unknown limits field",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "limits.extra", true)),
			wantError: "limits contains unknown field",
		},
		{
			name:      "credential-like limits field is unknown",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "limits.password", "secret-value")),
			wantError: "limits contains unknown field",
		},
		{
			name:      "string includeSchema",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","includeSchema":"true","resultDelivery":%s}`, validDeliveryJSON),
			wantError: "includeSchema must be a boolean",
		},
		{
			name:      "string runId",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "runId", 243021)),
			wantError: "resultDelivery.runId must be a string",
		},
		{
			name:      "string maxFileBytes",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "limits.maxFileBytes", "33554432")),
			wantError: "resultDelivery.limits.maxFileBytes must be an integer",
		},
		{
			name:      "string limits object",
			body:      fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":%s}`, deliveryJSONObject(t, "limits", "33554432")),
			wantError: "limits must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, err := parseExecuteRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

// validDeliveryJSON is a compact, fully valid backend-injected result delivery. The
// values mirror the POC defaults: 32 MiB pages, 10 GiB total cap (below the 100 GiB
// hard ceiling), 1024 columns, 1 MiB schema, 128 pages, 30s timeout.
const validDeliveryJSON = `{"runId":"run-proof","taskId":"task-proof","artifactVersion":1,"uploadId":"upload-proof","baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake","limits":{"maxFileBytes":33554432,"maxResultBytes":10737418240,"maxRowBytes":33554432,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":128,"timeoutMs":30000}}`

// deliveryJSONObject returns the valid delivery JSON with one field overridden. Nested
// limit fields use the "limits.<field>" path.
func deliveryJSONObject(t *testing.T, path string, value interface{}) string {
	t.Helper()
	var delivery map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(validDeliveryJSON), &delivery))
	if path == "" {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		return string(encoded)
	}
	if key, nested := strings.CutPrefix(path, "limits."); nested {
		limits, ok := delivery["limits"].(map[string]interface{})
		require.True(t, ok)
		limits[key] = value
	} else {
		delivery[path] = value
	}
	encoded, err := json.Marshal(delivery)
	require.NoError(t, err)
	return string(encoded)
}

// deliveryJSONWithout returns the valid delivery JSON with fields removed. Nested limit
// fields use the "limits.<field>" path.
func deliveryJSONWithout(t *testing.T, paths ...string) string {
	t.Helper()
	var delivery map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(validDeliveryJSON), &delivery))
	for _, path := range paths {
		if key, nested := strings.CutPrefix(path, "limits."); nested {
			limits, ok := delivery["limits"].(map[string]interface{})
			require.True(t, ok)
			delete(limits, key)
		} else {
			delete(delivery, path)
		}
	}
	encoded, err := json.Marshal(delivery)
	require.NoError(t, err)
	return string(encoded)
}

func executeRequestBody(deliveryJSON string) string {
	return fmt.Sprintf(`{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","includeSchema":true,"resultDelivery":%s}`, deliveryJSON)
}

func TestParseExecuteRequestBuildsTypedPagedJSONRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(executeRequestBody(validDeliveryJSON)))
	req.Header.Set("Content-Type", "application/json")

	parsed, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "postgres", parsed.Integration)
	assert.Equal(t, RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, parsed.Target)
	assert.Equal(t, "SELECT 1 AS value", parsed.Query)
	assert.True(t, parsed.IncludeSchema)
	require.NotNil(t, parsed.ResultDelivery)
	assert.Equal(t, "run-proof", parsed.ResultDelivery.RunID)
	assert.Equal(t, "task-proof", parsed.ResultDelivery.TaskID)
	assert.Equal(t, RemoteQueryArtifactVersion, parsed.ResultDelivery.ArtifactVersion)
	assert.Equal(t, "upload-proof", parsed.ResultDelivery.UploadID)
	assert.Equal(t, "https://dd.datad0g.com/api/unstable/its-agent-intake", parsed.ResultDelivery.BaseURL)
	require.NotNil(t, parsed.ResultDelivery.Limits)
	assert.Equal(t, &RemoteQueryUploadLimits{
		MaxFileBytes:   32 << 20,
		MaxResultBytes: 10 << 30,
		MaxRowBytes:    32 << 20,
		MaxColumns:     1024,
		MaxSchemaBytes: 1 << 20,
		MaxPages:       128,
		TimeoutMs:      30000,
	}, parsed.ResultDelivery.Limits)
}

func TestParseExecuteRequestAllowsDatabaseInstanceTarget(t *testing.T) {
	body := strings.Replace(executeRequestBody(validDeliveryJSON),
		`"target":{"host":"localhost","port":5432,"dbname":"postgres"}`,
		`"target":{"database_instance":"Rq-Proof-A1-DB1"}`, 1)
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	parsed, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, RemoteQueryExecuteTarget{DatabaseInstance: "Rq-Proof-A1-DB1"}, parsed.Target)
}

// TestParseExecuteRequestAllowsDatabaseInstanceWithDbnameTarget proves the managed
// instance + requested execution database mode parses on the execute wire and
// survives the typed request boundary with both fields intact.
func TestParseExecuteRequestAllowsDatabaseInstanceWithDbnameTarget(t *testing.T) {
	body := strings.Replace(executeRequestBody(validDeliveryJSON),
		`"target":{"host":"localhost","port":5432,"dbname":"postgres"}`,
		`"target":{"database_instance":"Rq-Proof-A1-DB1","dbname":"rq_requested_db"}`, 1)
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	parsed, err := parseExecuteRequest(req)
	require.NoError(t, err)
	assert.Equal(t, RemoteQueryExecuteTarget{DatabaseInstance: "Rq-Proof-A1-DB1", DBName: "rq_requested_db"}, parsed.Target)
}

func TestParseExecuteRequestRejectsMixedDatabaseInstanceTargetSelectors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "non-empty tuple fields",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "empty host field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":""},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "empty dbname field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":""},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target.dbname is required",
		},
		{
			name:      "dbname with null host field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","dbname":"rq_requested_db","host":null},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "null host field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","host":null},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target must specify exactly one selector mode",
		},
		{
			name:      "port field",
			body:      `{"integration":"postgres","target":{"database_instance":"rq-proof-a1-db1","port":5432},"query":"SELECT 1 AS value","resultDelivery":` + validDeliveryJSON + `}`,
			wantError: "target must specify exactly one selector mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			_, err := parseExecuteRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
		})
	}
}

func TestParseExecuteRequestRejectsOmittedResultDeliveryFields(t *testing.T) {
	tests := []struct {
		name      string
		delivery  string
		wantError string
	}{
		{name: "no delivery", delivery: ``, wantError: "result_delivery is required"},
		{name: "missing runId", delivery: deliveryJSONWithout(t, "runId"), wantError: "result_delivery.runId is required"},
		{name: "missing taskId", delivery: deliveryJSONWithout(t, "taskId"), wantError: "result_delivery.taskId is required"},
		{name: "missing artifactVersion", delivery: deliveryJSONWithout(t, "artifactVersion"), wantError: "result_delivery.artifactVersion is required"},
		{name: "missing uploadId", delivery: deliveryJSONWithout(t, "uploadId"), wantError: "result_delivery.uploadId is required"},
		{name: "missing baseUrl", delivery: deliveryJSONWithout(t, "baseUrl"), wantError: "result_delivery.baseUrl is required"},
		{name: "missing limits", delivery: deliveryJSONWithout(t, "limits"), wantError: "result_delivery.limits is required"},
		{name: "missing maxFileBytes", delivery: deliveryJSONWithout(t, "limits.maxFileBytes"), wantError: "result_delivery.limits.maxFileBytes is required"},
		{name: "missing maxResultBytes", delivery: deliveryJSONWithout(t, "limits.maxResultBytes"), wantError: "result_delivery.limits.maxResultBytes is required"},
		{name: "missing maxRowBytes", delivery: deliveryJSONWithout(t, "limits.maxRowBytes"), wantError: "result_delivery.limits.maxRowBytes is required"},
		{name: "missing maxColumns", delivery: deliveryJSONWithout(t, "limits.maxColumns"), wantError: "result_delivery.limits.maxColumns is required"},
		{name: "missing maxSchemaBytes", delivery: deliveryJSONWithout(t, "limits.maxSchemaBytes"), wantError: "result_delivery.limits.maxSchemaBytes is required"},
		{name: "missing maxPages", delivery: deliveryJSONWithout(t, "limits.maxPages"), wantError: "result_delivery.limits.maxPages is required"},
		{name: "missing timeoutMs", delivery: deliveryJSONWithout(t, "limits.timeoutMs"), wantError: "result_delivery.limits.timeoutMs is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"`
			if tt.delivery != "" {
				body += `,"resultDelivery":` + tt.delivery
			}
			body += `}`

			req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			_, err := parseExecuteRequest(req)
			require.Error(t, err)
			assert.Equal(t, tt.wantError, err.Error())
			assert.NotContains(t, err.Error(), "scoped-upload-token")
		})
	}
}

func TestParseExecuteRequestRejectsInvalidIntegration(t *testing.T) {
	body := strings.Replace(executeRequestBody(validDeliveryJSON), `"integration":"postgres"`, `"integration":"my-sql"`, 1)
	req := httptest.NewRequest(http.MethodPost, RemoteQueryExecuteEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	_, err := parseExecuteRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integration contains invalid characters")
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

// pagedTestDelivery builds a fully valid typed delivery so cap and relation tests only
// vary the fields under test.
func pagedTestDelivery() *RemoteQueryResultDelivery {
	return &RemoteQueryResultDelivery{
		RunID:           "run-proof",
		TaskID:          "task-proof",
		ArtifactVersion: RemoteQueryArtifactVersion,
		UploadID:        "upload-proof",
		BaseURL:         "https://dd.datad0g.com/api/unstable/its-agent-intake",
		Limits: &RemoteQueryUploadLimits{
			MaxFileBytes:   32 << 20,
			MaxResultBytes: 10 << 30,
			MaxRowBytes:    32 << 20,
			MaxColumns:     1024,
			MaxSchemaBytes: 1 << 20,
			MaxPages:       128,
			TimeoutMs:      30000,
		},
	}
}

func TestNewRemoteQueryExecuteRequestValidation(t *testing.T) {
	target := RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}

	t.Run("valid delivery accepted", func(t *testing.T) {
		req, err := NewRemoteQueryExecuteRequest("postgres", target, "SELECT * FROM arbitrary_table", true, pagedTestDelivery())
		require.NoError(t, err)
		assert.Equal(t, "SELECT * FROM arbitrary_table", req.Query)
		assert.True(t, req.IncludeSchema)
	})

	t.Run("empty query", func(t *testing.T) {
		_, err := NewRemoteQueryExecuteRequest("postgres", target, "", false, pagedTestDelivery())
		require.Error(t, err)
		assert.EqualError(t, err, "query is required")
	})

	t.Run("bad target", func(t *testing.T) {
		_, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "", Port: 5432, DBName: "postgres"}, remoteQueryFixtureTableProofQuery, false, pagedTestDelivery())
		require.Error(t, err)
		assert.EqualError(t, err, "target.host is required")
	})

	t.Run("bad database instance target", func(t *testing.T) {
		_, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{DatabaseInstance: " rq-proof-a1-db1 "}, remoteQueryFixtureTableProofQuery, false, pagedTestDelivery())
		require.Error(t, err)
		assert.EqualError(t, err, "target.database_instance must not contain surrounding whitespace")
	})

	invalidDeliveries := []struct {
		name    string
		mutate  func(*RemoteQueryResultDelivery)
		wantErr string
	}{
		{name: "nil delivery", mutate: func(*RemoteQueryResultDelivery) {}, wantErr: "result_delivery is required"},
		{name: "missing runId", mutate: func(d *RemoteQueryResultDelivery) { d.RunID = "" }, wantErr: "result_delivery.runId is required"},
		{name: "missing taskId", mutate: func(d *RemoteQueryResultDelivery) { d.TaskID = "" }, wantErr: "result_delivery.taskId is required"},
		{name: "artifact version drift", mutate: func(d *RemoteQueryResultDelivery) { d.ArtifactVersion = 2 }, wantErr: "result_delivery.artifactVersion must be 1"},
		{name: "missing uploadId", mutate: func(d *RemoteQueryResultDelivery) { d.UploadID = "" }, wantErr: "result_delivery.uploadId is required"},
		{name: "uploadId with separators", mutate: func(d *RemoteQueryResultDelivery) { d.UploadID = "upload/proof" }, wantErr: "result_delivery.uploadId contains invalid characters"},
		{name: "missing baseUrl", mutate: func(d *RemoteQueryResultDelivery) { d.BaseURL = "" }, wantErr: "result_delivery.baseUrl is required"},
		{name: "nil limits", mutate: func(d *RemoteQueryResultDelivery) { d.Limits = nil }, wantErr: "result_delivery.limits is required"},
		{name: "zero maxFileBytes", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxFileBytes = 0 }, wantErr: "result_delivery.limits.maxFileBytes must be at least 1"},
		{name: "maxFileBytes above 128 MiB ceiling", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxFileBytes = (128 << 20) + 1 }, wantErr: fmt.Sprintf("result_delivery.limits.maxFileBytes must not exceed %d", remoteQueryUploadMaxFileBytes)},
		{name: "zero maxResultBytes", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxResultBytes = 0 }, wantErr: "result_delivery.limits.maxResultBytes must be at least 1"},
		{name: "maxResultBytes above 100 GiB cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxResultBytes = (100 << 30) + 1 }, wantErr: fmt.Sprintf("result_delivery.limits.maxResultBytes must not exceed %d", remoteQueryUploadMaxResultBytes)},
		{name: "zero maxRowBytes", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxRowBytes = 0 }, wantErr: "result_delivery.limits.maxRowBytes must be at least 1"},
		{name: "maxRowBytes above page cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxRowBytes = (32 << 20) + 1 }, wantErr: "result_delivery.limits.maxRowBytes must not exceed maxFileBytes"},
		{name: "zero maxColumns", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxColumns = 0 }, wantErr: "result_delivery.limits.maxColumns must be at least 1"},
		{name: "zero maxSchemaBytes", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxSchemaBytes = 0 }, wantErr: "result_delivery.limits.maxSchemaBytes must be at least 1"},
		{name: "zero maxPages", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxPages = 0 }, wantErr: "result_delivery.limits.maxPages must be at least 1"},
		{name: "zero timeoutMs", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.TimeoutMs = 0 }, wantErr: "result_delivery.limits.timeoutMs must be at least 1"},
		{name: "page cap above total cap", mutate: func(d *RemoteQueryResultDelivery) { d.Limits.MaxResultBytes = 16 << 20 }, wantErr: "result_delivery.limits.maxFileBytes must not exceed maxResultBytes"},
	}

	for _, tt := range invalidDeliveries {
		t.Run(tt.name, func(t *testing.T) {
			delivery := pagedTestDelivery()
			if tt.name != "nil delivery" {
				tt.mutate(delivery)
			} else {
				delivery = nil
			}
			_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, false, delivery)
			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestRemoteQueryExecuteHandlerDisabled(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: false, collector: fakeCollector{}}

	recorder := callExecuteHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value"}`)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"bridge_disabled"`)
}

func TestRemoteQueryExecuteHandlerRejectsInlineHTTPExecution(t *testing.T) {
	handler := &remoteQueryExecuteHandler{enabled: true, collector: fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: &fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}}}}}}

	// A fully valid paged-JSON request is accepted by shape validation but still
	// rejected: Remote Queries execute only over the AgentSecure streaming RPC.
	recorder := callExecuteHandler(handler, executeRequestBody(validDeliveryJSON))

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"invalid_request"`)
	assert.Contains(t, recorder.Body.String(), "streaming RPC")
	assert.NotContains(t, recorder.Body.String(), "secret-value")
	assert.NotContains(t, recorder.Body.String(), "scoped-upload-token")
}

func TestRemoteQueryExecuteServiceDispatchesPagedJSONRequest(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		events: []check.RemoteQueryStreamEvent{
			{Type: "metadata", MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages","includeSchema":true}`},
			{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":2,"totalBytes":18}}`},
		},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"}, "SELECT city, country FROM cities ORDER BY city", true, pagedTestDelivery())
	require.NoError(t, err)

	var events []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		events = append(events, event)
		return nil
	})

	require.Nil(t, result.Error)
	assert.Equal(t, runner.events, events)
	assert.Equal(t, 1, runner.streamCalls)
	assert.JSONEq(t, `{
		"operation": "produce_json_pages",
		"target": {"host": "localhost", "port": 5432, "dbname": "postgres"},
		"query": "SELECT city, country FROM cities ORDER BY city",
		"includeSchema": true,
		"resultDelivery": {
			"runId": "run-proof",
			"taskId": "task-proof",
			"artifactVersion": 1,
			"uploadId": "upload-proof",
			"baseUrl": "https://dd.datad0g.com/api/unstable/its-agent-intake",
			"limits": {"maxFileBytes": 33554432, "maxResultBytes": 10737418240, "maxRowBytes": 33554432, "maxColumns": 1024, "maxSchemaBytes": 1048576, "maxPages": 128, "timeoutMs": 30000}
		}
	}`, runner.streamSeen)
	assert.NotContains(t, runner.streamSeen, "integration")
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

// Concurrent queries on different integrations must not allocate two page buffers,
// and every runner exit must release admission for a subsequent execution.
func TestRemoteQueryExecutionAdmission(t *testing.T) {
	for _, exit := range []string{"success", "runner failure", "emitter failure"} {
		t.Run(exit, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			postgres := &fakeStreamRunnerCheck{
				fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\n"}},
				run: func(emit func(check.RemoteQueryStreamEvent) error) error {
					close(entered)
					<-release
					if exit == "runner failure" {
						return assert.AnError
					}
					return emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{}`})
				},
			}
			clickhouse := &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck{name: "clickhouse", loader: "python", provider: "file", instance: "server: localhost\nport: 8123\ndb: default\n"}}}
			// Separate service instances also share the process-wide admission boundary.
			pgService := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{postgres}}, true, true, nil)
			chService := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{clickhouse}}, true, true, nil)
			pgReq, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS value", false, pagedTestDelivery())
			require.NoError(t, err)
			chReq, err := NewRemoteQueryExecuteRequest("clickhouse", RemoteQueryExecuteTarget{Host: "localhost", Port: 8123, DBName: "default"}, "SELECT 1 AS value", false, pagedTestDelivery())
			require.NoError(t, err)
			done := make(chan RemoteQueryExecuteResult, 1)
			go func() {
				done <- pgService.ExecuteStream(context.Background(), pgReq, func(check.RemoteQueryStreamEvent) error {
					if exit == "emitter failure" {
						return assert.AnError
					}
					return nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first execution did not start")
			}
			emit := func(check.RemoteQueryStreamEvent) error { return nil }
			busy := chService.ExecuteStream(context.Background(), chReq, emit)
			assert.Equal(t, http.StatusServiceUnavailable, busy.HTTPStatus)
			assert.Zero(t, clickhouse.streamCalls)
			unblock()
			select {
			case result := <-done:
				assert.Equal(t, exit != "success", result.Error != nil)
			case <-time.After(5 * time.Second):
				t.Fatal("first execution did not return")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			assert.NotNil(t, chService.ExecuteStream(ctx, chReq, emit).Error)
			assert.Zero(t, clickhouse.streamCalls)
			assert.Nil(t, chService.ExecuteStream(context.Background(), chReq, emit).Error)
			assert.Equal(t, 1, clickhouse.streamCalls)
		})
	}
}

func TestRemoteQueryExecuteServiceDispatchesDatabaseInstanceTarget(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-value\n"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false, nil)
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{DatabaseInstance: "rq-proof-a1-db1"}, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, `"database_instance":"rq-proof-a1-db1"`)
	assert.Contains(t, runner.streamSeen, `"operation":"produce_json_pages"`)
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

func TestRemoteQueryExecuteServiceRejectsNonAllowlistedQueryByDefault(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
	assert.Equal(t, statusInvalidRequest, result.Error.Code)
	assert.Equal(t, "query is not allowed", result.Error.Message)
	assert.Equal(t, 0, runner.streamCalls)
}

func TestRemoteQueryExecuteServiceAllowsNonAllowlistedQueryWhenAllowlistDisabled(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false, nil)
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, "SELECT * FROM arbitrary_table")
}

// TestRemoteQueryExecuteServicePostgresTieredTupleSelection proves execute's
// matchExecutor computes its 0/1/many outcomes on the selected tier of the shared
// selection: the exact configured tuple wins over endpoint candidates, a lone
// endpoint candidate executes and forwards the requested dbname to the
// integration, and multiple endpoint candidates with no exact tuple stay
// ambiguous. Cross-check ambiguity is decided entirely in Go because the
// rtloader bridge forwards exactly one check to Python.
func TestRemoteQueryExecuteServicePostgresTieredTupleSelection(t *testing.T) {
	// tieredSelectionRunners builds fresh runners for each subtest: the exact
	// configured tuple, a check whose configured dbname differs, and an
	// autodiscovery-style check with no configured dbname, all on one endpoint.
	tieredSelectionRunners := func() (exact, differentDB, noConfiguredDB *fakeStreamRunnerCheck) {
		exact = &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: db.example.com\nport: 5432\ndbname: requested_db\npassword: secret-exact\n"}}}
		differentDB = &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: db.example.com\nport: 5432\ndbname: other_db\npassword: secret-different\n"}}}
		noConfiguredDB = &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "ad", instance: "host: db.example.com\nport: 5432\npassword: secret-ad\n"}}}
		return exact, differentDB, noConfiguredDB
	}
	requestedTarget := RemoteQueryExecuteTarget{Host: "db.example.com", Port: 5432, DBName: "requested_db"}

	t.Run("exact configured tuple wins over endpoint candidates", func(t *testing.T) {
		exactRunner, differentDBRunner, noConfiguredDBRunner := tieredSelectionRunners()
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: differentDBRunner}, fakeWrappedCheck{Check: noConfiguredDBRunner}, fakeWrappedCheck{Check: exactRunner},
		}}, true, false, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", requestedTarget, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.Nil(t, result.Error)
		assert.Equal(t, 1, exactRunner.streamCalls)
		assert.Zero(t, differentDBRunner.streamCalls)
		assert.Zero(t, noConfiguredDBRunner.streamCalls)
		assert.Contains(t, exactRunner.streamSeen, `"dbname":"requested_db"`)
		assert.NotContains(t, exactRunner.streamSeen, "secret-exact")
	})

	t.Run("lone endpoint candidate executes with the requested dbname", func(t *testing.T) {
		_, _, noConfiguredDBRunner := tieredSelectionRunners()
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: noConfiguredDBRunner},
		}}, true, false, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", requestedTarget, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.Nil(t, result.Error)
		assert.Equal(t, 1, noConfiguredDBRunner.streamCalls)
		assert.Contains(t, noConfiguredDBRunner.streamSeen, `"target":{"host":"db.example.com","port":5432,"dbname":"requested_db"}`)
		assert.NotContains(t, noConfiguredDBRunner.streamSeen, "secret-ad")
	})

	t.Run("multiple endpoint candidates with no exact tuple stay ambiguous", func(t *testing.T) {
		_, differentDBRunner, noConfiguredDBRunner := tieredSelectionRunners()
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: differentDBRunner}, fakeWrappedCheck{Check: noConfiguredDBRunner},
		}}, true, false, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", requestedTarget, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
		assert.Equal(t, "multiple matching integration checks found", result.Error.Message)
		assert.Zero(t, differentDBRunner.streamCalls)
		assert.Zero(t, noConfiguredDBRunner.streamCalls)
		assert.NotContains(t, result.Error.Message, "secret-different")
		assert.NotContains(t, result.Error.Message, "secret-ad")
	})

	t.Run("no endpoint match is target not found", func(t *testing.T) {
		_, differentDBRunner, _ := tieredSelectionRunners()
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: differentDBRunner},
		}}, true, false, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "other.example.com", Port: 5432, DBName: "requested_db"}, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Zero(t, differentDBRunner.streamCalls)
	})
}

// TestRemoteQueryExecuteServiceDatabaseInstanceWithDbnameForwardsBoth proves the
// managed instance + requested execution database mode survives the full
// Agent -> integration request JSON boundary with both selector fields intact.
func TestRemoteQueryExecuteServiceDatabaseInstanceWithDbnameForwardsBoth(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-value\n"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false, nil)
	req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{DatabaseInstance: "rq-proof-a1-db1", DBName: "rq_requested_db"}, "SELECT * FROM arbitrary_table", false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, `"database_instance":"rq-proof-a1-db1"`)
	assert.Contains(t, runner.streamSeen, `"dbname":"rq_requested_db"`)
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

func TestRemoteQueryExecuteServiceNoMatchAndAmbiguousAreSanitized(t *testing.T) {
	t.Run("no match", func(t *testing.T) {
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
		}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "otherhost", Port: 5432, DBName: "other"}, "SELECT 1 AS value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Equal(t, "no matching integration check found", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "secret-value")
		assert.NotContains(t, result.Error.Message, "other")
	})

	t.Run("ambiguous", func(t *testing.T) {
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}},
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}},
		}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
		assert.Equal(t, "multiple matching integration checks found", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "secret-one")
		assert.NotContains(t, result.Error.Message, "secret-two")
	})
}

func TestRemoteQueryExecuteServiceUnsupportedAndRunnerErrorAreSanitized(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
		}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
		assert.Equal(t, "matched integration check does not support remote query streaming", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "secret-value")
	})

	t.Run("runner error", func(t *testing.T) {
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			&fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}}, err: assert.AnError},
		}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
		assert.Equal(t, "remote query stream executor failed", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "secret-value")
		assert.NotContains(t, result.Error.Message, assert.AnError.Error())
	})
}

func TestRemoteQueryExecuteServiceMissingReceiptAndMalformedFinalAreExecutorErrors(t *testing.T) {
	t.Run("emit callback unavailable", func(t *testing.T) {
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{
			&fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, nil)

		require.NotNil(t, result.Error)
		assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
		assert.Equal(t, "remote query stream emitter is unavailable", result.Error.Message)
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
}

type fakeStreamRunnerCheck struct {
	fakeRunnerCheck
	events      []check.RemoteQueryStreamEvent
	streamSeen  string
	streamCalls int
	err         error
	run         func(func(check.RemoteQueryStreamEvent) error) error
}

func (f *fakeStreamRunnerCheck) RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error {
	// The runner only accepts its own check's integration name, mirroring the bridge
	// dispatching to the matched check.
	if integration != f.name {
		return assert.AnError
	}
	if f.err != nil {
		f.streamCalls++
		return f.err
	}
	f.streamCalls++
	f.streamSeen = requestJSON
	if f.run != nil {
		return f.run(emit)
	}
	for _, event := range f.events {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

const (
	// Typed int64 to mirror the production caps: the 100 GiB total cap overflows
	// 32-bit int (armhf) wherever an untyped use would default to int.
	remoteQueryPagedFileCeiling int64 = 128 << 20 // 128 MiB hard page cap ceiling
	remoteQueryPagedTotalCap    int64 = 100 << 30 // 100 GiB hard total cap
)

// TestRemoteQueryResultDeliveryPagedCaps proves the forwarding caps fail closed at the
// exact platform ceilings: 128 MiB pages, 100 GiB total result bytes.
func TestRemoteQueryResultDeliveryPagedCaps(t *testing.T) {
	target := RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}

	t.Run("128 MiB page accepted", func(t *testing.T) {
		delivery := pagedTestDelivery()
		delivery.Limits.MaxFileBytes = remoteQueryPagedFileCeiling
		delivery.Limits.MaxRowBytes = remoteQueryPagedFileCeiling
		_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, false, delivery)
		require.NoError(t, err)
	})

	t.Run("128 MiB plus one page rejected", func(t *testing.T) {
		delivery := pagedTestDelivery()
		delivery.Limits.MaxFileBytes = remoteQueryPagedFileCeiling + 1
		_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, false, delivery)
		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("result_delivery.limits.maxFileBytes must not exceed %d", remoteQueryPagedFileCeiling))
	})

	t.Run("100 GiB total accepted", func(t *testing.T) {
		delivery := pagedTestDelivery()
		delivery.Limits.MaxResultBytes = remoteQueryPagedTotalCap
		_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, false, delivery)
		require.NoError(t, err)
	})

	t.Run("100 GiB plus one total rejected", func(t *testing.T) {
		delivery := pagedTestDelivery()
		delivery.Limits.MaxResultBytes = remoteQueryPagedTotalCap + 1
		_, err := NewRemoteQueryExecuteRequest("postgres", target, remoteQueryFixtureTableProofQuery, false, delivery)
		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("result_delivery.limits.maxResultBytes must not exceed %d", remoteQueryPagedTotalCap))
	})
}

// TestRemoteQueryResultDelivery100GiBJSONFidelity proves the 100 GiB result cap survives
// the Agent -> integration request JSON boundary as an exact JSON number. The int64
// limit fields marshal 100 GiB (107,374,182,400) without truncation, so the integration
// receives the backend-owned cap verbatim.
func TestRemoteQueryResultDelivery100GiBJSONFidelity(t *testing.T) {
	delivery := pagedTestDelivery()
	delivery.Limits.MaxFileBytes = 128 << 20
	delivery.Limits.MaxRowBytes = 128 << 20
	delivery.Limits.MaxResultBytes = 100 << 30
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		remoteQueryFixtureTableProofQuery, false, delivery)
	require.NoError(t, err)

	requestJSON, err := marshalExecuteRequest(req.internal())
	require.NoError(t, err)
	assert.Contains(t, requestJSON, `"maxResultBytes":107374182400`)
	assert.Contains(t, requestJSON, `"timeoutMs":30000`)
	assert.NotContains(t, requestJSON, "api_key")
	assert.NotContains(t, requestJSON, "application_key")
}
