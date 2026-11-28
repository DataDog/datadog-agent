// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import "maps"

// stringCache memoizes strings built by the check, such as tags, across its
// runs.
//
// The aggregator keeps the tags and the hostname of every series it gets, as
// long as the check keeps sending it. When each run builds them again, the
// aggregator keeps the strings of the run that sent a series first, and the
// check those of its last run: two copies of each string. Reusing the strings
// of the previous runs keeps a single one.
//
// The strings that the last keepRuns runs didn't use are dropped, so that those
// of deleted objects don't accumulate. As the aggregator keeps the contexts that
// the last check_sampler_bucket_commits_count_expiry runs sent, the check uses
// the same number, not to build again the strings of a context that missed a
// run while the aggregator keeps the old ones.
type stringCache[K comparable] struct {
	// The entries are pointers so that a hit updates lastRun without writing
	// in the map: a write replaces the key of the entry with the one of the
	// caller, whose strings may be those of an object.
	entries map[K]*stringCacheEntry
	// peak is the largest number of entries since entries was allocated: a
	// map doesn't shrink when entries are deleted.
	peak int
	// run counts the runs of the check that ended.
	run uint64
	// keepRuns is the number of runs that keep a string they don't use, at
	// least 1.
	keepRuns uint64
}

// newStringCache returns a cache that drops the strings that the last keepRuns
// runs didn't use.
func newStringCache[K comparable](keepRuns int) stringCache[K] {
	return stringCache[K]{keepRuns: uint64(max(keepRuns, 1))}
}

type stringCacheEntry struct {
	s       string
	lastRun uint64
}

// endRun drops the strings that the last keepRuns runs, including the one that
// ends, didn't use.
func (c *stringCache[K]) endRun() {
	keepRuns := max(c.keepRuns, 1)
	for key, entry := range c.entries {
		if c.run-entry.lastRun >= keepRuns {
			delete(c.entries, key)
		}
	}
	// Release the memory of the entries of a past peak, e.g. after the cluster
	// scaled down, as a map keeps its size when entries are deleted. A run that
	// replaced all the strings, as a rollout does, doubled the peak too.
	if c.peak > 0 && len(c.entries) <= c.peak/2 {
		// Not maps.Clone, which keeps the capacity of the map it copies
		entries := make(map[K]*stringCacheEntry, len(c.entries))
		maps.Copy(entries, c.entries)
		c.entries = entries
		c.peak = len(entries)
	}
	c.run++
}

// get returns the string of key, and whether it is cached.
func (c *stringCache[K]) get(key K) (string, bool) {
	entry, found := c.entries[key]
	if !found {
		return "", false
	}
	entry.lastRun = c.run
	return entry.s, true
}

// put caches s as the string of key.
func (c *stringCache[K]) put(key K, s string) {
	if c.entries == nil {
		c.entries = make(map[K]*stringCacheEntry)
	}
	c.entries[key] = &stringCacheEntry{s: s, lastRun: c.run}
	c.peak = max(c.peak, len(c.entries))
}

// tagCacheKey identifies a tag: its key, once mapped, and its value.
type tagCacheKey struct {
	key, value string
}
