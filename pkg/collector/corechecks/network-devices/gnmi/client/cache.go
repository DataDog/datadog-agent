// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"maps"
	"sync"
	"time"
)

// CacheEntry holds the latest decoded value for a subscribed path.
type CacheEntry struct {
	Value     any
	Timestamp time.Time
	Keys      map[string]string
}

type cache struct {
	mu       sync.RWMutex
	entries  map[string]storedEntry
	byLookup map[string]string
}

type storedEntry struct {
	path  normalizedPath
	key   CacheKey
	entry CacheEntry
}

func newCache() *cache {
	return &cache{
		entries:  make(map[string]storedEntry),
		byLookup: make(map[string]string),
	}
}

func (c *cache) set(path normalizedPath, entry CacheEntry) {
	key := path.cacheKey()
	id := path.id()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = storedEntry{path: path, key: key, entry: entry}
	c.byLookup[key.cacheKey()] = id
}

func (c *cache) deletePrefix(prefix normalizedPath) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// A linear scan keeps the initial implementation simple. If cache sizes make
	// similar-path lookups expensive, add a secondary path index or trie while
	// retaining normalizedPath as the canonical representation.
	for id, stored := range c.entries {
		if stored.path.matchesPrefix(prefix) {
			delete(c.entries, id)
			lookupKey := stored.key.cacheKey()
			if c.byLookup[lookupKey] == id {
				delete(c.byLookup, lookupKey)
			}
		}
	}
}

func (c *cache) get(key CacheKey) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.byLookup[key.cacheKey()]
	if !ok {
		return CacheEntry{}, false
	}
	stored, ok := c.entries[id]
	if !ok {
		return CacheEntry{}, false
	}
	return cloneCacheEntry(stored.entry), true
}

func (c *cache) snapshot() []CachedValue {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]CachedValue, 0, len(c.entries))
	for _, stored := range c.entries {
		out = append(out, CachedValue{
			Key:   cloneCacheKey(stored.key),
			Entry: cloneCacheEntry(stored.entry),
		})
	}
	return out
}

func (c *cache) replace(other *cache) {
	other.mu.RLock()
	entries := make(map[string]storedEntry, len(other.entries))
	for id, stored := range other.entries {
		entries[id] = stored
	}
	byLookup := maps.Clone(other.byLookup)
	other.mu.RUnlock()

	c.mu.Lock()
	c.entries = entries
	c.byLookup = byLookup
	c.mu.Unlock()
}

func cloneCacheEntry(entry CacheEntry) CacheEntry {
	cloned := entry
	cloned.Keys = maps.Clone(entry.Keys)
	return cloned
}

func cloneCacheKey(key CacheKey) CacheKey {
	key.Keys = maps.Clone(key.Keys)
	return key
}

func cloneKeys(keys map[string]string) map[string]string {
	return maps.Clone(keys)
}
