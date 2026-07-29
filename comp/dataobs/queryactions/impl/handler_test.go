// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"encoding/json"
	"fmt"
	"testing"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
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
		activeConfigs: make(map[string]activeConfigEntry),
		managedBases:  make(map[string]*managedBaseEntry),
	}
}

func TestInstanceHasDOEnabled(t *testing.T) {
	assert.False(t, instanceHasDOEnabled(map[string]any{}))
	assert.False(t, instanceHasDOEnabled(map[string]any{"data_observability": "not-a-map"}))
	assert.False(t, instanceHasDOEnabled(map[string]any{"data_observability": map[string]any{}}))
	assert.False(t, instanceHasDOEnabled(map[string]any{"data_observability": map[string]any{"enabled": false}}))
	assert.True(t, instanceHasDOEnabled(map[string]any{"data_observability": map[string]any{"enabled": true}}))
}

func TestMatchesIdentifier_HostOnly(t *testing.T) {
	instance := map[string]any{"host": "localhost", "dbname": "production"}

	t.Run("matching host", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost"}
		assert.True(t, matchesIdentifier(instance, dbID))
	})

	t.Run("mismatching host", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "otherhost"}
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

func TestBuildCheckConfig_MultipleQueries(t *testing.T) {
	c := &component{
		log: logmock.New(t),
	}

	payload := &DOQueryPayload{
		ConfigID:     "test-config-1",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
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
		"data_observability": map[string]any{
			"enabled": true,
		},
	}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id-1")
	require.NoError(t, err)

	assert.Equal(t, "postgres", checkCfg.Name)
	assert.Equal(t, "file", checkCfg.Provider)
	assert.Equal(t, "node1", checkCfg.NodeName)
	require.Len(t, checkCfg.Instances, 1)

	var instance map[string]any
	err = yaml.Unmarshal(checkCfg.Instances[0], &instance)
	require.NoError(t, err)

	// Verify full postgres instance fields are preserved
	assert.Equal(t, "localhost", instance["host"])
	assert.Equal(t, 5432, instance["port"])
	assert.Equal(t, "datadog", instance["username"])
	assert.Equal(t, "secret", instance["password"])

	// Verify data_observability section
	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok, "data_observability should be a map")
	assert.Equal(t, true, doConfig["enabled"])
	assert.Equal(t, "rc-id-1", doConfig["config_id"])
	assert.Equal(t, 10, doConfig["collection_interval"])

	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok, "queries should be a list")
	require.Len(t, queries, 2)

	q1, ok := queries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 100, q1["monitor_id"])
	assert.Equal(t, "run_query", q1["type"])
	assert.Equal(t, "SELECT count(*) FROM orders", q1["query"])
	assert.Equal(t, 60, q1["interval_seconds"])
	assert.Equal(t, 10000, q1["query_timeout"])

	entity1, ok := q1["entity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "postgres", entity1["platform"])
	assert.Equal(t, "my-account", entity1["account"])
	assert.Equal(t, "shop", entity1["database"])
	assert.Equal(t, "public", entity1["schema"])
	assert.Equal(t, "orders", entity1["table"])

	q2, ok := queries[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 200, q2["monitor_id"])
	assert.Equal(t, "SELECT avg(price) FROM products", q2["query"])
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
	pgInstance := map[string]any{"host": "localhost", "port": 5432, "data_observability": map[string]any{"enabled": true}}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id-custom")
	require.NoError(t, err)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(checkCfg.Instances[0], &instance))

	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)

	q, ok := queries[0].(map[string]any)
	require.True(t, ok)

	csf, ok := q["custom_sql_select_fields"].(map[string]any)
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
	pgInstance := map[string]any{"host": "localhost", "port": 5432, "data_observability": map[string]any{"enabled": true}}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-id")
	require.NoError(t, err)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(checkCfg.Instances[0], &instance))

	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	q, ok := queries[0].(map[string]any)
	require.True(t, ok)

	_, hasCsf := q["custom_sql_select_fields"]
	assert.False(t, hasCsf, "custom_sql_select_fields should be absent when nil")
}

// --- onRCUpdate tests ---

