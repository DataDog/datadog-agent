// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// scheduleKSMCheck handles the special case of KSM checks with resource sharding
// Returns true if the check was sharded, false if it should use normal scheduling
func (d *dispatcher) scheduleKSMCheck(config integration.Config) bool {
	if !d.ksmSharding.IsEnabled() || !d.ksmSharding.IsKSMCheck(config) {
		// Not a KSM check or sharding disabled, use normal scheduling
		return false
	}

	// KSM resource sharding requires advanced dispatching for optimal distribution
	if !d.advancedDispatching.Load() {
		log.Warnf("KSM sharding is enabled but advanced dispatching is not. Advanced dispatching is recommended for KSM sharding. Falling back to normal scheduling.")
		return false
	}

	// Check if this KSM check should be sharded
	if !d.ksmSharding.ShouldShardKSMCheck(config) {
		// KSM check but not suitable for sharding (e.g., only one resource group)
		log.Infof("KSM check %s not suitable for sharding, using normal scheduling", config.Digest())
		return false
	}

	// Check if this config was already sharded (to avoid re-sharding on every Schedule call)
	if d.isAlreadySharded(config) {
		log.Debugf("KSM config %s already sharded, skipping re-sharding", config.Digest())
		return true // Prevent the original config from being scheduled on top of shards
	}

	// Check if we have any cluster check runners available
	// IMPORTANT: Without CLC runners, sharded KSM checks cannot run and will become "dangling"
	// This prevents silent failures where sharding is enabled but no runners are deployed
	runners := d.getAvailableRunners()
	if len(runners) == 0 {
		log.Errorf("KSM sharding is enabled but no cluster check runners (CLC runners) are available.")
		log.Errorf("Enable clusterChecksRunner in your Helm chart or set clc_runner_enabled: true in agent config.")
		log.Errorf("Without CLC runners, KSM checks will NOT run. Falling back to normal scheduling to prevent silent failure.")
		return false
	}

	// With only 1 runner, sharding provides no benefit (no parallelization)
	if len(runners) == 1 {
		log.Infof("Only 1 cluster check runner available. KSM sharding requires at least 2 runners for benefit. Using normal scheduling.")
		return false
	}

	// Create sharded configs based on resource groups
	// The number of shards will adapt to the number of available runners
	log.Infof("Creating resource-sharded KSM configs for %d CLC runners", len(runners))
	shardedConfigs, err := d.ksmSharding.CreateShardedKSMConfigs(config, len(runners))
	if err != nil {
		log.Warnf("Failed to create resource-sharded KSM configs: %v, falling back to normal scheduling", err)
		return false
	}

	log.Infof("Created %d resource-sharded KSM configs", len(shardedConfigs))

	// Schedule resource-sharded configs using dispatcher's logic
	// Advanced dispatching will distribute based on its algorithm
	totalSharded := 0
	shardedDigests := make([]string, 0, len(shardedConfigs))

	for _, cfg := range shardedConfigs {
		patchedCfg, err := d.patchConfiguration(cfg)
		if err != nil {
			log.Warnf("Cannot patch resource-sharded KSM config %s: %s", cfg.Digest(), err)
			continue
		}

		// Use d.add() which handles node selection and logging
		if d.add(patchedCfg) {
			totalSharded++
			shardedDigests = append(shardedDigests, patchedCfg.Digest())
		}
	}

	if totalSharded > 0 {
		log.Infof("Successfully sharded KSM check into %d resource-grouped checks", totalSharded)
		if d.advancedDispatching.Load() {
			log.Infof("Advanced dispatching enabled - configs will be rebalanced periodically")
		}
	} else {
		log.Warnf("KSM sharding enabled but no checks were distributed - check runner availability")
	}

	// Store that we've sharded this config to avoid re-sharding
	d.markAsSharded(config, shardedDigests)

	return true
}

// isAlreadySharded checks if a KSM config was already sharded
// Thread-safe: protected by ksmShardingMutex
func (d *dispatcher) isAlreadySharded(config integration.Config) bool {
	d.ksmShardingMutex.Lock()
	defer d.ksmShardingMutex.Unlock()

	if d.ksmShardedConfig.Name != "" && d.ksmShardedConfig.Digest() == config.Digest() {
		return true
	}
	return false
}

