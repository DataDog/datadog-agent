// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"strings"
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
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

func newCache() *cache {
	return &cache{
		entries: make(map[string]CacheEntry),
	}
}

func (c *cache) set(key CacheKey, entry CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key.cacheKey()] = entry
}

func (c *cache) delete(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key.cacheKey())
}

func (c *cache) deletePrefix(prefix CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		cacheKey, ok := parseCacheKey(key)
		if !ok {
			continue
		}
		if cacheKeyMatchesPrefix(cacheKey, prefix) {
			delete(c.entries, key)
		}
	}
}

func (c *cache) get(key CacheKey) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key.cacheKey()]
	if !ok {
		return CacheEntry{}, false
	}
	return cloneCacheEntry(entry), true
}

func (c *cache) snapshot() []CachedValue {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]CachedValue, 0, len(c.entries))
	for key, entry := range c.entries {
		cacheKey, ok := parseCacheKey(key)
		if !ok {
			continue
		}
		out = append(out, CachedValue{
			Key:   cacheKey,
			Entry: cloneCacheEntry(entry),
		})
	}
	return out
}

func cloneCacheEntry(entry CacheEntry) CacheEntry {
	cloned := entry
	if len(entry.Keys) > 0 {
		cloned.Keys = make(map[string]string, len(entry.Keys))
		for key, value := range entry.Keys {
			cloned.Keys[key] = value
		}
	}
	return cloned
}

func parseCacheKey(raw string) (CacheKey, bool) {
	open := strings.LastIndex(raw, "{")
	if open == -1 || !strings.HasSuffix(raw, "}") {
		return CacheKey{Path: raw}, true
	}

	path := raw[:open]
	keysRaw := raw[open+1 : len(raw)-1]
	if keysRaw == "" {
		return CacheKey{Path: path}, true
	}

	keys := make(map[string]string)
	for _, part := range strings.Split(keysRaw, ",") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			return CacheKey{}, false
		}
		keys[name] = value
	}
	return CacheKey{Path: path, Keys: keys}, true
}

func cloneKeys(keys map[string]string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(keys))
	for key, value := range keys {
		cloned[key] = value
	}
	return cloned
}
