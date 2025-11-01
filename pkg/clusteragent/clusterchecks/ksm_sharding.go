// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ResourceGroup represents a logical grouping of KSM collectors
type ResourceGroup struct {
	Name        string   // Human-readable name (pods, nodes, others)
	Collectors  []string // KSM collector names
	Description string   // Why these are grouped together
}

// KSMShardingManager handles the sharding logic for KSM checks by resource type
type KSMShardingManager struct {
	enabled bool
}

// NewKSMShardingManager creates a new KSM sharding manager
func NewKSMShardingManager(enabled bool) *KSMShardingManager {
	return &KSMShardingManager{
		enabled: enabled,
	}
}

// IsEnabled returns whether KSM sharding is enabled
func (m *KSMShardingManager) IsEnabled() bool {
	return m.enabled
}

// IsKSMCheck returns true if the config is a KSM check
func (m *KSMShardingManager) IsKSMCheck(config integration.Config) bool {
	return config.Name == "kubernetes_state_core" || config.Name == "kubernetes_state"
}

// AnalyzeKSMConfig analyzes a KSM configuration and returns collectors grouped by resource type
// Simple strategy: {pods}, {nodes}, {everything else}
func (m *KSMShardingManager) AnalyzeKSMConfig(config integration.Config) ([]ResourceGroup, error) {
	if !m.IsKSMCheck(config) {
		return nil, fmt.Errorf("not a KSM check")
	}

	// Parse the KSM configuration
	type ksmInstance struct {
		Collectors []string `yaml:"collectors"`
	}

	var instances []ksmInstance
	for _, data := range config.Instances {
		var instance ksmInstance
		if err := yaml.Unmarshal(data, &instance); err != nil {
			log.Warnf("Failed to parse KSM instance config: %v", err)
			continue
		}
		instances = append(instances, instance)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no valid KSM instances found")
	}

	instance := instances[0]

	// If no collectors specified, KSM collects everything
	// For safety, we don't shard when collecting everything
	if len(instance.Collectors) == 0 {
		log.Info("KSM config has no collectors specified (collecting all), sharding disabled for safety")
		return nil, fmt.Errorf("no collectors specified, sharding disabled")
	}

	// Categorize collectors: pods, nodes, everything else
	var podCollectors []string
	var nodeCollectors []string
	var otherCollectors []string

	for _, collector := range instance.Collectors {
		switch collector {
		case "pods":
			podCollectors = append(podCollectors, collector)
		case "nodes":
			nodeCollectors = append(nodeCollectors, collector)
		default:
			otherCollectors = append(otherCollectors, collector)
		}
	}

	// Build resource groups only for collectors that are present
	var groups []ResourceGroup

	if len(podCollectors) > 0 {
		groups = append(groups, ResourceGroup{
			Name:        "pods",
			Collectors:  podCollectors,
			Description: "Pod metrics (highest cardinality)",
		})
	}

	if len(nodeCollectors) > 0 {
		groups = append(groups, ResourceGroup{
			Name:        "nodes",
			Collectors:  nodeCollectors,
			Description: "Node metrics (high cardinality)",
		})
	}

	if len(otherCollectors) > 0 {
		groups = append(groups, ResourceGroup{
			Name:        "others",
			Collectors:  otherCollectors,
			Description: "All other resource types",
		})
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no collectors found after parsing")
	}

	return groups, nil
}

// ShouldShardKSMCheck determines if a KSM check should be sharded
func (m *KSMShardingManager) ShouldShardKSMCheck(config integration.Config) bool {
	if !m.enabled || !m.IsKSMCheck(config) {
		return false
	}

	groups, err := m.AnalyzeKSMConfig(config)
	if err != nil {
		log.Debugf("KSM sharding disabled: %v", err)
		return false
	}

	// Only shard if we have more than 1 group
	// (otherwise there's no benefit to sharding)
	if len(groups) <= 1 {
		log.Infof("KSM check has only %d resource group(s), sharding not beneficial", len(groups))
		return false
	}

	// Log the sharding decision
	log.Infof("KSM resource sharding enabled: will create %d sharded checks", len(groups))

	for _, group := range groups {
		log.Infof("  - Group '%s': %d collector(s) - %v", group.Name, len(group.Collectors), group.Collectors)
	}

	return true
}

// CreateShardedKSMConfigs creates sharded KSM configurations based on resource groups
// The number of shards adapts to the number of available runners for optimal distribution
// - 2 runners: Creates 2 shards (pods separate, others combined)
// - 3+ runners: Creates 3 shards (pods, nodes, others)
// Returns array of sharded configs
func (m *KSMShardingManager) CreateShardedKSMConfigs(
	baseConfig integration.Config,
	numRunners int,
) ([]integration.Config, error) {

	groups, err := m.AnalyzeKSMConfig(baseConfig)
	if err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no resource groups to shard")
	}

	// Adapt sharding strategy based on number of runners
	groups = m.adaptGroupsForRunners(groups, numRunners)

	var shardedConfigs []integration.Config

	// Create a config for each resource group
	for _, group := range groups {
		shardConfig := m.createKSMConfigForResourceGroup(baseConfig, group)
		shardedConfigs = append(shardedConfigs, shardConfig)
	}

	log.Infof("Created %d resource-sharded KSM configs for %d runners", len(shardedConfigs), numRunners)

	return shardedConfigs, nil
}

