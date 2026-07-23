// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestIsKSMCheck(t *testing.T) {
	manager := newKSMShardingManager(true)

	tests := []struct {
		name     string
		config   integration.Config
		expected bool
	}{
		{
			name: "kubernetes_state_core is KSM check",
			config: integration.Config{
				Name: "kubernetes_state_core",
			},
			expected: true,
		},
		{
			name: "kubernetes_state (Python) is not supported for sharding",
			config: integration.Config{
				Name: "kubernetes_state",
			},
			expected: false,
		},
		{
			name: "other check is not KSM check",
			config: integration.Config{
				Name: "prometheus",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isKSMCheck(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnalyzeKSMConfig(t *testing.T) {
	manager := newKSMShardingManager(true)

	tests := []struct {
		name           string
		config         integration.Config
		expectedGroups []resourceGroup
		expectError    bool
	}{
		{
			name:   "pods only",
			config: createKSMConfig([]string{"pods"}),
			expectedGroups: []resourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
			},
			expectError: false,
		},
		{
			name:   "nodes only",
			config: createKSMConfig([]string{"nodes"}),
			expectedGroups: []resourceGroup{
				{Name: "nodes", Collectors: []string{"nodes"}},
			},
			expectError: false,
		},
		{
			name:   "others only",
			config: createKSMConfig([]string{"deployments", "services"}),
			expectedGroups: []resourceGroup{
				{Name: "others", Collectors: []string{"deployments", "services"}},
			},
			expectError: false,
		},
		{
			name:   "all three groups",
			config: createKSMConfig([]string{"pods", "nodes", "deployments", "services", "configmaps"}),
			expectedGroups: []resourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
				{Name: "nodes", Collectors: []string{"nodes"}},
				{Name: "others", Collectors: []string{"deployments", "services", "configmaps"}},
			},
			expectError: false,
		},
		{
			name:   "mixed order",
			config: createKSMConfig([]string{"services", "pods", "deployments", "nodes"}),
			expectedGroups: []resourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
				{Name: "nodes", Collectors: []string{"nodes"}},
				{Name: "others", Collectors: []string{"services", "deployments"}},
			},
			expectError: false,
		},
		{
			name: "not a KSM check",
			config: integration.Config{
				Name: "prometheus",
			},
			expectedGroups: nil,
			expectError:    true,
		},
		{
			name: "cluster_check is false - analyzeKSMConfig doesn't validate ClusterCheck",
			config: integration.Config{
				Name:         "kubernetes_state_core",
				ClusterCheck: false,
				Instances:    []integration.Data{integration.Data("collectors: [pods, nodes]")},
			},
			expectedGroups: []resourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
				{Name: "nodes", Collectors: []string{"nodes"}},
			},
			expectError: false,
		},
		{
			name:           "multiple instances - returns error",
			config:         createKSMConfigWithMultipleInstances(),
			expectedGroups: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, err := manager.analyzeKSMConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, len(tt.expectedGroups), len(groups))

			for i, expectedGroup := range tt.expectedGroups {
				assert.Equal(t, expectedGroup.Name, groups[i].Name)
				assert.ElementsMatch(t, expectedGroup.Collectors, groups[i].Collectors)
			}
		})
	}
}

func TestAnalyzeKSMConfig_EmptyCollectors(t *testing.T) {
	manager := newKSMShardingManager(true)

	// When collectors is empty, should use options.DefaultResources which includes:
	// pods, nodes, deployments, services, configmaps, secrets, etc.
	// Should create 3 groups: pods, nodes, others
	config := createKSMConfig([]string{})

	groups, err := manager.analyzeKSMConfig(config)
	require.NoError(t, err)

	// Should have 3 groups: pods, nodes, others
	assert.Equal(t, 3, len(groups))

	assert.Equal(t, "pods", groups[0].Name)
	assert.Equal(t, []string{"pods"}, groups[0].Collectors)

	assert.Equal(t, "nodes", groups[1].Name)
	assert.Equal(t, []string{"nodes"}, groups[1].Collectors)

	assert.Equal(t, "others", groups[2].Name)
	// others should have multiple default collectors
	assert.Greater(t, len(groups[2].Collectors), 0, "others group should have collectors from DefaultResources")
}

