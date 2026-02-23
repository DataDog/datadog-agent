// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cache holds cache related files
package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTwoLayersLRU(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](2)
	assert.Nil(t, err)

	t.Run("not-exists", func(t *testing.T) {
		value, exists := cache.Get("a", 1)
		assert.False(t, exists)
		assert.Equal(t, value, 0)
	})

	t.Run("add-no-eviction", func(t *testing.T) {
		evicted := cache.Add("a", 1, 44)
		assert.False(t, evicted)
		assert.Equal(t, cache.Len(), 1)
	})

	t.Run("get-key", func(t *testing.T) {
		value, exists := cache.Get("a", 1)
		assert.True(t, exists)
		assert.Equal(t, value, 44)
	})

	t.Run("add-no-eviction", func(t *testing.T) {
		evicted := cache.Add("a", 2, 55)
		assert.False(t, evicted)
		assert.Equal(t, cache.Len(), 2)
	})

	t.Run("remove-key2", func(t *testing.T) {
		removed := cache.RemoveKey2(2, "a")
		assert.Equal(t, 1, removed)
		assert.Equal(t, cache.Len(), 1)
	})

	t.Run("add-no-eviction", func(t *testing.T) {
		evicted := cache.Add("b", 10, 99)
		assert.False(t, evicted)
		assert.Equal(t, cache.Len(), 2)
	})

	t.Run("remove-key2-oldest", func(t *testing.T) {
		k1, k2, v, evicted := cache.RemoveOldest()
		assert.True(t, evicted)
		assert.Equal(t, k1, "a")
		assert.Equal(t, k2, 1)
		assert.Equal(t, v, 44)
		assert.Equal(t, cache.Len(), 1)
	})

	// now the oldest is b/10
	t.Run("add-eviction", func(t *testing.T) {
		evicted := cache.Add("c", 20, 990)
		assert.False(t, evicted)
		assert.Equal(t, cache.Len(), 2)

		evicted = cache.Add("d", 30, 1990)
		assert.True(t, evicted)
		assert.Equal(t, cache.Len(), 2)

		_, exists := cache.Get("b", 10)
		assert.False(t, exists)
	})

	t.Run("remove-key1", func(t *testing.T) {
		exists := cache.RemoveKey1("c")
		assert.True(t, exists)
		assert.Equal(t, cache.Len(), 1)

		_, exists = cache.Get("c", 20)
		assert.False(t, exists)
	})

	t.Run("walk", func(t *testing.T) {
		var count int

		cache.Walk(func(_ string, _, _ int) {
			count++
		})

		assert.Equal(t, count, 1)
	})
}

func TestTwoLayersLRURemoveKey1NotFound(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](10)
	assert.NoError(t, err)

	removed := cache.RemoveKey1("nonexistent")
	assert.False(t, removed)
}

func TestTwoLayersLRURemoveKey2AllKeys(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](10)
	assert.NoError(t, err)

	// Add entries where k1="a" has both k2=1 and k2=2, so removing k2=1
	// doesn't empty the layer (avoiding concurrent modification issues)
	cache.Add("a", 1, 100)
	cache.Add("a", 2, 150)
	cache.Add("b", 1, 200)
	cache.Add("b", 3, 250)

	// Remove k2=1 from all k1 layers (no specific keys provided)
	removed := cache.RemoveKey2(1)
	assert.Equal(t, 2, removed)
	assert.Equal(t, 2, cache.Len())

	// k2=1 should be gone from both "a" and "b"
	_, exists := cache.Get("a", 1)
	assert.False(t, exists)
	_, exists = cache.Get("b", 1)
	assert.False(t, exists)

	// Other entries should still be there
	v, exists := cache.Get("a", 2)
	assert.True(t, exists)
	assert.Equal(t, 150, v)
}

func TestTwoLayersLRURemoveOldestEmpty(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](10)
	assert.NoError(t, err)

	_, _, _, evicted := cache.RemoveOldest()
	assert.False(t, evicted)
}

func TestTwoLayersLRUUpdateExistingKey(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](10)
	assert.NoError(t, err)

	cache.Add("a", 1, 100)
	assert.Equal(t, 1, cache.Len())

	// Update existing key should not change length
	cache.Add("a", 1, 200)
	assert.Equal(t, 1, cache.Len())

	v, exists := cache.Get("a", 1)
	assert.True(t, exists)
	assert.Equal(t, 200, v)
}

func TestTwoLayersLRUWalkInner(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, string](10)
	assert.NoError(t, err)

	cache.Add("group1", 1, "a")
	cache.Add("group1", 2, "b")
	cache.Add("group1", 3, "c")
	cache.Add("group2", 10, "x")

	// Walk inner for group1
	collected := map[int]string{}
	cache.WalkInner("group1", func(k2 int, v string) bool {
		collected[k2] = v
		return true
	})
	assert.Len(t, collected, 3)
	assert.Equal(t, "a", collected[1])
	assert.Equal(t, "b", collected[2])
	assert.Equal(t, "c", collected[3])
}

func TestTwoLayersLRUWalkInnerEarlyStop(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, string](10)
	assert.NoError(t, err)

	cache.Add("g", 1, "a")
	cache.Add("g", 2, "b")
	cache.Add("g", 3, "c")

	// Stop after first element
	var count int
	cache.WalkInner("g", func(_ int, _ string) bool {
		count++
		return false
	})
	assert.Equal(t, 1, count)
}

func TestTwoLayersLRUWalkInnerNonexistent(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, string](10)
	assert.NoError(t, err)

	// Walking a nonexistent key should be a no-op
	var count int
	cache.WalkInner("nonexistent", func(_ int, _ string) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count)
}

func TestTwoLayersLRUGetMissingK2(t *testing.T) {
	cache, err := NewTwoLayersLRU[string, int, int](10)
	assert.NoError(t, err)

	cache.Add("a", 1, 100)

	// K1 exists but K2 doesn't
	_, exists := cache.Get("a", 999)
	assert.False(t, exists)
}
