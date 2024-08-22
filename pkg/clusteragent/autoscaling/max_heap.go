// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"container/heap"
	"time"
)

// TimestampKey is a struct that holds a timestamp and key for a `DatadogPodAutoscaler` object
type TimestampKey struct {
	Timestamp time.Time
	Key       string
}

// MaxHeap is a heap that sorts TimestampKey objects by timestamp in descending order
type MaxHeap []TimestampKey

// Len returns the length of the heap
func (h MaxHeap) Len() int { return len(h) }

// Less returns true if the timestamp at index i is after the timestamp at index j
func (h MaxHeap) Less(i, j int) bool { return h[i].Timestamp.After(h[j].Timestamp) }

// Swap swaps the elements at indices i and j
func (h MaxHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push adds an element to the heap while preserving max heap ordering
func (h *MaxHeap) Push(x interface{}) {
	*h = append(*h, x.(TimestampKey))
}

// Pop removes the top element from the heap while preserving max heap ordering
func (h *MaxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Peek returns the top element of the heap without removing it
func (h *MaxHeap) Peek() TimestampKey {
	return (*h)[0]
}

// NewMaxHeap returns a new MaxHeap
func NewMaxHeap() *MaxHeap {
	h := &MaxHeap{}
	heap.Init(h)
	return h
}

// FindIdx returns the index of the given key in the heap and a boolean indicating if the key was found
func (h *MaxHeap) FindIdx(key string) (int, bool) {
	for idx, k := range *h {
		if k.Key == key {
			return idx, true
		}
	}
	return 0, false
}

// AutoscalingHeap is a struct that holds a MaxHeap and a set of keys that exist in the heap
type AutoscalingHeap struct {
	MaxHeap MaxHeap
	Keys    map[string]bool
	maxSize int
}

// NewAutoscalingHeap returns a new AutoscalingHeap with the given max size
func NewAutoscalingHeap(maxSize int) *AutoscalingHeap {
	return &AutoscalingHeap{
		MaxHeap: *NewMaxHeap(),
		Keys:    make(map[string]bool),
		maxSize: maxSize,
	}
}

// InsertIntoHeap returns true if the key already exists in the max heap or was inserted correctly
func (h *AutoscalingHeap) InsertIntoHeap(k TimestampKey) bool {
	// Already in heap, do not try to insert
	if _, ok := h.Keys[k.Key]; ok {
		return true
	}

	if h.MaxHeap.Len() >= h.maxSize {
		top := h.MaxHeap.Peek()
		// If the new key is newer than or equal to the top key, do not insert
		if top.Timestamp.Before(k.Timestamp) || top.Timestamp.Equal(k.Timestamp) {
			return false
		} else {
			delete(h.Keys, top.Key)
			heap.Pop(&h.MaxHeap)
		}
	}

	heap.Push(&h.MaxHeap, k)
	h.Keys[k.Key] = true
	return true
}

// DeleteFromHeap removes the given key from the max heap
func (h *AutoscalingHeap) DeleteFromHeap(key string) {
	// Key did not exist in heap, return early
	if _, ok := h.Keys[key]; !ok {
		return
	}
	idx, found := h.MaxHeap.FindIdx(key)
	if !found {
		return
	}
	heap.Remove(&h.MaxHeap, idx)
	delete(h.Keys, key)
}
