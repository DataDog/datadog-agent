// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBomCache_HitsWithinTTL(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 100,
	}

	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Digest: bomDigest{
			TopLevelTokens: []topLevelToken{
				{Value: "/opt/example"},
			},
		},
		Timestamp: time.Now(),
	}

	result := c.getBomDigests([]string{"/var/db/receipts/com.example.bom"})
	require.Len(t, result["/var/db/receipts/com.example.bom"].TopLevelTokens, 1)
	assert.Equal(t, "/opt/example", result["/var/db/receipts/com.example.bom"].TopLevelTokens[0].Value)
}

func TestBomCache_ExpiredEntryRefetches(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/var/db/receipts/com.example.bom"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/var/db/receipts/com.example.bom": {
				Digest: bomDigest{
					TopLevelTokens: []topLevelToken{{Value: "/opt/example"}},
				},
				Cacheable: true,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        5 * time.Millisecond,
		maxEntries: 100,
	}

	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Digest: bomDigest{
			TopLevelTokens: []topLevelToken{
				{Value: "/stale"},
			},
		},
		Timestamp: time.Now().Add(-time.Second),
	}

	result := c.getBomDigests([]string{"/var/db/receipts/com.example.bom"})
	digest := result["/var/db/receipts/com.example.bom"]
	assert.Equal(t, []topLevelToken{{Value: "/opt/example"}}, digest.TopLevelTokens)
}

func TestBomCache_EvictsWhenFull(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/bom/c"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/bom/c": {
				Digest:    bomDigest{},
				Cacheable: true,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}

	oldest := time.Now().Add(-10 * time.Minute)
	c.entries["/bom/a"] = &bomCacheEntry{Digest: bomDigest{}, Timestamp: oldest}
	c.entries["/bom/b"] = &bomCacheEntry{Digest: bomDigest{}, Timestamp: time.Now()}

	// Inserting a third entry should evict the oldest (/bom/a).
	c.getBomDigests([]string{"/bom/c"})

	assert.LessOrEqual(t, len(c.entries), 2)
	_, hasA := c.entries["/bom/a"]
	assert.False(t, hasA, "oldest entry should be evicted")
	_, hasB := c.entries["/bom/b"]
	assert.True(t, hasB, "newer entry should be retained")
}

func TestBomCache_DoesNotCacheNonCacheableFetches(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/bom/fail"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/bom/fail": {
				Digest:    bomDigest{TopLevelTokens: []topLevelToken{{Value: "/transient"}}},
				Cacheable: false,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}

	result := c.getBomDigests([]string{"/bom/fail"})
	assert.Equal(t, []topLevelToken{{Value: "/transient"}}, result["/bom/fail"].TopLevelTokens)
	_, exists := c.entries["/bom/fail"]
	assert.False(t, exists, "non-cacheable fetch results should not be stored")
}

func TestBatchLsbom_EmptyInput(t *testing.T) {
	result := batchLsbom(nil)
	assert.Nil(t, result)

	result = batchLsbom([]string{})
	assert.Nil(t, result)
}
