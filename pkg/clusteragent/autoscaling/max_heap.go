// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"container/heap"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

// TimestampKey is a struct that holds a timestamp and key for a `DatadogPodAutoscaler` object
type TimestampKey struct {
	Timestamp time.Time
	Key       string
}

// MaxTimestampKeyHeap is a heap that sorts TimestampKey objects by timestamp in descending order
type MaxTimestampKeyHeap []TimestampKey

// Len returns the length of the heap
func (h MaxTimestampKeyHeap) Len() int { return len(h) }

// Less returns true if the timestamp at index i is after the timestamp at index j
func (h MaxTimestampKeyHeap) Less(i, j int) bool {
	return LessThan(h[i], h[j])
}

// LessThan returns true if the timestamp of k1 is after the timestamp of k2
func LessThan(k1, k2 TimestampKey) bool {
	return k1.Timestamp.After(k2.Timestamp) || (k1.Timestamp.Equal(k2.Timestamp) && k1.Key < k2.Key)
}

// Swap swaps the elements at indices i and j
func (h MaxTimestampKeyHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push adds an element to the heap while preserving max heap ordering
func (h *MaxTimestampKeyHeap) Push(x interface{}) {
	*h = append(*h, x.(TimestampKey))
}

// Pop removes the top element from the heap while preserving max heap ordering
func (h *MaxTimestampKeyHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Peek returns the top element of the heap without removing it
func (h *MaxTimestampKeyHeap) Peek() TimestampKey {
	return (*h)[0]
}

// NewMaxHeap returns a new MaxTimestampKeyHeap
func NewMaxHeap() *MaxTimestampKeyHeap {
	h := &MaxTimestampKeyHeap{}
	heap.Init(h)
	return h
}

// FindIdx returns the index of the given key in the heap and a boolean indicating if the key was found
func (h *MaxTimestampKeyHeap) FindIdx(key string) (int, bool) {
	for idx, k := range *h {
		if k.Key == key {
			return idx, true
		}
	}
	return 0, false
}

// HashHeap is a struct that holds a MaxHeap and a set of keys that exist in the heap
type HashHeap struct {
	MaxHeap MaxTimestampKeyHeap
	Keys    map[string]bool
	maxSize int
}

// NewHashHeap returns a new MaxHeap with the given max size
func NewHashHeap(maxSize int) *HashHeap {
	return &HashHeap{
		MaxHeap: *NewMaxHeap(),
		Keys:    make(map[string]bool),
		maxSize: maxSize,
	}
}

// InsertIntoHeap returns true if the key already exists in the max heap or was inserted correctly
// Used as an ObserverFunc; accept sender as parameter to match ObserverFunc signature
func (h *HashHeap) InsertIntoHeap(key, _sender string, obj any) {
	// Already in heap, do not try to insert
	if h.Exists(key) {
		return
	}

	// Cast object into PodAutoscalerInternal
	pai, ok := obj.(model.PodAutoscalerInternal)
	if !ok {
		return
	}

	if pai.CreationTimestamp().IsZero() {
		return
	}

	newTimestampKey := TimestampKey{
		Timestamp: pai.CreationTimestamp(),
		Key:       key,
	}

	if h.MaxHeap.Len() >= h.maxSize {
		top := h.MaxHeap.Peek()
		// If the new key is newer than or equal to the top key, do not insert
		if LessThan(newTimestampKey, top) {
			return
		}
		delete(h.Keys, top.Key)
		heap.Pop(&h.MaxHeap)
	}

	heap.Push(&h.MaxHeap, newTimestampKey)
	h.Keys[key] = true
}

// DeleteFromHeap removes the given key from the max heap
// Used as an ObserverFunc; accept sender and obj as parameter to match ObserverFunc signature
func (h *HashHeap) DeleteFromHeap(key, _sender string, _obj any) {
	// Key did not exist in heap, return early
	if !h.Exists(key) {
		return
	}
	idx, found := h.MaxHeap.FindIdx(key)
	if !found {
		return
	}
	heap.Remove(&h.MaxHeap, idx)
	delete(h.Keys, key)
}

// Exists returns true if the given key exists in the heap
func (h *HashHeap) Exists(key string) bool {
	_, ok := h.Keys[key]
	return ok
}