func TestShouldShardKSMCheck(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		config   integration.Config
		expected bool
	}{
		{
			name:     "sharding disabled",
			enabled:  false,
			config:   createKSMConfig([]string{"pods", "nodes"}),
			expected: false,
		},
		{
			name:     "not a KSM check",
			enabled:  true,
			config:   integration.Config{Name: "prometheus"},
			expected: false,
		},
		{
			name:    "cluster_check is false",
			enabled: true,
			config: integration.Config{
				Name:         "kubernetes_state_core",
				ClusterCheck: false,
				Instances:    []integration.Data{integration.Data("collectors: [pods, nodes]")},
			},
			expected: false,
		},
		{
			name:     "empty collectors - uses defaults and shards",
			enabled:  true,
			config:   createKSMConfig([]string{}),
			expected: true, // Should shard when using default collectors (includes pods, nodes, others)
		},
		{
			name:     "single group - pods only",
			enabled:  true,
			config:   createKSMConfig([]string{"pods"}),
			expected: false,
		},
		{
			name:     "single group - nodes only",
			enabled:  true,
			config:   createKSMConfig([]string{"nodes"}),
			expected: false,
		},
		{
			name:     "single group - others only",
			enabled:  true,
			config:   createKSMConfig([]string{"deployments", "services"}),
			expected: false,
		},
		{
			name:     "two groups - pods and nodes",
			enabled:  true,
			config:   createKSMConfig([]string{"pods", "nodes"}),
			expected: true,
		},
		{
			name:     "two groups - pods and others",
			enabled:  true,
			config:   createKSMConfig([]string{"pods", "deployments"}),
			expected: true,
		},
		{
			name:     "three groups",
			enabled:  true,
			config:   createKSMConfig([]string{"pods", "nodes", "deployments"}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newKSMShardingManager(tt.enabled)
			result := manager.shouldShardKSMCheck(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateShardedKSMConfigs(t *testing.T) {
	manager := newKSMShardingManager(true)

	tests := []struct {
		name           string
		config         integration.Config
		expectedShards int
		expectError    bool
	}{
		{
			name:           "pods and nodes",
			config:         createKSMConfig([]string{"pods", "nodes"}),
			expectedShards: 2,
			expectError:    false,
		},
		{
			name:           "all three groups",
			config:         createKSMConfig([]string{"pods", "nodes", "deployments", "services"}),
			expectedShards: 3,
			expectError:    false,
		},
		{
			name:           "pods and others",
			config:         createKSMConfig([]string{"pods", "services", "deployments"}),
			expectedShards: 2,
			expectError:    false,
		},
		{
			name:           "empty collectors - uses defaults and creates 3 shards",
			config:         createKSMConfig([]string{}),
			expectedShards: 3, // Should create 3 shards: pods, nodes, others
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := manager.createShardedKSMConfigs(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedShards, len(configs))

			// Verify each config is valid and has correct structure
			for _, config := range configs {
				assert.Equal(t, "kubernetes_state_core", config.Name)
				assert.Equal(t, 1, len(config.Instances))

				// Parse instance to verify structure
				var instance map[string]interface{}
				err := yaml.Unmarshal(config.Instances[0], &instance)
				require.NoError(t, err)

				// Check collectors field exists
				collectors, ok := instance["collectors"]
				assert.True(t, ok)
				assert.NotEmpty(t, collectors)

				// Check skip_leader_election is set
				skipLeaderElection, ok := instance["skip_leader_election"]
				assert.True(t, ok)
				assert.True(t, skipLeaderElection.(bool))
			}
		})
	}
}

func TestCreateShardedKSMConfigs_PreservesTags(t *testing.T) {
	manager := newKSMShardingManager(true)

	// Create config with existing tags
	config := createKSMConfigWithTags([]string{"pods", "nodes"}, []string{"env:prod", "team:platform"})

	configs, err := manager.createShardedKSMConfigs(config)
	require.NoError(t, err)
	assert.Equal(t, 2, len(configs))

	// Verify each config preserves original tags
	// Note: We no longer add ksm_resource_group tag to avoid cluttering user metrics
	for _, shardConfig := range configs {
		var instance map[string]interface{}
		err := yaml.Unmarshal(shardConfig.Instances[0], &instance)
		require.NoError(t, err)

		tags, ok := instance["tags"].([]interface{})
		require.True(t, ok)

		tagStrings := make([]string, len(tags))
		for i, tag := range tags {
			tagStrings[i] = tag.(string)
		}

		// Should have original tags only (no ksm_resource_group)
		assert.Contains(t, tagStrings, "env:prod")
		assert.Contains(t, tagStrings, "team:platform")
		assert.Equal(t, 2, len(tagStrings), "Should only have original tags")
	}
}

func TestShouldShardKSMCheck_MultipleInstances(t *testing.T) {
	manager := newKSMShardingManager(true)
	config := createKSMConfigWithMultipleInstances()

	// Two shardable instances is still unsupported -> should not shard
	result := manager.shouldShardKSMCheck(config)
	assert.False(t, result, "Should not shard when multiple shardable instances configured")
}

func TestClassifyKSMInstances(t *testing.T) {
	t.Run("single shardable, no aggregate", func(t *testing.T) {
		shardable, passthrough, err := classifyKSMInstances(createKSMConfig([]string{"pods", "nodes"}))
		require.NoError(t, err)
		assert.NotNil(t, shardable)
		assert.Empty(t, passthrough)
	})

	t.Run("shardable + aggregate", func(t *testing.T) {
		shardable, passthrough, err := classifyKSMInstances(createKSMConfigWithAggregate([]string{"pods", "nodes", "deployments"}))
		require.NoError(t, err)
		assert.NotNil(t, shardable)
		require.Len(t, passthrough, 1)
	})

	t.Run("aggregate only -> no shardable instance", func(t *testing.T) {
		cfg := integration.Config{
			Name:         "kubernetes_state_core",
			ClusterCheck: true,
			Instances:    []integration.Data{mustYAML(map[string]interface{}{"pod_collection_mode": "cluster_aggregates_only"})},
		}
		_, _, err := classifyKSMInstances(cfg)
		assert.Error(t, err)
	})

	t.Run("two shardable -> error", func(t *testing.T) {
		_, _, err := classifyKSMInstances(createKSMConfigWithMultipleInstances())
		assert.Error(t, err)
	})
}

func TestShouldShardKSMCheck_WithAggregate(t *testing.T) {
	manager := newKSMShardingManager(true)
	cfg := createKSMConfigWithAggregate([]string{"pods", "nodes", "deployments"})
	assert.True(t, manager.shouldShardKSMCheck(cfg), "combined instance + cluster_aggregates_only should still shard")
}

// TestCreateShardedKSMConfigs_WithAggregate: a combined cluster_unassigned instance
// plus a cluster_aggregates_only instance shards into pods/nodes/others (3 configs),
// with the aggregate instance riding on the pods shard (not a separate 4th config).
func TestCreateShardedKSMConfigs_WithAggregate(t *testing.T) {
	manager := newKSMShardingManager(true)
	cfg := createKSMConfigWithAggregate([]string{"pods", "nodes", "deployments", "services"})

	configs, err := manager.createShardedKSMConfigs(cfg)
	require.NoError(t, err)
	require.Len(t, configs, 3, "pods/nodes/others; aggregate rides the pods shard")

	aggregateCount := 0
	var podsShard *integration.Config
	for i := range configs {
		assert.Equal(t, "kubernetes_state_core", configs[i].Name)
		for _, raw := range configs[i].Instances {
			var inst map[string]interface{}
			require.NoError(t, yaml.Unmarshal(raw, &inst))
			if inst["pod_collection_mode"] == "cluster_aggregates_only" {
				aggregateCount++
				assert.Equal(t, true, inst["skip_leader_election"], "aggregate must skip leader election on a runner")
			}
			if inst["pod_collection_mode"] == "cluster_unassigned" {
				// Every cluster_unassigned shard must keep suppressing its own
				// .total, else it collides with the aggregate instance.
				assert.Equal(t, "cluster_unassigned", inst["pod_collection_mode"], "shards must preserve pod_collection_mode")
				assert.Equal(t, true, inst["cluster_aggregates_enabled"], "shards must preserve cluster_aggregates_enabled")
			}
			if cols, ok := inst["collectors"].([]interface{}); ok && len(cols) == 1 && cols[0] == "pods" {
				podsShard = &configs[i]
			}
		}
	}

	assert.Equal(t, 1, aggregateCount, "aggregate instance must appear exactly once")
	require.NotNil(t, podsShard, "a pods shard must exist")
	assert.Len(t, podsShard.Instances, 2, "pods shard carries cluster_unassigned pods + cluster_aggregates_only")
}

// TestCreateShardedKSMConfigs_AggregateFallbackNoPods: if the shardable instance has
// no pods collector, the aggregate is dispatched as its own config (not dropped).
func TestCreateShardedKSMConfigs_AggregateFallbackNoPods(t *testing.T) {
	manager := newKSMShardingManager(true)
	configs, err := manager.createShardedKSMConfigs(createKSMConfigWithAggregate([]string{"nodes", "deployments"}))
	require.NoError(t, err)
	require.Len(t, configs, 3, "nodes shard + others shard + standalone aggregate")

	standaloneAggregate := false
	for _, c := range configs {
		if len(c.Instances) == 1 {
			var inst map[string]interface{}
			_ = yaml.Unmarshal(c.Instances[0], &inst)
			if inst["pod_collection_mode"] == "cluster_aggregates_only" {
				standaloneAggregate = true
			}
		}
	}
	assert.True(t, standaloneAggregate, "aggregate dispatched standalone when there is no pods shard")
}

// TestCreateShardedKSMConfigs_AggregateFirstOrdering guards order-independence:
// the shardable instance must be used as the shard base even when the
// cluster_aggregates_only instance is listed first. If the sharder regressed to
// baseConfig.Instances[0], it would shard the aggregate (which has no collectors)
// and this would fail.
func TestCreateShardedKSMConfigs_AggregateFirstOrdering(t *testing.T) {
	manager := newKSMShardingManager(true)
	aggregate := map[string]interface{}{"pod_collection_mode": "cluster_aggregates_only"}
	shardable := map[string]interface{}{
		"collectors":                 []string{"pods", "nodes", "deployments"},
		"pod_collection_mode":        "cluster_unassigned",
		"cluster_aggregates_enabled": true,
	}
	cfg := integration.Config{
		Name:         "kubernetes_state_core",
		ClusterCheck: true,
		Instances:    []integration.Data{mustYAML(aggregate), mustYAML(shardable)}, // aggregate FIRST
	}

	configs, err := manager.createShardedKSMConfigs(cfg)
	require.NoError(t, err)
	require.Len(t, configs, 3, "must shard the cluster_unassigned instance, not the aggregate")

	// Every shard's shardable instance must carry cluster_unassigned collectors,
	// proving Instances[0] (the aggregate) was not used as the shard base.
	sawUnassigned := false
	for _, c := range configs {
		for _, raw := range c.Instances {
			var inst map[string]interface{}
			require.NoError(t, yaml.Unmarshal(raw, &inst))
			if inst["pod_collection_mode"] == "cluster_unassigned" {
				sawUnassigned = true
				assert.Contains(t, inst, "collectors")
			}
		}
	}
	assert.True(t, sawUnassigned, "shards must be built from the cluster_unassigned instance")
}

// Helper functions

func createKSMConfig(collectors []string) integration.Config {
	return createKSMConfigWithTags(collectors, nil)
}

func createKSMConfigWithTags(collectors []string, tags []string) integration.Config {
	instance := map[string]interface{}{
		"collectors": collectors,
	}
	if tags != nil {
		instance["tags"] = tags
	}

	data, _ := yaml.Marshal(instance)

	return integration.Config{
		Name:         "kubernetes_state_core",
		Instances:    []integration.Data{integration.Data(data)},
		ClusterCheck: true,
	}
}

func mustYAML(m map[string]interface{}) integration.Data {
	data, _ := yaml.Marshal(m)
	return integration.Data(data)
}

// createKSMConfigWithAggregate builds a single config with a shardable
// cluster_unassigned instance (given collectors) plus a cluster_aggregates_only
// instance — the "one config, both features" shape.
func createKSMConfigWithAggregate(shardableCollectors []string) integration.Config {
	shardable := map[string]interface{}{
		"collectors":                 shardableCollectors,
		"pod_collection_mode":        "cluster_unassigned",
		"cluster_aggregates_enabled": true,
	}
	aggregate := map[string]interface{}{
		"pod_collection_mode": "cluster_aggregates_only",
	}
	return integration.Config{
		Name:         "kubernetes_state_core",
		ClusterCheck: true,
		Instances:    []integration.Data{mustYAML(shardable), mustYAML(aggregate)},
	}
}

func createKSMConfigWithMultipleInstances() integration.Config {
	// Create first instance with pods and nodes
	instance1 := map[string]interface{}{
		"collectors": []string{"pods", "nodes"},
	}
	data1, _ := yaml.Marshal(instance1)

	// Create second instance with different collectors
	instance2 := map[string]interface{}{
		"collectors": []string{"deployments", "services"},
	}
	data2, _ := yaml.Marshal(instance2)

	return integration.Config{
		Name:         "kubernetes_state_core",
		Instances:    []integration.Data{integration.Data(data1), integration.Data(data2)},
		ClusterCheck: true,
	}
}
