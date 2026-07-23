// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"errors"
	"fmt"
	"sort"

	"go.yaml.in/yaml/v2"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resourceGroup represents a logical grouping of KSM collectors
type resourceGroup struct {
	Name        string   // Human-readable name (pods, nodes, others)
	Collectors  []string // KSM collector names
	Description string   // Why these are grouped together
}

// ksmShardingManager handles the sharding logic for KSM checks by resource type
type ksmShardingManager struct {
	enabled bool
}

// newKSMShardingManager creates a new KSM sharding manager
func newKSMShardingManager(enabled bool) *ksmShardingManager {
	return &ksmShardingManager{
		enabled: enabled,
	}
}

// isEnabled returns whether KSM sharding is enabled
func (m *ksmShardingManager) isEnabled() bool {
	return m.enabled
}

// isKSMCheck returns true if the config is a KSM check
// Only kubernetes_state_core (Go implementation) is supported for sharding
// The legacy kubernetes_state (Python) check doesn't support the "collectors" parameter
func (m *ksmShardingManager) isKSMCheck(config integration.Config) bool {
	return config.Name == "kubernetes_state_core"
}

// defaultKSMCollectors returns the KSM default resource collectors with
// "endpoints" added back for backward compatibility (upstream KSM v2.18
// replaced "endpoints" with "endpointslices" in its defaults).
func defaultKSMCollectors() []string {
	collectors := options.DefaultResources.AsSlice()
	if _, found := options.DefaultResources["endpoints"]; !found {
		collectors = append(collectors, "endpoints")
	}
	return collectors
}

// clusterAggregatesOnlyMode is the pod_collection_mode of the dedicated KSM
// instance that emits only the cluster-aggregate `.total` family. It does a
// full-pod watch and must never be sharded; it is dispatched alongside the
// shards instead. Kept as a string literal to avoid importing the ksm check
// package (which is not built with the clusterchecks tag).
const clusterAggregatesOnlyMode = "cluster_aggregates_only"

// classifyKSMInstances splits a KSM config's instances into the single shardable
// instance (the combined cluster_unassigned / default check) and any pass-through
// instances (cluster_aggregates_only) that must be dispatched as-is rather than
// sharded. Returns an error unless exactly one shardable instance is present.
func classifyKSMInstances(config integration.Config) (shardable integration.Data, passthrough []integration.Data, err error) {
	type modeOnly struct {
		PodCollectionMode string `yaml:"pod_collection_mode"`
	}

	var shardables []integration.Data
	for _, data := range config.Instances {
		var mo modeOnly
		if uerr := yaml.Unmarshal(data, &mo); uerr != nil {
			log.Warnf("Failed to parse KSM instance config: %v", uerr)
			continue
		}
		if mo.PodCollectionMode == clusterAggregatesOnlyMode {
			passthrough = append(passthrough, data)
			continue
		}
		shardables = append(shardables, data)
	}

	if len(shardables) == 0 {
		return nil, nil, errors.New("no shardable KSM instance found")
	}
	if len(shardables) > 1 {
		return nil, nil, fmt.Errorf("KSM sharding supports a single shardable instance, got %d (excluding %s)", len(shardables), clusterAggregatesOnlyMode)
	}
	return shardables[0], passthrough, nil
}

// analyzeKSMConfig analyzes a KSM configuration and returns collectors grouped by resource type
// Simple strategy: {pods}, {nodes}, {everything else}
func (m *ksmShardingManager) analyzeKSMConfig(config integration.Config) ([]resourceGroup, error) {
	// Analyze only the single shardable instance; cluster_aggregates_only
	// instances are dispatched alongside the shards, not sharded.
	shardable, _, err := classifyKSMInstances(config)
	if err != nil {
		return nil, err
	}

	type ksmInstance struct {
		Collectors []string `yaml:"collectors"`
	}
	var instance ksmInstance
	if err := yaml.Unmarshal(shardable, &instance); err != nil {
		return nil, fmt.Errorf("failed to parse shardable KSM instance: %w", err)
	}

	// If no collectors specified, KSM defaults to collecting all resources (options.DefaultResources)
	// See kubernetes_state.go:Configure for the same fallback logic
	// We use the same defaults for sharding to provide a seamless experience
	var collectorsToShard []string
	if len(instance.Collectors) == 0 {
		defaultCollectors := defaultKSMCollectors()
		log.Infof("KSM config has no collectors specified. Using default collectors for sharding: %v", defaultCollectors)
		collectorsToShard = defaultCollectors
	} else {
		collectorsToShard = instance.Collectors
	}

	// Categorize collectors: pods, nodes, everything else
	var hasPods bool
	var hasNodes bool
	var otherCollectors []string

	for _, collector := range collectorsToShard {
		switch collector {
		case "pods":
			hasPods = true
		case "nodes":
			hasNodes = true
		default:
			otherCollectors = append(otherCollectors, collector)
		}
	}

	// Build resource groups only for collectors that are present
	var groups []resourceGroup

	if hasPods {
		groups = append(groups, resourceGroup{
			Name:        "pods",
			Collectors:  []string{"pods"},
			Description: "Pod metrics (highest cardinality)",
		})
	}

	if hasNodes {
		groups = append(groups, resourceGroup{
			Name:        "nodes",
			Collectors:  []string{"nodes"},
			Description: "Node metrics (high cardinality)",
		})
	}

	if len(otherCollectors) > 0 {
		groups = append(groups, resourceGroup{
			Name:        "others",
			Collectors:  otherCollectors,
			Description: "All other resource types",
		})
	}

	if len(groups) == 0 {
		return nil, errors.New("no collectors found after parsing")
	}

	return groups, nil
}

