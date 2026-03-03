// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// scheduleKSMCheck handles the special case of KSM checks with resource sharding
// Returns true if the check was sharded, false if it should use normal scheduling
func (d *dispatcher) scheduleKSMCheck(config integration.Config) bool {
	if !d.ksmSharding.isEnabled() || !d.ksmSharding.isKSMCheck(config) {
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
	if !d.ksmSharding.shouldShardKSMCheck(config) {
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
	shardedConfigs, err := d.ksmSharding.createShardedKSMConfigs(config)
	if err != nil {
		log.Warnf("Failed to create resource-sharded KSM configs: %v, falling back to normal scheduling", err)
		return false
	}

	log.Debugf("Created %d resource-sharded KSM configs", len(shardedConfigs))

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

	_, exists := d.ksmShardedConfigs[config.Digest()]
	return exists
}

// markAsSharded marks a KSM config as having been sharded and tracks the sharded digests
// Thread-safe: protected by ksmShardingMutex
func (d *dispatcher) markAsSharded(config integration.Config, shardedDigests []string) {
	d.ksmShardingMutex.Lock()
	defer d.ksmShardingMutex.Unlock()

	d.ksmShardedConfigs[config.Digest()] = shardedDigests
}

// unscheduleKSMCheck removes all sharded KSM configs if this is a sharded KSM check
// Returns true if sharded configs were removed, false otherwise
// Thread-safe: protected by ksmShardingMutex
func (d *dispatcher) unscheduleKSMCheck(config integration.Config) bool {
	if !d.ksmSharding.isEnabled() || !d.ksmSharding.isKSMCheck(config) {
		return false
	}

	if !d.isAlreadySharded(config) {
		return false
	}

	// Atomically read the digests and clear tracking
	// We must hold the lock to avoid TOCTOU race between checking and reading digests
	d.ksmShardingMutex.Lock()
	defer d.ksmShardingMutex.Unlock()

	configDigest := config.Digest()
	shardDigests, exists := d.ksmShardedConfigs[configDigest]
	if !exists {
		return false
	}

	// Copy digests and remove from map while holding the lock
	digestsCopy := make([]string, len(shardDigests))
	copy(digestsCopy, shardDigests)
	digestCount := len(digestsCopy)
	delete(d.ksmShardedConfigs, configDigest)

	log.Infof("Unscheduling sharded KSM check %s (removing %d shards)", configDigest, digestCount)

	// Remove all sharded configs (outside the lock since this is expensive)
	for _, digest := range digestsCopy {
		log.Debugf("Removing KSM shard with digest %s", digest)
		d.removeConfig(digest)
	}

	log.Infof("Successfully unscheduled all sharded KSM configs")
	return true
}
