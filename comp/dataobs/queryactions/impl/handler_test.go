// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mockAutodiscovery wraps a noop autodiscovery Component and overrides GetUnresolvedConfigs
// to return a fixed set of configs. This follows the same pattern as kafka_actions_test.go.
type mockAutodiscovery struct {
	autodiscovery.Component
	configs []integration.Config
}

func (m *mockAutodiscovery) GetUnresolvedConfigs() []integration.Config {
	return m.configs
}

func newMockAutodiscovery(t *testing.T, configs []integration.Config) autodiscovery.Component {
	t.Helper()
	return &mockAutodiscovery{
		Component: fxutil.Test[autodiscovery.Component](t, noopautoconfig.Module()),
		configs:   configs,
	}
}

func newTestComponentWithAC(t *testing.T, configs []integration.Config) *component {
	t.Helper()
	return &component{
		log:           logmock.New(t),
		ac:            newMockAutodiscovery(t, configs),
		activeConfigs: make(map[string]integration.Config),
	}
}

func TestIsPostgresIntegration(t *testing.T) {
	assert.True(t, isPostgresIntegration("postgres"))
	assert.False(t, isPostgresIntegration("mysql"))
	assert.False(t, isPostgresIntegration("redis"))
	assert.False(t, isPostgresIntegration(""))
}

