// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eviction provides shared eviction scoring algorithms for patterns and tags.
package eviction

import (
	"container/heap"
	"time"
)

// EvictLowestScoring evicts up to numToEvict items with the lowest eviction scores.
func EvictLowestScoring(collection EvictableCollection, numToEvict int, decayFactor float64) (evicted []Evictable) {
	if numToEvict <= 0 {
		return nil
	}

	now := time.Now()

	// Build heap of all items with their scores
	h := &evictionHeap{
		items: make([]heapItem, 0),
	}

	// Collect all items and calculate scores
	for _, item := range collection.CollectEvictables() {
		score := CalculateScore(
			item.GetFrequency(),
			item.GetCreatedAt(),
			item.GetLastAccessAt(),
			now,
			decayFactor,
		)
		h.items = append(h.items, heapItem{
			item:  item,
			score: score,
		})
	}

	// Build the min-heap: O(N) operation
	heap.Init(h)

	// Extract and evict the N items with lowest scores
	evicted = make([]Evictable, 0, numToEvict)
	for i := 0; i < numToEvict && h.Len() > 0; i++ {
		item := heap.Pop(h).(heapItem)
		collection.RemoveEvictable(item.item)
		evicted = append(evicted, item.item)
	}

	return evicted
}

// EvictToMemoryTarget evicts items until the target memory is freed.
// It uses actual item sizes rather than averages for precision.
func EvictToMemoryTarget(collection EvictableCollection, targetBytesToFree int64, decayFactor float64) (evicted []Evictable) {
	if targetBytesToFree <= 0 {
		return nil
	}

	now := time.Now()

	// Build heap of all items sorted by eviction score
	h := &evictionHeap{
		items: make([]heapItem, 0),
	}

	for _, item := range collection.CollectEvictables() {
		score := CalculateScore(
			item.GetFrequency(),
			item.GetCreatedAt(),
			item.GetLastAccessAt(),
			now,
			decayFactor,
		)
		h.items = append(h.items, heapItem{
			item:  item,
			score: score,
		})
	}

	heap.Init(h)

	// Evict items until we've freed enough memory
	evicted = make([]Evictable, 0)
	bytesFreed := int64(0)

	for h.Len() > 0 && bytesFreed < targetBytesToFree {
		item := heap.Pop(h).(heapItem)
		collection.RemoveEvictable(item.item)
		bytesFreed += item.item.EstimatedBytes()
		evicted = append(evicted, item.item)
	}

	return evicted
}
