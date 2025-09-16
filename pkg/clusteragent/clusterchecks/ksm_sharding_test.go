// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestKSMShardingManager_IsKSMCheck(t *testing.T) {
	manager := NewKSMShardingManager(true)

	tests := []struct {
		name     string
		config   integration.Config
		expected bool
	}{
		{
			name:     "kubernetes_state_core check",
			config:   integration.Config{Name: "kubernetes_state_core"},
			expected: true,
		},
		{
			name:     "kubernetes_state legacy check",
			config:   integration.Config{Name: "kubernetes_state"},
			expected: true,
		},
		{
			name:     "non-KSM check",
			config:   integration.Config{Name: "http_check"},
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

func TestKSMShardingManager_IsNamespaceScopedCollector(t *testing.T) {
	manager := NewKSMShardingManager(true)

	tests := []struct {
		name      string
		collector string
		expected  bool
	}{
		// Namespace-scoped collectors
		{"pods", "pods", true},
		{"services", "services", true},
		{"deployments", "deployments", true},
		{"configmaps", "configmaps", true},
		{"secrets", "secrets", true},
		{"jobs", "jobs", true},
		{"cronjobs", "cronjobs", true},
		{"ingresses", "ingresses", true},

		// Cluster-scoped collectors (not in the known list)
		{"nodes", "nodes", false},
		{"namespaces", "namespaces", false},
		{"persistentvolumes", "persistentvolumes", false},
		{"clusterroles", "clusterroles", false},

		// Unknown collector (defaults to cluster-scoped)
		{"mycustomresource", "mycustomresource", false},

		// Test normalization
		{"pods with suffix", "pods_extended", true},
		{"uppercase", "PODS", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.IsNamespaceScopedCollector(tt.collector)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKSMShardingManager_AnalyzeKSMConfig(t *testing.T) {
	manager := NewKSMShardingManager(true)

	tests := []struct {
		name               string
		config             integration.Config
		expectedNamespaced []string
		expectedCluster    []string
		expectError        bool
	}{
		{
			name: "mixed collectors",
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"pods", "services", "nodes", "namespaces"},
					})),
				},
			},
			expectedNamespaced: []string{"pods", "services"},
			expectedCluster:    []string{"nodes", "namespaces"},
			expectError:        false,
		},
		{
			name: "only namespace-scoped collectors",
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"pods", "services", "deployments"},
					})),
				},
			},
			expectedNamespaced: []string{"pods", "services", "deployments"},
			expectedCluster:    []string{},
			expectError:        false,
		},
		{
			name: "only cluster-scoped collectors",
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"nodes", "namespaces"},
					})),
				},
			},
			expectedNamespaced: []string{},
			expectedCluster:    []string{"nodes", "namespaces"},
			expectError:        false,
		},
		{
			name: "no collectors specified (collect all)",
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{})),
				},
			},
			expectedNamespaced: nil,
			expectedCluster:    nil,
			expectError:        true, // Should error because we don't shard when collecting all
		},
		{
			name: "not a KSM check",
			config: integration.Config{
				Name: "http_check",
			},
			expectedNamespaced: nil,
			expectedCluster:    nil,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespaced, cluster, err := manager.AnalyzeKSMConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedNamespaced, namespaced)
				assert.ElementsMatch(t, tt.expectedCluster, cluster)
			}
		})
	}
}

func TestKSMShardingManager_ShouldShardKSMCheck(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		config   integration.Config
		expected bool
	}{
		{
			name:    "sharding disabled",
			enabled: false,
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"pods", "services"},
					})),
				},
			},
			expected: false,
		},
		{
			name:    "sharding enabled with namespace-scoped collectors",
			enabled: true,
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"pods", "services", "nodes"},
					})),
				},
			},
			expected: true,
		},
		{
			name:    "sharding enabled but only cluster-scoped collectors",
			enabled: true,
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{
						"collectors": []string{"nodes", "namespaces"},
					})),
				},
			},
			expected: false,
		},
		{
			name:    "sharding enabled but no collectors specified",
			enabled: true,
			config: integration.Config{
				Name: "kubernetes_state_core",
				Instances: []integration.Data{
					integration.Data(mustMarshalYAML(map[string]interface{}{})),
				},
			},
			expected: false,
		},
		{
			name:    "not a KSM check",
			enabled: true,
			config: integration.Config{
				Name: "http_check",
			},
			expected: false,
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

func TestKSMShardingManager_CreateShardedKSMConfigs(t *testing.T) {
	manager := NewKSMShardingManager(true)

	baseConfig := integration.Config{
		Name: "kubernetes_state_core",
		Instances: []integration.Data{
			integration.Data(mustMarshalYAML(map[string]interface{}{
				"collectors": []string{"pods", "services", "nodes"},
				"tags":       []string{"env:test"},
			})),
		},
	}

	namespaces := []string{"default", "kube-system", "prod-a", "prod-b"}

	shardedConfigs, clusterConfig, err := manager.CreateShardedKSMConfigs(baseConfig, namespaces)

	assert.NoError(t, err)

	// Check cluster-wide config
	assert.NotEmpty(t, clusterConfig.Name)
	assert.Equal(t, "kubernetes_state_core", clusterConfig.Name)

	// Parse cluster config instance
	var clusterInstance map[string]interface{}
	err = yaml.Unmarshal(clusterConfig.Instances[0], &clusterInstance)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"nodes"}, clusterInstance["collectors"])
	assert.Contains(t, clusterInstance["tags"], "ksm_shard_type:cluster-wide")

	// Check sharded configs - should be one per namespace
	assert.Equal(t, len(namespaces), len(shardedConfigs))

	// Check that each namespace has a config
	assignedNamespaces := make(map[string]bool)
	for _, config := range shardedConfigs {
		var instance map[string]interface{}
		err = yaml.Unmarshal(config.Instances[0], &instance)
		assert.NoError(t, err)

		// Check collectors
		assert.ElementsMatch(t, []string{"pods", "services"}, instance["collectors"])

		// Check namespace assignment
		namespaceList := instance["namespaces"].([]interface{})
		assert.Len(t, namespaceList, 1)
		ns := namespaceList[0].(string)
		assignedNamespaces[ns] = true

		// Check tags
		tags := instance["tags"].([]interface{})
		assert.Contains(t, tags, "ksm_shard_type:namespaced")
		assert.Contains(t, tags, "env:test") // Original tag preserved
	}

	// Verify all namespaces were assigned
	assert.Len(t, assignedNamespaces, len(namespaces))
	for _, ns := range namespaces {
		assert.True(t, assignedNamespaces[ns])
	}
}

func TestNormalizeCollectorName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pods", "pods"},
		{"PODS", "pods"},
		{"pods_extended", "pods"},
		{"apps/v1, Resource=deployments", "deployments"},
		{"apps/v1, Resource=deployments_extended", "deployments"},
		{"  pods  ", "pods"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCollectorName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to marshal YAML
func mustMarshalYAML(v interface{}) []byte {
	data, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