func TestMatchesDBName(t *testing.T) {
	t.Run("empty RC dbname matches empty instance dbname", func(t *testing.T) {
		instance := map[string]any{}
		dbID := &DBIdentifier{DBName: ""}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("empty RC dbname does not match instance with dbname", func(t *testing.T) {
		instance := map[string]any{"dbname": "mydb"}
		dbID := &DBIdentifier{DBName: ""}
		assert.False(t, matchesDBName(instance, dbID))
	})

	t.Run("empty instance dbname does not match specific RC dbname", func(t *testing.T) {
		instance := map[string]any{}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.False(t, matchesDBName(instance, dbID))
	})

	t.Run("matching dbnames", func(t *testing.T) {
		instance := map[string]any{"dbname": "mydb"}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("mismatching dbnames", func(t *testing.T) {
		instance := map[string]any{"dbname": "otherdb"}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.False(t, matchesDBName(instance, dbID))
	})
}

func TestMatchesIdentifier_SelfHosted(t *testing.T) {
	instance := map[string]any{"host": "localhost"}
	dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost"}
	assert.True(t, matchesIdentifier(instance, dbID))

	dbID = &DBIdentifier{Type: "self-hosted", Host: "otherhost"}
	assert.False(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_SelfHosted_WithDBName(t *testing.T) {
	instance := map[string]any{
		"host":   "localhost",
		"dbname": "production",
	}

	t.Run("matching dbname", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "production"}
		assert.True(t, matchesIdentifier(instance, dbID))
	})

	t.Run("mismatching dbname", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "staging"}
		assert.False(t, matchesIdentifier(instance, dbID))
	})

	t.Run("empty RC dbname does not match instance with dbname", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost"}
		assert.False(t, matchesIdentifier(instance, dbID))
	})
}

func TestMatchesIdentifier_RDS(t *testing.T) {
	instance := map[string]any{"host": "mydb.cluster-xxx.us-east-1.rds.amazonaws.com"}
	dbID := &DBIdentifier{Type: "rds", Host: "mydb.cluster-xxx.us-east-1.rds.amazonaws.com"}
	assert.True(t, matchesIdentifier(instance, dbID))

	dbID = &DBIdentifier{Type: "rds", Host: "otherdb.cluster-xxx.us-east-1.rds.amazonaws.com"}
	assert.False(t, matchesIdentifier(instance, dbID))
}

func TestExtractDBAuthFromInstance(t *testing.T) {
	instance := map[string]any{
		"host":        "localhost",
		"port":        5432,
		"username":    "datadog",
		"password":    "secret",
		"dbname":      "testdb",
		"ssl_mode":    "require",
		"extra_field": "should_not_appear",
	}
	auth := extractDBAuthFromInstance(instance)

	require.Equal(t, "localhost", auth["host"])
	require.Equal(t, 5432, auth["port"])
	require.Equal(t, "datadog", auth["username"])
	require.Equal(t, "secret", auth["password"])
	require.Equal(t, "testdb", auth["dbname"])
	require.Equal(t, "require", auth["ssl_mode"])
	_, ok := auth["extra_field"]
	assert.False(t, ok, "extra_field should not be in allowlist output")
}

func TestExtractDBAuthFromInstance_NestedMap(t *testing.T) {
	instance := map[string]any{
		"host":     "mydb.rds.amazonaws.com",
		"port":     5432,
		"username": "datadog",
		"password": "secret",
		"aws": map[string]any{
			"instance_endpoint": "my-rds-instance",
			"region":            "us-east-1",
		},
	}
	auth := extractDBAuthFromInstance(instance)

	require.Equal(t, "mydb.rds.amazonaws.com", auth["host"])
	awsMap, ok := auth["aws"].(map[string]any)
	require.True(t, ok, "aws should be a map[string]any")
	assert.Equal(t, "my-rds-instance", awsMap["instance_endpoint"])
	assert.Equal(t, "us-east-1", awsMap["region"])
}

func TestDBCredentialAllowList_ExcludesReservedKeys(t *testing.T) {
	instance := map[string]any{
		"host":             "localhost",
		"port":             5432,
		"remote_config_id": "should-not-appear",
		"db_type":          "should-not-appear",
		"db_identifier":    "should-not-appear",
		"queries":          []any{"should-not-appear"},
	}
	auth := extractDBAuthFromInstance(instance)

	_, hasRemoteConfigID := auth["remote_config_id"]
	assert.False(t, hasRemoteConfigID, "remote_config_id must not be in the allowlist")
	_, hasDBType := auth["db_type"]
	assert.False(t, hasDBType, "db_type must not be in the allowlist")
	_, hasDBIdentifier := auth["db_identifier"]
	assert.False(t, hasDBIdentifier, "db_identifier must not be in the allowlist")
	_, hasQueries := auth["queries"]
	assert.False(t, hasQueries, "queries must not be in the allowlist")
}

func TestBuildCheckConfig_MultipleQueries(t *testing.T) {
	c := &component{
		log: logmock.New(t),
	}

	payload := &DOQueryPayload{
		ConfigID:     "test-config-1",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "testdb"},
		Queries: []QuerySpec{
			{
				MonitorID:       100,
				Type:            "run_query",
				Query:           "SELECT count(*) FROM orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity: EntityMetadata{
					Platform: "postgres",
					Account:  "my-account",
					Database: "shop",
					Schema:   "public",
					Table:    "orders",
				},
			},
			{
				MonitorID:       200,
				Type:            "run_query",
				Query:           "SELECT avg(price) FROM products",
				IntervalSeconds: 300,
				TimeoutSeconds:  30,
				Entity: EntityMetadata{
					Platform: "postgres",
					Account:  "my-account",
					Database: "shop",
					Schema:   "public",
					Table:    "products",
				},
			},
		},
	}

	baseCfg := &integration.Config{
		Name:     "postgres",
		Provider: "file",
		NodeName: "node1",
	}

	pgInstance := map[string]any{
		"host":     "localhost",
		"port":     5432,
		"username": "datadog",
		"password": "secret",
	}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id-1")
	require.NoError(t, err)

	assert.Equal(t, "do_query_actions", checkCfg.Name)
	assert.Equal(t, "file", checkCfg.Provider)
	assert.Equal(t, "node1", checkCfg.NodeName)
	require.Len(t, checkCfg.Instances, 1)

	var instance map[string]any
	err = yaml.Unmarshal(checkCfg.Instances[0], &instance)
	require.NoError(t, err)

	assert.Equal(t, "rc-id-1", instance["remote_config_id"])
	assert.Equal(t, "postgres", instance["db_type"])

	dbID, ok := instance["db_identifier"].(map[string]interface{})
	require.True(t, ok, "db_identifier should be a map")
	assert.Equal(t, "localhost", dbID["host"])
	assert.Equal(t, "testdb", dbID["dbname"])

	assert.Equal(t, "localhost", instance["host"])
	assert.Equal(t, 5432, instance["port"])
	assert.Equal(t, "datadog", instance["username"])
	assert.Equal(t, "secret", instance["password"])

	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok, "queries should be a list")
	require.Len(t, queries, 2)

	q1, ok := queries[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 100, q1["monitor_id"])
	assert.Equal(t, "run_query", q1["type"])
	assert.Equal(t, "SELECT count(*) FROM orders", q1["query"])
	assert.Equal(t, 60, q1["interval_seconds"])
	assert.Equal(t, 10, q1["timeout_seconds"])

	entity1, ok := q1["entity"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "postgres", entity1["platform"])
	assert.Equal(t, "my-account", entity1["account"])
	assert.Equal(t, "shop", entity1["database"])
	assert.Equal(t, "public", entity1["schema"])
	assert.Equal(t, "orders", entity1["table"])

	q2, ok := queries[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 200, q2["monitor_id"])
	assert.Equal(t, "SELECT avg(price) FROM products", q2["query"])

	// Verify run_once is NOT present
	_, hasRunOnce := instance["run_once"]
	assert.False(t, hasRunOnce, "run_once should not be present in declarative model")
}

