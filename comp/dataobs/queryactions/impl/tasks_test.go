// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const (
	testTaskID      = "3f1c2a9e-8b7d-4c1e-9f2a-6d5e4c3b2a10"
	testOtherTaskID = "7a2b3c4d-1e2f-4a5b-8c9d-0e1f2a3b4c5d"
	testMonitorID   = "do-mysql-0123456789abcdef"
	testMySQLHost   = "db.internal"
)

var testNow = time.Unix(1790000000, 0)

// fakeForwarder records the events sent to the event platform.
type fakeForwarder struct {
	mu     sync.Mutex
	events []map[string]any
	types  []string
}

func (f *fakeForwarder) SendEventPlatformEvent(m *message.Message, eventType string) error {
	var event map[string]any
	if err := json.Unmarshal(m.GetContent(), &event); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	f.types = append(f.types, eventType)
	return nil
}

func (f *fakeForwarder) SendEventPlatformEventBlocking(m *message.Message, eventType string) error {
	return f.SendEventPlatformEvent(m, eventType)
}

func (f *fakeForwarder) Purge() map[string][]*message.Message {
	return nil
}

func (f *fakeForwarder) sent() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.events...)
}

// fakeEventPlatform is an eventplatform.Component whose forwarder may be unavailable.
type fakeEventPlatform struct {
	forwarder eventplatform.Forwarder
}

func (f fakeEventPlatform) Get() (eventplatform.Forwarder, bool) {
	return f.forwarder, f.forwarder != nil
}

func newTaskTestComponent(t *testing.T, configs []integration.Config) (*component, *fakeForwarder) {
	t.Helper()
	forwarder := &fakeForwarder{}
	c := newTestComponentWithAC(t, configs)
	c.eventPlatform = fakeEventPlatform{forwarder: forwarder}
	c.now = func() time.Time { return testNow }
	return c, forwarder
}

func setUnresolvedConfigs(c *component, configs []integration.Config) {
	c.ac.(*mockAutodiscovery).configs = configs
}

// mysqlBaseConfig is a user's mysql config with DBM, custom queries and DO enabled.
func mysqlBaseConfig() integration.Config {
	return integration.Config{
		Name:       "mysql",
		Provider:   "file",
		Source:     "file:/etc/datadog-agent/conf.d/mysql.d/conf.yaml",
		NodeName:   "node1",
		InitConfig: integration.Data("service: shop-db\n"),
		Instances: []integration.Data{integration.Data(`host: db.internal
port: 3306
username: datadog
password: ENC[mysql_password]
ssl:
  ca: /etc/ssl/ca.pem
tags:
  - env:prod
dbm: true
custom_queries:
  - query: SELECT 1
    columns:
      - name: one
        type: gauge
query_metrics:
  enabled: true
data_observability:
  enabled: true
`)},
	}
}

func taskConfigID(taskID string) string {
	return "do-mysql-once-" + taskID
}

func rcPath(configID string) string {
	return "datadog/2/DO_QUERY_ACTIONS/" + configID + "/config"
}

func buildTaskPayload(taskID, host string, expiresAt int64) DOTaskPayload {
	return DOTaskPayload{
		ConfigID:     taskConfigID(taskID),
		Kind:         taskKind,
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: host},
		Task: TaskSpec{
			TaskID:    taskID,
			CreatedAt: testNow.Unix() - 10,
			ExpiresAt: expiresAt,
			Statements: []TaskStatement{
				{ID: "s0", DBName: "shop", Query: "SELECT count(*)\n  FROM orders -- all\nWHERE 1 = 1", TimeoutSeconds: 300, MaxRows: 10000},
				{ID: "s1", DBName: "shop", Query: "SELECT 1", TimeoutSeconds: 30, MaxRows: 1},
			},
		},
	}
}

func taskRawConfig(t *testing.T, payload DOTaskPayload) state.RawConfig {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return state.RawConfig{Config: b, Metadata: state.Metadata{ID: payload.ConfigID}}
}

func taskUpdate(t *testing.T, taskID, host string) (string, state.RawConfig) {
	t.Helper()
	return rcPath(taskConfigID(taskID)), taskRawConfig(t, buildTaskPayload(taskID, host, testNow.Unix()+600))
}

