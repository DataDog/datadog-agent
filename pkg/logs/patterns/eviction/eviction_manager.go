// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eviction provides shared eviction scoring algorithms for patterns and tags.
package eviction

// Policy is the eviction policy type
type Policy int

const (
	// PolicyLFUDecay uses LFU with exponential age decay
	PolicyLFUDecay Policy = iota
)

// Strategy indicates which eviction method to use
type Strategy int

const (
	// StrategyNone indicates no eviction is needed
	StrategyNone Strategy = iota
	// StrategyByCount evicts a specific number of items
	StrategyByCount
	// StrategyByBytes evicts items until enough memory is freed
	StrategyByBytes
)

// Manager handles eviction using dual watermark system.
type Manager struct {
	MaxItemCount          int
	MaxMemoryBytes        int64
	EvictionHighWatermark float64 // Trigger eviction at this threshold
	EvictionLowWatermark  float64 // Evict back to this target
	AgeDecayFactor        float64
}

// ShouldEvict checks if eviction should be triggered based on high watermark thresholds.
func (m *Manager) ShouldEvict(itemCount int, estimatedBytes int64) (bool, bool) {
	countOverLimit := float64(itemCount) > float64(m.MaxItemCount)*m.EvictionHighWatermark
	bytesOverLimit := float64(estimatedBytes) > float64(m.MaxMemoryBytes)*m.EvictionHighWatermark
	return countOverLimit, bytesOverLimit
}

// EvictionTargets calculates how much to evict based on which threshold was exceeded.
func (m *Manager) EvictionTargets(itemCount int, estimatedBytes int64, countOverLimit, bytesOverLimit bool) (itemsToEvict int, bytesToFree int64, strategy Strategy) {
	switch {
	case bytesOverLimit:
		// Memory eviction
		targetBytes := m.applyWatermark(m.MaxMemoryBytes)
		bytesToFree := estimatedBytes - targetBytes
		return 0, bytesToFree, StrategyByBytes

	case countOverLimit:
		// Log count eviction
		targetCount := int(m.applyWatermark(int64(m.MaxItemCount)))
		itemsToEvict := max(itemCount-targetCount, 1)
		return itemsToEvict, 0, StrategyByCount

	default:
		return 0, 0, StrategyNone
	}
}

// applyWatermark applies the low watermark to a limit value
func (m *Manager) applyWatermark(limit int64) int64 {
	return int64(float64(limit) * m.EvictionLowWatermark)
}