func newTestComponent(t *testing.T) *component {
	t.Helper()
	return &component{
		log:           logmock.New(t),
		activeConfigs: make(map[string]activeConfigEntry),
		managedBases:  make(map[string]*managedBaseEntry),
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

func TestOnRCUpdate_EmptyQueriesDisables(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		NodeName:  "node1",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	// First: schedule a DO config so the base config becomes managed.
	enable := DOQueryPayload{
		ConfigID:     "cfg-1",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	enableJSON, err := json.Marshal(enable)
	require.NoError(t, err)
	collectStatuses(c, map[string]state.RawConfig{"path/config": {Config: enableJSON}})
	require.Contains(t, c.activeConfigs, "cfg-1")

	// Now: empty queries disables the DO config and the original base config must be restored.
	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/config": {Config: []byte(`{"config_id": "cfg-1", "queries": []}`)},
	})

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["path/config"].State)
	assert.Empty(t, c.activeConfigs)
	require.Len(t, changes.Unschedule, 1, "should unschedule the DO config")
	require.Len(t, changes.Schedule, 1, "should re-schedule original base config")
	assert.Equal(t, postgresCfg, changes.Schedule[0], "scheduled config should be the original base config")
}

func TestOnRCUpdate_ReconcileDisablesStaleConfigs(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	// First: schedule a DO config so the base config becomes managed.
	enable := DOQueryPayload{
		ConfigID:     "stale-config",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	enableJSON, err := json.Marshal(enable)
	require.NoError(t, err)
	collectStatuses(c, map[string]state.RawConfig{"path/stale": {Config: enableJSON}})
	require.Contains(t, c.activeConfigs, "stale-config")

	// Update snapshot contains only a config without config_id — stale-config should be disabled
	// and the original base config restored.
	_, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/other": {Config: []byte(`{"some_field": true}`)},
	})

	assert.Empty(t, c.activeConfigs)
	require.Len(t, changes.Unschedule, 1, "should unschedule the DO config")
	require.Len(t, changes.Schedule, 1, "should re-schedule original base config")
	assert.Equal(t, postgresCfg, changes.Schedule[0])
}

// --- removeActiveConfig tests ---

func TestRemoveActiveConfig_NotFound(t *testing.T) {
	c := newTestComponent(t)
	changes := integration.ConfigChanges{}
	c.removeActiveConfig("nonexistent", &changes)
	assert.Empty(t, changes.Schedule)
	assert.Empty(t, changes.Unschedule)
}

func TestRemoveActiveConfig_Found(t *testing.T) {
	baseCfg := &integration.Config{Name: "postgres", Provider: "file"}
	doCheckConfig := integration.Config{Name: "postgres", Provider: "do_query_actions"}
	c := newTestComponent(t)
	c.activeConfigs["my-config"] = activeConfigEntry{
		checkConfig: doCheckConfig,
		baseCfg:     baseCfg,
		matchHost:   "localhost",
	}
	changes := integration.ConfigChanges{}

	c.removeActiveConfig("my-config", &changes)

	assert.Empty(t, c.activeConfigs, "config should be removed from activeConfigs")
	require.Len(t, changes.Unschedule, 1, "should unschedule previous DO config")
	assert.Equal(t, doCheckConfig, changes.Unschedule[0])
	assert.Empty(t, changes.Schedule, "removeActiveConfig should NOT re-schedule base config")
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
			integration.Data("host: localhost\nport: 5432\nusername: datadog\npassword: secret\ndbname: mydb\ndata_observability:\n  enabled: true\n"),
		},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-happy",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
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
	require.Len(t, changes.Schedule, 1, "expected one scheduled DO check")
	require.Len(t, changes.Unschedule, 1, "should unschedule base file-provider config to prevent duplicate")
	assert.Equal(t, "postgres", changes.Schedule[0].Name)
	assert.Equal(t, "file", changes.Schedule[0].Provider)
	assert.Equal(t, "node1", changes.Schedule[0].NodeName)

	require.Len(t, changes.Schedule[0].Instances, 1)
	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes.Schedule[0].Instances[0], &instance))
	assert.Equal(t, "localhost", instance["host"])
	assert.Equal(t, "datadog", instance["username"])

	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, doConfig["enabled"])
	assert.Equal(t, "rc-id-happy", doConfig["config_id"])

	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)
	q := queries[0].(map[string]any)
	assert.Equal(t, "SELECT count(*) FROM orders", q["query"])

	require.Contains(t, c.activeConfigs, "cfg-happy")
}

