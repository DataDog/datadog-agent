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
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ksmsharding "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
)

// createKSMHashConfig builds a KSM cluster-check config whose single shardable
// instance requests hash sharding. instance is merged in as-is so tests can
// inject malformed values.
func createKSMHashConfig(instance map[string]interface{}) integration.Config {
	data, _ := yaml.Marshal(instance)

	return integration.Config{
		Name:         ksmCheckName,
		Instances:    []integration.Data{integration.Data(data)},
		ClusterCheck: true,
	}
}

// shardInstance unmarshals a shard config's single instance.
func shardInstance(t *testing.T, config integration.Config) map[string]interface{} {
	t.Helper()
	require.Len(t, config.Instances, 1)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(config.Instances[0], &instance))
	return instance
}

func TestParseHashSharding(t *testing.T) {
	tests := []struct {
		name             string
		instance         string
		expectedCriteria []string
		expectedCount    int
		expectAbsent     bool
		expectError      bool
	}{
		{
			name:             "by namespace",
			instance:         "shard_criteria: [namespace]\nshard_count: 4",
			expectedCriteria: []string{ksmsharding.CriterionNamespace},
			expectedCount:    4,
		},
		{
			name:             "by resource",
			instance:         "shard_criteria: [resource]\nshard_count: 2",
			expectedCriteria: []string{ksmsharding.CriterionResource},
			expectedCount:    2,
		},
		{
			name:             "by namespace and resource",
			instance:         "shard_criteria: [namespace, resource]\nshard_count: 6",
			expectedCriteria: []string{ksmsharding.CriterionNamespace, ksmsharding.CriterionResource},
			expectedCount:    6,
		},
		{
			// The agreement invariant: shards compose the rendezvous key by
			// joining criteria, so input order must not survive parsing.
			name:             "reversed criteria does not materialize in config",
			instance:         "shard_criteria: [resource, namespace]\nshard_count: 6",
			expectedCriteria: []string{ksmsharding.CriterionNamespace, ksmsharding.CriterionResource},
			expectedCount:    6,
		},
		{
			name:         "no shard_criteria selects resource-group sharding",
			instance:     "collectors: [pods, nodes]",
			expectAbsent: true,
		},
		{
			name:         "empty shard_criteria selects resource-group sharding",
			instance:     "shard_criteria: []\nshard_count: 4",
			expectAbsent: true,
		},
		{
			name:        "shard_count missing",
			instance:    "shard_criteria: [namespace]",
			expectError: true,
		},
		{
			name:        "shard_count of 1 is not sharding",
			instance:    "shard_criteria: [namespace]\nshard_count: 1",
			expectError: true,
		},
		{
			name:        "negative shard_count",
			instance:    "shard_criteria: [namespace]\nshard_count: -3",
			expectError: true,
		},
		{
			name:        "unknown criterion",
			instance:    "shard_criteria: [namespace, node]\nshard_count: 4",
			expectError: true,
		},
		{
			name:        "duplicate criterion",
			instance:    "shard_criteria: [namespace, namespace]\nshard_count: 4",
			expectError: true,
		},
		{
			name:        "unparseable instance",
			instance:    "shard_criteria: [namespace\nshard_count: 4",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHashSharding(integration.Data(tt.instance))

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, got, "a misconfigured request must not yield a usable config")
				return
			}

			require.NoError(t, err)

			if tt.expectAbsent {
				assert.Nil(t, got, "absent shard_criteria must report not-requested, not an error")
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.expectedCriteria, got.criteria)
			assert.Equal(t, tt.expectedCount, got.count)
		})
	}
}

