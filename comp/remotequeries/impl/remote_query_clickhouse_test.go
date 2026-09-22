// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// ClickHouse routing proof.
//
// The AP/PAR wire contract stays {host, port, dbname} for every integration; for
// ClickHouse those are normalized aliases of the check's instance-config triple
// {server, port, db}. These tests prove the Agent-side bridge:
//
//  1. parses ClickHouse instance config by its canonical field names, applying the
//     check's documented defaults (port 8123, database `default`) when the keys are
//     absent and failing closed on present-but-invalid values or the deprecated
//     `host` alias;
//  2. matches the required wire dbname exactly against the effective database, so an
//     instance only runs queries against the database it actually monitors;
//  3. renders the database_instance identifier the check itself emits — the default
//     template $server:$port:$db over the raw configured server and the effective
//     port and database — instead of guessing;
//  4. keeps the generic bridge import datadog_checks.<integration>.remote_query
//     untouched: the Agent routes and resolves targets but never inspects query
//     text, which belongs to the backend and the matched integration executor.

func TestClickHouseTupleTargetMatching(t *testing.T) {
	tests := []struct {
		name        string
		instance    string
		target      remoteQueryTarget
		wantMatched bool
	}{
		{
			name:        "server port db alias the wire host port dbname",
			instance:    "server: ch.local\nport: 8123\ndb: analytics\nusername: default\npassword: secret-value\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: true,
		},
		{
			name:        "wire host normalization applies to server",
			instance:    "server: CH.Local.\nport: 8123\ndb: analytics\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: true,
		},
		{
			name:        "explicit default database matches wire dbname default",
			instance:    "server: ch.local\nport: 8123\ndb: default\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: true,
		},
		{
			name:        "absent db matches the wire dbname default",
			instance:    "server: ch.local\nport: 8123\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: true,
		},
		{
			name:        "absent port matches the check HTTP default port",
			instance:    "server: ch.local\ndb: analytics\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: true,
		},
		{
			name:        "minimal config matches the effective default endpoint",
			instance:    "server: ch.local\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: true,
		},
		{
			name:        "explicit db is compared exactly",
			instance:    "server: ch.local\ndb: Analytics\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: false,
		},
		{
			name:        "explicit db does not match a different wire dbname",
			instance:    "server: ch.local\ndb: default\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: false,
		},
		{
			name:        "absent db does not match a non-default wire dbname",
			instance:    "server: ch.local\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "analytics"},
			wantMatched: false,
		},
		{
			name:        "native protocol port does not match the HTTP endpoint",
			instance:    "server: ch.local\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 9000, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "postgres-shaped config is not an accidental alias",
			instance:    "host: ch.local\nport: 8123\ndbname: default\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "deprecated host alias is not accepted for server",
			instance:    "host: ch.local\nport: 8123\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "missing server fails closed",
			instance:    "port: 8123\ndb: default\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "empty server fails closed",
			instance:    "server: \"\"\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "null port fails closed",
			instance:    "server: ch.local\nport:\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "string port fails closed",
			instance:    "server: ch.local\nport: \"8123\"\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "out of range port fails closed",
			instance:    "server: ch.local\nport: 70000\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "null db fails closed",
			instance:    "server: ch.local\ndb:\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "empty explicit db fails closed",
			instance:    "server: ch.local\ndb: \"\"\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "non-string db fails closed",
			instance:    "server: ch.local\ndb: 12\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
		{
			name:        "malformed config fails closed",
			instance:    "server: [unclosed\n",
			target:      remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"},
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceTarget, ok := parseIntegrationInstanceTarget("clickhouse", tt.instance)
			assert.Equal(t, tt.wantMatched, ok && instanceTarget.matches(tt.target))
		})
	}
}