// TestOnRCUpdate_UpdateReplacesExistingCheck verifies that two sequential onRCUpdate calls
// with the same config_id correctly unschedule the previous check and schedule the updated one.
func TestOnRCUpdate_UpdateReplacesExistingCheck(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	mkPayload := func(query string) []byte {
		b, err := json.Marshal(DOQueryPayload{
			ConfigID:     "cfg-update",
			DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
			Queries:      []QuerySpec{{Type: "run_query", Query: query, IntervalSeconds: 60, TimeoutSeconds: 10}},
		})
		require.NoError(t, err)
		return b
	}

	// First update: schedule initial version. Unschedules the base file-provider config.
	_, changes1 := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg": {Config: mkPayload("SELECT 1")},
	})
	require.Len(t, changes1.Schedule, 1, "first update should schedule the DO check")
	require.Len(t, changes1.Unschedule, 1, "first update should unschedule base config")
	require.Contains(t, c.activeConfigs, "cfg-update")

	// Second update: same config_id, different query. The base config is already managed
	// (unscheduled on the first update), so only the previous DO config is unscheduled and the
	// new one scheduled — the base config is not touched again.
	_, changes2 := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg": {Config: mkPayload("SELECT 2")},
	})
	require.Len(t, changes2.Unschedule, 1, "should unschedule only the previous DO config")
	require.Len(t, changes2.Schedule, 1, "should schedule only the new DO check")

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes2.Schedule[0].Instances[0], &instance))
	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)
	assert.Equal(t, "SELECT 2", queries[0].(map[string]any)["query"])
}

// hostsOf parses every instance of a config and returns the set of host values, for asserting
// which postgres instances a scheduled config covers.
func hostsOf(t *testing.T, cfg integration.Config) map[string]bool {
	t.Helper()
	hosts := make(map[string]bool, len(cfg.Instances))
	for _, instanceData := range cfg.Instances {
		var instance map[string]any
		require.NoError(t, yaml.Unmarshal(instanceData, &instance))
		host, _ := instance["host"].(string)
		hosts[host] = true
	}
	return hosts
}

// findScheduledWithHost returns the scheduled config that contains an instance with the given
// host, failing the test if none is found.
func findScheduledWithHost(t *testing.T, changes integration.ConfigChanges, host string) integration.Config {
	t.Helper()
	for _, cfg := range changes.Schedule {
		if hostsOf(t, cfg)[host] {
			return cfg
		}
	}
	t.Fatalf("no scheduled config contains host %q", host)
	return integration.Config{}
}

