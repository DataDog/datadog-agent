// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EvictionManager handles pattern eviction using dual watermark system.
// It wraps the generic eviction.Manager with clustering-specific configuration.
type EvictionManager struct {
	*eviction.Manager
}

// NewEvictionManager creates a new EvictionManager from config
func NewEvictionManager() *EvictionManager {
	cfg := pkgconfigsetup.Datadog()

	return &EvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          cfg.GetInt("logs_config.patterns.max_pattern_count"),
			MaxMemoryBytes:        int64(cfg.GetInt("logs_config.patterns.max_memory_bytes")),
			EvictionHighWatermark: cfg.GetFloat64("logs_config.patterns.eviction_high_watermark"),
			EvictionLowWatermark:  cfg.GetFloat64("logs_config.patterns.eviction_low_watermark"),
			AgeDecayFactor:        cfg.GetFloat64("logs_config.patterns.age_decay_factor"),
		},
	}
}

// Evict performs eviction on the cluster manager based on which threshold was exceeded
// and returns the evicted patterns.
func (em *EvictionManager) Evict(cm *ClusterManager, patternCount int, estimatedBytes int64, countOverLimit, bytesOverLimit bool) []*Pattern {
	var evicted []*Pattern

	numToEvict, bytesToFree, strategy := em.EvictionTargets(patternCount, estimatedBytes, countOverLimit, bytesOverLimit)

	switch strategy {
	// Memory-based eviction
	case eviction.StrategyByBytes:
		evicted = cm.EvictToMemoryTarget(bytesToFree, em.AgeDecayFactor)

		highWatermarkBytes := int64(float64(em.MaxMemoryBytes) * em.EvictionHighWatermark)
		targetBytes := int64(float64(em.MaxMemoryBytes) * em.EvictionLowWatermark)
		log.Tracef("Evicted %d patterns: memory %d bytes exceeded high watermark %d bytes (%.0f%% of %d max), now targeting %d bytes (%.0f%%)",
			len(evicted), estimatedBytes, highWatermarkBytes,
			em.EvictionHighWatermark*100, em.MaxMemoryBytes,
			targetBytes, em.EvictionLowWatermark*100)

	// Log count-based eviction
	case eviction.StrategyByCount:
		evicted = cm.EvictLowestScoringPatterns(numToEvict, em.AgeDecayFactor)

		highWatermarkCount := int(float64(em.MaxItemCount) * em.EvictionHighWatermark)
		targetCount := int(float64(em.MaxItemCount) * em.EvictionLowWatermark)
		log.Tracef("Evicted %d patterns: count %d exceeded high watermark %d (%.0f%% of %d max), now targeting %d (%.0f%%)",
			len(evicted), patternCount, highWatermarkCount,
			em.EvictionHighWatermark*100, em.MaxItemCount,
			targetCount, em.EvictionLowWatermark*100)
	}
	return evicted
}
