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
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestIsKSMCheck(t *testing.T) {
	manager := NewKSMShardingManager(true)

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
			name: "kubernetes_state is KSM check",
			config: integration.Config{
				Name: "kubernetes_state",
			},
			expected: true,
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
			result := manager.IsKSMCheck(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnalyzeKSMConfig(t *testing.T) {
	manager := NewKSMShardingManager(true)

	tests := []struct {
		name           string
		config         integration.Config
		expectedGroups []ResourceGroup
		expectError    bool
	}{
		{
			name:   "pods only",
			config: createKSMConfig([]string{"pods"}),
			expectedGroups: []ResourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
			},
			expectError: false,
		},
		{
			name:   "nodes only",
			config: createKSMConfig([]string{"nodes"}),
			expectedGroups: []ResourceGroup{
				{Name: "nodes", Collectors: []string{"nodes"}},
			},
			expectError: false,
		},
		{
			name:   "others only",
			config: createKSMConfig([]string{"deployments", "services"}),
			expectedGroups: []ResourceGroup{
				{Name: "others", Collectors: []string{"deployments", "services"}},
			},
			expectError: false,
		},
		{
			name:   "all three groups",
			config: createKSMConfig([]string{"pods", "nodes", "deployments", "services", "configmaps"}),
			expectedGroups: []ResourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
				{Name: "nodes", Collectors: []string{"nodes"}},
				{Name: "others", Collectors: []string{"deployments", "services", "configmaps"}},
			},
			expectError: false,
		},
		{
			name:   "mixed order",
			config: createKSMConfig([]string{"services", "pods", "deployments", "nodes"}),
			expectedGroups: []ResourceGroup{
				{Name: "pods", Collectors: []string{"pods"}},
				{Name: "nodes", Collectors: []string{"nodes"}},
				{Name: "others", Collectors: []string{"services", "deployments"}},
			},
			expectError: false,
		},
		{
			name:           "empty collectors list",
			config:         createKSMConfig([]string{}),
			expectedGroups: nil,
			expectError:    true,
		},
		{
			name: "not a KSM check",
			config: integration.Config{
				Name: "prometheus",
			},
			expectedGroups: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, err := manager.AnalyzeKSMConfig(tt.config)

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
			name:     "empty collectors",
			enabled:  true,
			config:   createKSMConfig([]string{}),
			expected: false,
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
			manager := NewKSMShardingManager(tt.enabled)
			result := manager.ShouldShardKSMCheck(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateShardedKSMConfigs(t *testing.T) {
	manager := NewKSMShardingManager(true)

	tests := []struct {
		name           string
		config         integration.Config
		numRunners     int
		expectedShards int
		expectError    bool
	}{
		{
			name:           "pods and nodes with 3 runners",
			config:         createKSMConfig([]string{"pods", "nodes"}),
			numRunners:     3,
			expectedShards: 2,
			expectError:    false,
		},
		{
			name:           "all three groups with 3 runners",
			config:         createKSMConfig([]string{"pods", "nodes", "deployments", "services"}),
			numRunners:     3,
			expectedShards: 3,
			expectError:    false,
		},
		{
			name:           "all three groups with 2 runners - adaptive sharding",
			config:         createKSMConfig([]string{"pods", "nodes", "deployments", "services"}),
			numRunners:     2,
			expectedShards: 2, // pods separate, nodes+others combined
			expectError:    false,
		},
		{
			name:           "pods and others with 3 runners",
			config:         createKSMConfig([]string{"pods", "services", "deployments"}),
			numRunners:     3,
			expectedShards: 2,
			expectError:    false,
		},
		{
			name:           "empty collectors",
			config:         createKSMConfig([]string{}),
			numRunners:     3,
			expectedShards: 0,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := manager.CreateShardedKSMConfigs(tt.config, tt.numRunners)

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
	manager := NewKSMShardingManager(true)

	// Create config with existing tags
	config := createKSMConfigWithTags([]string{"pods", "nodes"}, []string{"env:prod", "team:platform"})

	configs, err := manager.CreateShardedKSMConfigs(config, 3)
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

func TestGetExistingTags(t *testing.T) {
	tests := []struct {
		name     string
		instance map[string]interface{}
		expected []string
	}{
		{
			name: "string slice tags",
			instance: map[string]interface{}{
				"tags": []string{"env:prod", "team:platform"},
			},
			expected: []string{"env:prod", "team:platform"},
		},
		{
			name: "interface slice tags",
			instance: map[string]interface{}{
				"tags": []interface{}{"env:prod", "team:platform"},
			},
			expected: []string{"env:prod", "team:platform"},
		},
		{
			name:     "no tags",
			instance: map[string]interface{}{},
			expected: []string{},
		},
		{
			name: "empty tags",
			instance: map[string]interface{}{
				"tags": []string{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getExistingTags(tt.instance)
			assert.Equal(t, tt.expected, result)
		})
	}
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
