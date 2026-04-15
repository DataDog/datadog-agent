// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

func TestReverseCacheRingAddBelowCapacity(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		evicted, didEvict := r.add(ckey.ContextKey(i))
		assert.False(t, didEvict, "should not evict before full")
		assert.Equal(t, ckey.ContextKey(0), evicted)
	}
	assert.Equal(t, uint(defaultReverseCacheCapacity), r.count)
}

func TestReverseCacheRingEvictsOldest(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	// Fill the ring
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		r.add(ckey.ContextKey(i))
	}

	// Next add should evict key 0 (oldest)
	evicted, didEvict := r.add(ckey.ContextKey(1000))
	require.True(t, didEvict)
	assert.Equal(t, ckey.ContextKey(0), evicted)

	// Next should evict key 1
	evicted, didEvict = r.add(ckey.ContextKey(1001))
	require.True(t, didEvict)
	assert.Equal(t, ckey.ContextKey(1), evicted)
}

func TestReverseCacheRingEvictsInOrder(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		r.add(ckey.ContextKey(i))
	}

	// Wrap around fully and verify FIFO eviction order
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		evicted, didEvict := r.add(ckey.ContextKey(1000 + i))
		require.True(t, didEvict)
		assert.Equal(t, ckey.ContextKey(i), evicted, "eviction %d should evict key %d", i, i)
	}
}

func TestReverseCacheRingForEach(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	r.add(ckey.ContextKey(10))
	r.add(ckey.ContextKey(20))
	r.add(ckey.ContextKey(30))

	var collected []ckey.ContextKey
	r.forEach(func(k ckey.ContextKey) {
		collected = append(collected, k)
	})
	assert.Equal(t, []ckey.ContextKey{10, 20, 30}, collected)
}

func TestReverseCacheRingForEachAfterWrap(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	for i := 0; i < defaultReverseCacheCapacity+5; i++ {
		r.add(ckey.ContextKey(i))
	}

	var collected []ckey.ContextKey
	r.forEach(func(k ckey.ContextKey) {
		collected = append(collected, k)
	})
	assert.Equal(t, defaultReverseCacheCapacity, len(collected))
	// After wrapping, the ring contains keys 5..defaultReverseCacheCapacity+4
	for _, k := range collected {
		assert.True(t, k >= 5 && k <= ckey.ContextKey(defaultReverseCacheCapacity+4), "key %d should be in range [5,%d]", k, defaultReverseCacheCapacity+4)
	}
	assert.Equal(t, defaultReverseCacheCapacity, len(collected))
}

func TestReverseCacheRingForEachEmpty(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	called := false
	r.forEach(func(_ ckey.ContextKey) {
		called = true
	})
	assert.False(t, called)
}

func TestReverseCacheRingCountNeverExceedsCapacity(t *testing.T) {
	r := reverseCacheRing{capacity: defaultReverseCacheCapacity}
	for i := 0; i < defaultReverseCacheCapacity*3; i++ {
		r.add(ckey.ContextKey(i))
		assert.LessOrEqual(t, r.count, uint(defaultReverseCacheCapacity))
	}
}

func TestTagFilterCacheAddAndGet(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	entry := tagFilterCacheEntry{
		contextKey:  ckey.ContextKey(100),
		taggerKey:   ckey.TagsKey(200),
		metricKey:   ckey.TagsKey(300),
		removedTags: 2,
	}
	sc.add(ckey.ContextKey(1), entry)

	got, ok := sc.get(ckey.ContextKey(1))
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestTagFilterCacheGetMiss(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	_, ok := sc.get(ckey.ContextKey(999))
	assert.False(t, ok)
}

func TestTagFilterCacheMultiplePreFilterSamePostFilter(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	postKey := ckey.ContextKey(100)
	for i := 0; i < 5; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	// All pre-filter keys should be retrievable
	for i := 0; i < 5; i++ {
		got, ok := sc.get(ckey.ContextKey(i))
		require.True(t, ok, "pre-filter key %d should exist", i)
		assert.Equal(t, postKey, got.contextKey)
	}
}

func TestTagFilterCacheDeleteRemovesForwardEntries(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	postKey := ckey.ContextKey(100)
	for i := 0; i < 5; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	// Delete by post-filter key should remove all forward entries
	sc.delete(postKey)

	for i := 0; i < 5; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.False(t, ok, "pre-filter key %d should be gone after delete", i)
	}

	// Reverse entry should be gone too
	assert.Nil(t, sc.reverseCache[postKey])
}

