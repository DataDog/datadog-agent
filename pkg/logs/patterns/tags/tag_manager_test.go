// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTagManager(t *testing.T) {
	tm := NewTagManager()
	require.NotNil(t, tm)
	assert.Equal(t, 0, tm.Count())
}

func addStrings(tm *TagManager, values ...string) {
	for _, value := range values {
		tm.AddString(value)
	}
}

func TestTagManager_GetStringID(t *testing.T) {
	tm := NewTagManager()

	addStrings(tm, "env", "production")

	id, exists := tm.GetStringID("env")
	assert.True(t, exists)
	assert.NotZero(t, id)

	id, exists = tm.GetStringID("does-not-exist")
	assert.False(t, exists)
	assert.Equal(t, uint64(0), id)
}

func TestTagManager_ObserveDynamicStringAddsAfterRepeatedUse(t *testing.T) {
	tm := NewTagManager()

	dictID, isNew, shouldEncode := tm.ObserveDynamicString("INFO")
	assert.Zero(t, dictID)
	assert.False(t, isNew)
	assert.False(t, shouldEncode)
	assert.Equal(t, 0, tm.Count())

	dictID, isNew, shouldEncode = tm.ObserveDynamicString("INFO")
	assert.NotZero(t, dictID)
	assert.True(t, isNew)
	assert.True(t, shouldEncode)
	assert.Equal(t, 1, tm.Count())

	dictIDAgain, isNew, shouldEncode := tm.ObserveDynamicString("INFO")
	assert.Equal(t, dictID, dictIDAgain)
	assert.False(t, isNew)
	assert.True(t, shouldEncode)
	assert.Equal(t, 1, tm.Count())
}

func TestTagManager_ObserveDynamicStringUsesExistingEntry(t *testing.T) {
	tm := NewTagManager()
	existingID, added := tm.AddString("INFO")
	require.True(t, added)

	dictID, isNew, shouldEncode := tm.ObserveDynamicString("INFO")
	assert.Equal(t, existingID, dictID)
	assert.False(t, isNew)
	assert.True(t, shouldEncode)
	assert.Equal(t, 1, tm.Count())
}

func TestTagManager_ObserveDynamicStringSkipsHighCardinalityShapes(t *testing.T) {
	tm := NewTagManager()

	values := []string{
		"a",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400e29b41d4a716446655440000",
		"2026-04-28",
		"2026-04-28T12:34:56Z",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			for i := 0; i < 3; i++ {
				dictID, isNew, shouldEncode := tm.ObserveDynamicString(value)
				assert.Zero(t, dictID)
				assert.False(t, isNew)
				assert.False(t, shouldEncode)
			}
		})
	}
	assert.Equal(t, 0, tm.Count())
}

func TestTagManager_Concurrency(t *testing.T) {
	tm := NewTagManager()

	// Number of goroutines
	numGoroutines := 10
	tagsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Each goroutine adds the same set of tags repeatedly
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < tagsPerGoroutine; j++ {
				addStrings(tm, "env", "production", "service", "api", "team", "platform")
			}
		}(i)
	}

	wg.Wait()

	// Should only have 6 unique strings (3 keys + 3 values)
	assert.Equal(t, 6, tm.Count())
}

func TestTagManager_EvictLowestScoringStrings(t *testing.T) {
	tm := NewTagManager()

	// Add some tag entries
	addStrings(tm, "env", "production", "service", "api", "team", "platform")

	// Add more entries with varied usage
	for i := 0; i < 5; i++ {
		addStrings(tm, "env", "production") // Increases usage count
	}

	initialCount := tm.Count()
	require.Equal(t, 6, initialCount, "should have 6 entries (3 keys + 3 values)")

	// Evict 2 entries (the least used ones)
	evictedIDs := tm.EvictLowestScoringStrings(2, 1.0)

	assert.Len(t, evictedIDs, 2)
	assert.Equal(t, 4, tm.Count(), "should have 4 entries remaining")

	// The most used entries (env, production) should still exist
	_, exists := tm.GetStringID("env")
	assert.True(t, exists, "frequently used 'env' should not be evicted")
	_, exists = tm.GetStringID("production")
	assert.True(t, exists, "frequently used 'production' should not be evicted")
}

func TestTagManager_EvictToMemoryTarget(t *testing.T) {
	tm := NewTagManager()

	// Add entries
	addStrings(tm, "env", "production", "service", "api", "team", "platform", "region", "us-east-1")

	initialMemory := tm.EstimatedMemoryBytes()
	require.Greater(t, initialMemory, int64(0))

	// Evict entries until we free at least 50 bytes
	targetBytes := int64(50)
	evictedIDs := tm.EvictToMemoryTarget(targetBytes, 1.0)

	assert.NotEmpty(t, evictedIDs)

	finalMemory := tm.EstimatedMemoryBytes()
	assert.Less(t, finalMemory, initialMemory, "memory usage should decrease")
}

func TestTagManager_EstimatedMemoryBytes(t *testing.T) {
	tm := NewTagManager()

	// Empty manager should have 0 bytes
	assert.Equal(t, int64(0), tm.EstimatedMemoryBytes())

	// Add some entries
	addStrings(tm, "env", "production")

	memory := tm.EstimatedMemoryBytes()
	assert.Greater(t, memory, int64(0), "should have positive memory usage")

	// Add more entries
	addStrings(tm, "service", "api")

	newMemory := tm.EstimatedMemoryBytes()
	assert.Greater(t, newMemory, memory, "memory should increase with more entries")
}

func TestTagManager_EvictZero(t *testing.T) {
	tm := NewTagManager()

	addStrings(tm, "env", "production")

	// Evicting 0 or negative should do nothing
	evictedIDs := tm.EvictLowestScoringStrings(0, 1.0)
	assert.Nil(t, evictedIDs)
	assert.Equal(t, 2, tm.Count())

	evictedIDs = tm.EvictToMemoryTarget(0, 1.0)
	assert.Nil(t, evictedIDs)
	assert.Equal(t, 2, tm.Count())
}

func TestTagEntry_EstimatedBytes(t *testing.T) {
	entry := &tagEntry{
		id:           1,
		str:          "test",
		usageCount:   10,
		createdAt:    time.Now(),
		lastAccessAt: time.Now(),
	}

	bytes := entry.EstimatedBytes()
	// string header (16) + len("test") (4) + uint64 (8) + int64 (8) + 2*time.Time (48)
	expectedMin := int64(16 + 4 + 8 + 8 + 48)
	assert.GreaterOrEqual(t, bytes, expectedMin)
}
