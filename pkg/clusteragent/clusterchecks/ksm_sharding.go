// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// KnownNamespaceScopedCollectors are the collectors we KNOW are namespace-scoped
// Everything else defaults to cluster-scoped for safety
var KnownNamespaceScopedCollectors = map[string]bool{
	// Core namespace-scoped resources
	"pods":                   true,
	"services":               true,
	"endpoints":              true,
	"deployments":            true,
	"replicasets":            true,
	"statefulsets":           true,
	"daemonsets":             true,
	"jobs":                   true,
	"cronjobs":               true,
	"configmaps":             true,
	"secrets":                true,
	"serviceaccounts":        true,
	"persistentvolumeclaims": true,
	"resourcequotas":         true,
	"limitranges":            true,
	"replicationcontrollers": true,

	// Networking
	"ingresses":       true,
	"networkpolicies": true,

	// Policy
	"poddisruptionbudgets": true,

	// Autoscaling (namespace-scoped version)
	"horizontalpodautoscalers": true,
	"verticalpodautoscalers":   true,

	// RBAC (namespace-scoped versions)
	"roles":        true,
	"rolebindings": true,
}

// KSMShardingManager handles the sharding logic for KSM checks
type KSMShardingManager struct {
	enabled    bool
	numBuckets int
}

// NewKSMShardingManager creates a new KSM sharding manager
func NewKSMShardingManager(enabled bool) *KSMShardingManager {
	return &KSMShardingManager{
		enabled:    enabled,
		numBuckets: 10, // Default to 10 buckets for namespace sharding
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

// IsNamespaceScopedCollector determines if a collector is namespace-scoped
func (m *KSMShardingManager) IsNamespaceScopedCollector(collector string) bool {
	// Normalize the collector name
	collector = normalizeCollectorName(collector)

	// Only return true for known namespace-scoped collectors
	// Everything else is treated as cluster-scoped (safe default)
	return KnownNamespaceScopedCollectors[collector]
}

// AnalyzeKSMConfig analyzes a KSM configuration and returns namespace-scoped and cluster-scoped collectors
func (m *KSMShardingManager) AnalyzeKSMConfig(config integration.Config) ([]string, []string, error) {
	if !m.IsKSMCheck(config) {
		return nil, nil, fmt.Errorf("not a KSM check")
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
		return nil, nil, fmt.Errorf("no valid KSM instances found")
	}

	instance := instances[0]

	// If no collectors specified, KSM collects everything
	// For safety, we don't shard when collecting everything
	if len(instance.Collectors) == 0 {
		log.Info("KSM config has no collectors specified (collecting all), sharding disabled for safety")
		return nil, nil, fmt.Errorf("no collectors specified, sharding disabled")
	}

	var namespacedCollectors []string
	var clusterCollectors []string

	// Split collectors by type
	for _, collector := range instance.Collectors {
		if m.IsNamespaceScopedCollector(collector) {
			namespacedCollectors = append(namespacedCollectors, collector)
		} else {
			// Default to cluster-scoped (safe approach)
			clusterCollectors = append(clusterCollectors, collector)

			// Only log if it's not a well-known cluster resource
			if !isWellKnownClusterResource(collector) {
				log.Debugf("Collector '%s' not in namespace-scoped list, treating as cluster-scoped", collector)
			}
		}
	}

	return namespacedCollectors, clusterCollectors, nil
}

// ShouldShardKSMCheck determines if a KSM check should be sharded
func (m *KSMShardingManager) ShouldShardKSMCheck(config integration.Config) bool {
	if !m.enabled || !m.IsKSMCheck(config) {
		return false
	}

	namespacedCollectors, clusterCollectors, err := m.AnalyzeKSMConfig(config)
	if err != nil {
		log.Debugf("KSM sharding disabled: %v", err)
		return false
	}

	// Only shard if we have namespace-scoped collectors
	if len(namespacedCollectors) == 0 {
		log.Infof("KSM check has no namespace-scoped collectors, skipping sharding")
		return false
	}

	// Log the sharding decision
	totalCollectors := len(namespacedCollectors) + len(clusterCollectors)
	shardablePercent := float64(len(namespacedCollectors)) / float64(totalCollectors) * 100

	log.Infof("KSM sharding analysis: %d/%d collectors are namespace-scoped (%.1f%% shardable)",
		len(namespacedCollectors), totalCollectors, shardablePercent)

	if len(clusterCollectors) > 0 {
		log.Infof("Cluster-scoped collectors that will run separately: %v", clusterCollectors)
	}

	return true
}

// CreateShardedKSMConfigs creates sharded KSM configurations
// Returns namespace-scoped configs and a cluster-scoped config (if needed)
func (m *KSMShardingManager) CreateShardedKSMConfigs(
	baseConfig integration.Config,
	namespaces []string,
) ([]integration.Config, integration.Config, error) {

	namespacedCollectors, clusterCollectors, err := m.AnalyzeKSMConfig(baseConfig)
	if err != nil {
		return nil, baseConfig, err
	}

	var shardedConfigs []integration.Config

	// Create cluster-wide config if we have cluster-scoped collectors
	var clusterConfig integration.Config
	if len(clusterCollectors) > 0 {
		clusterConfig = m.createKSMConfigWithCollectors(baseConfig, clusterCollectors, "", "cluster-wide")
	}

	// Create namespace-sharded configs using bucketing
	if len(namespacedCollectors) > 0 {
		if len(namespaces) == 0 {
			// Workloadmeta not ready - don't create any sharded configs
			// We'll fall back to normal (non-sharded) scheduling
			// This avoids creating empty buckets that would waste resources
			log.Infof("No namespaces available yet for KSM sharding, will retry later")
			return nil, clusterConfig, fmt.Errorf("no namespaces available for sharding")
		}
		// Group known namespaces into buckets
		buckets := m.bucketNamespaces(namespaces)

		// Log bucket distribution
		log.Infof("Bucketed %d namespaces into %d buckets:", len(namespaces), len(buckets))
		for bucketID, nsInBucket := range buckets {
			log.Debugf("  Bucket %d: %d namespaces - %v", bucketID, len(nsInBucket), nsInBucket)
		}

		// Create a config for each non-empty bucket
		for bucketID, nsInBucket := range buckets {
			if len(nsInBucket) > 0 {
				bucketConfig := m.createKSMConfigWithNamespaces(baseConfig, namespacedCollectors, nsInBucket, "namespaced", bucketID)
				shardedConfigs = append(shardedConfigs, bucketConfig)
			}
		}

		// Log if we skipped any empty buckets
		if len(shardedConfigs) < m.numBuckets && len(namespaces) >= m.numBuckets {
			log.Debugf("Created %d bucket configs out of %d possible buckets (skipped empty buckets)", len(shardedConfigs), m.numBuckets)
		}
	}

	return shardedConfigs, clusterConfig, nil
}

// bucketNamespaces groups namespaces into buckets using consistent hashing
func (m *KSMShardingManager) bucketNamespaces(namespaces []string) map[int][]string {
	buckets := make(map[int][]string)

	// Always use hash-based bucketing for consistency
	// This ensures a namespace always goes to the same bucket regardless of total namespace count
	for _, ns := range namespaces {
		bucketID := m.hashNamespaceToBucket(ns)
		buckets[bucketID] = append(buckets[bucketID], ns)
	}

	return buckets
}

// hashNamespaceToBucket uses consistent hashing to assign a namespace to a bucket
func (m *KSMShardingManager) hashNamespaceToBucket(namespace string) int {
	h := fnv.New32a()
	h.Write([]byte(namespace))
	return int(h.Sum32() % uint32(m.numBuckets))
}

// createKSMConfigWithNamespaces creates a KSM config for multiple namespaces
func (m *KSMShardingManager) createKSMConfigWithNamespaces(
	baseConfig integration.Config,
	collectors []string,
	namespaces []string,
	shardType string,
	_ int, // bucketID - removed from tags but kept for logging
) integration.Config {
	// Sort namespaces for consistent ordering
	sort.Strings(namespaces)

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

	// Set collectors
	instance["collectors"] = collectors

	// Set namespace filter for the bucket
	instance["namespaces"] = namespaces

	// Preserve existing tags only - avoid adding new tags to reduce cardinality
	tags := getExistingTags(instance)
	// Only add shard type tag for identifying the config type
	tags = append(tags, fmt.Sprintf("ksm_shard_type:%s", shardType))
	instance["tags"] = tags

	// Serialize back to YAML
	data, _ := yaml.Marshal(instance)
	config.Instances = []integration.Data{integration.Data(data)}

	return config
}

// createKSMConfigWithCollectors creates a KSM config with specific collectors and namespace
func (m *KSMShardingManager) createKSMConfigWithCollectors(
	baseConfig integration.Config,
	collectors []string,
	namespace string,
	shardType string,
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

	// Set collectors
	instance["collectors"] = collectors

	// Set namespace filter if provided
	if namespace != "" {
		instance["namespaces"] = []string{namespace}
	}

	// Add tags to identify the shard type
	tags := getExistingTags(instance)
	tags = append(tags, fmt.Sprintf("ksm_shard_type:%s", shardType))
	if namespace != "" {
		tags = append(tags, fmt.Sprintf("kube_namespace:%s", namespace))
	}
	instance["tags"] = tags

	// Serialize back to YAML
	data, _ := yaml.Marshal(instance)
	config.Instances = []integration.Data{integration.Data(data)}

	return config
}

// Helper functions

func normalizeCollectorName(collector string) string {
	collector = strings.TrimSpace(collector)

	// Handle KSM 2.x format: "apps/v1, Resource=deployments"
	// Check case-insensitively for "resource="
	lowerCollector := strings.ToLower(collector)
	if strings.Contains(lowerCollector, "resource=") {
		// Find the position case-insensitively and extract after it
		idx := strings.Index(lowerCollector, "resource=")
		if idx != -1 {
			collector = strings.TrimSpace(collector[idx+len("resource="):])
		}
	}

	// Convert to lowercase after extracting
	collector = strings.ToLower(collector)

	// Remove "_extended" suffix if present
	collector = strings.TrimSuffix(collector, "_extended")

	return collector
}

func isWellKnownClusterResource(collector string) bool {
	collector = normalizeCollectorName(collector)

	// Well-known cluster resources (for logging purposes)
	clusterResources := []string{
		"nodes", "namespaces", "persistentvolumes", "storageclasses",
		"clusterroles", "clusterrolebindings", "certificatesigningrequests",
		"volumeattachments", "apiservices", "customresourcedefinitions",
		"verticalpodautoscalers", "leases", "mutatingwebhookconfigurations",
		"validatingwebhookconfigurations", "priorityclasses", "csidrivers",
		"csinodes", "ingressclasses",
	}

	for _, cr := range clusterResources {
		if collector == cr {
			return true
		}
	}
	return false
}

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
