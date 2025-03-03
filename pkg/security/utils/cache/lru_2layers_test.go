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