func monitorUpdate(t *testing.T) (string, state.RawConfig) {
	t.Helper()
	b, err := json.Marshal(DOQueryPayload{
		ConfigID:     testMonitorID,
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: testMySQLHost},
		Queries:      []QuerySpec{{MonitorID: 7, Type: "run_query", Query: "SELECT count(*) FROM orders", IntervalSeconds: 60, TimeoutSeconds: 10}},
	})
	require.NoError(t, err)
	return rcPath(testMonitorID), state.RawConfig{Config: b, Metadata: state.Metadata{ID: testMonitorID}}
}

// drainTaskChanges pops every queued task change and merges them for assertions.
func drainTaskChanges(c *component) integration.ConfigChanges {
	merged := integration.ConfigChanges{}
	for {
		changes, ok := c.taskChanges.pop()
		if !ok {
			return merged
		}
		merged.Schedule = append(merged.Schedule, changes.Schedule...)
		merged.Unschedule = append(merged.Unschedule, changes.Unschedule...)
	}
}

// isEmpty makes IsEmpty (a pointer method) callable on function results.
func isEmpty(changes integration.ConfigChanges) bool {
	return changes.IsEmpty()
}

func parseInstance(t *testing.T, cfg integration.Config) map[string]any {
	t.Helper()
	require.Len(t, cfg.Instances, 1)
	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &instance))
	return instance
}

func hasDOTask(t *testing.T, cfg integration.Config) bool {
	t.Helper()
	for _, instanceData := range cfg.Instances {
		var instance map[string]any
		require.NoError(t, yaml.Unmarshal(instanceData, &instance))
		if _, ok := instance["do_task"]; ok {
			return true
		}
	}
	return false
}

func TestIsTaskConfigID(t *testing.T) {
	for configID, want := range map[string]bool{
		"do-mysql-once-" + testTaskID:    true,
		"do-postgres-once-" + testTaskID: true,
		"do-mysql-0123456789abcdef":      false,
		"do-0123456789abcdef":            false,
		"do-mysql-once":                  false,
		"xdo-mysql-once-" + testTaskID:   false,
	} {
		assert.Equal(t, want, isTaskConfigID(configID), configID)
	}
}

func TestSplitTaskUpdates(t *testing.T) {
	taskPath, taskRaw := taskUpdate(t, testTaskID, testMySQLHost)
	monitorPath, monitorRaw := monitorUpdate(t)
	// Without RC metadata the payload's config_id decides.
	noMetadataTask := taskRawConfig(t, buildTaskPayload(testOtherTaskID, testMySQLHost, testNow.Unix()+600))
	noMetadataTask.Metadata = state.Metadata{}

	monitors, tasks := splitTaskUpdates(map[string]state.RawConfig{
		taskPath:                           taskRaw,
		monitorPath:                        monitorRaw,
		"path/no-metadata-task":            noMetadataTask,
		"path/invalid-json-without-an-ID":  {Config: []byte(`{not json`)},
		"path/monitor-with-empty-queries-": {Config: []byte(`{"config_id":"do-mysql-fedcba9876543210","queries":[]}`)},
	})

	assert.ElementsMatch(t, []string{monitorPath, "path/invalid-json-without-an-ID", "path/monitor-with-empty-queries-"}, keys(monitors))
	require.Len(t, tasks, 2)
	assert.Equal(t, taskPath, tasks[taskConfigID(testTaskID)].path)
	assert.Equal(t, "path/no-metadata-task", tasks[taskConfigID(testOtherTaskID)].path)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestOnRCUpdate_TasksAndMonitorsAreIsolated checks that tasks never show up in the monitor path's
// changes or state, and that adding or removing either kind of config never touches the other.
func TestOnRCUpdate_TasksAndMonitorsAreIsolated(t *testing.T) {
	base := mysqlBaseConfig()
	c, _ := newTaskTestComponent(t, []integration.Config{base})
	taskPath, taskRaw := taskUpdate(t, testTaskID, testMySQLHost)
	monitorPath, monitorRaw := monitorUpdate(t)

	// 1. A monitor and a task arrive together.
	statuses, monitorChanges := collectStatuses(c, map[string]state.RawConfig{taskPath: taskRaw, monitorPath: monitorRaw})
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[taskPath].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[monitorPath].State)
	require.Len(t, monitorChanges.Schedule, 1, "monitor path schedules only the monitor check")
	require.Len(t, monitorChanges.Unschedule, 1, "monitor path replaces only the base config")
	assert.False(t, hasDOTask(t, monitorChanges.Schedule[0]))
	assert.Equal(t, base.Digest(), monitorChanges.Unschedule[0].Digest())
	assert.Equal(t, []string{testMonitorID}, keys(c.activeConfigs))

	taskChanges := drainTaskChanges(c)
	require.Len(t, taskChanges.Schedule, 1)
	assert.Empty(t, taskChanges.Unschedule, "a new task never unschedules anything")
	assert.True(t, hasDOTask(t, taskChanges.Schedule[0]))
	taskCheck := taskChanges.Schedule[0]

	// 2. The monitor is removed; the task stays. Only the monitor path reacts.
	_, monitorChanges = collectStatuses(c, map[string]state.RawConfig{taskPath: taskRaw})
	assert.Empty(t, c.activeConfigs)
	require.Len(t, monitorChanges.Schedule, 1, "the base config is restored")
	assert.Equal(t, base.Digest(), monitorChanges.Schedule[0].Digest())
	for _, cfg := range monitorChanges.Unschedule {
		assert.False(t, hasDOTask(t, cfg), "monitor removal must not unschedule the task check")
	}
	assert.True(t, isEmpty(drainTaskChanges(c)), "an unchanged task is left alone")

	// 3. The monitor comes back; the task is left alone again.
	_, _ = collectStatuses(c, map[string]state.RawConfig{taskPath: taskRaw, monitorPath: monitorRaw})
	assert.True(t, isEmpty(drainTaskChanges(c)))

	// 4. The task is removed; only its own check is unscheduled.
	_, monitorChanges = collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw})
	for _, cfg := range append(monitorChanges.Schedule, monitorChanges.Unschedule...) {
		assert.False(t, hasDOTask(t, cfg))
	}
	assert.Contains(t, c.activeConfigs, testMonitorID)
	taskChanges = drainTaskChanges(c)
	assert.Empty(t, taskChanges.Schedule)
	require.Len(t, taskChanges.Unschedule, 1)
	assert.Equal(t, taskCheck.Digest(), taskChanges.Unschedule[0].Digest())
}