// TestOnRCUpdate_PreservesUnrelatedInstances is the regression test for the bug where a
// do-query-actions config replaced the whole file-provider postgres config, dropping sibling
// instances. A base config with two instances (localhost + an RDS endpoint) receives a DO config
// targeting only localhost; the RDS instance must stay scheduled, and only localhost may carry
// the DO queries.
func TestOnRCUpdate_PreservesUnrelatedInstances(t *testing.T) {
	const rdsHost = "iceberg-test-postgres-demo-rds.c0lma4q6o85w.us-east-1.rds.amazonaws.com"
	postgresCfg := integration.Config{
		Name:     "postgres",
		Provider: "file",
		Instances: []integration.Data{
			integration.Data("host: localhost\nport: 5432\ndbname: testdb\ndata_observability:\n  enabled: true\ntags:\n  - env:demo\n"),
			integration.Data("host: " + rdsHost + "\nport: 5432\ndbname: testdb\ndata_observability:\n  enabled: true\ntags:\n  - env:rds\n"),
		},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-local",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-local": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-local"].State)

	// The original two-instance base config is unscheduled.
	require.Len(t, changes.Unschedule, 1)
	assert.Equal(t, map[string]bool{"localhost": true, rdsHost: true}, hostsOf(t, changes.Unschedule[0]))

	// Two configs scheduled: the DO check for localhost and the remainder holding the RDS instance.
	require.Len(t, changes.Schedule, 2)

	doCfg := findScheduledWithHost(t, changes, "localhost")
	require.Len(t, doCfg.Instances, 1, "DO config should carry only the targeted localhost instance")
	var doInstance map[string]any
	require.NoError(t, yaml.Unmarshal(doCfg.Instances[0], &doInstance))
	_, hasDO := doInstance["data_observability"].(map[string]any)["queries"]
	assert.True(t, hasDO, "localhost instance should carry DO queries")

	remainder := findScheduledWithHost(t, changes, rdsHost)
	require.Len(t, remainder.Instances, 1, "remainder should hold only the untargeted RDS instance")
	assert.Equal(t, "file", remainder.Provider, "remainder keeps the base config provider")
	var rdsInstance map[string]any
	require.NoError(t, yaml.Unmarshal(remainder.Instances[0], &rdsInstance))
	assert.Equal(t, rdsHost, rdsInstance["host"])
	_, rdsHasQueries := rdsInstance["data_observability"].(map[string]any)["queries"]
	assert.False(t, rdsHasQueries, "RDS instance must remain a plain DBM instance with no DO queries")
}

// TestBuildRemainder_SapHanaServerKey is a focused regression test for buildRemainder using the
// "server" key and the "host:port" identifier form sap_hana backends actually send. sap_hana
// instances key the host under "server" (not "host") with a separate "port", while the RC
// identifier arrives as "server:port" (e.g. "172.17.128.2:39041"). buildRemainder must recognize
// the targeted server as DO-managed and drop it from the remainder. Before the fix it compared the
// absent "host" key against the "host:port" identifier, so no sap_hana instance ever matched and
// the targeted one was wrongly kept, duplicating collection.
func TestBuildRemainder_SapHanaServerKey(t *testing.T) {
	const targetedServer = "172.17.128.2"
	const siblingServer = "172.17.128.3"
	const port = 39041
	base := &integration.Config{
		Name:     "sap_hana",
		Provider: "file",
		Instances: []integration.Data{
			integration.Data(fmt.Sprintf("server: %s\nport: %d\n", targetedServer, port)),
			integration.Data(fmt.Sprintf("server: %s\nport: %d\n", siblingServer, port)),
		},
	}
	// matchedHosts holds DBIdentifier.Host verbatim: the "host:port" form for sap_hana.
	targetedHostPort := fmt.Sprintf("%s:%d", targetedServer, port)
	siblingHostPort := fmt.Sprintf("%s:%d", siblingServer, port)

	t.Run("targeted server excluded, sibling kept", func(t *testing.T) {
		remainder := buildRemainder(base, map[string]bool{targetedHostPort: true})
		require.NotNil(t, remainder, "sibling instance must keep the remainder alive")
		require.Len(t, remainder.Instances, 1, "targeted server must be excluded from the remainder")
		var instance map[string]any
		require.NoError(t, yaml.Unmarshal(remainder.Instances[0], &instance))
		assert.Equal(t, siblingServer, instance["server"], "remainder should hold only the untargeted sibling")
	})

	t.Run("all servers targeted yields nil remainder", func(t *testing.T) {
		remainder := buildRemainder(base, map[string]bool{targetedHostPort: true, siblingHostPort: true})
		assert.Nil(t, remainder, "no instances should remain when every sap_hana server is DO-managed")
	})
}

// TestOnRCUpdate_SapHana_ExcludesTargetedInstanceFromRemainder is the end-to-end regression test
// for the "3 parallel sap_hana check instances" bug, using the real "host:port" identifier form.
// A base config bundles two sap_hana instances (keyed by "server"); a DO config targets only the
// first via a "server:port" identifier. The targeted server must run solely as the DO check while
// the sibling stays in the remainder. Before the fix the remainder wrongly kept the targeted
// server too, so it ran both as the DO check and in the remainder alongside the original
// file-provider config.
func TestOnRCUpdate_SapHana_ExcludesTargetedInstanceFromRemainder(t *testing.T) {
	const targetedServer = "172.17.128.2"
	const siblingServer = "172.17.128.3"
	const port = 39041
	sapHanaCfg := integration.Config{
		Name:     "sap_hana",
		Provider: "file",
		Instances: []integration.Data{
			integration.Data(fmt.Sprintf("server: %s\nport: %d\ndata_observability:\n  enabled: true\n", targetedServer, port)),
			integration.Data(fmt.Sprintf("server: %s\nport: %d\ndata_observability:\n  enabled: true\n", siblingServer, port)),
		},
	}
	c := newTestComponentWithAC(t, []integration.Config{sapHanaCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-saphana",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: fmt.Sprintf("%s:%d", targetedServer, port)},
		Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-saphana": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-saphana"].State)

	// The original two-instance base config is unscheduled.
	require.Len(t, changes.Unschedule, 1)

	// Exactly two configs scheduled: the DO check for the targeted server and the remainder holding
	// only the untargeted sibling. A third scheduled instance would be the regression.
	require.Len(t, changes.Schedule, 2)

	serversOf := func(cfg integration.Config) []string {
		servers := make([]string, 0, len(cfg.Instances))
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			require.NoError(t, yaml.Unmarshal(instanceData, &instance))
			servers = append(servers, instance["server"].(string))
		}
		return servers
	}

	var doCfg, remainder *integration.Config
	for i := range changes.Schedule {
		var instance map[string]any
		require.NoError(t, yaml.Unmarshal(changes.Schedule[i].Instances[0], &instance))
		if _, hasQueries := instance["data_observability"].(map[string]any)["queries"]; hasQueries {
			doCfg = &changes.Schedule[i]
		} else {
			remainder = &changes.Schedule[i]
		}
	}
	require.NotNil(t, doCfg, "a DO check config should be scheduled")
	require.NotNil(t, remainder, "a remainder config should be scheduled")

	assert.Equal(t, []string{targetedServer}, serversOf(*doCfg), "DO check should carry only the targeted sap_hana server")
	assert.Equal(t, []string{siblingServer}, serversOf(*remainder), "remainder must exclude the targeted server and keep only the sibling")
}