func TestShouldShardKSMHashCheck(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		config   integration.Config
		expected bool
	}{
		{
			name:    "sharding disabled",
			enabled: false,
			config: createKSMHashConfig(map[string]interface{}{
				"shard_criteria": []string{"namespace"}, "shard_count": 4,
			}),
			expected: false,
		},
		{
			name:     "not a KSM check",
			enabled:  true,
			config:   integration.Config{Name: "prometheus", ClusterCheck: true},
			expected: false,
		},
		{
			name:    "cluster_check is false",
			enabled: true,
			config: integration.Config{
				Name:         ksmCheckName,
				ClusterCheck: false,
				Instances:    []integration.Data{integration.Data("shard_criteria: [namespace]\nshard_count: 4")},
			},
			expected: false,
		},
		{
			name:    "valid hash sharding request",
			enabled: true,
			config: createKSMHashConfig(map[string]interface{}{
				"shard_criteria": []string{"namespace"}, "shard_count": 4,
			}),
			expected: true,
		},
		{
			name:     "no shard_criteria is another strategy's business",
			enabled:  true,
			config:   createKSMConfig([]string{"pods", "nodes"}),
			expected: false,
		},
		{
			name:    "misconfigured request is declined, not downgraded",
			enabled: true,
			config: createKSMHashConfig(map[string]interface{}{
				"shard_criteria": []string{"namespace"}, "shard_count": 1,
			}),
			expected: false,
		},
		{
			name:    "no shardable instance",
			enabled: true,
			config: integration.Config{
				Name:         ksmCheckName,
				ClusterCheck: true,
				Instances: []integration.Data{
					integration.Data("pod_collection_mode: cluster_aggregates_only"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newKSMHashShardingManager(tt.enabled)
			assert.Equal(t, tt.expected, manager.shouldShardKSMHashCheck(tt.config))
		})
	}
}

// TestKSMShardingStrategiesAreMutuallyExclusive ensures strategy selection does
// not depend on registration order.
func TestKSMShardingStrategiesAreMutuallyExclusive(t *testing.T) {
	hash := newKSMHashShardingManager(true)
	resource := newKSMShardingManager(true)

	tests := []struct {
		name           string
		config         integration.Config
		expectHash     bool
		expectResource bool
	}{
		{
			name: "valid hash request goes to hash sharding only",
			config: createKSMHashConfig(map[string]interface{}{
				"collectors":     []string{"pods", "nodes"},
				"shard_criteria": []string{"namespace"},
				"shard_count":    4,
			}),
			expectHash:     true,
			expectResource: false,
		},
		{
			name:           "no hash request goes to resource sharding only",
			config:         createKSMConfig([]string{"pods", "nodes"}),
			expectHash:     false,
			expectResource: true,
		},
		{
			// shard_count: 1 is a typo'd hash request. Neither strategy may
			// claim it, so it schedules unsharded rather than being silently
			// resource-group sharded.
			name: "broken hash request is claimed by neither",
			config: createKSMHashConfig(map[string]interface{}{
				"collectors":     []string{"pods", "nodes"},
				"shard_criteria": []string{"namespace"},
				"shard_count":    1,
			}),
			expectHash:     false,
			expectResource: false,
		},
		{
			name: "unknown criterion is claimed by neither",
			config: createKSMHashConfig(map[string]interface{}{
				"collectors":     []string{"pods", "nodes"},
				"shard_criteria": []string{"node"},
				"shard_count":    4,
			}),
			expectHash:     false,
			expectResource: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash := hash.shouldShard(tt.config)
			gotResource := resource.shouldShard(tt.config)

			assert.Equal(t, tt.expectHash, gotHash, "hash strategy")
			assert.Equal(t, tt.expectResource, gotResource, "resource strategy")
			assert.False(t, gotHash && gotResource, "at most one strategy may claim a config")
		})
	}
}

func TestCreateShardedKSMHashConfigs(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	config := createKSMHashConfig(map[string]interface{}{
		"collectors":     []string{"deployments", "replicasets"},
		"shard_criteria": []string{"resource", "namespace"},
		"shard_count":    3,
	})

	shards, err := manager.createShardedKSMHashConfigs(config)
	require.NoError(t, err)
	require.Len(t, shards, 3)

	seenIDs := make(map[int]struct{}, len(shards))
	digests := make(map[string]struct{}, len(shards))

	for _, shard := range shards {
		instance := shardInstance(t, shard)

		// Written from the single parsed source, in canonical order, so every
		// shard composes the same rendezvous key.
		assert.Equal(t, []interface{}{ksmsharding.CriterionNamespace, ksmsharding.CriterionResource}, instance["shard_criteria"])
		assert.Equal(t, 3, instance["shard_count"])
		assert.Equal(t, true, instance["skip_leader_election"])
		// Non-sharding keys are inherited.
		assert.Equal(t, []interface{}{"deployments", "replicasets"}, instance["collectors"])

		id, ok := instance["shard_id"].(int)
		require.True(t, ok, "shard_id must be present and an int")
		assert.GreaterOrEqual(t, id, 0)
		assert.Less(t, id, 3)
		seenIDs[id] = struct{}{}

		digests[shard.Digest()] = struct{}{}
	}

	assert.Len(t, seenIDs, 3, "shard_id must cover 0..count-1 without repeats")
	// shard_id is the only field that differs, so it is solely responsible for
	// keeping digests distinct. Without it the dispatcher would track one shard.
	assert.Len(t, digests, 3, "each shard must have a distinct dispatch identity")
}

