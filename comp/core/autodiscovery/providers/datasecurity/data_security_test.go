// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockedRcClient struct{}

func (m *mockedRcClient) SubscribeAgentTask() {}

func (m *mockedRcClient) Subscribe(data.Product, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

type mockedAutodiscovery struct {
	autodiscovery.Component
	configs []integration.Config
}

func (m *mockedAutodiscovery) GetUnresolvedConfigs() []integration.Config {
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
port: 5432
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
      port: 5432
      dbname: app
      username: datadog
      password: secret
`

func postgresIntegration() integration.Config {
	return integration.Config{
		Name:      postgresIntegrationName,
		Instances: []integration.Data{integration.Data(postgresInstanceConfig)},
	}
}

func TestControllerUpdate(t *testing.T) {
	tests := []struct {
		name            string
		adConfigs       []integration.Config
		wantState       state.ApplyState
		wantErrContains string
		wantInstance    string // expected emitted check instance (YAML); empty means nothing scheduled
	}{
		{
			name:         "postgres integration present schedules check",
			adConfigs:    []integration.Config{postgresIntegration()},
			wantState:    state.ApplyStateAcknowledged,
			wantInstance: wantCheckInstance,
		},
		{
			name:            "no postgres integration returns error",
			adConfigs:       []integration.Config{},
			wantState:       state.ApplyStateError,
			wantErrContains: "postgres integration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &controller{
				ac:            getMockedAutodiscovery(t, tt.adConfigs),
				rcclient:      &mockedRcClient{},
				configChanges: make(chan integration.ConfigChanges, 10),
			}

			updateStatus := map[string]state.ApplyStatus{}
			c.update(map[string]state.RawConfig{
				"config_1": {Config: []byte(scanTaskConfig), Metadata: state.Metadata{ID: "rc-id"}},
			}, func(path string, status state.ApplyStatus) {
				updateStatus[path] = status
			})

			assert.Equal(t, tt.wantState, updateStatus["config_1"].State)
			if tt.wantErrContains != "" {
				assert.Contains(t, updateStatus["config_1"].Error, tt.wantErrContains)
			}

			if tt.wantInstance == "" {
				return
			}

			cfg := <-c.Stream(context.Background())
			require.Len(t, cfg.Schedule, 1)
			assert.Empty(t, cfg.Unschedule)

			scheduled := cfg.Schedule[0]
			assert.Equal(t, dataSecurityCheckName, scheduled.Name)
			require.Len(t, scheduled.Instances, 1)
			assert.Equal(t, asYAML(t, tt.wantInstance), asYAML(t, string(scheduled.Instances[0])))
		})
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

func TestToInt(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected int
	}{
		{"int", 42, 42},
		{"int64", int64(42), 42},
		{"uint16", uint16(42), 42},
		{"float64", float64(42), 42},
		{"nil falls back to default", nil, 5432},
		{"string falls back to default", "42", 5432},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, toInt(tt.value, 5432))
		})
	}
}
