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

func TestScheduleKSMCheck_Disabled(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(false),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(true)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	result := d.scheduleKSMCheck(config)
	assert.False(t, result, "Should return false when sharding is disabled")
}

func TestScheduleKSMCheck_NotKSMCheck(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(true)

	config := integration.Config{
		Name:         "prometheus",
		ClusterCheck: true,
	}

	result := d.scheduleKSMCheck(config)
	assert.False(t, result, "Should return false for non-KSM checks")
}

func TestScheduleKSMCheck_NoAdvancedDispatching(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(false)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	result := d.scheduleKSMCheck(config)
	assert.False(t, result, "Should return false when advanced dispatching is disabled")
}

func TestScheduleKSMCheck_NotShardable(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(true)

	// Config with only one group (pods only) - not shardable
	config := createTestKSMConfig([]string{"pods"})

	result := d.scheduleKSMCheck(config)
	assert.False(t, result, "Should return false when check has only one resource group")
}

func TestScheduleKSMCheck_NoRunners(t *testing.T) {
	d := &dispatcher{
		ksmSharding: NewKSMShardingManager(true),
		store:       newClusterStore(),
	}
	d.advancedDispatching.Store(true)

	config := createTestKSMConfig([]string{"pods", "nodes"})

	// No runners in store
	result := d.scheduleKSMCheck(config)
	assert.False(t, result, "Should return false when no runners are available")
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

func TestGetAvailableRunners(t *testing.T) {
	d := &dispatcher{
		store: newClusterStore(),
	}

	// Initially no runners
	runners := d.getAvailableRunners()
	assert.Empty(t, runners)

	// Add some runners
	d.store.Lock()
	d.store.nodes["runner-1"] = &nodeStore{name: "runner-1", nodetype: cctypes.NodeTypeCLCRunner}
	d.store.nodes["runner-2"] = &nodeStore{name: "runner-2", nodetype: cctypes.NodeTypeCLCRunner}
	d.store.nodes["runner-3"] = &nodeStore{name: "runner-3", nodetype: cctypes.NodeTypeCLCRunner}
	d.store.Unlock()

	runners = d.getAvailableRunners()
	assert.Len(t, runners, 3)
	assert.ElementsMatch(t, []string{"runner-1", "runner-2", "runner-3"}, runners)
}

func TestValidateClusterTaggerForKSM_Success(t *testing.T) {
	// Setup config with cluster tagger enabled
	mockConfig := pkgconfigsetup.Datadog()
	mockConfig.SetWithoutSource("cluster_agent.collect_kubernetes_tags", true)
	mockConfig.SetWithoutSource("clc_runner_remote_tagger_enabled", true)

	d := &dispatcher{}

	result := d.validateClusterTaggerForKSM()
	assert.True(t, result, "Validation should pass when cluster tagger is enabled")
}

func TestValidateClusterTaggerForKSM_MissingClusterTags(t *testing.T) {
	// Setup config without cluster tagger
	mockConfig := pkgconfigsetup.Datadog()
	mockConfig.SetWithoutSource("cluster_agent.collect_kubernetes_tags", false)

	d := &dispatcher{}

	result := d.validateClusterTaggerForKSM()
	assert.True(t, result, "Validation should pass with warnings when cluster_agent.collect_kubernetes_tags is disabled")
}

func TestValidateClusterTaggerForKSM_MissingRemoteTagger(t *testing.T) {
	// Setup config with cluster tagger enabled but remote tagger disabled
	mockConfig := pkgconfigsetup.Datadog()
	mockConfig.SetWithoutSource("cluster_agent.collect_kubernetes_tags", true)
	mockConfig.SetWithoutSource("clc_runner_remote_tagger_enabled", false)

	d := &dispatcher{}

	// Should still pass but with warning (remote tagger is not hard requirement)
	result := d.validateClusterTaggerForKSM()
	assert.True(t, result, "Validation should pass even if remote tagger is disabled (with warning)")
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