// TestOnRCUpdate_MultipleDOConfigsSameBase verifies that two DO configs targeting two different
// instances of the same base config never leave an instance both in the remainder and as a DO
// check (which would double-run it). With both instances targeted, no remainder is scheduled.
func TestOnRCUpdate_MultipleDOConfigsSameBase(t *testing.T) {
	const rdsHost = "rds.example.com"
	postgresCfg := integration.Config{
		Name:     "postgres",
		Provider: "file",
		Instances: []integration.Data{
			integration.Data("host: localhost\ndata_observability:\n  enabled: true\n"),
			integration.Data("host: " + rdsHost + "\ndata_observability:\n  enabled: true\n"),
		},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	mkPayload := func(configID, host string) []byte {
		b, err := json.Marshal(DOQueryPayload{
			ConfigID:     configID,
			DBIdentifier: DBIdentifier{Type: "self-hosted", Host: host},
			Queries:      []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}},
		})
		require.NoError(t, err)
		return b
	}

	_, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/local": {Config: mkPayload("cfg-local", "localhost")},
		"path/rds":   {Config: mkPayload("cfg-rds", rdsHost)},
	})

	// Both instances are DO-targeted, so the base config is unscheduled and there is no remainder.
	require.Len(t, changes.Unschedule, 1, "only the original base config is unscheduled")
	require.Len(t, changes.Schedule, 2, "two DO checks, no remainder")
	for _, cfg := range changes.Schedule {
		require.Len(t, cfg.Instances, 1, "each scheduled config is a single-instance DO check")
	}

	// Disabling one DO config restores a remainder holding the still-targeted other instance.
	_, changes2 := collectStatuses(c, map[string]state.RawConfig{
		"path/local": {Config: mkPayload("cfg-local", "localhost")},
		"path/rds":   {Config: []byte(`{"config_id": "cfg-rds", "queries": []}`)},
	})
	assert.NotContains(t, c.activeConfigs, "cfg-rds")
	require.Contains(t, c.activeConfigs, "cfg-local")
	remainder := findScheduledWithHost(t, changes2, rdsHost)
	require.Len(t, remainder.Instances, 1)
	assert.Equal(t, map[string]bool{rdsHost: true}, hostsOf(t, remainder))
}