func TestOnTaskUpdate_SchedulesOnceAndUnschedulesWhenConfigDisappears(t *testing.T) {
	c, forwarder := newTaskTestComponent(t, []integration.Config{mysqlBaseConfig()})
	taskPath, taskRaw := taskUpdate(t, testTaskID, testMySQLHost)
	snapshot := map[string]state.RawConfig{taskPath: taskRaw}

	statuses, _ := collectStatuses(c, snapshot)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[taskPath].State)
	first := drainTaskChanges(c)
	require.Len(t, first.Schedule, 1)

	for i := 0; i < 3; i++ {
		statuses, _ = collectStatuses(c, snapshot)
		assert.Equal(t, state.ApplyStateAcknowledged, statuses[taskPath].State, "the stored status is re-reported")
		assert.True(t, isEmpty(drainTaskChanges(c)), "a task still in the snapshot is never rescheduled")
	}

	_, _ = collectStatuses(c, map[string]state.RawConfig{})
	gone := drainTaskChanges(c)
	assert.Empty(t, gone.Schedule)
	require.Len(t, gone.Unschedule, 1)
	assert.Equal(t, first.Schedule[0].Digest(), gone.Unschedule[0].Digest())
	assert.Empty(t, c.tasks)
	assert.Empty(t, forwarder.sent(), "a task that runs sends no event from the agent")
}

func assertTaskErrorEvent(t *testing.T, event map[string]any, taskID, errorKind string) {
	t.Helper()
	assert.Equal(t, taskConfigID(taskID), event["config_id"])
	assert.Equal(t, taskID, event["task_id"])
	assert.NotContains(t, event, "statement_id")
	assert.EqualValues(t, 0, event["chunk_index"])
	assert.EqualValues(t, 1, event["chunk_count"])
	assert.Equal(t, "error", event["status"])
	assert.Equal(t, errorKind, event["error_kind"])
	assert.Equal(t, "schedule", event["error_phase"])
	assert.EqualValues(t, testNow.UnixMilli(), event["timestamp"])
	assert.Equal(t, []any{}, event["rows"])
	assert.Equal(t, []any{}, event["columns"])
	assert.EqualValues(t, 0, event["row_count"])
	assert.NotEmpty(t, event["error"])
}

