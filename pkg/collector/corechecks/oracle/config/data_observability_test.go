// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package config

import (
	"fmt"
	"maps"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestDataObservabilityConfiguration(t *testing.T) {
	cfg, err := NewCheckConfig([]byte(`username: datadog
dbm: false
query_timeout: 20
collect_schemas:
  enabled: true
custom_queries:
  - query: SELECT 1 FROM dual
data_observability:
  enabled: true
  collection_interval: 2.5
  run_sync: true
  config_id: remote-config-id
  queries:
    - monitor_id: 42
      dbname: orcl.PDB1
      type: freshness
      query: "SELECT MAX(\"updated_at\")\nFROM \"APP\".\"orders\""
      interval_seconds: 60
      query_timeout: 1500
      entity:
        platform: oracle
        account: oracle-instance
        database: orcl.PDB1
        schema: APP
        table: orders
      custom_sql_select_fields:
        metric_config_id: 123
        entity_id: table-entity-id
    - monitor_id: 0
      dbname: orcl.PDB2
      query: SELECT COUNT(*) FROM "APP"."orders"
      schedule: '*/5 * * * *'
      interval_seconds: -1
      query_timeout: 30000
      entity: {}
`), nil)
	require.NoError(t, err)
	assert.False(t, cfg.DBM)
	assert.True(t, cfg.Schemas.Enabled)
	require.Len(t, cfg.InstanceConfig.CustomQueries, 1)
	assert.Equal(t, "SELECT 1 FROM dual", cfg.InstanceConfig.CustomQueries[0].Query)
	assert.Equal(t, 20*time.Second, cfg.QueryTimeoutDuration())

	do := cfg.DataObservability
	assert.True(t, do.Enabled)
	assert.Equal(t, 2.5, do.CollectionInterval)
	assert.True(t, do.RunSync)
	assert.Equal(t, "remote-config-id", do.ConfigID)
	require.Len(t, do.Queries, 2)
	q := do.Queries[0]
	assert.EqualValues(t, 42, q.MonitorID)
	assert.Equal(t, "orcl.PDB1", q.DBName)
	assert.Equal(t, "freshness", q.Type)
	assert.Equal(t, "SELECT MAX(\"updated_at\")\nFROM \"APP\".\"orders\"", q.Query)
	assert.EqualValues(t, 60, q.IntervalSeconds)
	assert.Equal(t, 1500*time.Millisecond, q.QueryTimeoutDuration())
	assert.Equal(t, &DataObservabilityEntity{
		Platform: "oracle", Account: "oracle-instance", Database: "orcl.PDB1", Schema: "APP", Table: "orders",
	}, q.Entity)
	require.NotNil(t, q.CustomSQLSelectFields)
	require.NotNil(t, q.CustomSQLSelectFields.MetricConfigID)
	require.NotNil(t, q.CustomSQLSelectFields.EntityID)
	assert.EqualValues(t, 123, *q.CustomSQLSelectFields.MetricConfigID)
	assert.Equal(t, "table-entity-id", *q.CustomSQLSelectFields.EntityID)
	assert.Zero(t, do.Queries[1].MonitorID)
	assert.Equal(t, "orcl.PDB2", do.Queries[1].DBName)
	assert.Equal(t, "*/5 * * * *", do.Queries[1].Schedule)
	assert.Nil(t, do.Queries[1].CustomSQLSelectFields)
}

func TestDataObservabilityDefaults(t *testing.T) {
	for _, instance := range []string{
		"",
		"data_observability: {}",
		"data_observability:\n  enabled: false",
		"data_observability:\n  enabled: true",
		"data_observability:\n  collection_interval: 0",
		"data_observability:\n  collection_interval: -1",
		"data_observability:\n  collection_interval: .nan",
		"data_observability:\n  collection_interval: .inf",
		"data_observability:\n  collection_interval: 1e20",
	} {
		t.Run(instance, func(t *testing.T) {
			cfg, err := NewCheckConfig([]byte("username: datadog\n"+instance), nil)
			require.NoError(t, err)
			assert.Equal(t, float64(10), cfg.DataObservability.CollectionInterval)
			assert.Empty(t, cfg.DataObservability.Queries)
			assert.False(t, cfg.DBM)
			assert.True(t, cfg.QuerySamples.Enabled)
			assert.True(t, cfg.SysMetrics.Enabled)
		})
	}
}

func TestInvalidDataObservabilityQueriesDoNotBlockValidQueries(t *testing.T) {
	validQuery := map[string]any{
		"monitor_id":       42,
		"dbname":           "orcl.PDB1",
		"query":            "SELECT COUNT(*) FROM APP.orders",
		"query_timeout":    30000,
		"interval_seconds": 60,
		"entity":           map[string]any{},
	}
	tests := []struct {
		field string
		value any
	}{
		{"dbname", nil},
		{"dbname", "  "},
		{"query", nil},
		{"query", "\n  "},
		{"query_timeout", nil},
		{"query_timeout", -1},
		{"query_timeout", int64(math.MaxInt64)},
		{"entity", nil},
		{"interval_seconds", nil},
		{"interval_seconds", -1},
		{"interval_seconds", int64(math.MaxInt64)},
		{"schedule", "invalid"},
		{"schedule", "61 * * * *"},
		{"schedule", "@hourly"},
		{"schedule", "* * * * * *"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s=%v", tt.field, tt.value), func(t *testing.T) {
			invalidQuery := maps.Clone(validQuery)
			invalidQuery[tt.field] = tt.value
			instance, err := yaml.Marshal(map[string]any{
				"username": "datadog",
				"data_observability": map[string]any{
					"enabled": true,
					"queries": []map[string]any{invalidQuery, validQuery},
				},
			})
			require.NoError(t, err)
			cfg, err := NewCheckConfig(instance, nil)
			require.NoError(t, err)
			require.Len(t, cfg.DataObservability.Queries, 1)
			assert.Equal(t, validQuery["query"], cfg.DataObservability.Queries[0].Query)
			assert.EqualValues(t, 60, cfg.DataObservability.Queries[0].IntervalSeconds)
			assert.Empty(t, cfg.DataObservability.Queries[0].Schedule)
			assert.True(t, cfg.SysMetrics.Enabled)
		})
	}
}
