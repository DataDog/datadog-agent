// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
)

// TestTagEvictionManager_ShouldEvict_CountThreshold tests count-based eviction triggers
func TestTagEvictionManager_ShouldEvict_CountThreshold(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tests := []struct {
		name             string
		tagCount         int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "below both thresholds",
			tagCount:         500,
			estimatedBytes:   500_000,
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "exactly at count high watermark",
			tagCount:         900, // 1000 * 0.9
			estimatedBytes:   500_000,
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "just above count high watermark",
			tagCount:         901,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "at max count",
			tagCount:         1000,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "over max count",
			tagCount:         1100,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := em.ShouldEvict(tt.tagCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestTagEvictionManager_ShouldEvict_MemoryThreshold tests memory-based eviction triggers
func TestTagEvictionManager_ShouldEvict_MemoryThreshold(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tests := []struct {
		name             string
		tagCount         int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "exactly at memory high watermark",
			tagCount:         500,
			estimatedBytes:   900_000, // 1_000_000 * 0.9
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "just above memory high watermark",
			tagCount:         500,
			estimatedBytes:   900_001,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "at max memory",
			tagCount:         500,
			estimatedBytes:   1_000_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "over max memory",
			tagCount:         500,
			estimatedBytes:   1_100_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := em.ShouldEvict(tt.tagCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestTagEvictionManager_ShouldEvict_BothThresholds tests when both thresholds are exceeded
func TestTagEvictionManager_ShouldEvict_BothThresholds(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	// Both thresholds exceeded
	countLimit, bytesLimit := em.ShouldEvict(1000, 1_000_000)
	assert.True(t, countLimit, "count limit should be true")
	assert.True(t, bytesLimit, "bytes limit should be true")
}

// TestTagEvictionManager_EvictionTargets_CountBased tests count-based eviction target calculations
func TestTagEvictionManager_EvictionTargets_CountBased(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tests := []struct {
		name              string
		tagCount          int
		estimatedBytes    int64
		countOverLimit    bool
		bytesOverLimit    bool
		expectNumToEvict  int
		expectBytesToFree int64
		expectStrategy    eviction.Strategy
	}{
		{
			name:              "count over limit - evict to low watermark",
			tagCount:          950,
			estimatedBytes:    500_000,
			countOverLimit:    true,
			bytesOverLimit:    false,
			expectNumToEvict:  250, // 950 - (1000 * 0.7)
			expectBytesToFree: 0,
			expectStrategy:    eviction.StrategyByCount,
		},
		{
			name:              "count at max - evict to low watermark",
			tagCount:          1000,
			estimatedBytes:    500_000,
			countOverLimit:    true,
			bytesOverLimit:    false,
			expectNumToEvict:  300, // 1000 - 700
			expectBytesToFree: 0,
			expectStrategy:    eviction.StrategyByCount,
		},
		{
			name:              "count barely over - evict at least 1",
			tagCount:          701,
			estimatedBytes:    500_000,
			countOverLimit:    true,
			bytesOverLimit:    false,
			expectNumToEvict:  1, // min(701 - 700, 1)
			expectBytesToFree: 0,
			expectStrategy:    eviction.StrategyByCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numToEvict, bytesToFree, strategy := em.EvictionTargets(
				tt.tagCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectNumToEvict, numToEvict, "numToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestTagEvictionManager_EvictionTargets_MemoryBased tests memory-based eviction target calculations
func TestTagEvictionManager_EvictionTargets_MemoryBased(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tests := []struct {
		name              string
		tagCount          int
		estimatedBytes    int64
		countOverLimit    bool
		bytesOverLimit    bool
		expectNumToEvict  int
		expectBytesToFree int64
		expectStrategy    eviction.Strategy
	}{
		{
			name:              "memory over limit - evict to low watermark",
			tagCount:          500,
			estimatedBytes:    950_000,
			countOverLimit:    false,
			bytesOverLimit:    true,
			expectNumToEvict:  0,
			expectBytesToFree: 250_000, // 950_000 - (1_000_000 * 0.7)
			expectStrategy:    eviction.StrategyByBytes,
		},
		{
			name:              "memory at max - evict to low watermark",
			tagCount:          500,
			estimatedBytes:    1_000_000,
			countOverLimit:    false,
			bytesOverLimit:    true,
			expectNumToEvict:  0,
			expectBytesToFree: 300_000, // 1_000_000 - 700_000
			expectStrategy:    eviction.StrategyByBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numToEvict, bytesToFree, strategy := em.EvictionTargets(
				tt.tagCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectNumToEvict, numToEvict, "numToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestTagEvictionManager_EvictionTargets_PrioritizeMemory tests that memory eviction takes priority
func TestTagEvictionManager_EvictionTargets_PrioritizeMemory(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	// Both limits exceeded - memory should take priority
	numToEvict, bytesToFree, strategy := em.EvictionTargets(1000, 1_000_000, true, true)

	assert.Equal(t, 0, numToEvict, "numToEvict should be 0 when evicting by memory")
	assert.Equal(t, int64(300_000), bytesToFree, "should evict to memory low watermark")
	assert.Equal(t, eviction.StrategyByBytes, strategy, "should prioritize memory-based eviction")
}

// TestTagEvictionManager_Evict_Integration tests the full eviction flow
func TestTagEvictionManager_Evict_Integration(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          100,
			MaxMemoryBytes:        10_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tm := NewTagManager()

	// Add 100 tags (at count limit)
	for i := 0; i < 100; i++ {
		tag := fmt.Sprintf("tag-%d", i)
		tm.AddString(tag)
	}

	initialCount := tm.Count()
	require.Equal(t, 100, initialCount)

	// Manually adjust some tags to have low scores (simulate old, rarely used tags)
	tm.mu.Lock()
	now := time.Now()
	idx := 0
	for _, entry := range tm.stringToEntry {
		if idx < 50 {
			// Low score tags
			entry.usageCount = 1
			entry.createdAt = now.Add(-30 * 24 * time.Hour)
			entry.lastAccessAt = now.Add(-29 * 24 * time.Hour)
		} else {
			// High score tags
			entry.usageCount = 1000
			entry.createdAt = now.Add(-1 * time.Hour)
			entry.lastAccessAt = now
		}
		idx++
	}
	tm.mu.Unlock()

	initialBytes := tm.EstimatedMemoryBytes()

	// Trigger count-based eviction
	countLimit, bytesLimit := em.ShouldEvict(initialCount, initialBytes)
	require.True(t, countLimit, "count limit should be exceeded")

	// Perform eviction
	em.Evict(tm, initialCount, initialBytes, countLimit, bytesLimit)

	// Verify tags were evicted
	finalCount := tm.Count()
	assert.Less(t, finalCount, initialCount, "tags should be evicted")
	assert.LessOrEqual(t, finalCount, 70, "should evict to around low watermark (70)")

	// Verify we're below the limit now
	countLimit, _ = em.ShouldEvict(finalCount, tm.EstimatedMemoryBytes())
	assert.False(t, countLimit, "should be below count limit after eviction")
}

// TestTagEvictionManager_Evict_MemoryBased tests memory-based eviction
func TestTagEvictionManager_Evict_MemoryBased(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        3_000, // Low memory limit
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tm := NewTagManager()
	now := time.Now()

	// Add tags with long names to consume memory
	for i := 0; i < 50; i++ {
		tag := fmt.Sprintf("very-long-tag-name-to-consume-more-memory-%d", i)
		tm.AddString(tag)
	}

	// Adjust scores manually
	tm.mu.Lock()
	idx := 0
	for _, entry := range tm.stringToEntry {
		if idx%2 == 0 {
			// Low score
			entry.usageCount = 1
			entry.createdAt = now.Add(-30 * 24 * time.Hour)
			entry.lastAccessAt = now.Add(-29 * 24 * time.Hour)
		} else {
			// High score
			entry.usageCount = 1000
			entry.createdAt = now
			entry.lastAccessAt = now
		}
		idx++
	}
	tm.mu.Unlock()

	initialBytes := tm.EstimatedMemoryBytes()

	// Check if we're over memory limit
	_, bytesLimit := em.ShouldEvict(tm.Count(), initialBytes)
	if !bytesLimit {
		t.Skip("Not over memory limit, skipping memory eviction test")
	}

	// Perform eviction
	em.Evict(tm, tm.Count(), initialBytes, false, bytesLimit)

	// Verify memory was freed
	finalBytes := tm.EstimatedMemoryBytes()
	assert.Less(t, finalBytes, initialBytes, "memory should decrease after eviction")
}

// TestTagEvictionManager_Evict_NoEvictionNeeded tests that nothing happens when no eviction is needed
func TestTagEvictionManager_Evict_NoEvictionNeeded(t *testing.T) {
	em := &TagEvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        1_000_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	tm := NewTagManager()

	// Add only 10 tags (well below threshold)
	for i := 0; i < 10; i++ {
		tm.AddString(fmt.Sprintf("tag-%d", i))
	}

	initialCount := tm.Count()
	initialBytes := tm.EstimatedMemoryBytes()

	// Should not trigger eviction
	countLimit, bytesLimit := em.ShouldEvict(initialCount, initialBytes)
	assert.False(t, countLimit, "count limit should not be exceeded")
	assert.False(t, bytesLimit, "bytes limit should not be exceeded")

	// Call evict anyway (should be a no-op)
	em.Evict(tm, initialCount, initialBytes, false, false)

	// Verify nothing changed
	assert.Equal(t, initialCount, tm.Count(), "count should not change")
	assert.Equal(t, initialBytes, tm.EstimatedMemoryBytes(), "memory should not change")
}