func TestOnTaskUpdate_ExpiredTaskFailsFast(t *testing.T) {
	for name, expiresAt := range map[string]int64{
		"expired earlier":     testNow.Unix() - 1,
		"expires right now":   testNow.Unix(),
		"expired long before": testNow.Unix() - 86400,
	} {
		t.Run(name, func(t *testing.T) {
			c, forwarder := newTaskTestComponent(t, []integration.Config{mysqlBaseConfig()})
			path := rcPath(taskConfigID(testTaskID))
			snapshot := map[string]state.RawConfig{path: taskRawConfig(t, buildTaskPayload(testTaskID, testMySQLHost, expiresAt))}

			statuses, _ := collectStatuses(c, snapshot)
			assert.Equal(t, state.ApplyStateError, statuses[path].State)
			assert.Contains(t, statuses[path].Error, "expired")
			assert.True(t, isEmpty(drainTaskChanges(c)))

			events := forwarder.sent()
			require.Len(t, events, 1)
			assert.Equal(t, []string{doQueryResultsEventType}, forwarder.types)
			assertTaskErrorEvent(t, events[0], testTaskID, "expired")
			assert.Equal(t, "mysql", events[0]["db_type"])
			assert.Equal(t, testMySQLHost, events[0]["db_host"])

			// The same snapshot again reports the same status without a second event.
			statuses, _ = collectStatuses(c, snapshot)
			assert.Equal(t, state.ApplyStateError, statuses[path].State)
			assert.Len(t, forwarder.sent(), 1)

			// The config disappearing unschedules nothing, since nothing was scheduled.
			_, _ = collectStatuses(c, map[string]state.RawConfig{})
			assert.True(t, isEmpty(drainTaskChanges(c)))
		})
	}
}

func TestOnTaskUpdate_NoMatchingInstanceFailsFast(t *testing.T) {
	doDisabled := mysqlBaseConfig()
	doDisabled.Instances = []integration.Data{integration.Data("host: db.internal\nport: 3306\nusername: datadog\n")}
	postgres := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: db.internal\nport: 5432\ndata_observability:\n  enabled: true\n")},
	}

	for _, tc := range []struct {
		name    string
		configs []integration.Config
		host    string
	}{
		{name: "different host", configs: []integration.Config{mysqlBaseConfig()}, host: "other.internal"},
		{name: "DO not enabled on the instance", configs: []integration.Config{doDisabled}, host: testMySQLHost},
		{name: "only a postgres instance on that host", configs: []integration.Config{postgres}, host: testMySQLHost},
		{name: "no integration configs", configs: nil, host: testMySQLHost},
		{name: "empty identifier host", configs: []integration.Config{mysqlBaseConfig()}, host: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, forwarder := newTaskTestComponent(t, tc.configs)
			path, raw := taskUpdate(t, testTaskID, tc.host)

			statuses, monitorChanges := collectStatuses(c, map[string]state.RawConfig{path: raw})
			assert.Equal(t, state.ApplyStateError, statuses[path].State)
			assert.True(t, monitorChanges.IsEmpty())
			assert.True(t, isEmpty(drainTaskChanges(c)))

			events := forwarder.sent()
			require.Len(t, events, 1)
			assertTaskErrorEvent(t, events[0], testTaskID, "no_matching_instance")
		})
	}
}

func TestOnTaskUpdate_UnsupportedPlatformFailsFast(t *testing.T) {
	postgres := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: db.internal\nport: 5432\ndata_observability:\n  enabled: true\n")},
	}
	c, forwarder := newTaskTestComponent(t, []integration.Config{postgres})
	payload := buildTaskPayload(testTaskID, testMySQLHost, testNow.Unix()+600)
	payload.ConfigID = "do-postgres-once-" + testTaskID
	path := rcPath(payload.ConfigID)

	statuses, _ := collectStatuses(c, map[string]state.RawConfig{path: taskRawConfig(t, payload)})
	assert.Equal(t, state.ApplyStateError, statuses[path].State)
	assert.Contains(t, statuses[path].Error, "only supported for mysql")
	assert.True(t, isEmpty(drainTaskChanges(c)))

	events := forwarder.sent()
	require.Len(t, events, 1)
	assert.Equal(t, "no_matching_instance", events[0]["error_kind"])
	assert.Equal(t, "postgres", events[0]["db_type"])
}

