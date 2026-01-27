// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestEvictionManager_ShouldEvict_CountThreshold tests count-based eviction triggers
func TestEvictionManager_ShouldEvict_CountThreshold(t *testing.T) {
	em := &EvictionManager{
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
		patternCount     int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "below both thresholds",
			patternCount:     500,
			estimatedBytes:   500_000,
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "exactly at count high watermark",
			patternCount:     900, // 1000 * 0.9
			estimatedBytes:   500_000,
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "just above count high watermark",
			patternCount:     901,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "at max count",
			patternCount:     1000,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "over max count",
			patternCount:     1100,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := em.ShouldEvict(tt.patternCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestEvictionManager_ShouldEvict_MemoryThreshold tests memory-based eviction triggers
func TestEvictionManager_ShouldEvict_MemoryThreshold(t *testing.T) {
	em := &EvictionManager{
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
		patternCount     int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "exactly at memory high watermark",
			patternCount:     500,
			estimatedBytes:   900_000, // 1_000_000 * 0.9
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "just above memory high watermark",
			patternCount:     500,
			estimatedBytes:   900_001,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "at max memory",
			patternCount:     500,
			estimatedBytes:   1_000_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "over max memory",
			patternCount:     500,
			estimatedBytes:   1_100_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := em.ShouldEvict(tt.patternCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestEvictionManager_ShouldEvict_BothThresholds tests when both thresholds are exceeded
func TestEvictionManager_ShouldEvict_BothThresholds(t *testing.T) {
	em := &EvictionManager{
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

// TestEvictionManager_EvictionTargets_CountBased tests count-based eviction target calculations
func TestEvictionManager_EvictionTargets_CountBased(t *testing.T) {
	em := &EvictionManager{
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
		patternCount      int
		estimatedBytes    int64
		countOverLimit    bool
		bytesOverLimit    bool
		expectNumToEvict  int
		expectBytesToFree int64
		expectStrategy    eviction.Strategy
	}{
		{
			name:              "count over limit - evict to low watermark",
			patternCount:      950,
			estimatedBytes:    500_000,
			countOverLimit:    true,
			bytesOverLimit:    false,
			expectNumToEvict:  250, // 950 - (1000 * 0.7)
			expectBytesToFree: 0,
			expectStrategy:    eviction.StrategyByCount,
		},
		{
			name:              "count at max - evict to low watermark",
			patternCount:      1000,
			estimatedBytes:    500_000,
			countOverLimit:    true,
			bytesOverLimit:    false,
			expectNumToEvict:  300, // 1000 - 700
			expectBytesToFree: 0,
			expectStrategy:    eviction.StrategyByCount,
		},
		{
			name:              "count barely over - evict at least 1",
			patternCount:      701,
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
				tt.patternCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectNumToEvict, numToEvict, "numToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestEvictionManager_EvictionTargets_MemoryBased tests memory-based eviction target calculations
func TestEvictionManager_EvictionTargets_MemoryBased(t *testing.T) {
	em := &EvictionManager{
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
		patternCount      int
		estimatedBytes    int64
		countOverLimit    bool
		bytesOverLimit    bool
		expectNumToEvict  int
		expectBytesToFree int64
		expectStrategy    eviction.Strategy
	}{
		{
			name:              "memory over limit - evict to low watermark",
			patternCount:      500,
			estimatedBytes:    950_000,
			countOverLimit:    false,
			bytesOverLimit:    true,
			expectNumToEvict:  0,
			expectBytesToFree: 250_000, // 950_000 - (1_000_000 * 0.7)
			expectStrategy:    eviction.StrategyByBytes,
		},
		{
			name:              "memory at max - evict to low watermark",
			patternCount:      500,
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
				tt.patternCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectNumToEvict, numToEvict, "numToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestEvictionManager_EvictionTargets_PrioritizeMemory tests that memory eviction takes priority
func TestEvictionManager_EvictionTargets_PrioritizeMemory(t *testing.T) {
	em := &EvictionManager{
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

// TestEvictionManager_Evict_Integration tests the full eviction flow
func TestEvictionManager_Evict_Integration(t *testing.T) {
	em := &EvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          100,
			MaxMemoryBytes:        10_000,
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	cm := NewClusterManager()
	now := time.Now()

	// Add 100 unique patterns (at count limit)
	for i := 0; i < 100; i++ {
		tl := token.NewTokenList()
		// Make each pattern unique
		tl.Add(token.NewToken(token.TokenWord, fmt.Sprintf("pattern-%d", i), token.NotWildcard))
		pattern, _, _, _ := cm.Add(tl)

		// Make some patterns more valuable (higher scores)
		if i < 50 {
			pattern.LogCount = 100 // Low score
			pattern.CreatedAt = now.Add(-10 * 24 * time.Hour)
			pattern.LastAccessAt = now.Add(-9 * 24 * time.Hour)
		} else {
			pattern.LogCount = 10000 // High score
			pattern.CreatedAt = now.Add(-1 * 24 * time.Hour)
			pattern.LastAccessAt = now
		}
	}

	initialCount := cm.PatternCount()
	require.Equal(t, 100, initialCount)

	// Trigger count-based eviction
	countLimit, bytesLimit := em.ShouldEvict(initialCount, cm.EstimatedBytes())
	require.True(t, countLimit, "count limit should be exceeded")

	// Perform eviction
	_ = em.Evict(cm, initialCount, cm.EstimatedBytes(), countLimit, bytesLimit)

	// Verify patterns were evicted
	finalCount := cm.PatternCount()
	assert.Less(t, finalCount, initialCount, "patterns should be evicted")
	assert.LessOrEqual(t, finalCount, 70, "should evict to around low watermark (70)")

	// Verify we're below the limit now
	countLimit, _ = em.ShouldEvict(finalCount, cm.EstimatedBytes())
	assert.False(t, countLimit, "should be below count limit after eviction")
}

// TestEvictionManager_Evict_MemoryBased tests memory-based eviction
func TestEvictionManager_Evict_MemoryBased(t *testing.T) {
	em := &EvictionManager{
		Manager: &eviction.Manager{
			MaxItemCount:          1000,
			MaxMemoryBytes:        5_000, // Low memory limit
			EvictionHighWatermark: 0.9,
			EvictionLowWatermark:  0.7,
			AgeDecayFactor:        0.5,
		},
	}

	cm := NewClusterManager()
	now := time.Now()

	// Add unique patterns until memory is exceeded
	for i := 0; i < 50; i++ {
		tl := token.NewTokenList()
		// Make each pattern unique
		tl.Add(token.NewToken(token.TokenWord, fmt.Sprintf("test_pattern_with_long_name_for_memory_%d", i), token.NotWildcard))
		pattern, _, _, _ := cm.Add(tl)

		// Vary scores
		if i%2 == 0 {
			pattern.LogCount = 100
			pattern.CreatedAt = now.Add(-10 * 24 * time.Hour)
			pattern.LastAccessAt = now.Add(-9 * 24 * time.Hour)
		} else {
			pattern.LogCount = 10000
			pattern.CreatedAt = now
			pattern.LastAccessAt = now
		}
	}

	initialBytes := cm.EstimatedBytes()

	// Check if we're over memory limit
	_, bytesLimit := em.ShouldEvict(cm.PatternCount(), initialBytes)
	if !bytesLimit {
		t.Skip("Not over memory limit, skipping memory eviction test")
	}

	// Perform eviction
	_ = em.Evict(cm, cm.PatternCount(), initialBytes, false, bytesLimit)

	// Verify memory was freed
	finalBytes := cm.EstimatedBytes()
	assert.Less(t, finalBytes, initialBytes, "memory should decrease after eviction")
}
