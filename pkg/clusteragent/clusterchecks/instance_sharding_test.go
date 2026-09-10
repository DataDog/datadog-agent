// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func createTestMultiInstanceConfig(name string, numInstances int) integration.Config {
	instances := make([]integration.Data, 0, numInstances)
	for i := 0; i < numInstances; i++ {
		instances = append(instances, integration.Data(fmt.Sprintf(`host: "host%d"`, i)))
	}
	return integration.Config{
		Name:         name,
		Instances:    instances,
		ClusterCheck: true,
	}
}

func TestShouldShardInstances(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		excludedChecks map[string]struct{}
		config         integration.Config
		expected       bool
	}{
		{
			name:     "multi-instance cluster check",
			config:   createTestMultiInstanceConfig("postgres", 3),
			expected: true,
		},
		{
			name:     "single instance",
			config:   createTestMultiInstanceConfig("postgres", 1),
			expected: false,
		},
		{
			name: "not a cluster check",
			config: integration.Config{
				Name:         "postgres",
				Instances:    []integration.Data{integration.Data("a"), integration.Data("b")},
				ClusterCheck: false,
			},
			expected: false,
		},
		{
			name:     "KSM check is excluded from generic sharding",
			config:   createTestMultiInstanceConfig(ksmCheckName, 3),
			expected: false,
		},
		{
			name: "config with logs section is excluded",
			config: integration.Config{
				Name:         "postgres",
				Instances:    []integration.Data{integration.Data("a"), integration.Data("b")},
				ClusterCheck: true,
				LogsConfig:   integration.Data(`source: postgres`),
			},
			expected: false,
		},
		{
			name:           "explicitly excluded check name",
			excludedChecks: map[string]struct{}{"postgres": {}},
			config:         createTestMultiInstanceConfig("postgres", 3),
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newInstanceShardingManager(true, tt.excludedChecks)
			assert.Equal(t, tt.expected, m.shouldShardInstances(tt.config))
		})
	}
}

func TestCreateShardedInstanceConfigs(t *testing.T) {
	baseConfig := createTestMultiInstanceConfig("postgres", 3)
	baseConfig.InitConfig = integration.Data("min_collection_interval: 15")

	m := newInstanceShardingManager(true, nil)
	shards := m.createShardedInstanceConfigs(baseConfig)

	assert.Len(t, shards, 3)

	digests := make(map[string]struct{}, len(shards))
	for i, shard := range shards {
		assert.Equal(t, baseConfig.Name, shard.Name)
		assert.Equal(t, baseConfig.InitConfig, shard.InitConfig)
		assert.Len(t, shard.Instances, 1)
		assert.Equal(t, baseConfig.Instances[i], shard.Instances[0])

		digest := shard.Digest()
		_, collision := digests[digest]
		assert.False(t, collision, "shard %d produced a colliding digest", i)
		digests[digest] = struct{}{}
	}
}

func TestCreateShardedInstanceConfigs_IdenticalInstancesGetDistinctDigests(t *testing.T) {
	baseConfig := integration.Config{
		Name: "postgres",
		Instances: []integration.Data{
			integration.Data(`host: "same-host"`),
			integration.Data(`host: "same-host"`),
		},
		ClusterCheck: true,
	}

	m := newInstanceShardingManager(true, nil)
	shards := m.createShardedInstanceConfigs(baseConfig)

	assert.Len(t, shards, 2)
	assert.NotEqual(t, shards[0].Digest(), shards[1].Digest())
}
