// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
//  4. mirrors the integrations-core ClickHouse executor's corrected proof-query
//     allowlist exactly — the Agent accepts no proof query the executor would reject
//     and rejects none it accepts — while fixture-dependent Postgres proof SQL can
//     never reach a ClickHouse executor and the generic bridge import
//     datadog_checks.<integration>.remote_query stays untouched.

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

// TestRemoteQueryMatchHandlerFailsClosedForUnknownIntegration proves the
// instance-config registry is explicit: an integration the bridge has not been taught
// never matches, even with a config shape that happens to parse.
func TestRemoteQueryMatchHandlerFailsClosedForUnknownIntegration(t *testing.T) {
	handler := &remoteQueryMatchHandler{enabled: true, collector: fakeCollector{checks: []check.Check{
		fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 3306\ndbname: mysql\npassword: secret-value\n"},
	}}}

	recorder := callMatchHandler(handler, `{"integration":"mysql","target":{"host":"localhost","port":3306,"dbname":"mysql"}}`)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"target_not_found"`)
	assert.NotContains(t, recorder.Body.String(), "secret-value")
}

// TestPostgresInstanceTargetParsingIsPreserved proves the Postgres parsing and
// identifier rendering are unchanged by the integration-aware registry.
func TestPostgresInstanceTargetParsingIsPreserved(t *testing.T) {
	t.Run("canonical fields with reported hostname", func(t *testing.T) {
		instanceTarget, ok := parseIntegrationInstanceTarget("postgres", "host: LocalHost.\nport: 5432\ndbname: postgres\nreported_hostname: rq-proof-a1-db1\n")
		require.True(t, ok)
		assert.Equal(t, integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"}, instanceTarget)
	})

	t.Run("custom template from tags", func(t *testing.T) {
		instanceTarget, ok := parseIntegrationInstanceTarget("postgres", "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\n")
		require.True(t, ok)
		assert.Equal(t, "rq-proof-a1-db1", instanceTarget.databaseInstance)
	})

	t.Run("default template without reported hostname fails closed for the identifier only", func(t *testing.T) {
		instanceTarget, ok := parseIntegrationInstanceTarget("postgres", "host: localhost\nport: 5432\ndbname: postgres\n")
		require.True(t, ok)
		assert.Empty(t, instanceTarget.databaseInstance)
		assert.True(t, instanceTarget.matches(remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "postgres"}))
		assert.False(t, instanceTarget.matches(remoteQueryTarget{DatabaseInstance: "localhost"}))
	})

	t.Run("clickhouse-shaped config does not parse as postgres", func(t *testing.T) {
		_, ok := parseIntegrationInstanceTarget("postgres", "server: localhost\nport: 5432\ndb: postgres\n")
		assert.False(t, ok)
	})
}

func newClickHouseStreamRunner(events []check.RemoteQueryStreamEvent) *fakeStreamRunnerCheck {
	return &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "clickhouse", loader: "python", provider: "file", instance: "server: localhost\nport: 8123\ndb: default\nusername: default\npassword: secret-value\n"}},
		events:          events,
	}
}

func clickHouseExecuteRequest(t *testing.T, integration string, query string) RemoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest(
		integration,
		RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 8123, DBName: "default"},
		query,
		false,
		pagedTestDelivery(),
	)
	require.NoError(t, err)
	return req
}

// TestProofQueryAllowlistIsIntegrationSpecific proves the proof-query contract:
// each set mirrors its executor's allowlist exactly, and dialect-specific proofs
// never cross integrations.
func TestProofQueryAllowlistIsIntegrationSpecific(t *testing.T) {
	// Postgres keeps its full proof set.
	assert.True(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryProofSeedQuery))
	assert.True(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryFixtureTableProofQuery))
	assert.True(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryMatrixIdentityProofQuery))
	assert.True(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryBinaryPayloadProofQuery))
	for query := range remoteQueryLargePayloadProofQueries {
		assert.True(t, isRemoteQueryAllowedProofQuery("postgres", query), query)
	}

	// ClickHouse mirrors the corrected executor's allowlist exactly: the
	// dialect-neutral seed, the fixture-free identity proof, the valid-UTF-8 unhex
	// binary payload, and the six deterministic bounded-repeat payload queries.
	assert.True(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryProofSeedQuery))
	assert.True(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryClickHouseIdentityProofQuery))
	assert.True(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryClickHouseBinaryPayloadProofQuery))
	for _, sizeBytes := range remoteQueryClickHouseProofPayloadSizesBytes {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)
		assert.True(t, isRemoteQueryAllowedProofQuery("clickhouse", query), query)
	}

	// The ClickHouse payload queries are dialect-specific: Postgres keeps its
	// direct repeat('x', N) form and rejects the concat form.
	for _, sizeBytes := range remoteQueryClickHouseProofPayloadSizesBytes {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)
		assert.False(t, isRemoteQueryAllowedProofQuery("postgres", query), query)
	}

	// The legacy ClickHouse proof queries the executor corrected are rejected here
	// too: the non-UTF-8 binary payload and every direct repeat('x', N) payload whose
	// count sits at or above the ClickHouse repeat cap.
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT unhex('00ff80') AS payload"))
	for query := range remoteQueryLargePayloadProofQueries {
		assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", query), query)
	}

	// Fixture-dependent Postgres proofs never reach a ClickHouse executor.
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryFixtureTableProofQuery))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryMatrixIdentityProofQuery))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", remoteQueryBinaryPayloadProofQuery))

	// And the ClickHouse-specific proofs never reach a Postgres executor.
	assert.False(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryClickHouseIdentityProofQuery))
	assert.False(t, isRemoteQueryAllowedProofQuery("postgres", remoteQueryClickHouseBinaryPayloadProofQuery))

	// Nearby executable-but-non-allowlisted ClickHouse queries are rejected: a
	// within-cap repeat count the executor never pinned, and the same 1 MiB total
	// built with a different part split — the allowlist matches exact query strings,
	// not payload sizes.
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT repeat('x', 1000000) AS payload"))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT concat(repeat('x', 500000), repeat('x', 548576)) AS payload"))

	// Arbitrary SQL, near-misses, and unknown integrations are rejected everywhere.
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT * FROM arbitrary_table"))
	assert.False(t, isRemoteQueryAllowedProofQuery("postgres", "SELECT * FROM arbitrary_table"))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT currentDatabase() AS current_db FROM remote_query_identity"))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT hostname() AS host, currentUser() AS user, version() AS version"))
	assert.False(t, isRemoteQueryAllowedProofQuery("clickhouse", "SELECT unhex('006162') AS payload_"))
	assert.False(t, isRemoteQueryAllowedProofQuery("mysql", remoteQueryProofSeedQuery))
	assert.False(t, isRemoteQueryAllowedProofQuery("", remoteQueryProofSeedQuery))
}

// TestClickHouseProofQueriesMirrorTheCorrectedExecutorAllowlist proves the
// Agent-side ClickHouse proof set is exactly the corrected integrations-core executor
// allowlist: nine queries — the dialect-neutral seed, the identity proof, the binary
// payload proof, and the six deterministic payload queries — and nothing else. The
// count and the three fixed queries are cross-repo contract mirrored one for one,
// not local convenience.
func TestClickHouseProofQueriesMirrorTheCorrectedExecutorAllowlist(t *testing.T) {
	expected := make(map[string]struct{}, 9)
	for _, query := range []string{
		remoteQueryProofSeedQuery,
		remoteQueryClickHouseIdentityProofQuery,
		remoteQueryClickHouseBinaryPayloadProofQuery,
	} {
		expected[query] = struct{}{}
	}
	for _, sizeBytes := range remoteQueryClickHouseProofPayloadSizesBytes {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)
		expected[query] = struct{}{}
	}

	registered := clickHouseProofQueries()
	assert.Len(t, registered, 9)
	assert.Equal(t, expected, registered)
}

// TestClickHouseProofPayloadQueriesAreDeterministicBoundedAndExact proves the
// construction the corrected executor pins: every proof payload query is a pure
// function of its size, every repeat() count stays within the ClickHouse repeat cap
// real servers enforce, and the concatenated parts sum to exactly the intended
// payload byte count.
func TestClickHouseProofPayloadQueriesAreDeterministicBoundedAndExact(t *testing.T) {
	// The intended sizes are the pinned power-of-two byte counts, 1 MiB through 32 MiB.
	assert.Equal(t, []int{1 << 20, 2 << 20, 4 << 20, 8 << 20, 16 << 20, 32 << 20}, remoteQueryClickHouseProofPayloadSizesBytes)

	for _, sizeBytes := range remoteQueryClickHouseProofPayloadSizesBytes {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)

		// Deterministic construction: one size builds one stable SQL string, and the
		// registered allowlist carries exactly that string.
		again, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)
		assert.Equal(t, again, query)
		assert.True(t, isRemoteQueryAllowedProofQuery("clickhouse", query))

		repeatArguments := clickHouseRepeatArguments(t, query)
		require.NotEmpty(t, repeatArguments)

		// Real servers reject repeat() counts above the hard cap (Code 131), and the
		// parts must sum to exactly the intended payload byte count.
		total := 0
		for _, argument := range repeatArguments {
			assert.LessOrEqual(t, argument, remoteQueryClickHouseRepeatCap)
			total += argument
		}
		assert.Equal(t, sizeBytes, total)
	}
}

var clickHouseRepeatArgumentPattern = regexp.MustCompile(`repeat\('x', (\d+)\)`)

// clickHouseRepeatArguments extracts the repeat('x', N) counts of a proof payload
// query in order.
func clickHouseRepeatArguments(t *testing.T, query string) []int {
	t.Helper()
	matches := clickHouseRepeatArgumentPattern.FindAllStringSubmatch(query, -1)
	arguments := make([]int, 0, len(matches))
	for _, match := range matches {
		argument, err := strconv.Atoi(match[1])
		require.NoError(t, err)
		arguments = append(arguments, argument)
	}
	return arguments
}

// TestClickHouseProofPayloadQueriesMatchTheExecutorStrings byte-for-byte pins the
// helper's output to the corrected executor's strings: the smallest sizes as
// literals, and 32 MiB through an expectation assembled independently of the helper
// under test — thirty-three million-byte parts plus the 554,432-byte remainder.
func TestClickHouseProofPayloadQueriesMatchTheExecutorStrings(t *testing.T) {
	// 1 MiB: one full-cap part plus the 48,576-byte remainder.
	query, err := remoteQueryClickHouseProofPayloadQuery(1048576)
	require.NoError(t, err)
	assert.Equal(t, "SELECT concat(repeat('x', 1000000), repeat('x', 48576)) AS payload", query)

	// 2 MiB: two full-cap parts plus the 97,152-byte remainder.
	query, err = remoteQueryClickHouseProofPayloadQuery(2097152)
	require.NoError(t, err)
	assert.Equal(t, "SELECT concat(repeat('x', 1000000), repeat('x', 1000000), repeat('x', 97152)) AS payload", query)

	// 32 MiB: thirty-three full-cap parts plus the 554,432-byte remainder.
	query, err = remoteQueryClickHouseProofPayloadQuery(33554432)
	require.NoError(t, err)
	expected := "SELECT concat(" +
		strings.TrimSuffix(strings.Repeat("repeat('x', 1000000), ", 33), ", ") +
		", repeat('x', 554432)) AS payload"
	assert.Equal(t, expected, query)
}

// TestClickHouseProofPayloadQueryRejectsNonPositiveSizes mirrors the executor's
// constructor guard: a non-positive size is an error, never a degenerate query.
func TestClickHouseProofPayloadQueryRejectsNonPositiveSizes(t *testing.T) {
	for _, sizeBytes := range []int{0, -1, -1048576} {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.Error(t, err)
		assert.Empty(t, query)
	}
}

// clickHouseExecutorProofQueries returns the exact proof-query set the integrations-
// core ClickHouse executor allowlists, so the dispatch test tracks the cross-repo
// contract rather than restating the Agent-side map.
func clickHouseExecutorProofQueries(t *testing.T) []string {
	t.Helper()
	queries := []string{
		remoteQueryProofSeedQuery,
		remoteQueryClickHouseIdentityProofQuery,
		remoteQueryClickHouseBinaryPayloadProofQuery,
	}
	for _, sizeBytes := range remoteQueryClickHouseProofPayloadSizesBytes {
		query, err := remoteQueryClickHouseProofPayloadQuery(sizeBytes)
		require.NoError(t, err)
		queries = append(queries, query)
	}
	return queries
}

func TestRemoteQueryExecuteServiceClickHouseDispatchesExecutorProofQueries(t *testing.T) {
	for _, query := range clickHouseExecutorProofQueries(t) {
		t.Run(query, func(t *testing.T) {
			runner := newClickHouseStreamRunner([]check.RemoteQueryStreamEvent{
				{Type: "metadata", MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages"}`},
				{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`},
			})
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
			req := clickHouseExecuteRequest(t, "clickhouse", query)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.Nil(t, result.Error)
			assert.Equal(t, 1, runner.streamCalls)
			// The wire target forwards verbatim under the generic contract keys: host
			// aliases ClickHouse server and dbname aliases ClickHouse db. The integration
			// name stays off the request wire; only the bridge import uses it.
			assert.Contains(t, runner.streamSeen, `"operation":"produce_json_pages"`)
			assert.Contains(t, runner.streamSeen, `"target":{"host":"localhost","port":8123,"dbname":"default"}`)
			assert.Contains(t, runner.streamSeen, query)
			assert.NotContains(t, runner.streamSeen, "integration")
			assert.NotContains(t, runner.streamSeen, "secret-value")
		})
	}
}

func TestRemoteQueryExecuteServiceClickHouseDatabaseInstanceTarget(t *testing.T) {
	runner := newClickHouseStreamRunner([]check.RemoteQueryStreamEvent{
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`},
	})
	runner.instance = "server: localhost\ndatabase_identifier:\n  template: $rq_database_instance\ntags:\n  - rq_database_instance:rq-ch-a1\npassword: secret-value\n"
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
	req, err := NewRemoteQueryExecuteRequest("clickhouse", RemoteQueryExecuteTarget{DatabaseInstance: "rq-ch-a1"}, remoteQueryProofSeedQuery, false, pagedTestDelivery())
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, `"database_instance":"rq-ch-a1"`)
	assert.NotContains(t, runner.streamSeen, "secret-value")
}