func TestCreateShardedKSMHashConfigs_StandaloneAggregates(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	config := createKSMHashConfig(map[string]interface{}{
		"pod_collection_mode":        clusterUnassignedMode,
		"cluster_aggregates_enabled": true,
		"shard_criteria":             []string{"namespace"},
		"shard_count":                2,
	})
	config.Instances = append(config.Instances,
		integration.Data("pod_collection_mode: "+clusterAggregatesOnlyMode))

	shards, err := manager.createShardedKSMHashConfigs(config)
	require.NoError(t, err)
	require.Len(t, shards, 3, "2 shards plus one standalone aggregates config")

	// The aggregates instance is dispatched as its own config rather than
	// pinned to a shard, so rebalancing can place the full-cluster pod watch on
	// whichever runner has headroom.
	aggregate := shardInstance(t, shards[2])
	assert.Equal(t, clusterAggregatesOnlyMode, aggregate["pod_collection_mode"])
	assert.Equal(t, true, aggregate["skip_leader_election"])
	assert.NotContains(t, aggregate, "shard_id", "the aggregates instance must not be sharded")

	for _, shard := range shards[:2] {
		assert.NotEqual(t, clusterAggregatesOnlyMode, shardInstance(t, shard)["pod_collection_mode"])
	}
}

// TestCreateShardedKSMHashConfigs_DropsExtraAggregates pins the drop: every
// cluster_aggregates_only instance does its own full-cluster pod watch and emits
// the same .total family, so dispatching all of them inflates .total by the
// number configured.
func TestCreateShardedKSMHashConfigs_DropsExtraAggregates(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	config := createKSMHashConfig(map[string]interface{}{
		"shard_criteria": []string{"namespace"},
		"shard_count":    2,
	})
	for i := 0; i < 3; i++ {
		config.Instances = append(config.Instances,
			integration.Data("pod_collection_mode: "+clusterAggregatesOnlyMode))
	}

	shards, err := manager.createShardedKSMHashConfigs(config)
	require.NoError(t, err)
	require.Len(t, shards, 3, "2 shards plus exactly one aggregates config")
	require.Len(t, shards[2].Instances, 1, "extras are dropped, not bundled")
}

func TestCreateShardedKSMHashConfigs_PreservesBaseConfig(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	config := createKSMHashConfig(map[string]interface{}{
		"shard_criteria": []string{"namespace"},
		"shard_count":    2,
	})
	config.InitConfig = integration.Data("timeout: 30")
	config.Source = "file:/etc/datadog-agent/conf.d/kubernetes_state_core.d/conf.yaml"
	config.PodNamespace = "datadog"
	config.ImageName = "agent"

	shards, err := manager.createShardedKSMHashConfigs(config)
	require.NoError(t, err)
	require.Len(t, shards, 2)

	for _, shard := range shards {
		// init_config is shared by all instances of a config, so each shard
		// needs its own copy now that it is its own config.
		assert.Equal(t, config.InitConfig, shard.InitConfig)
		assert.Equal(t, config.Source, shard.Source)
		// Read by configmgr.go's secret-resolution ACL checks.
		assert.Equal(t, config.PodNamespace, shard.PodNamespace)
		assert.Equal(t, config.ImageName, shard.ImageName)
		// Omitting CELSelector/Discovery keeps IsTemplate() false so the runner
		// schedules the shard directly instead of waiting for a service match.
		assert.False(t, shard.IsTemplate())
	}
}

func TestCreateShardedKSMHashConfigs_RejectsNonHashConfig(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	// shouldShard declined this config, so reaching createShardedConfigs means
	// it changed underneath us. Erroring routes it to unsharded scheduling
	// rather than silently hash-sharding something that never asked for it.
	_, err := manager.createShardedKSMHashConfigs(createKSMConfig([]string{"pods", "nodes"}))
	assert.Error(t, err)
}

// TestCreateShardedKSMHashConfigs_NonSuppressingShardableStillDispatches pins
// the diagnostic's contract on the hash path: a shardable that fails to
// suppress its own .total alongside a cluster_aggregates_only instance is
// reported, not refused: refusing falls through to unsharded scheduling, which
// still double-counts .total (2x rather than N+1x) and loses the sharding too.
// Severity must be the certain one, since every hash shard carries the full
// collector set rather than only the pods shard carrying pods.
func TestCreateShardedKSMHashConfigs_NonSuppressingShardableStillDispatches(t *testing.T) {
	manager := newKSMHashShardingManager(true)

	config := createKSMHashConfig(map[string]interface{}{
		// No cluster_unassigned/cluster_aggregates_enabled pair, so every shard
		// emits its own .total on top of the aggregates instance.
		"collectors":     []string{"pods", "nodes"},
		"shard_criteria": []string{"namespace"},
		"shard_count":    3,
	})
	config.Instances = append(config.Instances,
		integration.Data("pod_collection_mode: "+clusterAggregatesOnlyMode))

	shards, err := manager.createShardedKSMHashConfigs(config)
	require.NoError(t, err, "the misconfiguration is logged, not fatal")
	require.Len(t, shards, 4, "3 shards plus the standalone aggregates config")
}
