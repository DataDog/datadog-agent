// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eviction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestManager_ShouldEvict_CountThreshold tests count-based threshold detection
func TestManager_ShouldEvict_CountThreshold(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	tests := []struct {
		name             string
		itemCount        int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "below both thresholds",
			itemCount:        500,
			estimatedBytes:   500_000,
			expectCountLimit: false,
			expectBytesLimit: false,
		},
		{
			name:             "exactly at count high watermark",
			itemCount:        900, // 1000 * 0.9
			estimatedBytes:   500_000,
			expectCountLimit: false, // not greater than
			expectBytesLimit: false,
		},
		{
			name:             "just above count high watermark",
			itemCount:        901,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "at max count",
			itemCount:        1000,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
		{
			name:             "over max count",
			itemCount:        1100,
			estimatedBytes:   500_000,
			expectCountLimit: true,
			expectBytesLimit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := manager.ShouldEvict(tt.itemCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestManager_ShouldEvict_MemoryThreshold tests memory-based threshold detection
func TestManager_ShouldEvict_MemoryThreshold(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	tests := []struct {
		name             string
		itemCount        int
		estimatedBytes   int64
		expectCountLimit bool
		expectBytesLimit bool
	}{
		{
			name:             "exactly at memory high watermark",
			itemCount:        500,
			estimatedBytes:   900_000, // 1_000_000 * 0.9
			expectCountLimit: false,
			expectBytesLimit: false, // not greater than
		},
		{
			name:             "just above memory high watermark",
			itemCount:        500,
			estimatedBytes:   900_001,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "at max memory",
			itemCount:        500,
			estimatedBytes:   1_000_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
		{
			name:             "over max memory",
			itemCount:        500,
			estimatedBytes:   1_100_000,
			expectCountLimit: false,
			expectBytesLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countLimit, bytesLimit := manager.ShouldEvict(tt.itemCount, tt.estimatedBytes)
			assert.Equal(t, tt.expectCountLimit, countLimit, "count limit mismatch")
			assert.Equal(t, tt.expectBytesLimit, bytesLimit, "bytes limit mismatch")
		})
	}
}

// TestManager_ShouldEvict_BothThresholds tests when both thresholds are exceeded
func TestManager_ShouldEvict_BothThresholds(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	// Both count and memory over limit
	countLimit, bytesLimit := manager.ShouldEvict(950, 950_000)
	assert.True(t, countLimit, "count should be over limit")
	assert.True(t, bytesLimit, "bytes should be over limit")
}

// TestManager_EvictionTargets_CountBased tests count-based eviction target calculation
func TestManager_EvictionTargets_CountBased(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	tests := []struct {
		name               string
		itemCount          int
		estimatedBytes     int64
		countOverLimit     bool
		bytesOverLimit     bool
		expectItemsToEvict int
		expectBytesToFree  int64
		expectStrategy     Strategy
	}{
		{
			name:               "count over limit - evict to low watermark",
			itemCount:          950,
			estimatedBytes:     500_000,
			countOverLimit:     true,
			bytesOverLimit:     false,
			expectItemsToEvict: 250, // 950 - (1000 * 0.7) = 950 - 700
			expectBytesToFree:  0,
			expectStrategy:     StrategyByCount,
		},
		{
			name:               "count at max - evict to low watermark",
			itemCount:          1000,
			estimatedBytes:     500_000,
			countOverLimit:     true,
			bytesOverLimit:     false,
			expectItemsToEvict: 300, // 1000 - 700
			expectBytesToFree:  0,
			expectStrategy:     StrategyByCount,
		},
		{
			name:               "count barely over - evict at least 1",
			itemCount:          701,
			estimatedBytes:     500_000,
			countOverLimit:     true,
			bytesOverLimit:     false,
			expectItemsToEvict: 1, // max(701 - 700, 1)
			expectBytesToFree:  0,
			expectStrategy:     StrategyByCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itemsToEvict, bytesToFree, strategy := manager.EvictionTargets(
				tt.itemCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectItemsToEvict, itemsToEvict, "itemsToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestManager_EvictionTargets_MemoryBased tests memory-based eviction target calculation
func TestManager_EvictionTargets_MemoryBased(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	tests := []struct {
		name               string
		itemCount          int
		estimatedBytes     int64
		countOverLimit     bool
		bytesOverLimit     bool
		expectItemsToEvict int
		expectBytesToFree  int64
		expectStrategy     Strategy
	}{
		{
			name:               "memory over limit - evict to low watermark",
			itemCount:          500,
			estimatedBytes:     950_000,
			countOverLimit:     false,
			bytesOverLimit:     true,
			expectItemsToEvict: 0,
			expectBytesToFree:  250_000, // 950_000 - (1_000_000 * 0.7) = 950_000 - 700_000
			expectStrategy:     StrategyByBytes,
		},
		{
			name:               "memory at max - evict to low watermark",
			itemCount:          500,
			estimatedBytes:     1_000_000,
			countOverLimit:     false,
			bytesOverLimit:     true,
			expectItemsToEvict: 0,
			expectBytesToFree:  300_000, // 1_000_000 - 700_000
			expectStrategy:     StrategyByBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itemsToEvict, bytesToFree, strategy := manager.EvictionTargets(
				tt.itemCount, tt.estimatedBytes, tt.countOverLimit, tt.bytesOverLimit)

			assert.Equal(t, tt.expectItemsToEvict, itemsToEvict, "itemsToEvict mismatch")
			assert.Equal(t, tt.expectBytesToFree, bytesToFree, "bytesToFree mismatch")
			assert.Equal(t, tt.expectStrategy, strategy, "strategy mismatch")
		})
	}
}

// TestManager_EvictionTargets_PrioritizeMemory tests that memory eviction takes priority over count
func TestManager_EvictionTargets_PrioritizeMemory(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	// Both limits exceeded - memory should take priority
	itemsToEvict, bytesToFree, strategy := manager.EvictionTargets(1000, 1_000_000, true, true)

	assert.Equal(t, 0, itemsToEvict, "itemsToEvict should be 0 when evicting by memory")
	assert.Equal(t, int64(300_000), bytesToFree, "should evict to memory low watermark")
	assert.Equal(t, StrategyByBytes, strategy, "should prioritize memory-based eviction")
}

// TestManager_EvictionTargets_NeitherLimit tests when no limits are exceeded
func TestManager_EvictionTargets_NeitherLimit(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	// Neither limit exceeded
	itemsToEvict, bytesToFree, strategy := manager.EvictionTargets(500, 500_000, false, false)

	assert.Equal(t, 0, itemsToEvict, "itemsToEvict should be 0")
	assert.Equal(t, int64(0), bytesToFree, "bytesToFree should be 0")
	assert.Equal(t, StrategyNone, strategy, "strategy should be StrategyNone")
}

// TestManager_applyWatermark tests watermark calculation
func TestManager_applyWatermark(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          1000,
		MaxMemoryBytes:        1_000_000,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	tests := []struct {
		name     string
		limit    int64
		expected int64
	}{
		{"watermark on 1000", 1000, 700},             // 1000 * 0.7
		{"watermark on 1000000", 1_000_000, 700_000}, // 1_000_000 * 0.7
		{"watermark on 100", 100, 70},                // 100 * 0.7
		{"watermark on 0", 0, 0},                     // 0 * 0.7
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.applyWatermark(tt.limit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestManager_DifferentWatermarks tests manager with different watermark values
func TestManager_DifferentWatermarks(t *testing.T) {
	tests := []struct {
		name               string
		highWatermark      float64
		lowWatermark       float64
		itemCount          int
		expectCountLimit   bool
		expectItemsToEvict int
	}{
		{
			name:               "high watermark 0.8, low 0.6",
			highWatermark:      0.8,
			lowWatermark:       0.6,
			itemCount:          850,
			expectCountLimit:   true, // 850 > 800
			expectItemsToEvict: 250,  // 850 - 600
		},
		{
			name:               "high watermark 0.95, low 0.85",
			highWatermark:      0.95,
			lowWatermark:       0.85,
			itemCount:          960,
			expectCountLimit:   true, // 960 > 950
			expectItemsToEvict: 110,  // 960 - 850
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				MaxItemCount:          1000,
				MaxMemoryBytes:        1_000_000,
				EvictionHighWatermark: tt.highWatermark,
				EvictionLowWatermark:  tt.lowWatermark,
				AgeDecayFactor:        0.5,
			}

			countLimit, _ := manager.ShouldEvict(tt.itemCount, 0)
			assert.Equal(t, tt.expectCountLimit, countLimit)

			if tt.expectCountLimit {
				itemsToEvict, _, _ := manager.EvictionTargets(tt.itemCount, 0, true, false)
				assert.Equal(t, tt.expectItemsToEvict, itemsToEvict)
			}
		})
	}
}

// TestManager_ZeroLimits tests manager with zero limits
func TestManager_ZeroLimits(t *testing.T) {
	manager := &Manager{
		MaxItemCount:          0,
		MaxMemoryBytes:        0,
		EvictionHighWatermark: 0.9,
		EvictionLowWatermark:  0.7,
		AgeDecayFactor:        0.5,
	}

	// With zero limits, nothing should trigger eviction
	countLimit, bytesLimit := manager.ShouldEvict(100, 1000)
	assert.True(t, countLimit, "any count exceeds 0")
	assert.True(t, bytesLimit, "any bytes exceed 0")
}
