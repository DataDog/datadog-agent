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
)

func createTestKafkaConsumerConfig(instances ...map[string]interface{}) integration.Config {
	config := integration.Config{
		Name:         kafkaConsumerCheckName,
		InitConfig:   integration.Data("kafka_timeout: 60\n"),
		ClusterCheck: true,
	}
	for _, instance := range instances {
		data, _ := yaml.Marshal(instance)
		config.Instances = append(config.Instances, integration.Data(data))
	}
	return config
}

func kafkaConsumerInstance(options map[string]interface{}) map[string]interface{} {
	instance := map[string]interface{}{
		"kafka_connect_str":                "kafka:9092",
		"monitor_unlisted_consumer_groups": true,
		"tags":                             []string{"kafka_instance:example"},
	}
	for key, value := range options {
		instance[key] = value
	}
	return instance
}

func TestShouldShardKafkaConsumer(t *testing.T) {
	sharded := kafkaConsumerInstance(map[string]interface{}{"consumer_groups_shard_count": 3})

	withLogs := createTestKafkaConsumerConfig(sharded)
	withLogs.LogsConfig = integration.Data(`[{"type":"file"}]`)

	nodeAgentCheck := createTestKafkaConsumerConfig(sharded)
	nodeAgentCheck.ClusterCheck = false

	otherCheck := createTestKafkaConsumerConfig(sharded)
	otherCheck.Name = "postgres"

	tests := []struct {
		name     string
		enabled  bool
		config   integration.Config
		expected bool
	}{
		{name: "shard count set", enabled: true, config: createTestKafkaConsumerConfig(sharded), expected: true},
		{name: "sharding disabled", enabled: false, config: createTestKafkaConsumerConfig(sharded), expected: false},
		{name: "not kafka_consumer", enabled: true, config: otherCheck, expected: false},
		{name: "not a cluster check", enabled: true, config: nodeAgentCheck, expected: false},
		{name: "logs config", enabled: true, config: withLogs, expected: false},
		{name: "no shard count", enabled: true, config: createTestKafkaConsumerConfig(kafkaConsumerInstance(nil)), expected: false},
		{
			name:     "shard count of one",
			enabled:  true,
			config:   createTestKafkaConsumerConfig(kafkaConsumerInstance(map[string]interface{}{"consumer_groups_shard_count": 1})),
			expected: false,
		},
		{
			// The user already split the instance by hand: leave it alone.
			name:    "shard index already set",
			enabled: true,
			config: createTestKafkaConsumerConfig(kafkaConsumerInstance(map[string]interface{}{
				"consumer_groups_shard_count": 3, "consumer_groups_shard_index": 0,
			})),
			expected: false,
		},
		{name: "several instances", enabled: true, config: createTestKafkaConsumerConfig(sharded, sharded), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKafkaConsumerShardingManager(tt.enabled)
			assert.Equal(t, tt.expected, m.isEnabled() && m.shouldShard(tt.config))
		})
	}
}

func TestCreateShardedKafkaConsumerConfigs(t *testing.T) {
	base := createTestKafkaConsumerConfig(kafkaConsumerInstance(map[string]interface{}{"consumer_groups_shard_count": 3}))
	base.ServiceID = "file:/etc/datadog-agent/conf.d/kafka_consumer.yaml"

	shards, err := newKafkaConsumerShardingManager(true).createShardedConfigs(base)
	require.NoError(t, err)
	require.Len(t, shards, 3)

	digests := map[string]struct{}{}
	for i, shard := range shards {
		require.Len(t, shard.Instances, 1)
		var instance map[string]interface{}
		require.NoError(t, yaml.Unmarshal(shard.Instances[0], &instance))

		assert.Equal(t, i, instance["consumer_groups_shard_index"])
		assert.Equal(t, 3, instance["consumer_groups_shard_count"])
		// Everything else in the instance is carried over unchanged.
		assert.Equal(t, "kafka:9092", instance["kafka_connect_str"])
		assert.Equal(t, true, instance["monitor_unlisted_consumer_groups"])
		assert.Equal(t, []interface{}{"kafka_instance:example"}, instance["tags"])

		assert.Equal(t, base.InitConfig, shard.InitConfig)
		assert.True(t, shard.ClusterCheck)
		digests[shard.Digest()] = struct{}{}
	}
	assert.Len(t, digests, 3, "each shard must be a distinct config so it is dispatched on its own")
}

func TestPrepareKafkaConsumerShardSchedule(t *testing.T) {
	d := newTestDispatcher(false, false)
	d.shardingStrategies = append([]shardingStrategy{newKafkaConsumerShardingManager(true)}, d.shardingStrategies...)
	config := createTestKafkaConsumerConfig(kafkaConsumerInstance(map[string]interface{}{"consumer_groups_shard_count": 4}))

	shards, handled := d.prepareShardSchedule(config)
	require.True(t, handled)
	assert.Len(t, shards, 4)
	assert.True(t, d.shards.isTracked(config.Digest()))

	digests, handled := d.prepareShardUnschedule(config)
	assert.True(t, handled)
	assert.Len(t, digests, 4)
}