func TestOnTaskUpdate_InvalidPayloadIsRejectedWithoutEvent(t *testing.T) {
	valid := buildTaskPayload(testTaskID, testMySQLHost, testNow.Unix()+600)
	for _, tc := range []struct {
		name   string
		mutate func(*DOTaskPayload)
		raw    []byte
	}{
		{name: "invalid JSON", raw: []byte(`{"kind": "task", "task": [}`)},
		{name: "monitor kind", mutate: func(p *DOTaskPayload) { p.Kind = "monitor" }},
		{name: "missing kind", mutate: func(p *DOTaskPayload) { p.Kind = "" }},
		{name: "empty task_id", mutate: func(p *DOTaskPayload) { p.Task.TaskID = "" }},
		{name: "task_id not in config ID", mutate: func(p *DOTaskPayload) { p.Task.TaskID = testOtherTaskID }},
		{name: "payload config_id differs", mutate: func(p *DOTaskPayload) { p.ConfigID = taskConfigID(testOtherTaskID) }},
		{name: "missing expires_at", mutate: func(p *DOTaskPayload) { p.Task.ExpiresAt = 0 }},
		{name: "no statements", mutate: func(p *DOTaskPayload) { p.Task.Statements = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, forwarder := newTaskTestComponent(t, []integration.Config{mysqlBaseConfig()})
			configID := taskConfigID(testTaskID)
			raw := tc.raw
			if raw == nil {
				payload := valid
				payload.Task.Statements = append([]TaskStatement(nil), valid.Task.Statements...)
				tc.mutate(&payload)
				var err error
				raw, err = json.Marshal(payload)
				require.NoError(t, err)
			}
			path := rcPath(configID)

			statuses, monitorChanges := collectStatuses(c, map[string]state.RawConfig{path: {Config: raw, Metadata: state.Metadata{ID: configID}}})
			assert.Equal(t, state.ApplyStateError, statuses[path].State)
			assert.NotEmpty(t, statuses[path].Error)
			assert.True(t, monitorChanges.IsEmpty(), "an invalid task never reaches the monitor path")
			assert.Empty(t, c.activeConfigs)
			assert.True(t, isEmpty(drainTaskChanges(c)))
			assert.Empty(t, forwarder.sent())
		})
	}
}

func TestOnTaskUpdate_CheckConfigCopiesOnlyConnectionFields(t *testing.T) {
	base := mysqlBaseConfig()
	c, _ := newTaskTestComponent(t, []integration.Config{base})
	path, raw := taskUpdate(t, testTaskID, testMySQLHost)

	_, _ = collectStatuses(c, map[string]state.RawConfig{path: raw})
	changes := drainTaskChanges(c)
	require.Len(t, changes.Schedule, 1)
	cfg := changes.Schedule[0]

	assert.Equal(t, "mysql", cfg.Name)
	assert.Equal(t, taskCheckSource, cfg.Source)
	assert.Equal(t, taskProviderName, cfg.Provider)
	assert.Equal(t, "node1", cfg.NodeName)
	assert.Equal(t, base.InitConfig, cfg.InitConfig)
	assert.Empty(t, cfg.ADIdentifiers)
	assert.Empty(t, cfg.LogsConfig)

	instance := parseInstance(t, cfg)
	assert.Equal(t, testMySQLHost, instance["host"])
	assert.Equal(t, 3306, instance["port"])
	assert.Equal(t, "datadog", instance["username"])
	assert.Equal(t, "ENC[mysql_password]", instance["password"], "secret handles are copied undecrypted")
	assert.Equal(t, map[string]any{"ca": "/etc/ssl/ca.pem"}, instance["ssl"])
	assert.Equal(t, []any{"env:prod"}, instance["tags"])

	for _, key := range []string{"data_observability", "custom_queries", "query_metrics", "dbm"} {
		assert.NotContains(t, instance, key)
	}
	assert.False(t, instanceHasDOEnabled(instance), "a task config must never be picked as a DO base")
	assert.Equal(t, true, instance["run_once"])

	doTask, ok := instance["do_task"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, taskConfigID(testTaskID), doTask["config_id"])
	assert.Equal(t, testTaskID, doTask["task_id"])
	assert.Equal(t, testNow.Unix()+600, int64(doTask["expires_at"].(int)))
	statements, ok := doTask["statements"].([]any)
	require.True(t, ok)
	require.Len(t, statements, 2)
	assert.Equal(t, map[string]any{
		"id":              "s0",
		"dbname":          "shop",
		"query":           "SELECT count(*)\n  FROM orders -- all\nWHERE 1 = 1",
		"timeout_seconds": 300,
		"max_rows":        10000,
	}, statements[0])
	assert.Equal(t, "s1", statements[1].(map[string]any)["id"])
}