func TestClickHouseDatabaseInstanceTargetMatching(t *testing.T) {
	tests := []struct {
		name        string
		instance    string
		target      remoteQueryTarget
		wantMatched bool
	}{
		{
			name:        "default template renders the effective server port db",
			instance:    "server: ch.local\nport: 8123\ndb: analytics\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:analytics"},
			wantMatched: true,
		},
		{
			name:        "default template applies the check defaults",
			instance:    "server: ch.local\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"},
			wantMatched: true,
		},
		{
			name:        "default template keeps the raw configured server",
			instance:    "server: CH.Local.\n",
			target:      remoteQueryTarget{DatabaseInstance: "CH.Local.:8123:default"},
			wantMatched: true,
		},
		{
			name:        "the identifier is not normalized when the check would not",
			instance:    "server: CH.Local.\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"},
			wantMatched: false,
		},
		{
			name:        "custom template from config tags",
			instance:    "server: ch.local\ntags:\n  - rq_database_instance:rq-ch-a1\ndatabase_identifier:\n  template: $rq_database_instance\n",
			target:      remoteQueryTarget{DatabaseInstance: "rq-ch-a1"},
			wantMatched: true,
		},
		{
			name:        "custom template composes server port db",
			instance:    "server: ch.local\ndb: analytics\ndatabase_identifier:\n  template: $db@$server:$port\n",
			target:      remoteQueryTarget{DatabaseInstance: "analytics@ch.local:8123"},
			wantMatched: true,
		},
		{
			name:        "unknown template variable fails closed",
			instance:    "server: ch.local\ndatabase_identifier:\n  template: $resolved_hostname\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local"},
			wantMatched: false,
		},
		{
			name:        "non-map database identifier fails closed",
			instance:    "server: ch.local\ndatabase_identifier: rq-ch-a1\n",
			target:      remoteQueryTarget{DatabaseInstance: "rq-ch-a1"},
			wantMatched: false,
		},
		{
			name:        "identifier without template fails closed",
			instance:    "server: ch.local\ndatabase_identifier:\n  other: value\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"},
			wantMatched: false,
		},
		{
			name:        "empty explicit template fails closed",
			instance:    "server: ch.local\ndatabase_identifier:\n  template: \"\"\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"},
			wantMatched: false,
		},
		{
			name:        "non-string template fails closed",
			instance:    "server: ch.local\ndatabase_identifier:\n  template: 12\n",
			target:      remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"},
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceTarget, ok := parseIntegrationInstanceTarget("clickhouse", tt.instance)
			assert.Equal(t, tt.wantMatched, ok && instanceTarget.matches(tt.target))
		})
	}
}

// TestClickHouseIdentifierFailureDoesNotDisableTupleMatching proves identifier
// rendering is only consulted for the database_instance selector: when it cannot be
// rendered faithfully the tuple selector still matches.
func TestClickHouseIdentifierFailureDoesNotDisableTupleMatching(t *testing.T) {
	instanceTarget, ok := parseIntegrationInstanceTarget("clickhouse", "server: ch.local\ndatabase_identifier:\n  template: $resolved_hostname\n")
	require.True(t, ok)
	assert.Empty(t, instanceTarget.databaseInstance)
	assert.True(t, instanceTarget.matches(remoteQueryTarget{Host: "ch.local", Port: 8123, DBName: "default"}))
	assert.False(t, instanceTarget.matches(remoteQueryTarget{DatabaseInstance: "ch.local:8123:default"}))
}

