// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
	// Without it: no rebalancing occurs and all shards might be placed on the same runner
	if !d.advancedDispatching.Load() {
		log.Warnf("KSM sharding is enabled but advanced dispatching is disabled. Advanced dispatching is required for proper shard distribution and rebalancing. Falling back to normal scheduling.")
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
	// Note: Even with 0 or 1 runners, we still create shards
	// Rebalancing will handle distribution as runners come online
	runnerCount, _ := d.store.CountNodeTypes()

	if runnerCount == 0 {
		log.Warnf("KSM sharding is enabled but no cluster check runners (CLC runners) are currently available.")
	}

	// Create sharded configs based on resource groups
	// Always do sharding (pods, nodes, others) regardless of current runner count
	// Rebalancing will automatically distribute shards optimally as runners scale up/down
	shardedConfigs, err := d.ksmSharding.CreateShardedKSMConfigs(config, runnerCount)
	if err != nil {
		log.Warnf("Failed to create resource-sharded KSM configs: %v, falling back to normal scheduling", err)
		return false
	}

	log.Infof("Created %d resource-sharded KSM configs", len(shardedConfigs))

	// Schedule resource-sharded configs using dispatcher's logic
	// Shards are created and tracked regardless of current runner availability
	// They will be picked up by runners as they come online (via dangling configs)
	shardedDigests := make([]string, 0, len(shardedConfigs))

	for _, cfg := range shardedConfigs {
		patchedCfg, err := d.patchConfiguration(cfg)
		if err != nil {
			log.Warnf("Cannot patch resource-sharded KSM config %s: %s", cfg.Digest(), err)
			continue
		}

		// Add the shard (will go to dangling if no runners available)
		d.add(patchedCfg)
		shardedDigests = append(shardedDigests, patchedCfg.Digest())
	}

	if len(shardedDigests) == 0 {
		log.Errorf("KSM sharding enabled but failed to create any shards - falling back to normal scheduling")
		return false
	}

	log.Infof("Successfully sharded KSM check into %d resource-grouped checks", len(shardedDigests))

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

// validateClusterTaggerForKSM validates that cluster tagger is enabled for KSM resource sharding.
// Returns true if Kubernetes resource collection is enabled, false if disabled.
// Logs warnings if disabled, informing users about potential tag limitations.
// Sharding always proceeds regardless of return value - this only informs the caller of the status.
func (d *dispatcher) validateClusterTaggerForKSM() bool {
	// KSM uses the cluster tagger only for namespace labels/annotations as tags.
	// When cluster_agent.collect_kubernetes_tags is enabled:
	// - DCA collects namespace objects into workloadmeta
	// - Cluster tagger can propagate namespace labels/annotations as tags to objects in that namespace
	//
	// Note: This only works with global tag configuration (e.g., kubernetesResourcesLabelsAsTags),
	// not with check-specific label configuration.
	//
	// KSM does NOT use the cluster tagger for pod→deployment or other cross-resource relationships.
	// Those relationships are handled by KSM itself through the Kubernetes API.
	//
	// Important: Sharding can break the label_joins option in KSM configs, because label_joins
	// requires the related objects to be collected in the same KSM instance. If your config uses
	// label_joins across different resource types (e.g., joining pod labels to node metrics),
	// those resources must be in the same shard or label_joins will not work correctly.

	collectKubernetesTags := pkgconfigsetup.Datadog().GetBool("cluster_agent.collect_kubernetes_tags")

	if !collectKubernetesTags {
		log.Info("cluster_agent.collect_kubernetes_tags is disabled. KSM sharding will proceed but namespace labels/annotations will not be propagated as tags to resources.")
		// Note: We proceed with sharding. KSM metrics will still be collected, but without
		// namespace-level label/annotation tags from the cluster tagger.
	} else {
		log.Info("KSM resource sharding is enabled with namespace tag propagation (cluster_agent.collect_kubernetes_tags)")
	}
	return collectKubernetesTags
}
