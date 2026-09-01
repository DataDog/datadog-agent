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
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// newTestDispatcher builds a minimal dispatcher for sharding tests. Both
// managers are always present (never nil), since prepareShardSchedule and
// prepareShardUnschedule touch whichever one matches the config's check
// name, regardless of which strategy the test cares about.
func newTestDispatcher(ksmEnabled, instanceEnabled bool) *dispatcher {
	d := &dispatcher{
		ksmSharding:      newKSMShardingManager(ksmEnabled),
		instanceSharding: newInstanceShardingManager(instanceEnabled, nil),
		shards:           newShardTracker(),
		store:            newClusterStore(),
	}
	d.advancedDispatching.Store(true)
	return d
}

func TestPrepareKSMSchedule(t *testing.T) {
	tests := []struct {
		name                string
		shardingEnabled     bool
		advancedDispatching bool
		config              integration.Config
		expectedHandled     bool
	}{
		{
			name:                "sharding disabled",
			shardingEnabled:     false,
			advancedDispatching: true,
			config:              createTestKSMConfig([]string{"pods", "nodes"}),
			expectedHandled:     false,
		},
		{
			name:                "not a KSM check",
			shardingEnabled:     true,
			advancedDispatching: true,
			config: integration.Config{
				Name:         "prometheus",
				ClusterCheck: true,
			},
			expectedHandled: false,
		},
		{
			name:                "sharding still applies without advanced dispatching",
			shardingEnabled:     true,
			advancedDispatching: false,
			config:              createTestKSMConfig([]string{"pods", "nodes"}),
			expectedHandled:     true,
		},
		{
			name:                "not shardable - only one resource group",
			shardingEnabled:     true,
			advancedDispatching: true,
			config:              createTestKSMConfig([]string{"pods"}),
			expectedHandled:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(tt.shardingEnabled, false)
			d.advancedDispatching.Store(tt.advancedDispatching)

			shards, handled := d.prepareShardSchedule(tt.config)
			assert.Equal(t, tt.expectedHandled, handled)
			if tt.expectedHandled {
				assert.NotEmpty(t, shards)
			} else {
				assert.Empty(t, shards)
			}
		})
	}
}

func TestPrepareInstanceShardSchedule(t *testing.T) {
	tests := []struct {
		name                string
		shardingEnabled     bool
		advancedDispatching bool
		config              integration.Config
		expectedHandled     bool
	}{
		{
			name:                "sharding disabled",
			shardingEnabled:     false,
			advancedDispatching: true,
			config:              createTestMultiInstanceConfig("postgres", 3),
			expectedHandled:     false,
		},
		{
			name:                "single instance config",
			shardingEnabled:     true,
			advancedDispatching: true,
			config:              createTestMultiInstanceConfig("postgres", 1),
			expectedHandled:     false,
		},
		{
			name:                "sharding still applies without advanced dispatching",
			shardingEnabled:     true,
			advancedDispatching: false,
			config:              createTestMultiInstanceConfig("postgres", 3),
			expectedHandled:     true,
		},
		{
			name:                "KSM check is left to KSM sharding",
			shardingEnabled:     true,
			advancedDispatching: true,
			config:              createTestMultiInstanceConfig(ksmCheckName, 3),
			expectedHandled:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(false, tt.shardingEnabled)
			d.advancedDispatching.Store(tt.advancedDispatching)

			shards, handled := d.prepareShardSchedule(tt.config)
			assert.Equal(t, tt.expectedHandled, handled)
			if tt.expectedHandled {
				assert.Len(t, shards, len(tt.config.Instances))
			} else {
				assert.Empty(t, shards)
			}
		})
	}
}

func TestPrepareShardSchedule_NoRunners(t *testing.T) {
	tests := []struct {
		name           string
		config         integration.Config
		expectedShards int
	}{
		{name: "ksm", config: createTestKSMConfig([]string{"pods", "nodes"}), expectedShards: 2},
		{name: "instance", config: createTestMultiInstanceConfig("postgres", 3), expectedShards: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(true, true)

			// No runners in store - sharding still succeeds, shards go to dangling state
			shards, handled := d.prepareShardSchedule(tt.config)
			assert.True(t, handled, "Should still create shards even with no runners (they become dangling)")
			assert.Len(t, shards, tt.expectedShards)

			assert.True(t, d.shards.isTracked(tt.config.Digest()), "Should track sharded config digest")
		})
	}
}