// TestOnTaskUpdate_UsesMonitorBaseWhenMonitorReplacedTheInstance checks a task for an instance a
// monitor already targets: autodiscovery then reports the monitor's copy, which has no
// init_config, and the task must neither use the copy's DO settings nor disturb the monitor.
func TestOnTaskUpdate_UsesMonitorBaseWhenMonitorReplacedTheInstance(t *testing.T) {
	base := mysqlBaseConfig()
	c, _ := newTaskTestComponent(t, []integration.Config{base})
	monitorPath, monitorRaw := monitorUpdate(t)

	_, monitorChanges := collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw})
	require.Len(t, monitorChanges.Schedule, 1)
	monitorCheck := monitorChanges.Schedule[0]
	require.Empty(t, monitorCheck.InitConfig, "precondition: the monitor copy drops init_config")
	// Autodiscovery now reports the monitor's copy in place of the user's config.
	setUnresolvedConfigs(c, []integration.Config{monitorCheck})
	managedBefore := keys(c.managedBases)

	taskPath, taskRaw := taskUpdate(t, testTaskID, testMySQLHost)
	statuses, _ := collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw, taskPath: taskRaw})
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[taskPath].State)

	changes := drainTaskChanges(c)
	require.Len(t, changes.Schedule, 1)
	cfg := changes.Schedule[0]
	assert.Equal(t, base.InitConfig, cfg.InitConfig)
	instance := parseInstance(t, cfg)
	assert.NotContains(t, instance, "data_observability")
	assert.Equal(t, "ENC[mysql_password]", instance["password"])

	assert.Equal(t, managedBefore, keys(c.managedBases), "the task must not change which base configs the monitor manages")
	assert.Equal(t, base.Digest(), c.activeConfigs[testMonitorID].baseCfg.Digest())
}

func TestOnTaskUpdate_MatchesMonitorCopyWhenNoOriginalIsKnown(t *testing.T) {
	// A monitor copy reported by autodiscovery without the component knowing its original (for
	// example right after a restart of the component) is still a valid source of connection fields.
	c, _ := newTaskTestComponent(t, nil)
	clone := mysqlBaseConfig()
	clone.Source = names.DOQueryActions
	clone.InitConfig = nil
	setUnresolvedConfigs(c, []integration.Config{clone})

	path, raw := taskUpdate(t, testTaskID, testMySQLHost)
	statuses, _ := collectStatuses(c, map[string]state.RawConfig{path: raw})
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[path].State)
	changes := drainTaskChanges(c)
	require.Len(t, changes.Schedule, 1)
	assert.NotContains(t, parseInstance(t, changes.Schedule[0]), "data_observability")
}

func TestOnTaskUpdate_DigestIsUniquePerTask(t *testing.T) {
	base := mysqlBaseConfig()
	c, _ := newTaskTestComponent(t, []integration.Config{base})
	firstPath, firstRaw := taskUpdate(t, testTaskID, testMySQLHost)
	secondPath, secondRaw := taskUpdate(t, testOtherTaskID, testMySQLHost)

	statuses, _ := collectStatuses(c, map[string]state.RawConfig{firstPath: firstRaw, secondPath: secondRaw})
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[firstPath].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[secondPath].State)

	changes := drainTaskChanges(c)
	require.Len(t, changes.Schedule, 2, "two tasks for the same instance run as two checks")
	digests := map[string]bool{base.Digest(): true}
	for _, cfg := range changes.Schedule {
		assert.False(t, digests[cfg.Digest()], "digest collision")
		digests[cfg.Digest()] = true
	}

	// Removing one task unschedules only that task's check.
	_, _ = collectStatuses(c, map[string]state.RawConfig{secondPath: secondRaw})
	gone := drainTaskChanges(c)
	require.Len(t, gone.Unschedule, 1)
	doTask := parseInstance(t, gone.Unschedule[0])["do_task"].(map[string]any)
	assert.Equal(t, testTaskID, doTask["task_id"])
}

func TestFindMatchingConfig_NeverPicksTaskChecks(t *testing.T) {
	base := mysqlBaseConfig()
	c, _ := newTaskTestComponent(t, []integration.Config{base})
	path, raw := taskUpdate(t, testTaskID, testMySQLHost)
	_, _ = collectStatuses(c, map[string]state.RawConfig{path: raw})
	changes := drainTaskChanges(c)
	require.Len(t, changes.Schedule, 1)

	// Autodiscovery reports the task check next to (and before) the user's config.
	setUnresolvedConfigs(c, []integration.Config{changes.Schedule[0], base})
	dbID := DBIdentifier{Type: "self-hosted", Host: testMySQLHost}

	cfg, _, _, err := c.findMatchingConfig(testMonitorID, &dbID, nil)
	require.NoError(t, err)
	assert.Equal(t, base.Digest(), cfg.Digest(), "monitors must match the user's config, not a task check")

	cfg, _, err = c.resolveTaskInstance(taskConfigID(testOtherTaskID), &dbID)
	require.NoError(t, err)
	assert.Equal(t, base.Digest(), cfg.Digest(), "tasks must match the user's config, not another task check")
}

