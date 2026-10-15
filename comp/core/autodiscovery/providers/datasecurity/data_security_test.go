// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockedRcClient captures the product and callback the controller registers so the test can
// drive updates through the same callback the controller subscribes with, rather than calling
// the unexported update method directly.
type mockedRcClient struct {
	product  data.Product
	callback func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

func (m *mockedRcClient) SubscribeAgentTask() {}

func (m *mockedRcClient) Subscribe(product data.Product, callback func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	m.product = product
	// callback is the controller's update method, which RC would normally invoke on each config change.
	m.callback = callback
}

type mockedAutodiscovery struct {
	autodiscovery.Component
	configs []integration.Config
}

func (m *mockedAutodiscovery) GetUnresolvedConfigs() []integration.Config {
	return m.configs
}

func (m *mockedAutodiscovery) GetAllConfigs() []integration.Config {
	return m.configs
}

func getMockedAutodiscovery(t *testing.T, configs []integration.Config) autodiscovery.Component {
	return &mockedAutodiscovery{
		Component: fxutil.Test[autodiscovery.Component](t, noopautoconfig.Module()),
		configs:   configs,
	}
}

// postgresInstanceConfig is a postgres integration instance the provider resolves
// the scan connection from.
const postgresInstanceConfig = `
host: db-host
port: 5678
username: datadog
password: secret
ssl: verify-full
ssl_root_cert: /etc/ssl/root.crt
`

// scanTaskConfig is a valid Data Security scan task RC payload (JSON, which is
// also valid YAML). The scanning rule carries a license so we can assert it is
// forwarded downstream.
const scanTaskConfig = `{
  "task_id": "task-1",
  "scanning_rules": [
    {"id": "rule-1", "license": "proprietary", "pattern": "\\d+"}
  ],
  "scan_data": [
    {
      "sub_task_id": "sub-1",
      "query": "SELECT * FROM users",
      "timeout_seconds": 30,
      "entity": {
        "platform": "postgres",
        "database_cluster_name": "cluster",
        "database_instance_name": "instance",
        "database_host_name": "db-host",
        "database": "app",
        "schema": "public",
        "table": "users"
      }
    }
  ]
}`

// scanTaskUnknownHostConfig is a valid scan task whose entity host does not match any configured
// postgres instance, so the connection cannot be resolved.
const scanTaskUnknownHostConfig = `{
  "task_id": "task-1",
  "scanning_rules": [
    {"id": "rule-1", "license": "proprietary", "pattern": "\\d+"}
  ],
  "scan_data": [
    {
      "sub_task_id": "sub-1",
      "query": "SELECT * FROM users",
      "timeout_seconds": 30,
      "entity": {
        "platform": "postgres",
        "database_cluster_name": "cluster",
        "database_instance_name": "instance",
        "database_host_name": "unknown-host",
        "database": "app",
        "schema": "public",
        "table": "users"
      }
    }
  ]
}`

// wantCheckInstance is the check instance the provider is expected to emit for
// scanTaskConfig once the local postgres connection has been resolved. The
// scanning rule (with its license) is forwarded verbatim.
const wantCheckInstance = `
min_collection_interval: 0
task_id: task-1
scanning_rules:
  - id: rule-1
    license: proprietary
    pattern: '\d+'
scan_data:
  - sub_task_id: sub-1
    query: SELECT * FROM users
    timeout_seconds: 30
    entity:
      platform: postgres
      database_cluster_name: cluster
      database_instance_name: instance
      database_host_name: db-host
      database: app
      schema: public
      table: users
    connection:
      host: db-host
      port: 5678
      dbname: app
      username: datadog
      password: secret
      ssl: verify-full
      ssl_root_cert: /etc/ssl/root.crt
`

// rawScanTask builds the RC payload for a scan task delivered at the given path/id.
func rawScanTask(id, scanTask string) state.RawConfig {
	return state.RawConfig{Config: []byte(scanTask), Metadata: state.Metadata{ID: id}}
}

func postgresIntegration() integration.Config {
	return integration.Config{
		Name:      postgresIntegrationName,
		Instances: []integration.Data{integration.Data(postgresInstanceConfig)},
	}
}

// newTestController builds a controller with the given autodiscovery configs and returns the mock
// RC client it subscribes through along with the provider.
func newTestController(t *testing.T, adConfigs []integration.Config) (*mockedRcClient, types.ConfigProvider) {
	rc, provider, _ := newTestControllerWithForwarder(t, adConfigs)
	return rc, provider
}

func TestPostgresSSLMode(t *testing.T) {
	tests := []struct {
		name       string
		instance   map[string]any
		enforceSSL bool
		want       string
		wantErr    bool
	}{
		{name: "not enforcing keeps unset", instance: map[string]any{"host": "db"}, want: ""},
		{name: "not enforcing keeps allow", instance: map[string]any{"host": "db", "ssl": "allow"}, want: "allow"},
		{name: "unset is elevated", instance: map[string]any{"host": "db"}, enforceSSL: true, want: "require"},
		{name: "empty is elevated", instance: map[string]any{"host": "db", "ssl": ""}, enforceSSL: true, want: "require"},
		{name: "disable is elevated", instance: map[string]any{"host": "db", "ssl": "disable"}, enforceSSL: true, want: "require"},
		{name: "allow is elevated", instance: map[string]any{"host": "db", "ssl": "allow"}, enforceSSL: true, want: "require"},
		{name: "prefer is elevated", instance: map[string]any{"host": "db", "ssl": "prefer"}, enforceSSL: true, want: "require"},
		{name: "invalid value is elevated", instance: map[string]any{"host": "db", "ssl": "bogus"}, enforceSSL: true, want: "require"},
		{name: "non-string value is elevated", instance: map[string]any{"host": "db", "ssl": true}, enforceSSL: true, want: "require"},
		{name: "require is kept", instance: map[string]any{"host": "db", "ssl": "require"}, enforceSSL: true, want: "require"},
		{name: "verify-ca is kept", instance: map[string]any{"host": "db", "ssl": "verify-ca"}, enforceSSL: true, want: "verify-ca"},
		{name: "verify-full is kept", instance: map[string]any{"host": "db", "ssl": "verify-full"}, enforceSSL: true, want: "verify-full"},
		{name: "not enforcing accepts unix socket", instance: map[string]any{"host": "/var/run/postgresql", "ssl": "allow"}, want: "allow"},
		{name: "enforcing rejects unix socket", instance: map[string]any{"host": "/var/run/postgresql", "ssl": "require"}, enforceSSL: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := instanceString(tt.instance, "host")
			got, err := postgresSSLMode(tt.instance, host, tt.enforceSSL)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// newTestControllerWithForwarder is newTestController that also returns the event platform
// forwarder recording the sds-results the controller sends.
func newTestControllerWithForwarder(t *testing.T, adConfigs []integration.Config) (*mockedRcClient, types.ConfigProvider, *recordingForwarder) {
	rc := &mockedRcClient{}
	fwd := &recordingForwarder{}
	provider := NewController(getMockedAutodiscovery(t, adConfigs), rc, &fakeEventPlatform{forwarder: fwd}, false)
	return rc, provider, fwd
}

// fakeEventPlatform is an eventplatform.Component exposing a recording forwarder.
type fakeEventPlatform struct {
	forwarder eventplatform.Forwarder
}

func (f *fakeEventPlatform) Get() (eventplatform.Forwarder, bool) {
	return f.forwarder, f.forwarder != nil
}

type sentEvent struct {
	eventType string
	raw       []byte
}

// recordingForwarder records the events sent to the event platform.
type recordingForwarder struct {
	mu     sync.Mutex
	events []sentEvent
}

func (f *recordingForwarder) SendEventPlatformEvent(e *message.Message, eventType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, sentEvent{eventType: eventType, raw: e.GetContent()})
	return nil
}

func (f *recordingForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
	return f.SendEventPlatformEvent(e, eventType)
}

func (f *recordingForwarder) Purge() map[string][]*message.Message { return nil }

// sdsResults decodes the sds-results sent so far, keyed by sub task ID.
func (f *recordingForwarder) sdsResults(t *testing.T) map[string]*sds.SdsResultPayload {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]*sds.SdsResultPayload{}
	for _, ev := range f.events {
		require.Equal(t, eventplatform.EventTypeSDSResult, ev.eventType)
		var p sds.SdsResultPayload
		require.NoError(t, proto.Unmarshal(ev.raw, &p))
		meta := p.GetScanResults()[0].GetScanMetadata().GetScanTaskMetadata()
		out[meta.GetSubTaskId()] = &p
	}
	return out
}

// TestControllerDoesNotSubscribeWithoutPostgres asserts the controller does not subscribe to RC
// until a postgres integration is configured.
func TestControllerDoesNotSubscribeWithoutPostgres(t *testing.T) {
	rc, _ := newTestController(t, nil)

	assert.Nil(t, rc.callback, "controller should not subscribe without a postgres integration")
}

func TestControllerUpdate(t *testing.T) {
	// wantScheduledConfig is the datasecurity check config the provider is expected to schedule
	// for scanTaskConfig.
	wantScheduledConfig := integration.Config{
		Name:      dataSecurityCheckName,
		Instances: []integration.Data{integration.Data(wantCheckInstance)},
	}
	wantScheduledConfigTask2 := integration.Config{
		Name:      dataSecurityCheckName,
		Instances: []integration.Data{integration.Data(strings.Replace(wantCheckInstance, "task-1", "task-2", 1))},
	}
	tests := []struct {
		name          string
		remoteConfigs []map[string]state.RawConfig // successive RC updates, each keyed by path
		wantStatus    map[string]state.ApplyStatus // expected apply status per path
		wantChanges   []integration.ConfigChanges  // expected ConfigChanges, one per RC update
	}{
		{
			name:          "matching scan task schedules check",
			remoteConfigs: []map[string]state.RawConfig{{"rc-1": rawScanTask("rc-1", scanTaskConfig)}},
			wantStatus:    map[string]state.ApplyStatus{"rc-1": {State: state.ApplyStateAcknowledged}},
			wantChanges:   []integration.ConfigChanges{{Schedule: []integration.Config{wantScheduledConfig}}},
		},
		{
			name:          "scan task with unknown host returns error",
			remoteConfigs: []map[string]state.RawConfig{{"rc-1": rawScanTask("rc-1", scanTaskUnknownHostConfig)}},
			wantStatus: map[string]state.ApplyStatus{"rc-1": {
				State: state.ApplyStateError,
				Error: `failed to build sub task "sub-1": postgres integration with host="unknown-host" not found`,
			}},
			wantChanges: []integration.ConfigChanges{{}},
		},
		{
			name:          "re-delivery at the same RC path unschedules the previous check",
			remoteConfigs: []map[string]state.RawConfig{{"rc-1": rawScanTask("rc-1", scanTaskConfig)}, {"rc-1": rawScanTask("rc-1", scanTaskConfig)}},
			wantStatus:    map[string]state.ApplyStatus{"rc-1": {State: state.ApplyStateAcknowledged}},
			wantChanges: []integration.ConfigChanges{
				{Schedule: []integration.Config{wantScheduledConfig}},
				{Schedule: []integration.Config{wantScheduledConfig}, Unschedule: []integration.Config{wantScheduledConfig}},
			},
		},
		{
			name: "modified task at the same RC path unschedules the previous check",
			remoteConfigs: []map[string]state.RawConfig{
				{"rc-1": rawScanTask("rc-1", scanTaskConfig)},
				{"rc-1": rawScanTask("rc-1", strings.Replace(scanTaskConfig, "task-1", "task-2", 1))},
			},
			wantStatus: map[string]state.ApplyStatus{"rc-1": {State: state.ApplyStateAcknowledged}},
			wantChanges: []integration.ConfigChanges{
				{Schedule: []integration.Config{wantScheduledConfig}},
				{Schedule: []integration.Config{wantScheduledConfigTask2}, Unschedule: []integration.Config{wantScheduledConfig}},
			},
		},
		{
			name: "deleted RC path unschedules its check",
			remoteConfigs: []map[string]state.RawConfig{
				{"rc-1": rawScanTask("rc-1", scanTaskConfig), "rc-2": rawScanTask("rc-2", scanTaskConfig)},
				{"rc-2": rawScanTask("rc-2", scanTaskConfig)},
			},
			wantStatus: map[string]state.ApplyStatus{
				"rc-1": {State: state.ApplyStateAcknowledged},
				"rc-2": {State: state.ApplyStateAcknowledged},
			},
			wantChanges: []integration.ConfigChanges{
				{Schedule: []integration.Config{wantScheduledConfig, wantScheduledConfig}},
				// rc-1 is unscheduled as deleted; rc-2 is re-delivered so it is rescheduled.
				{Schedule: []integration.Config{wantScheduledConfig}, Unschedule: []integration.Config{wantScheduledConfig, wantScheduledConfig}},
			},
		},
		{
			name:          "empty update after the last task is deleted unschedules its check",
			remoteConfigs: []map[string]state.RawConfig{{"rc-1": rawScanTask("rc-1", scanTaskConfig)}, {}},
			wantStatus:    map[string]state.ApplyStatus{"rc-1": {State: state.ApplyStateAcknowledged}},
			wantChanges: []integration.ConfigChanges{
				{Schedule: []integration.Config{wantScheduledConfig}},
				{Unschedule: []integration.Config{wantScheduledConfig}},
			},
		},
		{
			name:          "distinct RC paths in one update schedule independently",
			remoteConfigs: []map[string]state.RawConfig{{"rc-1": rawScanTask("rc-1", scanTaskConfig), "rc-2": rawScanTask("rc-2", scanTaskConfig)}},
			wantStatus: map[string]state.ApplyStatus{
				"rc-1": {State: state.ApplyStateAcknowledged},
				"rc-2": {State: state.ApplyStateAcknowledged},
			},
			wantChanges: []integration.ConfigChanges{{Schedule: []integration.Config{wantScheduledConfig, wantScheduledConfig}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A postgres integration is present, so the controller subscribes immediately and we
			// can drive the update through the callback it registered.
			rc, provider := newTestController(t, []integration.Config{postgresIntegration()})

			// rc.callback is equal to update method the controller subscribes with
			require.NotNil(t, rc.callback, "controller should subscribe when a postgres integration is present")

			// check if the controller subscribes to the correct product
			assert.Equal(t, data.ProductDataSecurityDBScanTasks, rc.product)

			streaming, ok := provider.(types.StreamingConfigProvider)
			require.True(t, ok, "controller should be a streaming config provider")
			changes := streaming.Stream(context.Background())

			// drain the initial empty snapshot pushed on creation
			receiveChanges(t, changes)

			require.Len(t, tt.wantChanges, len(tt.remoteConfigs), "each RC update needs an expected ConfigChanges")

			updateStatus := map[string]state.ApplyStatus{}
			for i, remoteConfig := range tt.remoteConfigs {
				// simulate an RC update carrying the full set of active scan tasks
				rc.callback(remoteConfig, func(path string, status state.ApplyStatus) {
					updateStatus[path] = status
				})

				// An update that schedules or unschedules at least one config emits one
				// ConfigChanges; an error-only or unchanged update emits none.
				want := tt.wantChanges[i]
				if len(want.Schedule) == 0 && len(want.Unschedule) == 0 {
					continue
				}
				assertConfigChangesEqual(t, want, receiveChanges(t, changes))
			}

			// Every path should reach the expected apply status.
			assert.Equal(t, tt.wantStatus, updateStatus)
		})
	}
}

// assertConfigChangesEqual asserts got matches want, comparing scheduled and unscheduled configs
// by name and instance. Instances are compared as YAML so the emitted JSON matches regardless of
// key order or formatting.
func assertConfigChangesEqual(t *testing.T, want, got integration.ConfigChanges) {
	t.Helper()
	require.Len(t, got.Schedule, len(want.Schedule))
	require.Len(t, got.Unschedule, len(want.Unschedule))
	for i := range want.Schedule {
		assertConfigEqual(t, want.Schedule[i], got.Schedule[i])
	}
	for i := range want.Unschedule {
		assertConfigEqual(t, want.Unschedule[i], got.Unschedule[i])
	}
}

// assertConfigEqual asserts two configs share the same name and YAML-equivalent instances.
func assertConfigEqual(t *testing.T, want, got integration.Config) {
	t.Helper()
	assert.Equal(t, want.Name, got.Name)
	require.Len(t, got.Instances, len(want.Instances))
	for i := range want.Instances {
		assert.Equal(t, asYAML(t, string(want.Instances[i])), asYAML(t, string(got.Instances[i])))
	}
}

// receiveChanges reads one ConfigChanges from the stream, failing the test instead of blocking
// indefinitely if nothing is emitted.
func receiveChanges(t *testing.T, changes <-chan integration.ConfigChanges) integration.ConfigChanges {
	t.Helper()
	select {
	case cfg := <-changes:
		return cfg
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for config changes")
		return integration.ConfigChanges{}
	}
}

// asYAML decodes a JSON/YAML document into a generic map so two configs can be
// compared regardless of key order or formatting.
func asYAML(t *testing.T, doc string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(doc), &out))
	return out
}

// scanTaskTwoSubTasksConfig has a sub task targeting a known host and one targeting an unknown host.
const scanTaskTwoSubTasksConfig = `{
  "task_id": "task-2",
  "scanning_rules": [
    {"id": "rule-1", "license": "proprietary", "pattern": "\\d+"},
    {"id": "rule-2", "license": "proprietary", "pattern": "[a-z]+"}
  ],
  "scan_data": [
    {
      "sub_task_id": "sub-ok",
      "query": "SELECT * FROM users",
      "timeout_seconds": 30,
      "entity": {"platform": "postgres", "database_cluster_name": "cluster", "database_instance_name": "instance", "database_host_name": "db-host", "database": "app", "schema": "public", "table": "users"}
    },
    {
      "sub_task_id": "sub-bad-host",
      "query": "SELECT * FROM orders",
      "timeout_seconds": 30,
      "entity": {"platform": "postgres", "database_cluster_name": "cluster", "database_instance_name": "instance", "database_host_name": "unknown-host", "database": "app", "schema": "public", "table": "orders"}
    },
    {
      "sub_task_id": "sub-bad-platform",
      "query": "SELECT 1",
      "timeout_seconds": 30,
      "entity": {"platform": "mysql", "database_instance_name": "instance", "database_host_name": "db-host", "database": "app", "schema": "public", "table": "t"}
    }
  ]
}`

// TestControllerReportsFailedSubTasks asserts a scan task that cannot be scheduled yields one ERROR
// sds-result per sub task.
func TestControllerReportsFailedSubTasks(t *testing.T) {
	rc, _, fwd := newTestControllerWithForwarder(t, []integration.Config{postgresIntegration()})
	require.NotNil(t, rc.callback)

	statuses := map[string]state.ApplyStatus{}
	rc.callback(map[string]state.RawConfig{"rc-1": rawScanTask("rc-1", scanTaskTwoSubTasksConfig)}, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})
	assert.Equal(t, state.ApplyStateError, statuses["rc-1"].State)

	results := fwd.sdsResults(t)
	require.Len(t, results, 3)

	// The sub task that failed to build carries its own reason; the others report the abort.
	reasons := map[string]string{}
	for id, p := range results {
		meta := p.GetScanResults()[0].GetScanMetadata().GetScanTaskMetadata()
		assert.Equal(t, "task-2", meta.GetTaskId())
		assert.Equal(t, sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata_ERROR, meta.GetStatus())
		assert.Equal(t, []string{"rule-1", "rule-2"}, p.GetRuleIds())
		assert.Equal(t, "postgres_table", p.GetResource().GetType())
		assert.NotZero(t, p.GetTimestamp())
		assert.NotNil(t, p.GetScanningSource().GetAgent())
		reasons[id] = meta.GetFailureReason()
	}
	assert.Equal(t, `sub task not run: scan task aborted: failed to build sub task "sub-bad-host": postgres integration with host="unknown-host" not found`, reasons["sub-ok"])
	assert.Equal(t, `postgres integration with host="unknown-host" not found`, reasons["sub-bad-host"])
	assert.Equal(t, `sub task not run: scan task aborted: failed to build sub task "sub-bad-host": postgres integration with host="unknown-host" not found`, reasons["sub-bad-platform"])

	table := results["sub-bad-host"].GetScanResults()[0].GetLocation().GetPostgresTable()
	assert.Equal(t, "unknown-host", table.GetDatabaseHostName())
	assert.Equal(t, "orders", table.GetTableName())
	assert.Equal(t, "instance.app.public.orders", results["sub-bad-host"].GetResource().GetName())
}

