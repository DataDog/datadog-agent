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

// prepareShardSchedule expands config into its sharded pieces if a sharding
// strategy claims it: KSM's resource-group split for kubernetes_state_core,
// or the generic per-instance split for any other multi-instance config.
// handled=true means the caller must not fall through to plain scheduling
func (d *dispatcher) prepareShardSchedule(config integration.Config) (shards []integration.Config, handled bool) {
	var eligible bool
	var label string

	if config.Name == ksmCheckName {
		eligible = d.ksmSharding.isEnabled() && d.ksmSharding.shouldShardKSMCheck(config)
		label = "resource-sharded KSM"
	} else {
		eligible = d.instanceSharding.isEnabled() && d.instanceSharding.shouldShardInstances(config)
		label = "instance-sharded"
	}

	if !eligible {
		return nil, false
	}

	if d.shards.isTracked(config.Digest()) {
		log.Debugf("Config %s already sharded, skipping re-sharding", config.Digest())
		return nil, true
	}

	var rawShards []integration.Config
	if config.Name == ksmCheckName {
		var err error
		rawShards, err = d.ksmSharding.createShardedKSMConfigs(config)
		if err != nil {
			log.Warnf("Failed to create %s configs for %s: %v, falling back to normal scheduling", label, config.Name, err)
			return nil, false
		}
	} else {
		rawShards = d.instanceSharding.createShardedInstanceConfigs(config)
	}

	patched, digests := d.patchShards(rawShards, label)
	if len(digests) == 0 {
		log.Warnf("Sharding enabled but failed to create any shards for %s, falling back to normal scheduling", config.Name)
		return nil, false
	}

	if len(digests) < len(rawShards) {
		log.Warnf("Only %d/%d %s configs for %s(%s) could be patched; the missing instance(s) will not run until fixed", len(digests), len(rawShards), label, config.Name, config.Digest())
	} else {
		log.Debugf("Successfully split %s into %d %s configs", config.Name, len(digests), label)
	}
	d.shards.mark(config.Digest(), digests)

	return patched, true
}

// prepareShardUnschedule returns the shard digests to remove if config was
// previously sharded by prepareShardSchedule.
// handled=true means the caller must not fall through to plain unscheduling.
func (d *dispatcher) prepareShardUnschedule(config integration.Config) (digests []string, handled bool) {
	if config.Name == ksmCheckName {
		if !d.ksmSharding.isEnabled() {
			return nil, false
		}
	} else if !d.instanceSharding.isEnabled() {
		return nil, false
	}

	shardDigests, exists := d.shards.pop(config.Digest())
	if !exists {
		return nil, false
	}

	log.Infof("Unscheduling sharded config %s (removing %d shards)", config.Digest(), len(shardDigests))
	return shardDigests, true
}

// patchShards patches each raw shard config, logging and skipping any that
// fail to patch, and returns the successfully patched configs along with
// their digests.
func (d *dispatcher) patchShards(rawShards []integration.Config, label string) ([]integration.Config, []string) {
	patched := make([]integration.Config, 0, len(rawShards))
	digests := make([]string, 0, len(rawShards))
	for _, cfg := range rawShards {
		p, err := d.patchConfiguration(cfg)
		if err != nil {
			log.Warnf("Cannot patch %s config %s: %s", label, cfg.Digest(), err)
			continue
		}
		patched = append(patched, p)
		digests = append(digests, p.Digest())
	}
	return patched, digests
}
