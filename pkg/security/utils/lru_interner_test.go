// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLRUStringInterner(t *testing.T) {
	t.Run("valid size", func(t *testing.T) {
		si := NewLRUStringInterner(100)
		require.NotNil(t, si)
		require.NotNil(t, si.store)
	})

	t.Run("small size", func(t *testing.T) {
		si := NewLRUStringInterner(1)
		require.NotNil(t, si)
	})

	t.Run("negative size panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewLRUStringInterner(-1)
		})
	})

	t.Run("zero size panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewLRUStringInterner(0)
		})
	})
}

func TestLRUStringInterner_Deduplicate(t *testing.T) {
	t.Run("returns same string for same value", func(t *testing.T) {
		si := NewLRUStringInterner(10)

		first := si.Deduplicate("hello")
		second := si.Deduplicate("hello")

		assert.Equal(t, first, second)
		assert.Equal(t, "hello", first)
	})

	t.Run("returns different strings for different values", func(t *testing.T) {
		si := NewLRUStringInterner(10)

		first := si.Deduplicate("hello")
		second := si.Deduplicate("world")

		assert.Equal(t, "hello", first)
		assert.Equal(t, "world", second)
	})

	t.Run("empty string", func(t *testing.T) {
		si := NewLRUStringInterner(10)
		result := si.Deduplicate("")
		assert.Equal(t, "", result)
	})

	t.Run("eviction on full cache", func(t *testing.T) {
		si := NewLRUStringInterner(2)

		si.Deduplicate("a")
		si.Deduplicate("b")
		si.Deduplicate("c") // Should evict "a"

		// "a" should have been evicted, but we can still deduplicate it
		result := si.Deduplicate("a")
		assert.Equal(t, "a", result)
	})
}

func TestLRUStringInterner_DeduplicateSlice(t *testing.T) {
	t.Run("deduplicate slice in place", func(t *testing.T) {
		si := NewLRUStringInterner(10)

		slice := []string{"hello", "world", "hello", "foo", "world"}
		si.DeduplicateSlice(slice)

		assert.Equal(t, []string{"hello", "world", "hello", "foo", "world"}, slice)
	})

	t.Run("empty slice", func(t *testing.T) {
		si := NewLRUStringInterner(10)
		slice := []string{}
		si.DeduplicateSlice(slice)
		assert.Empty(t, slice)
	})

	t.Run("single element", func(t *testing.T) {
		si := NewLRUStringInterner(10)
		slice := []string{"single"}
		si.DeduplicateSlice(slice)
		assert.Equal(t, []string{"single"}, slice)
	})
}

func TestLRUStringInterner_Concurrent(t *testing.T) {
	si := NewLRUStringInterner(100)

	var wg sync.WaitGroup
	numGoroutines := 100
	numIterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				si.Deduplicate("shared")
				si.Deduplicate("unique_" + string(rune(id)))
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no race conditions or deadlocks occur
}

func TestLRUStringInterner_DeduplicateSlice_Concurrent(t *testing.T) {
	si := NewLRUStringInterner(100)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slice := []string{"a", "b", "c", "a", "b"}
			si.DeduplicateSlice(slice)
		}()
	}

	wg.Wait()
	// Test passes if no race conditions or deadlocks occur
}
