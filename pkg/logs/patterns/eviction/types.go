// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eviction provides shared eviction scoring algorithms for patterns and tags.
package eviction

import (
	"time"
)

// Evictable represents any item that can be evicted based on usage patterns.
type Evictable interface {
	// GetFrequency returns the usage count/frequency
	GetFrequency() float64

	// GetCreatedAt returns when the item was created
	GetCreatedAt() time.Time

	// GetLastAccessAt returns when the item was last accessed
	GetLastAccessAt() time.Time

	// EstimatedBytes returns the approximate memory footprint of the item
	EstimatedBytes() int64
}

// EvictableCollection represents a collection of evictables that can be evicted.
type EvictableCollection interface {
	// CollectEvictables returns all evictable items from the collection
	CollectEvictables() []Evictable

	// RemoveEvictable removes a specific item from the collection
	RemoveEvictable(item Evictable)
}

// heapItem wraps an Evictable with its cached eviction score for heap operations.
type heapItem struct {
	item  Evictable
	score float64
}

// evictionHeap implements heap.Interface for efficient eviction based on scores.
// It's a min-heap: items with the lowest eviction scores bubble to the top.
type evictionHeap struct {
	items []heapItem
}

// Len returns the number of items in the heap
func (h evictionHeap) Len() int { return len(h.items) }

// Less reports whether item i should sort before item j (min-heap: lower scores first)
func (h evictionHeap) Less(i, j int) bool {
	return h.items[i].score < h.items[j].score
}

// Swap exchanges items i and j
func (h evictionHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

// Push adds an item to the heap (required by heap.Interface)
func (h *evictionHeap) Push(x interface{}) {
	h.items = append(h.items, x.(heapItem))
}

// Pop removes and returns the minimum item (required by heap.Interface)
func (h *evictionHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[0 : n-1]
	return item
}
