// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags provides a thread-safe dictionary manager for encoding tag strings into dictionary indices
// for efficient storage and transmission in log pattern clustering.
package tags

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TagEvictionManager handles tag dictionary eviction using dual watermark system.
// It wraps the generic eviction.Manager with tag-specific configuration.
type TagEvictionManager struct {
	*eviction.Manager
}

// NewTagEvictionManager creates a new TagEvictionManager from config
func NewTagEvictionManager() *TagEvictionManager {
	cfg := pkgconfigsetup.Datadog()

	return &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          cfg.GetInt("logs_config.tags.max_tag_count"),
			MaxMemoryBytes:        int64(cfg.GetInt("logs_config.tags.max_memory_bytes")),
			EvictionHighWatermark: cfg.GetFloat64("logs_config.tags.eviction_high_watermark"),
			EvictionLowWatermark:  cfg.GetFloat64("logs_config.tags.eviction_low_watermark"),
			AgeDecayFactor:        cfg.GetFloat64("logs_config.tags.age_decay_factor"),
		},
	}
}

// Evict performs eviction on the tag manager based on which threshold was exceeded
func (em *TagEvictionManager) Evict(tm *TagManager, tagCount int, estimatedBytes int64, countOverLimit, bytesOverLimit bool) {
	var evictedIDs []uint64

	numToEvict, bytesToFree, strategy := em.EvictionTargets(tagCount, estimatedBytes, countOverLimit, bytesOverLimit)

	switch strategy {

	// Memory-based eviction
	case eviction.StrategyByBytes:
		evictedIDs = tm.EvictToMemoryTarget(bytesToFree, em.AgeDecayFactor)

		highWatermarkBytes := int64(float64(em.MaxMemoryBytes) * em.EvictionHighWatermark)
		targetBytes := int64(float64(em.MaxMemoryBytes) * em.EvictionLowWatermark)
		log.Infof("Evicted %d tags: memory %d bytes exceeded high watermark %d bytes (%.0f%% of %d max), now targeting %d bytes (%.0f%%)",
			len(evictedIDs), estimatedBytes, highWatermarkBytes,
			em.EvictionHighWatermark*100, em.MaxMemoryBytes,
			targetBytes, em.EvictionLowWatermark*100)

	// Log count eviction
	case eviction.StrategyByCount:
		evictedIDs = tm.EvictLowestScoringStrings(numToEvict, em.AgeDecayFactor)

		highWatermarkCount := int(float64(em.MaxItemCount) * em.EvictionHighWatermark)
		targetCount := int(float64(em.MaxItemCount) * em.EvictionLowWatermark)
		log.Infof("Evicted %d tags: count %d exceeded high watermark %d (%.0f%% of %d max), now targeting %d (%.0f%%)",
			len(evictedIDs), tagCount, highWatermarkCount,
			em.EvictionHighWatermark*100, em.MaxItemCount,
			targetCount, em.EvictionLowWatermark*100)
	}
}