func TestOnTaskUpdate_ErrorWithoutForwarderStillReportsApplyState(t *testing.T) {
	for name, ep := range map[string]eventplatform.Component{
		"nil component":         nil,
		"forwarder unavailable": fakeEventPlatform{},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newTaskTestComponent(t, nil)
			c.eventPlatform = ep
			path, raw := taskUpdate(t, testTaskID, testMySQLHost)

			statuses, _ := collectStatuses(c, map[string]state.RawConfig{path: raw})
			assert.Equal(t, state.ApplyStateError, statuses[path].State)
		})
	}
}

// --- task provider ---

func receiveChanges(t *testing.T, ch <-chan integration.ConfigChanges) integration.ConfigChanges {
	t.Helper()
	select {
	case changes, ok := <-ch:
		require.True(t, ok, "channel closed unexpectedly")
		return changes
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task changes")
		return integration.ConfigChanges{}
	}
}

func namedChanges(i int) integration.ConfigChanges {
	cfg := integration.Config{Name: fmt.Sprintf("task-%d", i)}
	if i%2 == 0 {
		return integration.ConfigChanges{Schedule: []integration.Config{cfg}}
	}
	return integration.ConfigChanges{Unschedule: []integration.Config{cfg}}
}

func changesName(changes integration.ConfigChanges) string {
	if len(changes.Schedule) > 0 {
		return changes.Schedule[0].Name
	}
	return changes.Unschedule[0].Name
}

func TestTaskProvider_FirstMessageIsEmpty(t *testing.T) {
	p := &taskProvider{queue: newTaskChangesQueue()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.True(t, isEmpty(receiveChanges(t, p.Stream(ctx))))
	assert.Equal(t, taskProviderName, p.String())
	assert.Empty(t, p.GetConfigErrors())
}

func TestTaskProvider_DeliversEveryChangeInOrder(t *testing.T) {
	queue := newTaskChangesQueue()
	p := &taskProvider{queue: queue}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := p.Stream(ctx)
	require.True(t, isEmpty(receiveChanges(t, ch)))

	// A burst larger than any channel buffer, pushed while nobody reads.
	const burst = 500
	for i := 0; i < burst; i++ {
		queue.push(namedChanges(i))
	}
	queue.push(integration.ConfigChanges{}) // empty changes are not queued

	for i := 0; i < burst; i++ {
		assert.Equal(t, fmt.Sprintf("task-%d", i), changesName(receiveChanges(t, ch)))
	}
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra changes: %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTaskProvider_ConcurrentPushesAreNeverDropped(t *testing.T) {
	queue := newTaskChangesQueue()
	p := &taskProvider{queue: queue}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := p.Stream(ctx)
	require.True(t, isEmpty(receiveChanges(t, ch)))

	const writers, perWriter = 8, 100
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				queue.push(integration.ConfigChanges{Schedule: []integration.Config{{Name: fmt.Sprintf("w%d-%d", w, i)}}})
			}
		}(w)
	}

	seen := make(map[string]bool, writers*perWriter)
	for len(seen) < writers*perWriter {
		name := changesName(receiveChanges(t, ch))
		assert.False(t, seen[name], "duplicate delivery of %s", name)
		seen[name] = true
	}
	wg.Wait()
}