func TestBuildCheckConfig_CustomSQLSelectFields(t *testing.T) {
	c := &component{log: logmock.New(t)}

	payload := &DOQueryPayload{
		ConfigID: "test-config-custom",
		Queries: []QuerySpec{
			{
				Type:            "run_query",
				Query:           "SELECT dd_value FROM my_custom_metric",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Account: "acct", Database: "db", Schema: "s", Table: "t"},
				CustomSQLSelectFields: &CustomSQLSelectFields{
					MetricConfigID: 42,
					EntityID:       "entity-abc",
				},
			},
		},
	}

	baseCfg := &integration.Config{Name: "postgres"}
	pgInstance := map[string]any{"host": "localhost", "port": 5432}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id-custom")
	require.NoError(t, err)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(checkCfg.Instances[0], &instance))

	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok)
	require.Len(t, queries, 1)

	q, ok := queries[0].(map[string]interface{})
	require.True(t, ok)

	csf, ok := q["custom_sql_select_fields"].(map[string]interface{})
	require.True(t, ok, "custom_sql_select_fields should be present")
	assert.Equal(t, 42, csf["metric_config_id"])
	assert.Equal(t, "entity-abc", csf["entity_id"])
}

func TestBuildCheckConfig_NoCustomSQLSelectFields(t *testing.T) {
	c := &component{log: logmock.New(t)}

	payload := &DOQueryPayload{
		ConfigID: "test-config",
		Queries: []QuerySpec{
			{
				Type:            "run_query",
				Query:           "SELECT count(*) FROM orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Account: "acct", Database: "db", Schema: "s", Table: "t"},
			},
		},
	}

	baseCfg := &integration.Config{Name: "postgres"}
	pgInstance := map[string]any{"host": "localhost", "port": 5432}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id")
	require.NoError(t, err)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(checkCfg.Instances[0], &instance))

	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok)
	q, ok := queries[0].(map[string]interface{})
	require.True(t, ok)

	_, hasCsf := q["custom_sql_select_fields"]
	assert.False(t, hasCsf, "custom_sql_select_fields should be absent when nil")
}

// --- onRCUpdate tests ---

func newTestComponent(t *testing.T) *component {
	t.Helper()
	return &component{
		log:           logmock.New(t),
		activeConfigs: make(map[string]integration.Config),
	}
}

// collectStatuses calls onRCUpdate and returns the apply statuses and the resulting ConfigChanges.
func collectStatuses(c *component, updates map[string]state.RawConfig) (map[string]state.ApplyStatus, integration.ConfigChanges) {
	statuses := map[string]state.ApplyStatus{}
	changes := c.onRCUpdate(updates, func(path string, s state.ApplyStatus) {
		statuses[path] = s
	})
	return statuses, changes
}