// TestOnRCUpdate_NoMatchingPostgres_ReportsError verifies that when no postgres instance
// matches the RC identifier, the apply status is Error and no check is scheduled.
func TestOnRCUpdate_NoMatchingPostgres_ReportsError(t *testing.T) {
	c := newTestComponentWithAC(t, []integration.Config{}) // no postgres configs

	payload := DOQueryPayload{
		ConfigID:     "cfg-nomatch",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "notfound.example.com"},
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

// TestOnRCUpdate_HostOnlyMatching verifies that a config matches a postgres instance
// by host only, with per-query dbname routing to different databases.
func TestOnRCUpdate_HostOnlyMatching(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: testdb\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-hostonly",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				DBName:          "analyticsdb",
				Type:            "run_query",
				Query:           "SELECT count(*) AS dd_value FROM events.page_views",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-hostonly": {Config: payloadJSON},
	})

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-hostonly"].State)
	require.Len(t, changes.Schedule, 1, "should schedule the DO check")
	require.Len(t, changes.Unschedule, 1, "should unschedule base file-provider config")
	require.Contains(t, c.activeConfigs, "cfg-hostonly")
}

// TestBuildCheckConfig_PerQueryDBName verifies that the dbname field from each QuerySpec
// appears in the YAML output for each query entry.
func TestBuildCheckConfig_PerQueryDBName(t *testing.T) {
	c := &component{log: logmock.New(t)}

	payload := &DOQueryPayload{
		ConfigID: "cfg-multidb",
		Queries: []QuerySpec{
			{
				DBName:          "testdb",
				MonitorID:       100,
				Type:            "run_query",
				Query:           "SELECT count(*) AS dd_value FROM shop.orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "testdb", Schema: "shop", Table: "orders"},
			},
			{
				DBName:          "analyticsdb",
				MonitorID:       200,
				Type:            "run_query",
				Query:           "SELECT count(*) AS dd_value FROM events.clicks",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "analyticsdb", Schema: "events", Table: "clicks"},
			},
		},
	}

	baseCfg := &integration.Config{Name: "postgres"}
	pgInstance := map[string]any{"host": "localhost", "dbname": "testdb", "data_observability": map[string]any{"enabled": true}}

	checkCfg, err := c.buildCheckConfig(payload, baseCfg, pgInstance, "rc-multidb")
	require.NoError(t, err)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(checkCfg.Instances[0], &instance))

	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 2)

	q1 := queries[0].(map[string]any)
	assert.Equal(t, "testdb", q1["dbname"])
	assert.Equal(t, 100, q1["monitor_id"])

	q2 := queries[1].(map[string]any)
	assert.Equal(t, "analyticsdb", q2["dbname"])
	assert.Equal(t, 200, q2["monitor_id"])
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
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
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

// --- validateQuerySpec tests ---

// TestValidateQuerySpec_ValidScheduleOnly verifies that a query with a valid cron schedule
// and no interval_seconds passes validation and flows through to the scheduled check.
func TestValidateQuerySpec_ValidScheduleOnly(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		NodeName:  "node1",
		Instances: []integration.Data{integration.Data("host: localhost\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-cron-only",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				MonitorID:      42,
				Type:           "run_query",
				Query:          "SELECT count(*) FROM orders",
				Schedule:       "20 * * * *",
				TimeoutSeconds: 10,
				Entity:         EntityMetadata{Platform: "postgres", Database: "shop", Table: "orders"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-cron-only": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-cron-only"].State)
	require.Len(t, changes.Schedule, 1, "should schedule the DO check")

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes.Schedule[0].Instances[0], &instance))
	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 10, doConfig["collection_interval"], "collection_interval must always be 10")

	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)
	q := queries[0].(map[string]any)
	assert.Equal(t, "20 * * * *", q["schedule"], "schedule field should be injected into query YAML")
	_, hasInterval := q["interval_seconds"]
	assert.False(t, hasInterval, "interval_seconds should be absent when not set in the RC payload")
}

