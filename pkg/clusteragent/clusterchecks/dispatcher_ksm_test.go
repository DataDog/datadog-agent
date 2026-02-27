// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func TestScheduleKSMCheck(t *testing.T) {
	tests := []struct {
		name                string
		shardingEnabled     bool
		advancedDispatching bool
		config              integration.Config
		expectedResult      bool
	}{
		{
			name:                "sharding disabled",
			shardingEnabled:     false,
			advancedDispatching: true,
			config:              createTestKSMConfig([]string{"pods", "nodes"}),
			expectedResult:      false,
		},
		{
			name:                "not a KSM check",
			shardingEnabled:     true,
			advancedDispatching: true,
			config: integration.Config{
				Name:         "prometheus",
				ClusterCheck: true,
			},
			expectedResult: false,
		},
		{
			name:                "advanced dispatching disabled",
			shardingEnabled:     true,
			advancedDispatching: false,
			config:              createTestKSMConfig([]string{"pods", "nodes"}),
			expectedResult:      false,
		},
		{
			name:                "not shardable - only one resource group",
			shardingEnabled:     true,
			advancedDispatching: true,
			config:              createTestKSMConfig([]string{"pods"}),
			expectedResult:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dispatcher{
				ksmSharding: newKSMShardingManager(tt.shardingEnabled),
				store:       newClusterStore(),
			}
			d.advancedDispatching.Store(tt.advancedDispatching)

			result := d.scheduleKSMCheck(tt.config)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestScheduleKSMCheck_NoRunners(t *testing.T) {
	d := &dispatcher{
		ksmSharding:       newKSMShardingManager(true),
		store:             newClusterStore(),
		ksmShardedConfigs: make(map[string][]string),
	}
	d.advancedDispatching.Store(true)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// No runners in store - sharding still succeeds, shards go to dangling state
	result := d.scheduleKSMCheck(config)
	assert.True(t, result, "Should still create shards even with no runners (they become dangling)")

	// Verify shards were created and tracked
	assert.NotEmpty(t, d.ksmShardedConfigs, "Should track sharded config digests")
}

func TestScheduleKSMCheck_AlreadySharded(t *testing.T) {
	d := &dispatcher{
		ksmSharding:       newKSMShardingManager(true),
		store:             newClusterStore(),
		ksmShardedConfigs: make(map[string][]string),
	}
	d.advancedDispatching.Store(true)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// Mark config as already sharded
	d.markAsSharded(config, []string{})

	result := d.scheduleKSMCheck(config)
	assert.True(t, result, "Should return true when config is already sharded to prevent duplicate scheduling")
}

func TestIsAlreadySharded(t *testing.T) {
	d := &dispatcher{
		ksmSharding:       newKSMShardingManager(true),
		store:             newClusterStore(),
		ksmShardedConfigs: make(map[string][]string),
	}

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// Initially not sharded
	assert.False(t, d.isAlreadySharded(config))

	// Mark as sharded
	d.markAsSharded(config, []string{})

	// Now should be marked as sharded
	assert.True(t, d.isAlreadySharded(config))

	// Different config should not be marked
	differentConfig := createTestKSMConfig([]string{"pods", "deployments"})
	assert.False(t, d.isAlreadySharded(differentConfig))
}

func TestMarkAsSharded(t *testing.T) {
	d := &dispatcher{
		ksmShardedConfigs: make(map[string][]string),
	}

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// Initially no sharded configs
	assert.Empty(t, d.ksmShardedConfigs)

	d.markAsSharded(config, []string{"digest1", "digest2"})

	// Now should have the sharded config stored in the map
	shardDigests, exists := d.ksmShardedConfigs[config.Digest()]
	assert.True(t, exists, "Config digest should exist in map")
	assert.Equal(t, []string{"digest1", "digest2"}, shardDigests)
}

func TestScheduleKSMCheck_Integration(t *testing.T) {
	// This test simulates a successful sharding scenario
	// Create a dispatcher with all requirements met
	d := &dispatcher{
		ksmSharding:       newKSMShardingManager(true),
		store:             newClusterStore(),
		ksmShardedConfigs: make(map[string][]string),
	}
	d.advancedDispatching.Store(true)

	// Add runners
	d.store.Lock()
	d.store.nodes["runner-1"] = &nodeStore{name: "runner-1", nodetype: cctypes.NodeTypeCLCRunner}
	d.store.nodes["runner-2"] = &nodeStore{name: "runner-2", nodetype: cctypes.NodeTypeCLCRunner}
	d.store.Unlock()

	// Create KSM config with multiple groups
	config := createTestKSMConfig([]string{"pods", "nodes", "deployments"})

	// Verify it should be shardable
	assert.True(t, d.ksmSharding.shouldShardKSMCheck(config))

	// Note: Full scheduling test would require mocking patchConfiguration and add methods
	// which are complex. The unit tests above cover the individual components.
}

// Helper functions

func createTestKSMConfig(collectors []string) integration.Config {
	instance := map[string]interface{}{
		"collectors": collectors,
	}

	data, _ := yaml.Marshal(instance)

	return integration.Config{
		Name:         "kubernetes_state_core",
		Instances:    []integration.Data{integration.Data(data)},
		ClusterCheck: true,
	}
}
