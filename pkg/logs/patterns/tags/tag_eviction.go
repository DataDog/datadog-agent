// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
)

// CollectEvictables collects all tag entries as evictables (implements eviction.EvictableCollection).
func (tm *TagManager) CollectEvictables() []eviction.Evictable {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	evictables := make([]eviction.Evictable, 0, len(tm.stringToEntry))
	for _, entry := range tm.stringToEntry {
		evictables = append(evictables, entry)
	}
	return evictables
}

// RemoveEvictable removes a specific tag entry from the manager (implements eviction.EvictableCollection).
func (tm *TagManager) RemoveEvictable(item eviction.Evictable) {
	entry := item.(*tagEntry)
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cachedMemoryBytes.Add(-entry.EstimatedBytes())
	delete(tm.stringToEntry, entry.str)
	delete(tm.idToEntry, entry.id)
}

// EvictLowestScoringStrings evicts up to numToEvict tag entries with the lowest eviction scores.
// Returns the list of evicted IDs.
func (tm *TagManager) EvictLowestScoringStrings(numToEvict int, decayFactor float64) []uint64 {
	evictables := eviction.EvictLowestScoring(tm, numToEvict, decayFactor)
	if len(evictables) == 0 {
		return nil
	}
	evictedIDs := make([]uint64, len(evictables))
	for i, ev := range evictables {
		evictedIDs[i] = ev.(*tagEntry).id
	}
	return evictedIDs
}

// EvictToMemoryTarget evicts tag entries until the target memory is freed.
// It uses actual entry sizes rather than averages for precision.
// Returns the list of evicted IDs.
func (tm *TagManager) EvictToMemoryTarget(targetBytesToFree int64, decayFactor float64) []uint64 {
	evictables := eviction.EvictToMemoryTarget(tm, targetBytesToFree, decayFactor)
	if len(evictables) == 0 {
		return nil
	}
	evictedIDs := make([]uint64, len(evictables))
	for i, ev := range evictables {
		evictedIDs[i] = ev.(*tagEntry).id
	}
	return evictedIDs
}

// EstimatedMemoryBytes returns the estimated total memory usage of all tag entries
func (tm *TagManager) EstimatedMemoryBytes() int64 {
	return tm.cachedMemoryBytes.Load()
}