func TestTagFilterCacheDeleteNonExistent(_ *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	// Should not panic
	sc.delete(ckey.ContextKey(999))
}

func TestTagFilterCacheEvictsOldestPreFilterAtCapacity(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	postKey := ckey.ContextKey(500)
	// Fill ring to capacity
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	// All should be present
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.True(t, ok, "key %d should exist before eviction", i)
	}

	// Add one more — should evict key 0
	sc.add(ckey.ContextKey(1000), tagFilterCacheEntry{contextKey: postKey})
	_, ok := sc.get(ckey.ContextKey(0))
	assert.False(t, ok, "key 0 should be evicted")

	// Key 1000 should be present
	_, ok = sc.get(ckey.ContextKey(1000))
	assert.True(t, ok, "newly added key should exist")

	// Keys 1..99 should still be present
	for i := 1; i < defaultReverseCacheCapacity; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.True(t, ok, "key %d should still exist", i)
	}
}

func TestTagFilterCacheEvictionContinuity(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	postKey := ckey.ContextKey(500)

	total := defaultReverseCacheCapacity + defaultReverseCacheCapacity/2 // 1.5x capacity
	evicted := total - defaultReverseCacheCapacity                // first half-capacity should be evicted

	for i := 0; i < total; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	for i := 0; i < evicted; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.False(t, ok, "key %d should have been evicted", i)
	}
	for i := evicted; i < total; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.True(t, ok, "key %d should still exist", i)
	}
}

func TestTagFilterCacheClear(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	postKey := ckey.ContextKey(100)
	for i := 0; i < 10; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	sc.clear()

	for i := 0; i < 10; i++ {
		_, ok := sc.get(ckey.ContextKey(i))
		assert.False(t, ok, "key %d should be gone after clear", i)
	}
	assert.Empty(t, sc.cache)
	assert.Empty(t, sc.reverseCache)
}

func TestTagFilterCacheIndependentPostFilterKeys(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	// Two different post-filter keys with separate pre-filter keys
	sc.add(ckey.ContextKey(1), tagFilterCacheEntry{contextKey: ckey.ContextKey(100)})
	sc.add(ckey.ContextKey(2), tagFilterCacheEntry{contextKey: ckey.ContextKey(200)})

	// Delete one post-filter key should not affect the other
	sc.delete(ckey.ContextKey(100))

	_, ok := sc.get(ckey.ContextKey(1))
	assert.False(t, ok, "key 1 should be gone")

	got, ok := sc.get(ckey.ContextKey(2))
	require.True(t, ok, "key 2 should survive")
	assert.Equal(t, ckey.ContextKey(200), got.contextKey)
}

func TestTagFilterCacheOverwriteSamePreFilterKey(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)

	sc.add(ckey.ContextKey(1), tagFilterCacheEntry{contextKey: ckey.ContextKey(100), removedTags: 2})
	sc.add(ckey.ContextKey(1), tagFilterCacheEntry{contextKey: ckey.ContextKey(300), removedTags: 5})

	got, ok := sc.get(ckey.ContextKey(1))
	require.True(t, ok)
	assert.Equal(t, ckey.ContextKey(300), got.contextKey, "should have latest entry")
	assert.Equal(t, 5, got.removedTags, "should have latest entry")
}

func TestTagFilterCacheDeleteAfterEviction(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	postKey := ckey.ContextKey(500)

	// Fill past capacity so some entries are evicted
	for i := 0; i < defaultReverseCacheCapacity+10; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}

	// Delete should clean up all remaining forward entries
	sc.delete(postKey)

	assert.Empty(t, sc.cache, "all forward entries should be removed")
	assert.Nil(t, sc.reverseCache[postKey], "reverse entry should be removed")
}

func TestTagFilterCacheEvictionTelemetry(t *testing.T) {
	sc := newTagFilterCache(defaultReverseCacheCapacity)
	postKey := ckey.ContextKey(500)

	before := tlmFilteredTagsCacheEvict.Get()

	// Fill ring to capacity — no evictions yet
	for i := 0; i < defaultReverseCacheCapacity; i++ {
		sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey})
	}
	assert.Equal(t, before, tlmFilteredTagsCacheEvict.Get(), "no evictions before capacity reached")

	// Add 10 more — each should evict one entry
	for i := 0; i < 10; i++ {
		sc.add(ckey.ContextKey(defaultReverseCacheCapacity+i), tagFilterCacheEntry{contextKey: postKey})
	}
	assert.Equal(t, before+10, tlmFilteredTagsCacheEvict.Get(), "should count 10 evictions")
}

