// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
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
	rc := &mockedRcClient{}
	provider := NewController(getMockedAutodiscovery(t, adConfigs), rc)
	return rc, provider
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

				// An update that schedules at least one config emits one ConfigChanges; an
				// error-only update emits none.
				want := tt.wantChanges[i]
				if len(want.Schedule) == 0 {
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