func TestPrepareShardSchedule_AlreadySharded(t *testing.T) {
	tests := []struct {
		name   string
		config integration.Config
	}{
		{name: "ksm", config: createTestKSMConfig([]string{"pods", "nodes"})},
		{name: "instance", config: createTestMultiInstanceConfig("postgres", 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(true, true)
			d.shards.mark(tt.config.Digest(), []string{})

			shards, handled := d.prepareShardSchedule(tt.config)
			assert.True(t, handled, "Should return handled=true when config is already sharded, to prevent duplicate scheduling")
			assert.Empty(t, shards, "Already-sharded config produces no new shards to add")
		})
	}
}

func TestPrepareShardSchedule_EachInstanceIndividuallyPlaced(t *testing.T) {
	d := newTestDispatcher(false, true)

	d.store.Lock()
	d.store.nodes["runner-1"] = &nodeStore{name: "runner-1", nodetype: cctypes.NodeTypeCLCRunner, digestToConfig: make(map[string]integration.Config)}
	d.store.nodes["runner-2"] = &nodeStore{name: "runner-2", nodetype: cctypes.NodeTypeCLCRunner, digestToConfig: make(map[string]integration.Config)}
	d.store.nodes["runner-3"] = &nodeStore{name: "runner-3", nodetype: cctypes.NodeTypeCLCRunner, digestToConfig: make(map[string]integration.Config)}
	d.store.Unlock()

	config := createTestMultiInstanceConfig("postgres", 6)

	prepared, handled := d.prepareShardSchedule(config)
	require.True(t, handled)
	require.Len(t, prepared, 6)

	for _, patched := range prepared {
		d.add(patched)
	}

	shardDigests, tracked := d.shards.pop(config.Digest())
	require.True(t, tracked)
	require.Len(t, shardDigests, 6)

	// Each of the 6 single-instance shards must have been individually
	// dispatched (placement itself, random vs. least-loaded, is existing
	// dispatcher behavior tested elsewhere).
	d.store.RLock()
	defer d.store.RUnlock()
	for _, digest := range shardDigests {
		node, placed := d.store.digestToNode[digest]
		assert.True(t, placed, "shard %s should have been placed on a node", digest)
		assert.Contains(t, d.store.nodes, node)
	}
}

func TestPrepareShardUnschedule(t *testing.T) {
	tests := []struct {
		name           string
		config         integration.Config
		expectedShards int
	}{
		{name: "ksm", config: createTestKSMConfig([]string{"pods", "nodes"}), expectedShards: 2},
		{name: "instance", config: createTestMultiInstanceConfig("postgres", 3), expectedShards: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(true, true)

			shards, handled := d.prepareShardSchedule(tt.config)
			require.True(t, handled)
			require.Len(t, shards, tt.expectedShards)

			digests, handled := d.prepareShardUnschedule(tt.config)
			assert.True(t, handled)
			assert.Len(t, digests, tt.expectedShards)
			assert.False(t, d.shards.isTracked(tt.config.Digest()), "prepareShardUnschedule must clear tracking")
		})
	}
}

func TestPrepareShardUnschedule_NotSharded(t *testing.T) {
	tests := []struct {
		name   string
		config integration.Config
	}{
		{name: "ksm", config: createTestKSMConfig([]string{"pods", "nodes"})},
		{name: "instance", config: createTestMultiInstanceConfig("postgres", 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(true, true)
			digests, handled := d.prepareShardUnschedule(tt.config)
			assert.False(t, handled)
			assert.Empty(t, digests)
		})
	}
}

func TestNewDispatcher_ShardingEnabledWhenAdvancedDispatchingDisabled(t *testing.T) {
	tests := []struct {
		name      string
		configKey string
		config    integration.Config
		isEnabled func(d *dispatcher) bool
	}{
		{
			name:      "ksm",
			configKey: "cluster_checks.ksm_sharding_enabled",
			config:    createTestKSMConfig([]string{"pods", "nodes"}),
			isEnabled: func(d *dispatcher) bool { return d.ksmSharding.isEnabled() },
		},
		{
			name:      "instance",
			configKey: "cluster_checks.instance_sharding_enabled",
			config:    createTestMultiInstanceConfig("postgres", 3),
			isEnabled: func(d *dispatcher) bool { return d.instanceSharding.isEnabled() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("cluster_checks.advanced_dispatching_enabled", false)
			mockConfig.SetInTest(tt.configKey, true)

			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			d := newDispatcher(fakeTagger)

			assert.True(t, tt.isEnabled(d), "sharding doesn't require advanced dispatching")
			assert.NotPanics(t, func() { d.Schedule([]integration.Config{tt.config}) })
		})
	}
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
