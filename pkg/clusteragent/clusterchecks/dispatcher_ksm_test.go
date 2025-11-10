// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
				ksmSharding: NewKSMShardingManager(tt.shardingEnabled),
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
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(true)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// No runners in store - sharding still succeeds, shards go to dangling state
	result := d.scheduleKSMCheck(config)
	assert.True(t, result, "Should still create shards even with no runners (they become dangling)")

	// Verify shards were created and tracked
	assert.NotEmpty(t, d.ksmShardedDigests, "Should track sharded config digests")
}

func TestScheduleKSMCheck_AlreadySharded(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
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
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
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

func TestValidateClusterTaggerForKSM(t *testing.T) {
	tests := []struct {
		name                  string
		collectKubernetesTags bool
		remoteTaggerEnabled   bool
		expectedResult        bool
	}{
		{
			name:                  "all features enabled",
			collectKubernetesTags: true,
			remoteTaggerEnabled:   true,
			expectedResult:        true,
		},
		{
			name:                  "cluster tags disabled",
			collectKubernetesTags: false,
			remoteTaggerEnabled:   false,
			expectedResult:        false,
		},
		{
			name:                  "remote tagger disabled - still passes with warning",
			collectKubernetesTags: true,
			remoteTaggerEnabled:   false,
			expectedResult:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Datadog()
			mockConfig.SetWithoutSource("cluster_agent.collect_kubernetes_tags", tt.collectKubernetesTags)
			mockConfig.SetWithoutSource("clc_runner_remote_tagger_enabled", tt.remoteTaggerEnabled)

			d := &dispatcher{}

			result := d.validateClusterTaggerForKSM()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestMarkAsSharded(t *testing.T) {
	d := &dispatcher{}

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// Initially no sharded config
	assert.Empty(t, d.ksmShardedConfig.Name)

	d.markAsSharded(config, []string{"digest1", "digest2"})

	// Now should have the sharded config stored
	assert.Equal(t, config.Name, d.ksmShardedConfig.Name)
	assert.Equal(t, config.Digest(), d.ksmShardedConfig.Digest())
	assert.Equal(t, []string{"digest1", "digest2"}, d.ksmShardedDigests)
}

func TestScheduleKSMCheck_Integration(t *testing.T) {
	// This test simulates a successful sharding scenario
	// Create a dispatcher with all requirements met
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
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
	assert.True(t, d.ksmSharding.ShouldShardKSMCheck(config))

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
