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
// Uses quickselect (partial sort) for O(N) average-case selection of the K lowest items,
// avoiding the O(N + K log N) cost of a full heap build + extraction.
func EvictLowestScoring(collection EvictableCollection, numToEvict int, decayFactor float64) (evicted []Evictable) {
	if numToEvict <= 0 {
		return nil
	}

	now := time.Now()

	allItems := collection.CollectEvictables()
	n := len(allItems)
	if n == 0 {
		return nil
	}

	scored := make([]heapItem, n)
	for i, item := range allItems {
		scored[i] = heapItem{
			item: item,
			score: CalculateScore(
				item.GetFrequency(),
				item.GetCreatedAt(),
				item.GetLastAccessAt(),
				now,
				decayFactor,
			),
		}
	}

	k := numToEvict
	if k > n {
		k = n
	}

	// Quickselect partitions so scored[0:k] contains the k lowest-scoring items (unordered).
	if k < n {
		quickselectLowest(scored, k)
	}

	evicted = make([]Evictable, k)
	for i := 0; i < k; i++ {
		collection.RemoveEvictable(scored[i].item)
		evicted[i] = scored[i].item
	}

	return evicted
}

// quickselectLowest rearranges items so that the k items with the smallest scores
// end up at indices [0, k). Iterative with median-of-three pivot selection.
func quickselectLowest(items []heapItem, k int) {
	lo, hi := 0, len(items)-1
	target := k - 1
	for lo < hi {
		pivotIdx := medianOfThreePivot(items, lo, hi)
		pivotIdx = partitionItems(items, lo, hi, pivotIdx)
		if pivotIdx == target {
			return
		} else if pivotIdx < target {
			lo = pivotIdx + 1
		} else {
			hi = pivotIdx - 1
		}
	}
}

func medianOfThreePivot(items []heapItem, lo, hi int) int {
	mid := lo + (hi-lo)/2
	if items[lo].score > items[mid].score {
		items[lo], items[mid] = items[mid], items[lo]
	}
	if items[lo].score > items[hi].score {
		items[lo], items[hi] = items[hi], items[lo]
	}
	if items[mid].score > items[hi].score {
		items[mid], items[hi] = items[hi], items[mid]
	}
	return mid
}

func partitionItems(items []heapItem, lo, hi, pivotIdx int) int {
	pivot := items[pivotIdx].score
	items[pivotIdx], items[hi] = items[hi], items[pivotIdx]
	storeIdx := lo
	for i := lo; i < hi; i++ {
		if items[i].score < pivot {
			items[i], items[storeIdx] = items[storeIdx], items[i]
			storeIdx++
		}
	}
	items[storeIdx], items[hi] = items[hi], items[storeIdx]
	return storeIdx
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