// markAsSharded marks a KSM config as having been sharded and tracks the sharded digests
// Thread-safe: protected by ksmShardingMutex
func (d *dispatcher) markAsSharded(config integration.Config, shardedDigests []string) {
	d.ksmShardingMutex.Lock()
	defer d.ksmShardingMutex.Unlock()

	d.ksmShardedConfig = config
	d.ksmShardedDigests = shardedDigests
}

// unscheduleKSMCheck removes all sharded KSM configs if this is a sharded KSM check
// Returns true if sharded configs were removed, false otherwise
// Thread-safe: protected by ksmShardingMutex
func (d *dispatcher) unscheduleKSMCheck(config integration.Config) bool {
	if !d.ksmSharding.IsEnabled() || !d.ksmSharding.IsKSMCheck(config) {
		return false
	}

	if !d.isAlreadySharded(config) {
		return false
	}

	// Atomically read the digests and clear tracking
	// We must hold the lock to avoid TOCTOU race between checking and reading digests
	d.ksmShardingMutex.Lock()
	digestsCopy := make([]string, len(d.ksmShardedDigests))
	copy(digestsCopy, d.ksmShardedDigests)
	digestCount := len(digestsCopy)

	// Clear the tracking while holding the lock
	d.ksmShardedConfig = integration.Config{}
	d.ksmShardedDigests = nil
	d.ksmShardingMutex.Unlock()

	log.Infof("Unscheduling sharded KSM check %s (removing %d shards)", config.Digest(), digestCount)

	// Remove all sharded configs (outside the lock since this is expensive)
	for _, digest := range digestsCopy {
		log.Debugf("Removing KSM shard with digest %s", digest)
		d.removeConfig(digest)
	}

	log.Infof("Successfully unscheduled all sharded KSM configs")
	return true
}

// getAvailableRunners returns list of available cluster check runner node names
// Only returns nodes with NodeTypeCLCRunner, filtering out NodeTypeNodeAgent
func (d *dispatcher) getAvailableRunners() []string {
	d.store.RLock()
	defer d.store.RUnlock()

	var runners []string
	for nodeName, node := range d.store.nodes {
		if node.nodetype == cctypes.NodeTypeCLCRunner {
			runners = append(runners, nodeName)
		}
	}
	return runners
}

// validateClusterTaggerForKSM validates that cluster tagger is enabled for KSM resource sharding.
// Returns true if cluster tagger is enabled, false if disabled.
// Logs warnings if cluster tagger is disabled, informing users about missing cross-resource tags.
// Sharding always proceeds regardless of return value - this only informs the caller of the status.
func (d *dispatcher) validateClusterTaggerForKSM() bool {
	// When KSM is sharded by resource type, the Cluster Tagger provides cross-resource tags:
	// - Pod metrics get deployment/replicaset/daemonset/statefulset tags
	// - All metrics get namespace-level tags (team, env, etc.)
	//
	// The Cluster Tagger provides this by:
	// 1. Workloadmeta collects pods/deployments/replicasets in Cluster Agent
	// 2. Cluster Tagger generates owner relationships and tags
	// 3. CLC Runners query tags via Remote Tagger
	//
	// The key config that triggers pod/deployment collection is:
	// cluster_agent.collect_kubernetes_tags
	//
	// Sharding works WITHOUT cluster tagger, but cross-resource owner tags will be missing.
	// These features work independently of cluster tagger:
	// - podLabelsAsTags (from kubernetes_pod_labels_as_tags)
	// - Namespace-level tags (from kubernetesResourcesLabelsAsTags)

	clusterTaggerEnabled := pkgconfigsetup.Datadog().GetBool("cluster_agent.collect_kubernetes_tags")

	if !clusterTaggerEnabled {
		log.Info("cluster_agent.collect_kubernetes_tags is disabled. KSM sharding will proceed but some cross-resource owner tags will be missing.")
		// Note: We proceed with sharding even without cluster tagger.
		// Basic KSM monitoring and namespace-level correlation will still work.
	} else {
		log.Info("KSM resource sharding is enabled with cluster tagger support")
	}
	return clusterTaggerEnabled
}