// adaptGroupsForRunners adapts the resource groups based on the number of available runners
// Strategy:
// - 2 runners: Combine nodes + others (keep pods separate as highest cardinality)
// - 3+ runners: Keep all 3 groups separate
func (m *KSMShardingManager) adaptGroupsForRunners(groups []ResourceGroup, numRunners int) []ResourceGroup {
	// If we have 3 or more runners, or 3 or fewer groups, use groups as-is
	if numRunners >= 3 || len(groups) <= 2 {
		return groups
	}

	// For 2 runners with 3 groups: combine nodes + others, keep pods separate
	// This optimizes for the highest cardinality (pods) getting its own runner
	var podsGroup *ResourceGroup
	var combinedCollectors []string

	for _, group := range groups {
		if group.Name == "pods" {
			podsGroup = &group
		} else {
			combinedCollectors = append(combinedCollectors, group.Collectors...)
		}
	}

	adapted := []ResourceGroup{}

	// Add pods group first (highest priority)
	if podsGroup != nil {
		adapted = append(adapted, *podsGroup)
	}

	// Add combined group for everything else
	if len(combinedCollectors) > 0 {
		adapted = append(adapted, ResourceGroup{
			Name:        "nodes_and_others",
			Collectors:  combinedCollectors,
			Description: "Nodes and other resource types (combined for 2 runners)",
		})
	}

	log.Infof("Adapted %d resource groups to %d shards for %d runners", len(groups), len(adapted), numRunners)
	return adapted
}

// createKSMConfigForResourceGroup creates a KSM config for a specific resource group
func (m *KSMShardingManager) createKSMConfigForResourceGroup(
	baseConfig integration.Config,
	group ResourceGroup,
) integration.Config {
	// Create a new config by copying fields manually
	config := integration.Config{
		Name:                    baseConfig.Name,
		InitConfig:              baseConfig.InitConfig,
		MetricConfig:            baseConfig.MetricConfig,
		LogsConfig:              baseConfig.LogsConfig,
		ADIdentifiers:           baseConfig.ADIdentifiers,
		AdvancedADIdentifiers:   baseConfig.AdvancedADIdentifiers,
		Provider:                baseConfig.Provider,
		ServiceID:               baseConfig.ServiceID,
		TaggerEntity:            baseConfig.TaggerEntity,
		ClusterCheck:            baseConfig.ClusterCheck,
		NodeName:                baseConfig.NodeName,
		Source:                  baseConfig.Source,
		IgnoreAutodiscoveryTags: baseConfig.IgnoreAutodiscoveryTags,
		MetricsExcluded:         baseConfig.MetricsExcluded,
		LogsExcluded:            baseConfig.LogsExcluded,
	}

	// Parse existing instance config
	var instance map[string]interface{}
	if len(baseConfig.Instances) > 0 {
		if err := yaml.Unmarshal(baseConfig.Instances[0], &instance); err != nil {
			log.Warnf("Failed to unmarshal KSM instance config: %v", err)
			instance = make(map[string]interface{})
		}
	} else {
		instance = make(map[string]interface{})
	}

	// Sort collectors for consistent ordering
	collectors := make([]string, len(group.Collectors))
	copy(collectors, group.Collectors)
	sort.Strings(collectors)

	// Set collectors for this group
	instance["collectors"] = collectors

	// Enable skip_leader_election for cluster checks running on CLC runners
	// This prevents the "Leader Election not enabled" error
	instance["skip_leader_election"] = true

	// Note: We intentionally do NOT add ksm_resource_group tag to metrics
	// This is an internal implementation detail that would clutter user's metrics
	// Users care about business tags (kube_namespace, pod_name, etc.), not sharding strategy
	// For debugging, operators can see shard distribution via: agent clusterchecks

	// Serialize back to YAML
	data, _ := yaml.Marshal(instance)
	config.Instances = []integration.Data{integration.Data(data)}

	return config
}

// Helper functions

func getExistingTags(instance map[string]interface{}) []string {
	if tags, ok := instance["tags"].([]string); ok {
		return tags
	}
	if tags, ok := instance["tags"].([]interface{}); ok {
		strTags := make([]string, len(tags))
		for i, tag := range tags {
			strTags[i] = fmt.Sprintf("%v", tag)
		}
		return strTags
	}
	return []string{}
}
