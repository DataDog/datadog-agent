// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestPattern_ImplementsEvictable verifies Pattern correctly implements the eviction.Evictable interface
func TestPattern_ImplementsEvictable(t *testing.T) {
	now := time.Now()
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "test", token.NotWildcard))

	pattern := newPattern(tl, 123)
	pattern.LogCount = 500
	pattern.CreatedAt = now.Add(-10 * 24 * time.Hour)
	pattern.LastAccessAt = now.Add(-1 * time.Hour)

	// Verify all Evictable methods work
	assert.Equal(t, 500.0, pattern.GetFrequency(), "GetFrequency should return LogCount")
	assert.Equal(t, pattern.CreatedAt, pattern.GetCreatedAt(), "GetCreatedAt should return CreatedAt")
	assert.Equal(t, pattern.LastAccessAt, pattern.GetLastAccessAt(), "GetLastAccessAt should return LastAccessAt")
	assert.Greater(t, pattern.EstimatedBytes(), int64(0), "EstimatedBytes should be positive")
}

// TestClusterManager_ImplementsEvictableCollection verifies ClusterManager implements EvictableCollection
func TestClusterManager_ImplementsEvictableCollection(t *testing.T) {
	cm := NewClusterManager()

	// Add some patterns
	tl1 := token.NewTokenList()
	tl1.Add(token.NewToken(token.TokenWord, "error", token.NotWildcard))
	cm.Add(tl1)

	tl2 := token.NewTokenList()
	tl2.Add(token.NewToken(token.TokenWord, "warning", token.NotWildcard))
	cm.Add(tl2)

	// Test CollectEvictables
	evictables := cm.CollectEvictables()
	assert.Equal(t, 2, len(evictables), "Should collect all patterns as evictables")

	// Test RemoveEvictable
	cm.RemoveEvictable(evictables[0])
	assert.Equal(t, 1, cm.PatternCount(), "Should remove pattern via RemoveEvictable")
}

// TestClusterManager_EvictLowestScoringPatterns_Integration tests end-to-end eviction by count
func TestClusterManager_EvictLowestScoringPatterns_Integration(t *testing.T) {
	cm := NewClusterManager()
	now := time.Now()

	// Add 3 patterns with different characteristics
	// Pattern 1: High frequency, should be kept
	tl1 := token.NewTokenList()
	tl1.Add(token.NewToken(token.TokenWord, "error", token.NotWildcard))
	p1, _, _, _ := cm.Add(tl1)
	p1.LogCount = 10000
	p1.CreatedAt = now.Add(-10 * 24 * time.Hour)
	p1.LastAccessAt = now

	// Pattern 2: Low frequency, should be evicted
	tl2 := token.NewTokenList()
	tl2.Add(token.NewToken(token.TokenWord, "warning", token.NotWildcard))
	p2, _, _, _ := cm.Add(tl2)
	p2.LogCount = 100
	p2.CreatedAt = now.Add(-5 * 24 * time.Hour)
	p2.LastAccessAt = now.Add(-4 * 24 * time.Hour)

	// Pattern 3: Medium frequency, should be kept
	tl3 := token.NewTokenList()
	tl3.Add(token.NewToken(token.TokenWord, "info", token.NotWildcard))
	p3, _, _, _ := cm.Add(tl3)
	p3.LogCount = 5000
	p3.CreatedAt = now.Add(-15 * 24 * time.Hour)
	p3.LastAccessAt = now.Add(-1 * time.Hour)

	// Evict 1 pattern
	evicted := cm.EvictLowestScoringPatterns(1, 0.5)

	assert.Equal(t, 1, len(evicted), "Should evict exactly 1 pattern")
	assert.Equal(t, p2.PatternID, evicted[0].PatternID, "Should evict the lowest scoring pattern (p2)")
	assert.Equal(t, 2, cm.PatternCount(), "Should have 2 patterns remaining")
}

// TestClusterManager_EvictToMemoryTarget_Integration tests end-to-end eviction by memory
func TestClusterManager_EvictToMemoryTarget_Integration(t *testing.T) {
	cm := NewClusterManager()
	now := time.Now()

	// Add patterns with known memory footprints
	tl1 := token.NewTokenList()
	tl1.Add(token.NewToken(token.TokenWord, "error", token.NotWildcard))
	p1, _, _, _ := cm.Add(tl1)
	p1.LogCount = 100 // Low score
	p1.CreatedAt = now.Add(-10 * 24 * time.Hour)
	p1.LastAccessAt = now.Add(-9 * 24 * time.Hour)

	tl2 := token.NewTokenList()
	tl2.Add(token.NewToken(token.TokenWord, "warning", token.NotWildcard))
	p2, _, _, _ := cm.Add(tl2)
	p2.LogCount = 10000 // High score
	p2.CreatedAt = now.Add(-5 * 24 * time.Hour)
	p2.LastAccessAt = now

	initialBytes := cm.EstimatedBytes()

	// Evict until we free at least 50% of memory
	targetBytes := initialBytes / 2
	evicted := cm.EvictToMemoryTarget(targetBytes, 0.5)

	assert.Greater(t, len(evicted), 0, "Should evict at least one pattern")
	assert.Less(t, cm.EstimatedBytes(), initialBytes, "Memory should decrease")

	// Verify low-scoring pattern was evicted
	if len(evicted) > 0 {
		assert.Equal(t, p1.PatternID, evicted[0].PatternID, "Should evict lowest scoring pattern first")
	}
}

// TestClusterManager_EvictZero tests that evicting 0 or negative patterns returns nil
func TestClusterManager_EvictZero(t *testing.T) {
	cm := NewClusterManager()

	evicted := cm.EvictLowestScoringPatterns(0, 0.5)
	assert.Nil(t, evicted, "Evicting 0 patterns should return nil")

	evicted = cm.EvictLowestScoringPatterns(-5, 0.5)
	assert.Nil(t, evicted, "Evicting negative patterns should return nil")

	evicted = cm.EvictToMemoryTarget(0, 0.5)
	assert.Nil(t, evicted, "Evicting 0 bytes should return nil")

	evicted = cm.EvictToMemoryTarget(-100, 0.5)
	assert.Nil(t, evicted, "Evicting negative bytes should return nil")
}