func TestRemoteQueryExecuteServiceClickHouseRejectsNonAllowlistedQueries(t *testing.T) {
	rejected := []string{
		"SELECT * FROM arbitrary_table",
		"SELECT 1 AS other_value",
		remoteQueryFixtureTableProofQuery,
		remoteQueryMatrixIdentityProofQuery,
		remoteQueryBinaryPayloadProofQuery,
		"SELECT currentDatabase() AS current_db FROM remote_query_identity",
		"SELECT hostname() AS host, currentUser() AS user, version() AS version",
		// The legacy binary proof the executor corrected: a non-UTF-8 payload the
		// executor's JSON value contract fails closed on.
		"SELECT unhex('00ff80') AS payload",
		// The legacy direct large-payload form the executor corrected: the count
		// sits at or above the ClickHouse repeat cap, so real servers reject it.
		"SELECT repeat('x', 1048576) AS payload",
		"SELECT repeat('x', 33554432) AS payload",
		// Within the repeat cap and executable on a real server, but never pinned.
		"SELECT repeat('x', 1000000) AS payload",
		// The same 1 MiB total as an allowlisted query but split differently: the
		// allowlist matches exact query strings, not payload sizes.
		"SELECT concat(repeat('x', 500000), repeat('x', 548576)) AS payload",
	}

	for _, query := range rejected {
		t.Run(query, func(t *testing.T) {
			runner := newClickHouseStreamRunner(nil)
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
			req := clickHouseExecuteRequest(t, "clickhouse", query)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.NotNil(t, result.Error)
			assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
			assert.Equal(t, statusInvalidRequest, result.Error.Code)
			assert.Equal(t, "query is not allowed", result.Error.Message)
			assert.Equal(t, 0, runner.streamCalls)
		})
	}
}

