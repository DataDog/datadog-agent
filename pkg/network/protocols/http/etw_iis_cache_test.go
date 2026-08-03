// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetIISTagsCache() {
	iisTagsCacheMu.Lock()
	defer iisTagsCacheMu.Unlock()
	iisTagsCacheMap = make(map[[2]uint16]iisTagsCacheEntry)
}

func TestStoreIISTagsCache(t *testing.T) {
	t.Run("basic store and retrieve", func(t *testing.T) {
		resetIISTagsCache()

		key := [2]uint16{80, 5000}
		tags := []string{"service:web", "env:prod"}
		storeIISTagsCache(key, tags)

		result := GetIISTagsCache()
		require.Contains(t, result, "80-5000")
		assert.Equal(t, tags, result["80-5000"])
	})

	t.Run("update existing key", func(t *testing.T) {
		resetIISTagsCache()

		key := [2]uint16{80, 5000}
		storeIISTagsCache(key, []string{"service:old"})
		storeIISTagsCache(key, []string{"service:new"})

		result := GetIISTagsCache()
		assert.Equal(t, []string{"service:new"}, result["80-5000"])
	})

	t.Run("capacity limit evicts oldest entry", func(t *testing.T) {
		resetIISTagsCache()

		// Fill cache to capacity; the first entry (key {0,0}) will have the earliest expiry.
		for i := 0; i < iisTagsCacheMaxSize; i++ {
			key := [2]uint16{uint16(i), 0}
			storeIISTagsCache(key, []string{"tag"})
		}

		// Add one more — should evict the oldest (earliest expiry) entry
		key := [2]uint16{65535, 65535}
		storeIISTagsCache(key, []string{"new-entry"})

		result := GetIISTagsCache()
		assert.Contains(t, result, "65535-65535", "new entry should have been inserted after evicting oldest")
		assert.Equal(t, iisTagsCacheMaxSize, len(result), "cache size should remain at max capacity")
	})

	t.Run("evicts expired entry at capacity", func(t *testing.T) {
		resetIISTagsCache()

		// Insert one entry and manually expire it
		expiredKey := [2]uint16{1, 1}
		iisTagsCacheMu.Lock()
		iisTagsCacheMap[expiredKey] = iisTagsCacheEntry{
			tags:   []string{"expired"},
			expiry: time.Now().Add(-1 * time.Second),
		}
		iisTagsCacheMu.Unlock()

		// Fill up the rest of the cache
		for i := 2; i <= iisTagsCacheMaxSize; i++ {
			key := [2]uint16{uint16(i), 0}
			storeIISTagsCache(key, []string{"tag"})
		}

		// Now at capacity with one expired entry — new insert should succeed
		newKey := [2]uint16{65534, 65534}
		storeIISTagsCache(newKey, []string{"new-entry"})

		result := GetIISTagsCache()
		assert.Contains(t, result, "65534-65534")
	})
}

func TestGetIISTagsCache(t *testing.T) {
	t.Run("skips expired entries without deleting", func(t *testing.T) {
		resetIISTagsCache()

		// Add a valid and an expired entry
		storeIISTagsCache([2]uint16{80, 5000}, []string{"valid"})
		iisTagsCacheMu.Lock()
		iisTagsCacheMap[[2]uint16{81, 5001}] = iisTagsCacheEntry{
			tags:   []string{"expired"},
			expiry: time.Now().Add(-1 * time.Second),
		}
		iisTagsCacheMu.Unlock()

		result := GetIISTagsCache()
		assert.Contains(t, result, "80-5000")
		assert.NotContains(t, result, "81-5001")

		// Verify the expired entry still exists in the map (read-only)
		iisTagsCacheMu.Lock()
		_, exists := iisTagsCacheMap[[2]uint16{81, 5001}]
		iisTagsCacheMu.Unlock()
		assert.True(t, exists, "GetIISTagsCache should not delete expired entries")
	})

	t.Run("empty cache returns empty map", func(t *testing.T) {
		resetIISTagsCache()

		result := GetIISTagsCache()
		assert.Empty(t, result)
	})
}