func TestOnRCUpdate_InvalidJSON(t *testing.T) {
	c := newTestComponent(t)
	updates := map[string]state.RawConfig{
		"path/bad": {Config: []byte(`{not valid json`)},
	}
	statuses, _ := collectStatuses(c, updates)
	assert.Equal(t, state.ApplyStateError, statuses["path/bad"].State)
	assert.Empty(t, c.activeConfigs)
}

func TestOnRCUpdate_EmptyConfigID(t *testing.T) {
	c := newTestComponent(t)
	updates := map[string]state.RawConfig{
		"path/config": {Config: []byte(`{"config_id": ""}`)},
	}
	statuses, _ := collectStatuses(c, updates)
	assert.Equal(t, state.ApplyStateError, statuses["path/config"].State)
	assert.Empty(t, c.activeConfigs)
}

func TestOnRCUpdate_EmptyQueriesUnschedules(t *testing.T) {
	existing := integration.Config{Name: "do_query_actions"}
	c := newTestComponent(t)
	c.activeConfigs["cfg-1"] = existing

	updates := map[string]state.RawConfig{
		"path/config": {Config: []byte(`{"config_id": "cfg-1", "queries": []}`)},
	}
	statuses, changes := collectStatuses(c, updates)

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["path/config"].State)
	assert.Empty(t, c.activeConfigs)
	require.Len(t, changes.Unschedule, 1)
	assert.Equal(t, "do_query_actions", changes.Unschedule[0].Name)
}

func TestOnRCUpdate_ReconcileRemovesStaleConfigs(t *testing.T) {
	existing := integration.Config{Name: "do_query_actions"}
	c := newTestComponent(t)
	c.activeConfigs["stale-config"] = existing

	// Update snapshot contains only a config without config_id — stale-config should be unscheduled
	updates := map[string]state.RawConfig{
		"path/other": {Config: []byte(`{"some_field": true}`)},
	}
	_, changes := collectStatuses(c, updates)

	assert.Empty(t, c.activeConfigs)
	require.Len(t, changes.Unschedule, 1)
}

// --- collectUnschedule tests ---

func TestCollectUnschedule_NotFound(t *testing.T) {
	c := newTestComponent(t)
	changes := integration.ConfigChanges{}
	c.collectUnschedule("nonexistent", &changes)
	assert.Empty(t, changes.Unschedule)
	assert.Empty(t, c.activeConfigs)
}

func TestCollectUnschedule_Found(t *testing.T) {
	c := newTestComponent(t)
	c.activeConfigs["my-config"] = integration.Config{Name: "do_query_actions"}
	changes := integration.ConfigChanges{}

	c.collectUnschedule("my-config", &changes)

	assert.Empty(t, c.activeConfigs)
	require.Len(t, changes.Unschedule, 1)
	assert.Equal(t, "do_query_actions", changes.Unschedule[0].Name)
}

// --- Happy-path integration tests (require mocked autodiscovery) ---

// TestOnRCUpdate_ValidConfig_SchedulesCheck verifies the primary use-case: a valid RC config
// whose db_identifier matches a configured postgres instance results in a scheduled check.
func TestOnRCUpdate_ValidConfig_SchedulesCheck(t *testing.T) {
	postgresCfg := integration.Config{
		Name:     "postgres",
		Provider: "file",
		NodeName: "node1",
		Instances: []integration.Data{
			integration.Data("host: localhost\nport: 5432\nusername: datadog\npassword: secret\ndbname: mydb\n"),
		},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-happy",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "mydb"},
		Queries: []QuerySpec{
			{
				MonitorID:       42,
				Type:            "run_query",
				Query:           "SELECT count(*) FROM orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "mydb", Table: "orders"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	updates := map[string]state.RawConfig{
		"path/cfg-happy": {Config: payloadJSON, Metadata: state.Metadata{ID: "rc-id-happy"}},
	}
	statuses, changes := collectStatuses(c, updates)

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-happy"].State)
	require.Len(t, changes.Schedule, 1, "expected one scheduled check")
	assert.Empty(t, changes.Unschedule)
	assert.Equal(t, "do_query_actions", changes.Schedule[0].Name)
	assert.Equal(t, "file", changes.Schedule[0].Provider)
	assert.Equal(t, "node1", changes.Schedule[0].NodeName)

	require.Len(t, changes.Schedule[0].Instances, 1)
	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes.Schedule[0].Instances[0], &instance))
	assert.Equal(t, "rc-id-happy", instance["remote_config_id"])
	assert.Equal(t, "postgres", instance["db_type"])
	assert.Equal(t, "localhost", instance["host"])
	assert.Equal(t, "datadog", instance["username"])

	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok)
	require.Len(t, queries, 1)
	q := queries[0].(map[string]interface{})
	assert.Equal(t, "SELECT count(*) FROM orders", q["query"])

	require.Contains(t, c.activeConfigs, "cfg-happy")
}