// assertTagFilterCacheConsistent checks the invariant: every key in the reverse cache
// must have a corresponding entry in the forward cache that points back to the
// same post-filter context key.
func assertTagFilterCacheConsistent(t *testing.T, sc *tagFilterCache) {
	t.Helper()
	for postKey, ring := range sc.reverseCache {
		ring.forEach(func(preKey ckey.ContextKey) {
			entry, ok := sc.cache[preKey]
			if !ok {
				t.Errorf("reverse cache has pre-filter key %d → post-filter key %d, but forward cache missing pre-filter key", preKey, postKey)
				return
			}
			if entry.contextKey != postKey {
				t.Errorf("reverse cache says pre-filter %d → post-filter %d, but forward cache says pre-filter %d → post-filter %d",
					preKey, postKey, preKey, entry.contextKey)
			}
		})
	}
}

// FuzzTagFilterCacheConsistency exercises random sequences of add/delete/clear operations
// and verifies that the reverse cache always points to entries that exist in the forward cache.
func FuzzTagFilterCacheConsistency(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte{})
	// Seed with a sequence that fills past capacity
	seed := make([]byte, defaultReverseCacheCapacity*3+20)
	for i := range seed {
		seed[i] = byte(i % 256)
	}
	f.Add(seed)

	f.Fuzz(func(t *testing.T, data []byte) {
		sc := newTagFilterCache(defaultReverseCacheCapacity)

		for _, b := range data {
			op := b % 4
			key := ckey.ContextKey(b >> 2) // 64 distinct key values

			switch op {
			case 0, 1:
				// add (weighted higher — most common operation)
				// Mirror real usage: filterTags checks cache hit first, so never
				// re-adds a key that already exists in the forward cache.
				if _, exists := sc.get(key); exists {
					continue
				}
				postKey := ckey.ContextKey((b >> 4) % 4) // 4 distinct post-filter keys
				sc.add(key, tagFilterCacheEntry{
					contextKey:  postKey,
					removedTags: int(b),
				})
			case 2:
				// delete by post-filter key
				postKey := ckey.ContextKey((b >> 4) % 4)
				sc.delete(postKey)
			case 3:
				// clear
				sc.clear()
			}

			assertTagFilterCacheConsistent(t, sc)
		}
	})
}

// FuzzTagFilterCacheHighChurn focuses on many pre-filter keys mapping to a single post-filter key,
// exercising the ring buffer eviction path heavily.
func FuzzTagFilterCacheHighChurn(f *testing.F) {
	f.Add(uint16(0), uint16(200))
	f.Add(uint16(50), uint16(500))

	f.Fuzz(func(t *testing.T, start, count uint16) {
		sc := newTagFilterCache(defaultReverseCacheCapacity)
		postKey := ckey.ContextKey(999)

		for i := start; i < start+count; i++ {
			sc.add(ckey.ContextKey(i), tagFilterCacheEntry{contextKey: postKey, removedTags: int(i)})
			assertTagFilterCacheConsistent(t, sc)
		}

		// After all adds, verify ring didn't exceed capacity
		if ring := sc.reverseCache[postKey]; ring != nil {
			assert.LessOrEqual(t, ring.count, uint(defaultReverseCacheCapacity))
		}

		// Delete should leave everything clean
		sc.delete(postKey)
		assertTagFilterCacheConsistent(t, sc)
		assert.Empty(t, sc.cache)
	})
}

// FuzzTagFilterCacheInterleavedDeletes exercises interleaved adds and deletes across
// multiple post-filter keys.
func FuzzTagFilterCacheInterleavedDeletes(f *testing.F) {
	f.Add([]byte{10, 20, 30, 40, 50})

	f.Fuzz(func(t *testing.T, data []byte) {
		sc := newTagFilterCache(defaultReverseCacheCapacity)

		for i, b := range data {
			preKey := ckey.ContextKey(i)
			postKey := ckey.ContextKey(b % 8) // 8 post-filter buckets

			sc.add(preKey, tagFilterCacheEntry{contextKey: postKey})

			// Periodically delete a post-filter key
			if b%5 == 0 {
				sc.delete(ckey.ContextKey(b % 8))
			}

			assertTagFilterCacheConsistent(t, sc)
		}
	})
}
