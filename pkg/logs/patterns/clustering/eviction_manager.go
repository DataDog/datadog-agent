// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EvictionManager handles pattern eviction using dual watermark system.
// It follows the DogStatsD pattern for memory-based rate limiting.
type EvictionManager struct {
	maxPatternCount       int
	maxMemoryBytes        int64
	evictionHighWatermark float64 // Trigger eviction at this threshold
	evictionLowWatermark  float64 // Evict back to this target
	ageDecayFactor        float64
}

// NewEvictionManager creates a new EvictionManager from config
func NewEvictionManager() *EvictionManager {
	cfg := pkgconfigsetup.Datadog()

	return &EvictionManager{
		maxPatternCount:       cfg.GetInt("logs_config.patterns.max_pattern_count"),
		maxMemoryBytes:        int64(cfg.GetInt("logs_config.patterns.max_memory_bytes")),
		evictionHighWatermark: cfg.GetFloat64("logs_config.patterns.eviction_high_watermark"),
		evictionLowWatermark:  cfg.GetFloat64("logs_config.patterns.eviction_low_watermark"),
		ageDecayFactor:        cfg.GetFloat64("logs_config.patterns.age_decay_factor"),
	}
}

// ShouldEvict checks if eviction should be triggered based on high watermark thresholds
func (em *EvictionManager) ShouldEvict(patternCount int, estimatedBytes int64) (bool, bool) {
	countOverLimit := float64(patternCount) > float64(em.maxPatternCount)*em.evictionHighWatermark
	bytesOverLimit := float64(estimatedBytes) > float64(em.maxMemoryBytes)*em.evictionHighWatermark
	return countOverLimit, bytesOverLimit
}

// Evict performs eviction on the cluster manager based on which threshold was exceeded
func (em *EvictionManager) Evict(cm *ClusterManager, patternCount int, estimatedBytes int64, countOverLimit, bytesOverLimit bool) {
	var evicted []*Pattern

	switch {
	case bytesOverLimit:
		// Prioritize memory eviction
		targetBytes := em.applyWatermark(em.maxMemoryBytes)
		bytesToFree := estimatedBytes - targetBytes
		evicted = cm.EvictToMemoryTarget(bytesToFree, em.ageDecayFactor)

		highWatermarkBytes := int64(float64(em.maxMemoryBytes) * em.evictionHighWatermark)
		log.Infof("Evicted %d patterns: memory %d bytes exceeded high watermark %d bytes (%.0f%% of %d max), now targeting %d bytes (%.0f%%)",
			len(evicted), estimatedBytes, highWatermarkBytes,
			em.evictionHighWatermark*100, em.maxMemoryBytes,
			targetBytes, em.evictionLowWatermark*100)

	case countOverLimit:
		// Pattern count eviction
		targetCount := int(em.applyWatermark(int64(em.maxPatternCount)))
		numToEvict := max(patternCount-targetCount, 1)
		evicted = cm.EvictLowestScoringPatterns(numToEvict, em.ageDecayFactor)

		highWatermarkCount := int(float64(em.maxPatternCount) * em.evictionHighWatermark)
		log.Infof("Evicted %d patterns: count %d exceeded high watermark %d (%.0f%% of %d max), now targeting %d (%.0f%%)",
			len(evicted), patternCount, highWatermarkCount,
			em.evictionHighWatermark*100, em.maxPatternCount,
			targetCount, em.evictionLowWatermark*100)
	}

	// TODO: Add telemetry metrics for eviction events
	// Example: telemetry.Count("patterns.evicted", len(evicted))
}

// applyWatermark applies the low watermark to a limit value
func (em *EvictionManager) applyWatermark(limit int64) int64 {
	return int64(float64(limit) * em.evictionLowWatermark)
}