// TestOnRCUpdate_UpdateReplacesExistingCheck verifies that two sequential onRCUpdate calls
// with the same config_id correctly unschedule the previous check and schedule the updated one.
func TestOnRCUpdate_UpdateReplacesExistingCheck(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	mkPayload := func(query string) []byte {
		b, err := json.Marshal(DOQueryPayload{
			ConfigID:     "cfg-update",
			DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "mydb"},
			Queries:      []QuerySpec{{Type: "run_query", Query: query, IntervalSeconds: 60, TimeoutSeconds: 10}},
		})
		require.NoError(t, err)
		return b
	}

	// First update: schedule initial version.
	_, changes1 := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg": {Config: mkPayload("SELECT 1")},
	})
	require.Len(t, changes1.Schedule, 1, "first update should schedule the check")
	assert.Empty(t, changes1.Unschedule, "first update should not unschedule anything")
	require.Contains(t, c.activeConfigs, "cfg-update")

	firstCfg := changes1.Schedule[0]

	// Second update: same config_id, different query. Should unschedule the old and schedule the new.
	_, changes2 := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg": {Config: mkPayload("SELECT 2")},
	})
	require.Len(t, changes2.Schedule, 1, "second update should schedule the updated check")
	require.Len(t, changes2.Unschedule, 1, "second update should unschedule the previous check")
	assert.Equal(t, firstCfg, changes2.Unschedule[0], "unscheduled config should be the previous version")

	// Verify the new instance has the updated query.
	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes2.Schedule[0].Instances[0], &instance))
	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok)
	require.Len(t, queries, 1)
	assert.Equal(t, "SELECT 2", queries[0].(map[string]interface{})["query"])
}

// TestOnRCUpdate_NoMatchingPostgres_ReportsError verifies that when no postgres instance
// matches the RC identifier, the apply status is Error and no check is scheduled.
func TestOnRCUpdate_NoMatchingPostgres_ReportsError(t *testing.T) {
	c := newTestComponentWithAC(t, []integration.Config{}) // no postgres configs

	payload := DOQueryPayload{
		ConfigID:     "cfg-nomatch",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "notfound.example.com", DBName: "mydb"},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	updates := map[string]state.RawConfig{
		"path/cfg-nomatch": {Config: payloadJSON},
	}
	statuses, changes := collectStatuses(c, updates)

	assert.Equal(t, state.ApplyStateError, statuses["path/cfg-nomatch"].State)
	assert.Empty(t, changes.Schedule)
	assert.Empty(t, c.activeConfigs)
}

// TestOnRCUpdate_MalformedPostgresYAML_SurfacesParseError verifies that when a postgres
// instance's YAML is malformed, the error message from findPostgresConfig mentions the
// parse failure, not just "identifier not found".
func TestOnRCUpdate_MalformedPostgresYAML_SurfacesParseError(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("not: [valid: yaml")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-badyaml",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost", DBName: "mydb"},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	updates := map[string]state.RawConfig{
		"path/cfg-badyaml": {Config: payloadJSON},
	}
	statuses, _ := collectStatuses(c, updates)

	require.Equal(t, state.ApplyStateError, statuses["path/cfg-badyaml"].State)
	assert.Contains(t, statuses["path/cfg-badyaml"].Error, "YAML parse error",
		"error message should surface the YAML parse failure, not just 'identifier not found'")
}