func TestTaskProvider_UndeliveredChangesSurviveStreamRestart(t *testing.T) {
	queue := newTaskChangesQueue()
	p := &taskProvider{queue: queue}

	ctx1, cancel1 := context.WithCancel(context.Background())
	ch1 := p.Stream(ctx1)
	require.True(t, isEmpty(receiveChanges(t, ch1)))
	queue.push(namedChanges(0))
	queue.push(namedChanges(1))
	cancel1()

	var delivered []string
	for changes := range ch1 {
		delivered = append(delivered, changesName(changes))
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	ch2 := p.Stream(ctx2)
	require.True(t, isEmpty(receiveChanges(t, ch2)))
	for len(delivered) < 2 {
		delivered = append(delivered, changesName(receiveChanges(t, ch2)))
	}

	assert.Equal(t, []string{"task-0", "task-1"}, delivered, "each change is delivered exactly once, in order")
}

// TestStream_TaskChangesFlowThroughTaskProvider checks the wiring end to end: the RC callback
// subscribed by the monitor provider hands task configs to the task provider only.
func TestStream_TaskChangesFlowThroughTaskProvider(t *testing.T) {
	c, rc := newStreamComponent(t, []integration.Config{mysqlBaseConfig()})
	c.now = func() time.Time { return testNow }
	c.eventPlatform = fakeEventPlatform{forwarder: &fakeForwarder{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitorCh := c.Stream(ctx)
	require.True(t, isEmpty(receiveChanges(t, monitorCh)))
	taskCh := (&taskProvider{queue: c.taskChanges}).Stream(ctx)
	require.True(t, isEmpty(receiveChanges(t, taskCh)))

	callback := waitSubscribe(t, rc)
	path, raw := taskUpdate(t, testTaskID, testMySQLHost)
	statuses := map[string]state.ApplyStatus{}
	callback(map[string]state.RawConfig{path: raw}, func(p string, s state.ApplyStatus) { statuses[p] = s })

	assert.Equal(t, state.ApplyStateAcknowledged, statuses[path].State)
	changes := receiveChanges(t, taskCh)
	require.Len(t, changes.Schedule, 1)
	assert.True(t, hasDOTask(t, changes.Schedule[0]))

	select {
	case monitorChanges := <-monitorCh:
		t.Fatalf("task config leaked into the monitor provider: %+v", monitorChanges)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBuildTaskCheckConfig_QueryRoundTripsThroughYAML(t *testing.T) {
	payload := buildTaskPayload(testTaskID, testMySQLHost, testNow.Unix()+600)
	payload.Task.Statements[0].Query = "SELECT 'a: b', \"q\"\n\t-- tab\n  FROM t -- trailing\n"
	cfg, err := buildTaskCheckConfig(taskConfigID(testTaskID), &payload, &integration.Config{Name: "mysql"}, map[string]any{"host": "h"})
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(cfg.Instances[0]), `query: "SELECT`), "queries are emitted double-quoted")
	statements := parseInstance(t, cfg)["do_task"].(map[string]any)["statements"].([]any)
	assert.Equal(t, payload.Task.Statements[0].Query, statements[0].(map[string]any)["query"])
}

func TestOnRCUpdate_TaskArrivingOrLeavingLeavesMonitorAlone(t *testing.T) {
	c, _ := newTaskTestComponent(t, []integration.Config{mysqlBaseConfig()})
	monitorPath, monitorRaw := monitorUpdate(t)
	taskPath, taskRaw := taskUpdate(t, testTaskID, testMySQLHost)

	_, applied := collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw})
	require.Len(t, applied.Schedule, 1)

	// RC delivers the unchanged monitor again with every snapshot: when the task arrives and when
	// it leaves. Neither snapshot may unschedule and reschedule the monitor check.
	statuses, arrived := collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw, taskPath: taskRaw})
	assert.True(t, arrived.IsEmpty(), "unchanged monitor must not be re-sent: %+v", arrived)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[monitorPath].State)

	_, left := collectStatuses(c, map[string]state.RawConfig{monitorPath: monitorRaw})
	assert.True(t, left.IsEmpty(), "unchanged monitor must not be re-sent: %+v", left)

	require.Contains(t, c.activeConfigs, testMonitorID)
	active := c.activeConfigs[testMonitorID].checkConfig
	assert.Equal(t, applied.Schedule[0].Digest(), active.Digest())
}

func TestMergeConfigChanges(t *testing.T) {
	cfg := func(host string) integration.Config {
		return integration.Config{Name: "mysql", Instances: []integration.Data{integration.Data("host: " + host + "\n")}}
	}
	a, b, c, d := cfg("a"), cfg("b"), cfg("c"), cfg("d")
	digests := func(configs []integration.Config) []string {
		out := make([]string, 0, len(configs))
		for _, config := range configs {
			out = append(out, config.Digest())
		}
		return out
	}

	merged := mergeConfigChanges(
		integration.ConfigChanges{Schedule: []integration.Config{a, b}, Unschedule: []integration.Config{c}},
		// b is replaced by the newer update, a is left untouched by it, and d is new.
		integration.ConfigChanges{Schedule: []integration.Config{d, a}, Unschedule: []integration.Config{b}},
	)

	assert.ElementsMatch(t, digests([]integration.Config{a, d}), digests(merged.Schedule), "a survives once, b is dropped")
	assert.ElementsMatch(t, digests([]integration.Config{c, b}), digests(merged.Unschedule))
}