func TestRemoteQueryExecuteServiceClickHouseAllowlistDisabled(t *testing.T) {
	runner := newClickHouseStreamRunner([]check.RemoteQueryStreamEvent{
		{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`},
	})
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false, nil)
	req := clickHouseExecuteRequest(t, "clickhouse", "SELECT * FROM arbitrary_table")

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.Nil(t, result.Error)
	assert.Equal(t, 1, runner.streamCalls)
	assert.Contains(t, runner.streamSeen, "SELECT * FROM arbitrary_table")
}

// TestRemoteQueryExecuteServicePostgresProofSetUnchanged proves the ClickHouse work
// changed no Postgres execution behavior: the full Postgres proof set still dispatches
// and non-allowlisted queries are still rejected.
func TestRemoteQueryExecuteServicePostgresProofSetUnchanged(t *testing.T) {
	accepted := []string{
		remoteQueryProofSeedQuery,
		remoteQueryFixtureTableProofQuery,
		remoteQueryMatrixIdentityProofQuery,
		remoteQueryBinaryPayloadProofQuery,
		"SELECT repeat('x', 1048576) AS payload",
	}

	for _, query := range accepted {
		t.Run(query, func(t *testing.T) {
			runner := &fakeStreamRunnerCheck{
				fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
				events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
			}
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
			req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, query, false, pagedTestDelivery())
			require.NoError(t, err)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.Nil(t, result.Error)
			assert.Equal(t, 1, runner.streamCalls)
		})
	}

	t.Run("non-allowlisted query still rejected", func(t *testing.T) {
		runner := &fakeStreamRunnerCheck{
			fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\n"}},
		}
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, true, nil)
		req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1 AS other_value", false, pagedTestDelivery())
		require.NoError(t, err)

		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, statusInvalidRequest, result.Error.Code)
		assert.Equal(t, 0, runner.streamCalls)
	})
}
