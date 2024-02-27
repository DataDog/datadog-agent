// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import "sync"

// CyclicMap represents a map with a fixed capacity circular map. When the map is full, adding a new element
// will remove the oldest one.
type CyclicMap[K comparable, V any] struct {
	// mux allows concurrent access to the map.
	mux sync.RWMutex
	// capacity is the maximum number of elements the map can hold.
	capacity int
	// head is the index of the first element in the list.
	head int
	// tail is the index of the last element in the list.
	tail int
	// len is the number of elements in the list.
	len int
	// list of keys in the map.
	list []K
	// values is the map of keys to values.
	values map[K]V
	// onEvict is the callback called when an element is evicted.
	onEvict func(key K, value V)
}

// NewCyclicMap returns a new CyclicMap with the given capacity and onEvict callback.
func NewCyclicMap[K comparable, V any](capacity int, onEvict func(key K, value V)) *CyclicMap[K, V] {
	return &CyclicMap[K, V]{
		capacity: capacity,
		list:     make([]K, 0, capacity),
		values:   make(map[K]V),
		onEvict:  onEvict,
	}
}

// Add adds a new element to the map. If the map is full, the oldest element is removed.
func (c *CyclicMap[K, V]) Add(key K, value V) {
	c.mux.Lock()
	defer c.mux.Unlock()

	// If the list is full, remove the currentElement
	if c.len == c.capacity {
		c.removeOldestNoLock()
		c.list[c.head] = key
	} else {
		c.list = append(c.list, key)
	}
	c.head = (c.head + 1) % c.capacity
	c.len++
	c.values[key] = value
}

// Get returns the value associated with the given key, and a boolean indicating if the key was found.
func (c *CyclicMap[K, V]) Get(key K) (V, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	value, ok := c.values[key]
	return value, ok
}

// Pop removes the value associated with the given key and returns it if exists.
func (c *CyclicMap[K, V]) Pop(key K) (V, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	value, ok := c.values[key]
	if ok {
		delete(c.values, key)
		c.len--
	}
	return value, ok
}

// Len returns the number of elements in the map.
func (c *CyclicMap[K, V]) Len() int {
	c.mux.RLock()
	defer c.mux.RUnlock()

	return c.len
}

// RemoveOldest removes the oldest element from the map.
func (c *CyclicMap[K, V]) RemoveOldest() {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.removeOldestNoLock()
}

// removeOldestNoLock removes the oldest element from the map. Does not lock the map, the caller is responsible
// for locking it.
func (c *CyclicMap[K, V]) removeOldestNoLock() {
	if c.len == 0 {
		return
	}

	// Get the key to remove
	keyToRemove := c.list[c.tail]
	// Call the onEvict callback
	if c.onEvict != nil {
		c.onEvict(keyToRemove, c.values[keyToRemove])
	}
	// Remove the key from the map
	delete(c.values, keyToRemove)

	c.tail = (c.tail + 1) % c.capacity
	c.len--
}