func TestRemoteQueryMatchHandlerClickHouse(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "clickhouse", loader: "python", provider: "file", instance: "server: CH.Local.\nport: 8123\ndb: default\nusername: default\npassword: secret-value\n"},
		fakeCheck{name: "clickhouse", loader: "python", provider: "file", instance: "server: other.local\nport: 8123\ndb: default\npassword: other-secret\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: ch.local\nport: 8123\ndbname: default\npassword: pg-secret\n"},
	}}}

	t.Run("tuple selector", func(t *testing.T) {
		recorder := callMatchHandler(handler, `{"integration":"clickhouse","target":{"host":"ch.local","port":8123,"dbname":"default"}}`)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ok"`)
		assert.Contains(t, body, `"matched_count":1`)
		assert.Contains(t, body, `"integration":"clickhouse"`)
		assert.Contains(t, body, `"loader":"python"`)
		assert.Contains(t, body, `"config_provider":"file"`)
		assert.NotContains(t, body, "secret-value")
		assert.NotContains(t, body, "other-secret")
		assert.NotContains(t, body, "pg-secret")
	})

	t.Run("database instance selector uses the check's identifier", func(t *testing.T) {
		recorder := callMatchHandler(handler, `{"integration":"clickhouse","target":{"database_instance":"CH.Local.:8123:default"}}`)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ok"`)
		assert.Contains(t, body, `"matched_count":1`)
		assert.NotContains(t, body, "secret-value")
		assert.NotContains(t, body, "other-secret")
	})

	t.Run("wrong database does not match", func(t *testing.T) {
		recorder := callMatchHandler(handler, `{"integration":"clickhouse","target":{"host":"ch.local","port":8123,"dbname":"analytics"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"target_not_found"`)
	})
}

// TestRemoteQueryMatchHandlerFailsClosedForUnresolvableIntegration proves an
// integration with loaded checks but no integration-owned resolver fails the
// sweep as a resolution error, never a silent no-match: the sweep asks every
// loaded check, and a check that cannot provide the bridge resolver is an
// error. An integration with no loaded checks at all stays target_not_found.
func TestRemoteQueryMatchHandlerFailsClosedForUnresolvableIntegration(t *testing.T) {
	t.Run("loaded checks without the bridge resolver", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 3306\ndbname: mysql\npassword: secret-value\n"},
		}}}

		recorder := callMatchHandler(handler, `{"integration":"mysql","target":{"host":"localhost","port":3306,"dbname":"mysql"}}`)

		assert.Equal(t, http.StatusFailedDependency, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"resolution_error"`)
		assert.Contains(t, body, "loaded integration check does not support remote query resolution")
		assert.NotContains(t, body, "secret-value")
	})

	t.Run("no loaded checks of the integration", func(t *testing.T) {
		handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
			fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 3306\ndbname: mysql\npassword: secret-value\n"},
		}}}

		recorder := callMatchHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"target_not_found"`)
		assert.NotContains(t, recorder.Body.String(), "secret-value")
	})
}

// TestPostgresMatchingIsDelegatedToTheResolver proves the Go matcher no longer
// parses Postgres instance configs at all: Postgres eligibility is
// autodiscovery-aware and lives in the integration, so the config shape is
// deliberately untaught and parsing fails closed. Postgres matching happens only
// through the resolver sweep.
func TestPostgresMatchingIsDelegatedToTheResolver(t *testing.T) {
	for _, instance := range []string{
		"host: LocalHost.\nport: 5432\ndbname: postgres\nreported_hostname: rq-proof-a1-db1\n",
		"host: localhost\nport: 5432\n",
		"host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\n",
	} {
		_, ok := parseIntegrationInstanceTarget("postgres", instance)
		assert.False(t, ok, instance)
	}
}

func newClickHouseStreamRunner(events []check.RemoteQueryStreamEvent) *fakeStreamRunnerCheck {
	return &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "clickhouse", loader: "python", provider: "file", instance: "server: localhost\nport: 8123\ndb: default\nusername: default\npassword: secret-value\n"}},
		events:          events,
	}
}

func TestRemoteQueryExecuteServiceClickHouseDatabaseInstanceTarget(t *testing.T) {
	runner := newClickHouseStreamRunner([]check.RemoteQueryStreamEvent{
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`},
	})
	runner.instance = "server: localhost\ndatabase_identifier:\n  template: $rq_database_instance\ntags:\n  - rq_database_instance:rq-ch-a1\npassword: secret-value\n"
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, nil)
	req, err := NewRemoteQueryExecuteRequest("clickhouse", RemoteQueryExecuteTarget{DatabaseInstance: "rq-ch-a1"}, "SELECT 'hello world' AS message", false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, `"database_instance":"rq-ch-a1"`)
	assert.NotContains(t, runner.streamSeen, "secret-value")
}
