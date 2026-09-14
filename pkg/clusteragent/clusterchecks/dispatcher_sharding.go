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

// shardingStrategy is implemented by ksmShardingManager and instanceShardingManager,
// letting the dispatcher glue below try each strategy the same way instead of
// branching on which check name routes to which one.
type shardingStrategy interface {
	isEnabled() bool
	shouldShard(config integration.Config) bool
	createShardedConfigs(config integration.Config) ([]integration.Config, error)
	label() string
}

// prepareShardSchedule expands config into its sharded pieces if a sharding
// strategy claims it. handled=true means the caller must not fall through to
// plain scheduling.
func (d *dispatcher) prepareShardSchedule(config integration.Config) (shards []integration.Config, handled bool) {
	for _, strategy := range d.shardingStrategies {
		if !strategy.isEnabled() || !strategy.shouldShard(config) {
			continue
		}

		if d.shards.isTracked(config.Digest()) {
			log.Debugf("Config %s already sharded, skipping re-sharding", config.Digest())
			return nil, true
		}

		rawShards, err := strategy.createShardedConfigs(config)
		if err != nil {
			log.Warnf("Failed to create %s configs for %s: %v, falling back to normal scheduling", strategy.label(), config.Name, err)
			return nil, false
		}

		patched, digests := d.patchShards(rawShards, strategy.label())
		if len(digests) == 0 {
			log.Warnf("Sharding enabled but failed to create any shards for %s, falling back to normal scheduling", config.Name)
			return nil, false
		}

		if len(digests) < len(rawShards) {
			log.Warnf("Only %d/%d %s configs for %s(%s) could be patched; the missing instance(s) will not run until fixed", len(digests), len(rawShards), strategy.label(), config.Name, config.Digest())
		} else {
			log.Debugf("Successfully split %s into %d %s configs", config.Name, len(digests), strategy.label())
		}
		d.shards.mark(config.Digest(), digests)

		return patched, true
	}

	return nil, false
}

// prepareShardUnschedule returns the shard digests to remove if config was
// previously sharded by prepareShardSchedule.
// handled=true means the caller must not fall through to plain unscheduling.
func (d *dispatcher) prepareShardUnschedule(config integration.Config) (digests []string, handled bool) {
	claimed := false
	for _, strategy := range d.shardingStrategies {
		if strategy.isEnabled() && strategy.shouldShard(config) {
			claimed = true
			break
		}
	}
	if !claimed {
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