// shouldShardKSMCheck determines if a KSM check should be sharded
func (m *ksmShardingManager) shouldShardKSMCheck(config integration.Config) bool {
	if !m.enabled || !m.isKSMCheck(config) {
		return false
	}
	// Sharding only makes sense for cluster checks (dispatched to CLC runners)
	// If ClusterCheck is false, the check runs locally on the DCA and doesn't need sharding
	if !config.ClusterCheck {
		log.Warnf("KSM sharding requires cluster_check: true, but got cluster_check: false")
		return false
	}

	groups, err := m.analyzeKSMConfig(config)
	if err != nil {
		log.Warnf("KSM sharding disabled: %v", err)
		return false
	}

	// Only shard if we have more than 1 group
	// (otherwise there's no benefit to sharding)
	if len(groups) <= 1 {
		log.Debugf("KSM check has only %d resource group(s), sharding not beneficial", len(groups))
		return false
	}

	// Log the sharding decision
	log.Infof("KSM resource sharding enabled: will create %d sharded checks", len(groups))

	for _, group := range groups {
		log.Infof("  - Group '%s': %d collector(s) - %v", group.Name, len(group.Collectors), group.Collectors)
	}

	return true
}

// createShardedKSMConfigs creates sharded KSM configurations based on resource groups
// Creates one shard per resource group present in the config:
// - If config has pods collectors: creates pods shard
// - If config has nodes collectors: creates nodes shard
// - If config has other collectors: creates others shard
// Number of shards is independent of runner count - rebalancing handles distribution
func (m *ksmShardingManager) createShardedKSMConfigs(
	baseConfig integration.Config,
) ([]integration.Config, error) {

	shardable, passthrough, err := classifyKSMInstances(baseConfig)
	if err != nil {
		return nil, err
	}

	groups, err := m.analyzeKSMConfig(baseConfig)
	if err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return nil, errors.New("no resource groups to shard")
	}

	// Pass-through instances (cluster_aggregates_only) are not sharded — each does
	// a full-pod watch and must stay a single instance. Force skip_leader_election
	// since they run on a CLC runner, same as the shards.
	aggregateInstances := make([]integration.Data, 0, len(passthrough))
	for _, inst := range passthrough {
		var mm map[string]interface{}
		if uerr := yaml.Unmarshal(inst, &mm); uerr != nil {
			log.Warnf("Failed to unmarshal %s instance: %v", clusterAggregatesOnlyMode, uerr)
			mm = make(map[string]interface{})
		}
		mm["skip_leader_election"] = true
		data, _ := yaml.Marshal(mm)
		aggregateInstances = append(aggregateInstances, integration.Data(data))
	}

	// Always create shards (pods, nodes, others) regardless of runner count
	// Rebalancing will handle optimal distribution as runners scale up/down
	var shardedConfigs []integration.Config
	aggregatesAttached := false

	// Create a config for each resource group
	for _, group := range groups {
		shardConfig := m.createKSMConfigForResourceGroup(baseConfig, shardable, group)
		// Co-locate the aggregate instance(s) with the pods shard so a single
		// runner carries all pod-related watches (tiny unassigned-pod watch +
		// full-pod aggregate watch), keeping nodes/others on other runners.
		if group.Name == "pods" && len(aggregateInstances) > 0 {
			shardConfig.Instances = append(shardConfig.Instances, aggregateInstances...)
			aggregatesAttached = true
		}
		shardedConfigs = append(shardedConfigs, shardConfig)
	}

	// Fallback: no pods group to attach to (shardable instance has no pods
	// collector) — dispatch the aggregate instance(s) as their own config so
	// they are not dropped.
	if !aggregatesAttached && len(aggregateInstances) > 0 {
		aggConfig := baseConfig
		aggConfig.Instances = aggregateInstances
		shardedConfigs = append(shardedConfigs, aggConfig)
	}

	log.Infof("Created %d resource-sharded KSM configs", len(shardedConfigs))

	return shardedConfigs, nil
}

// createKSMConfigForResourceGroup creates a KSM config for a specific resource group
func (m *ksmShardingManager) createKSMConfigForResourceGroup(
	baseConfig integration.Config,
	shardableInstance integration.Data,
	group resourceGroup,
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

	// Parse the shardable instance config (not necessarily Instances[0] — a
	// cluster_aggregates_only instance may precede it).
	var instance map[string]interface{}
	if err := yaml.Unmarshal(shardableInstance, &instance); err != nil {
		log.Warnf("Failed to unmarshal shardable KSM instance config: %v", err)
		instance = make(map[string]interface{})
	}

	// Sort collectors for consistent ordering
	collectors := make([]string, len(group.Collectors))
	copy(collectors, group.Collectors)
	sort.Strings(collectors)

	// Set collectors for this group
	instance["collectors"] = collectors

	// Enable skip_leader_election for cluster checks running on CLC runners
	instance["skip_leader_election"] = true

	// Serialize back to YAML
	data, _ := yaml.Marshal(instance)
	config.Instances = []integration.Data{integration.Data(data)}

	return config
}
