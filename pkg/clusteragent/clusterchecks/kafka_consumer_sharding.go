// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kafkaConsumerCheckName = "kafka_consumer"
	// Instance options of the kafka_consumer check: the check only collects the
	// consumer groups whose stable hash maps to its index.
	kafkaConsumerShardCountKey = "consumer_groups_shard_count"
	kafkaConsumerShardIndexKey = "consumer_groups_shard_index"
)

// kafkaConsumerShardingManager splits a kafka_consumer instance that sets
// consumer_groups_shard_count into one config per shard index, so a Kafka
// cluster's consumer groups are collected by several cluster check runners.
// The check does the per-group filtering; this only fans the config out, so
// every shard of a set is generated from the same instance and cannot drift.
type kafkaConsumerShardingManager struct {
	enabled bool
}

func newKafkaConsumerShardingManager(enabled bool) *kafkaConsumerShardingManager {
	return &kafkaConsumerShardingManager{enabled: enabled}
}

func (m *kafkaConsumerShardingManager) isEnabled() bool {
	return m.enabled
}

// label satisfies shardingStrategy.
func (m *kafkaConsumerShardingManager) label() string {
	return "consumer-group-sharded kafka_consumer"
}

// shouldShard satisfies shardingStrategy. It claims a cluster check with a single
// instance that sets a shard count above 1 and leaves the shard index to us; an
// instance that already sets an index was split by hand and runs as-is.
func (m *kafkaConsumerShardingManager) shouldShard(config integration.Config) bool {
	if config.Name != kafkaConsumerCheckName || !config.ClusterCheck || config.LogsConfig != nil {
		return false
	}
	if len(config.Instances) != 1 {
		return false
	}
	count, hasIndex, err := parseKafkaConsumerShardOptions(config.Instances[0])
	if err != nil {
		log.Warnf("kafka_consumer sharding disabled for %s: %v", config.Digest(), err)
		return false
	}
	return count > 1 && !hasIndex
}

// createShardedConfigs satisfies shardingStrategy.
func (m *kafkaConsumerShardingManager) createShardedConfigs(config integration.Config) ([]integration.Config, error) {
	count, _, err := parseKafkaConsumerShardOptions(config.Instances[0])
	if err != nil {
		return nil, err
	}

	var instance map[string]interface{}
	if err := yaml.Unmarshal(config.Instances[0], &instance); err != nil {
		return nil, fmt.Errorf("failed to parse kafka_consumer instance: %w", err)
	}

	shards := make([]integration.Config, 0, count)
	for index := range count {
		instance[kafkaConsumerShardIndexKey] = index
		data, err := yaml.Marshal(instance)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize kafka_consumer shard %d: %w", index, err)
		}
		shard := config
		shard.Instances = []integration.Data{integration.Data(data)}
		shards = append(shards, shard)
	}
	return shards, nil
}

// parseKafkaConsumerShardOptions returns the instance's shard count (0 when unset)
// and whether it sets a shard index.
func parseKafkaConsumerShardOptions(instance integration.Data) (count int, hasIndex bool, err error) {
	var options struct {
		Count *int `yaml:"consumer_groups_shard_count"`
		Index *int `yaml:"consumer_groups_shard_index"`
	}
	if err := yaml.Unmarshal(instance, &options); err != nil {
		return 0, false, fmt.Errorf("failed to parse %s/%s: %w", kafkaConsumerShardCountKey, kafkaConsumerShardIndexKey, err)
	}
	if options.Count != nil {
		count = *options.Count
	}
	return count, options.Index != nil, nil
}
