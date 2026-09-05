// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// ksmCheckName is the KSM check name, which has its own dedicated resource-group
// sharding path (see ksm_sharding.go) and must not be double-processed here.
const ksmCheckName = "kubernetes_state_core"

// instanceShardingManager splits a multi-instance cluster check config into one
// single-instance config per original instance, so instances spread across
// cluster-check runners instead of always landing together on one runner.
type instanceShardingManager struct {
	enabled        bool
	excludedChecks map[string]struct{}
}

func newInstanceShardingManager(enabled bool, excludedChecks map[string]struct{}) *instanceShardingManager {
	return &instanceShardingManager{
		enabled:        enabled,
		excludedChecks: excludedChecks,
	}
}

func (m *instanceShardingManager) isEnabled() bool {
	return m.enabled
}

// shouldShardInstances returns true if config is eligible for instance sharding:
func (m *instanceShardingManager) shouldShardInstances(config integration.Config) bool {
	if !config.ClusterCheck {
		return false
	}
	if len(config.Instances) <= 1 {
		return false
	}
	if config.LogsConfig != nil {
		return false
	}
	if config.Name == ksmCheckName {
		return false
	}
	if _, excluded := m.excludedChecks[config.Name]; excluded {
		return false
	}
	return true
}

func (m *instanceShardingManager) createShardedInstanceConfigs(baseConfig integration.Config) []integration.Config {
	shardedConfigs := make([]integration.Config, 0, len(baseConfig.Instances))
	for i, instance := range baseConfig.Instances {
		shardedConfigs = append(shardedConfigs, createInstanceShardConfig(baseConfig, instance, i))
	}
	return shardedConfigs
}

func createInstanceShardConfig(baseConfig integration.Config, instance integration.Data, index int) integration.Config {
	shard := baseConfig
	shard.Instances = []integration.Data{instance}
	shard.ServiceID = fmt.Sprintf("%s#instance-shard-%d", baseConfig.ServiceID, index)
	return shard
}
