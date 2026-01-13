// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"fmt"
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

func TestTagManager_EncodeTagStrings_NewEntries(t *testing.T) {
	tm := NewTagManager()

	encoded, newEntries := tm.EncodeTagStrings([]string{"env:production"})

	require.Len(t, encoded, 1)
	require.Len(t, newEntries, 2) // key + value strings

	keyID := encoded[0].GetKey().GetDictIndex()
	valID := encoded[0].GetValue().GetDictIndex()

	assert.NotZero(t, keyID)
	assert.NotZero(t, valID)
	assert.NotEqual(t, keyID, valID)
	assert.Equal(t, "env", newEntries[keyID])
	assert.Equal(t, "production", newEntries[valID])
	assert.Equal(t, 2, tm.Count())
}

func TestTagManager_EncodeTagStrings_ReusesIDs(t *testing.T) {
	tm := NewTagManager()

	first, firstEntries := tm.EncodeTagStrings([]string{"env:production"})
	require.Len(t, first, 1)
	require.Len(t, firstEntries, 2)

	second, secondEntries := tm.EncodeTagStrings([]string{"env:production"})
	require.Len(t, second, 1)
	assert.Len(t, secondEntries, 0, "no new dictionary entries expected")

	assert.Equal(t, first[0].GetKey().GetDictIndex(), second[0].GetKey().GetDictIndex())
	assert.Equal(t, first[0].GetValue().GetDictIndex(), second[0].GetValue().GetDictIndex())
	assert.Equal(t, 2, tm.Count())
}

func TestTagManager_EncodeTagStrings_MixedNewAndExisting(t *testing.T) {
	tm := NewTagManager()

	_, firstEntries := tm.EncodeTagStrings([]string{"env:production"})
	require.Len(t, firstEntries, 2)

	_, secondEntries := tm.EncodeTagStrings([]string{"env:production", "service:api"})
	assert.Len(t, secondEntries, 2)
	assert.Equal(t, "service", secondEntries[3])
	assert.Equal(t, "api", secondEntries[4])
	assert.Equal(t, 4, tm.Count())
}

func TestTagManager_EncodeTagStrings_InvalidFormats(t *testing.T) {
	tm := NewTagManager()

	encoded, newEntries := tm.EncodeTagStrings([]string{
		"valid:tag",
		"",         // empty string should be skipped
		":novalue", // colon should not be used as a delimiter for key-only tags, skip it
	})

	assert.Len(t, encoded, 1)
	assert.Len(t, newEntries, 2)
	assert.Equal(t, 2, tm.Count())
}

func TestTagManager_EncodeTagStrings_KeyOnly(t *testing.T) {
	tm := NewTagManager()

	encoded, newEntries := tm.EncodeTagStrings([]string{
		"env",
		"service:", // assume colon is mistyped, result in a key-only tag
	})

	require.Len(t, encoded, 2)
	require.Len(t, newEntries, 2)

	keyOnly := encoded[0]
	assert.NotNil(t, keyOnly.GetKey())
	assert.Nil(t, keyOnly.GetValue())

	keyWithEmptyValue := encoded[1]
	assert.NotNil(t, keyWithEmptyValue.GetKey())
	assert.Nil(t, keyWithEmptyValue.GetValue())

	fmt.Println(newEntries)

	assert.Equal(t, 2, tm.Count())
}

func TestTagManager_EncodeTagStrings_EmptyInput(t *testing.T) {
	tm := NewTagManager()

	encoded, newEntries := tm.EncodeTagStrings(nil)

	assert.Len(t, encoded, 0)
	assert.Len(t, newEntries, 0)
	assert.Equal(t, 0, tm.Count())
}

func TestTagManager_GetStringID(t *testing.T) {
	tm := NewTagManager()

	_, newEntries := tm.EncodeTagStrings([]string{"env:production"})
	require.Len(t, newEntries, 2)

	id, exists := tm.GetStringID("env")
	assert.True(t, exists)
	assert.NotZero(t, id)

	id, exists = tm.GetStringID("does-not-exist")
	assert.False(t, exists)
	assert.Equal(t, uint64(0), id)
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
				encoded, _ := tm.EncodeTagStrings([]string{"env:production", "service:api", "team:platform"})
				assert.Len(t, encoded, 3)
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
	tm.EncodeTagStrings([]string{"env:production", "service:api", "team:platform"})

	// Add more entries with varied usage
	for i := 0; i < 5; i++ {
		tm.EncodeTagStrings([]string{"env:production"}) // Increases usage count
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
	tm.EncodeTagStrings([]string{"env:production", "service:api", "team:platform", "region:us-east-1"})

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
	tm.EncodeTagStrings([]string{"env:production"})

	memory := tm.EstimatedMemoryBytes()
	assert.Greater(t, memory, int64(0), "should have positive memory usage")

	// Add more entries
	tm.EncodeTagStrings([]string{"service:api"})

	newMemory := tm.EstimatedMemoryBytes()
	assert.Greater(t, newMemory, memory, "memory should increase with more entries")
}

func TestTagManager_EvictZero(t *testing.T) {
	tm := NewTagManager()

	tm.EncodeTagStrings([]string{"env:production"})

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