// TestValidateQuerySpec_BothScheduleAndInterval verifies that when both schedule and
// interval_seconds are set, the config flows through (cron wins downstream in Python).
func TestValidateQuerySpec_BothScheduleAndInterval(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		Instances: []integration.Data{integration.Data("host: localhost\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-both",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				MonitorID:       77,
				Type:            "run_query",
				Query:           "SELECT 1",
				IntervalSeconds: 300,
				Schedule:        "*/15 * * * *",
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "db", Table: "t"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-both": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-both"].State)
	require.Len(t, changes.Schedule, 1, "should schedule the DO check when both fields are set")

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes.Schedule[0].Instances[0], &instance))
	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 10, doConfig["collection_interval"], "collection_interval must always be 10")

	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)
	q := queries[0].(map[string]any)
	assert.Equal(t, "*/15 * * * *", q["schedule"], "schedule field should be injected")
	assert.Equal(t, 300, q["interval_seconds"], "interval_seconds should be present when set in the RC payload")
}

// TestValidateQuerySpec_NeitherSetRejected verifies that a query with neither schedule nor
// a positive interval_seconds is rejected with ApplyStateError and no check is scheduled.
func TestValidateQuerySpec_NeitherSetRejected(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-neither",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				MonitorID:       55,
				Type:            "run_query",
				Query:           "SELECT 1",
				IntervalSeconds: 0, // zero — invalid when no schedule
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "db", Table: "t"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-neither": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateError, statuses["path/cfg-neither"].State)
	assert.Contains(t, statuses["path/cfg-neither"].Error, "interval_seconds must be > 0 when schedule is unset")
	assert.Empty(t, changes.Schedule, "no check should be scheduled for invalid query")
}

// TestValidateQuerySpec_InvalidCronRejected verifies that a query with an invalid cron
// expression is rejected with ApplyStateError before any postgres config lookup occurs.
func TestValidateQuerySpec_InvalidCronRejected(t *testing.T) {
	// No postgres configs at all — if validation fires before findPostgresConfig, this test
	// will still report ApplyStateError (not "no matching postgres config").
	c := newTestComponentWithAC(t, []integration.Config{})

	payload := DOQueryPayload{
		ConfigID:     "cfg-badcron",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				MonitorID:      33,
				Type:           "run_query",
				Query:          "SELECT 1",
				Schedule:       "not-a-cron",
				TimeoutSeconds: 10,
				Entity:         EntityMetadata{Platform: "postgres", Database: "db", Table: "t"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-badcron": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateError, statuses["path/cfg-badcron"].State)
	assert.Contains(t, statuses["path/cfg-badcron"].Error, "invalid cron schedule",
		"error should mention the bad cron expression")
	assert.Empty(t, changes.Schedule, "no check should be scheduled for invalid cron")
}

// TestValidateQuerySpec_ValidIntervalOnly verifies that existing behavior is preserved:
// a query with only interval_seconds set (no schedule) flows through correctly and
// does not inject a schedule field into the YAML.
func TestValidateQuerySpec_ValidIntervalOnly(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Provider:  "file",
		Instances: []integration.Data{integration.Data("host: localhost\ndata_observability:\n  enabled: true\n")},
	}
	c := newTestComponentWithAC(t, []integration.Config{postgresCfg})

	payload := DOQueryPayload{
		ConfigID:     "cfg-interval-only",
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: "localhost"},
		Queries: []QuerySpec{
			{
				MonitorID:       99,
				Type:            "run_query",
				Query:           "SELECT count(*) FROM orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity:          EntityMetadata{Platform: "postgres", Database: "shop", Table: "orders"},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	statuses, changes := collectStatuses(c, map[string]state.RawConfig{
		"path/cfg-interval-only": {Config: payloadJSON},
	})

	require.Equal(t, state.ApplyStateAcknowledged, statuses["path/cfg-interval-only"].State)
	require.Len(t, changes.Schedule, 1, "should schedule the DO check")

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(changes.Schedule[0].Instances[0], &instance))
	doConfig, ok := instance["data_observability"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 10, doConfig["collection_interval"], "collection_interval must always be 10")

	queries, ok := doConfig["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)
	q := queries[0].(map[string]any)
	assert.Equal(t, 60, q["interval_seconds"], "interval_seconds should be present")
	_, hasSchedule := q["schedule"]
	assert.False(t, hasSchedule, "schedule field must be absent when not set on the query")
}
