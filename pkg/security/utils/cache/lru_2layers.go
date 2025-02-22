// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cache holds cache related files
package cache

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"
)

// TwoLayersLRU defines a two layers LRU cache.
type TwoLayersLRU[K1 comparable, K2 comparable, V any] struct {
	sync.RWMutex

	cache *simplelru.LRU[K1, *simplelru.LRU[K2, V]]
	len   *atomic.Uint64
	size  int
}

// NewTwoLayersLRU returns a new cache.
func NewTwoLayersLRU[K1 comparable, K2 comparable, V any](size int) (*TwoLayersLRU[K1, K2, V], error) {
	cache, err := simplelru.NewLRU[K1, *simplelru.LRU[K2, V]](size+1, nil) // +1 as we want to handle the eviction manually
	if err != nil {
		return nil, err
	}

	return &TwoLayersLRU[K1, K2, V]{
		cache: cache,
		len:   atomic.NewUint64(0),
		size:  size,
	}, nil
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (tll *TwoLayersLRU[K1, K2, V]) Add(k1 K1, k2 K2, v V) bool {
	tll.Lock()
	defer tll.Unlock()

	l2LRU, exists := tll.cache.Get(k1)
	if !exists {
		lru, err := simplelru.NewLRU[K2, V](tll.size, nil)
		if err != nil {
			return false
		}
		l2LRU = lru

		tll.cache.Add(k1, lru)
	}

	// check whether exists so that we propagate properly the lener
	if l2LRU.Contains(k2) {
		return l2LRU.Add(k2, v)
	}

	var evicted bool

	// handle len in order to generate potential evictions
	n := tll.len.Load()
	if n >= uint64(tll.size) {
		_, _, _, evicted = tll.removeOldest()
	}

	tll.len.Inc()

	return l2LRU.Add(k2, v) || evicted
}

// RemoveKey1 the whole layer 2 for the given key1.
func (tll *TwoLayersLRU[K1, K2, V]) RemoveKey1(k1 K1) bool {
	tll.Lock()
	defer tll.Unlock()

	l2LRU, exists := tll.cache.Peek(k1)
	if !exists {
		return false
	}

	size := l2LRU.Len()
	tll.len.Sub(uint64(size))

	tll.cache.Remove(k1)

	return true
}

// RemoveKey2 remove the entry in the second layer
func (tll *TwoLayersLRU[K1, K2, V]) RemoveKey2(k1 K1, k2 K2) bool {
	tll.Lock()
	defer tll.Unlock()

	l2LRU, exists := tll.cache.Peek(k1)
	if !exists {
		return false
	}
	if !l2LRU.Remove(k2) {
		return false
	}

	if l2LRU.Len() == 0 {
		tll.cache.Remove(k1)
	}

	tll.len.Dec()

	return true
}

// RemoveOldest removes the oldest element
func (tll *TwoLayersLRU[K1, K2, V]) RemoveOldest() (K1, K2, V, bool) {
	tll.Lock()
	defer tll.Unlock()
	return tll.removeOldest()
}

func (tll *TwoLayersLRU[K1, K2, V]) removeOldest() (k1 K1, k2 K2, v V, evicted bool) {
	k1, l2LRU, exists := tll.cache.GetOldest()
	if !exists {
		return
	}

	k2, v, evicted = l2LRU.RemoveOldest()

	// remove the lru if empty
	if l2LRU.Len() == 0 {
		tll.cache.Remove(k1)
	}

	if evicted {
		tll.len.Dec()
	}

	return k1, k2, v, evicted
}

// Get looks up key values from the cache.
func (tll *TwoLayersLRU[K1, K2, V]) Get(k1 K1, k2 K2) (v V, ok bool) {
	tll.Lock()
	defer tll.Unlock()

	l2LRU, exists := tll.cache.Get(k1)
	if !exists {
		return v, false
	}

	return l2LRU.Get(k2)
}

// Len returns the number of entries
func (tll *TwoLayersLRU[K1, K2, V]) Len() int {
	return int(tll.len.Load())
}

// Walk through all the keys
func (tll *TwoLayersLRU[K1, K2, V]) Walk(cb func(k1 K1, k2 K2, v V)) {
	tll.RLock()
	defer tll.RUnlock()

	for _, k1 := range tll.cache.Keys() {
		if l2LRU, exists := tll.cache.Peek(k1); exists {
			for _, k2 := range l2LRU.Keys() {
				if value, exists := l2LRU.Peek(k2); exists {
					cb(k1, k2, value)
				}
			}
		}
	}
}