// TestControllerReportsNothingOnSuccess asserts no sds-result is sent for a task that gets scheduled
// (the check reports its own results).
func TestControllerReportsNothingOnSuccess(t *testing.T) {
	rc, _, fwd := newTestControllerWithForwarder(t, []integration.Config{postgresIntegration()})
	rc.callback(map[string]state.RawConfig{"rc-1": rawScanTask("rc-1", scanTaskConfig)}, func(string, state.ApplyStatus) {})
	assert.Empty(t, fwd.sdsResults(t))
}

// TestControllerReportsPartiallyDecodedTask asserts the sub tasks readable from a payload that fails
// to decode are still reported.
func TestControllerReportsPartiallyDecodedTask(t *testing.T) {
	rc, _, fwd := newTestControllerWithForwarder(t, []integration.Config{postgresIntegration()})

	// timeout_seconds has the wrong type: decoding fails but sub_task_id is still read.
	badType := `{"task_id": "task-3", "scan_data": [{"sub_task_id": "sub-1", "timeout_seconds": "30"}]}`
	statuses := map[string]state.ApplyStatus{}
	rc.callback(map[string]state.RawConfig{"rc-1": rawScanTask("rc-1", badType)}, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})
	assert.Equal(t, state.ApplyStateError, statuses["rc-1"].State)

	results := fwd.sdsResults(t)
	require.Len(t, results, 1)
	meta := results["sub-1"].GetScanResults()[0].GetScanMetadata().GetScanTaskMetadata()
	assert.Equal(t, "task-3", meta.GetTaskId())
	assert.Contains(t, meta.GetFailureReason(), "failed to decode scan task")

	// A syntactically invalid payload has no readable sub task: nothing to report.
	rc.callback(map[string]state.RawConfig{"rc-2": rawScanTask("rc-2", `{not json`)}, func(string, state.ApplyStatus) {})
	assert.Len(t, fwd.sdsResults(t), 1)
}
