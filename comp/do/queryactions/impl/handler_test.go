// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIsPostgresIntegration(t *testing.T) {
	assert.True(t, isPostgresIntegration("postgres"))
	assert.False(t, isPostgresIntegration("mysql"))
	assert.False(t, isPostgresIntegration("redis"))
	assert.False(t, isPostgresIntegration(""))
}

func TestMatchesDBName(t *testing.T) {
	t.Run("empty RC dbname matches any instance", func(t *testing.T) {
		instance := map[string]any{"dbname": "mydb"}
		dbID := &DBIdentifier{DBName: ""}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("empty instance dbname matches any RC", func(t *testing.T) {
		instance := map[string]any{}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.True(t, matchesDBName(instance, dbID))
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

	t.Run("no dbname in RC matches any", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost"}
		assert.True(t, matchesIdentifier(instance, dbID))
	})
}

func TestMatchesIdentifier_UnknownType(t *testing.T) {
	instance := map[string]any{"host": "localhost"}
	dbID := &DBIdentifier{Type: "rds", Host: "localhost"}
	assert.False(t, matchesIdentifier(instance, dbID))
}

func TestExtractDBAuthFromInstanceData(t *testing.T) {
	instanceYAML := `
host: localhost
port: 5432
username: datadog
password: secret
dbname: testdb
ssl_mode: require
extra_field: should_not_appear
`
	auth, err := extractDBAuthFromInstanceData(integration.Data(instanceYAML))
	require.NoError(t, err)

	require.Equal(t, "localhost", auth["host"])
	require.Equal(t, 5432, auth["port"])
	require.Equal(t, "datadog", auth["username"])
	require.Equal(t, "secret", auth["password"])
	require.Equal(t, "testdb", auth["dbname"])
	require.Equal(t, "require", auth["ssl_mode"])
	_, ok := auth["extra_field"]
	assert.False(t, ok, "extra_field should not be in allowlist output")
}

func TestExtractDBAuthFromInstanceData_NestedMap(t *testing.T) {
	instanceYAML := `
host: mydb.rds.amazonaws.com
port: 5432
username: datadog
password: secret
aws:
  instance_endpoint: my-rds-instance
  region: us-east-1
`
	auth, err := extractDBAuthFromInstanceData(integration.Data(instanceYAML))
	require.NoError(t, err)

	require.Equal(t, "mydb.rds.amazonaws.com", auth["host"])
	awsMap, ok := auth["aws"].(map[string]any)
	require.True(t, ok, "aws should be converted to map[string]any")
	assert.Equal(t, "my-rds-instance", awsMap["instance_endpoint"])
	assert.Equal(t, "us-east-1", awsMap["region"])
}

func TestExtractDBAuthFromInstanceData_InvalidYAML(t *testing.T) {
	_, err := extractDBAuthFromInstanceData(integration.Data("not: [valid: yaml"))
	require.Error(t, err)
}

func TestBuildCheckConfig_MultipleQueries(t *testing.T) {
	c := &component{
		log: logmock.New(t),
	}

	payload := &DOQueryPayload{
		ConfigID: "test-config-1",
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

	instanceData := integration.Data(`
host: localhost
port: 5432
username: datadog
password: secret
`)

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, instanceData, "rc-id-1")
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
	instanceData := integration.Data("host: localhost\nport: 5432\n")

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, instanceData, "rc-id-custom")
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
	instanceData := integration.Data("host: localhost\nport: 5432\n")

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, instanceData, "rc-id")
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

func TestBuildCheckConfig_InvalidInstanceYAML(t *testing.T) {
	c := &component{log: logmock.New(t)}
	payload := &DOQueryPayload{Queries: []QuerySpec{{Query: "SELECT 1"}}}
	baseCfg := &integration.Config{Name: "postgres"}

	_, err := c.buildCheckConfig(payload, baseCfg, integration.Data("not: [valid: yaml"), "rc-id-1")
	require.Error(t, err)
}

// --- onDebugConfig tests ---

func newTestComponent(t *testing.T) *component {
	t.Helper()
	return &component{
		log:           logmock.New(t),
		configChanges: make(chan integration.ConfigChanges, 10),
		activeConfigs: make(map[string]integration.Config),
	}
}

func collectStatuses(c *component, updates map[string]state.RawConfig) map[string]state.ApplyStatus {
	statuses := map[string]state.ApplyStatus{}
	c.onDebugConfig(updates, func(path string, s state.ApplyStatus) {
		statuses[path] = s
	})
	return statuses
}

func TestOnDebugConfig_InvalidJSON(t *testing.T) {
	c := newTestComponent(t)
	updates := map[string]state.RawConfig{
		"path/bad": {Config: []byte(`{not valid json`)},
	}
	statuses := collectStatuses(c, updates)
	assert.Equal(t, state.ApplyStateError, statuses["path/bad"].State)
	assert.Empty(t, c.activeConfigs)
}

func TestOnDebugConfig_EmptyConfigID(t *testing.T) {
	c := newTestComponent(t)
	updates := map[string]state.RawConfig{
		"path/config": {Config: []byte(`{"config_id": ""}`)},
	}
	statuses := collectStatuses(c, updates)
	assert.Equal(t, state.ApplyStateError, statuses["path/config"].State)
	assert.Empty(t, c.activeConfigs)
}

func TestOnDebugConfig_EmptyQueriesUnschedules(t *testing.T) {
	existing := integration.Config{Name: "do_query_actions"}
	c := newTestComponent(t)
	c.activeConfigs["cfg-1"] = existing

	updates := map[string]state.RawConfig{
		"path/config": {Config: []byte(`{"config_id": "cfg-1", "queries": []}`)},
	}
	statuses := collectStatuses(c, updates)

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["path/config"].State)
	assert.Empty(t, c.activeConfigs)
	require.Len(t, c.configChanges, 1)
	change := <-c.configChanges
	require.Len(t, change.Unschedule, 1)
	assert.Equal(t, "do_query_actions", change.Unschedule[0].Name)
}

func TestOnDebugConfig_ReconcileRemovesStaleConfigs(t *testing.T) {
	existing := integration.Config{Name: "do_query_actions"}
	c := newTestComponent(t)
	c.activeConfigs["stale-config"] = existing

	// Update snapshot contains only a config without config_id — stale-config should be unscheduled
	updates := map[string]state.RawConfig{
		"path/other": {Config: []byte(`{"some_field": true}`)},
	}
	collectStatuses(c, updates)

	assert.Empty(t, c.activeConfigs)
	require.Len(t, c.configChanges, 1)
	change := <-c.configChanges
	require.Len(t, change.Unschedule, 1)
}

// --- unscheduleConfig tests ---

func TestUnscheduleConfig_NotFound(t *testing.T) {
	c := newTestComponent(t)
	c.unscheduleConfig("nonexistent")
	assert.Empty(t, c.configChanges)
	assert.Empty(t, c.activeConfigs)
}

func TestUnscheduleConfig_Found(t *testing.T) {
	c := newTestComponent(t)
	c.activeConfigs["my-config"] = integration.Config{Name: "do_query_actions"}

	c.unscheduleConfig("my-config")

	assert.Empty(t, c.activeConfigs)
	require.Len(t, c.configChanges, 1)
	change := <-c.configChanges
	require.Len(t, change.Unschedule, 1)
	assert.Equal(t, "do_query_actions", change.Unschedule[0].Name)
}

func TestUnscheduleConfig_ComponentClosed(t *testing.T) {
	c := newTestComponent(t)
	c.activeConfigs["my-config"] = integration.Config{Name: "do_query_actions"}
	c.closed = true
	// Drain the channel since close() isn't called here, just set the flag
	close(c.configChanges)

	// Should not send to closed channel (closed guard prevents it), should not panic
	c.unscheduleConfig("my-config")
	assert.Empty(t, c.activeConfigs)
}
