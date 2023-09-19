// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dentry

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type betterCache[K comparable, V any] struct {
	lock sync.Mutex
	lru  *simplelru.LRU[K, V]
}

func newBetterCache[K comparable, V any](size int) (*betterCache[K, V], error) {
	inner, err := simplelru.NewLRU[K, V](size, nil)
	if err != nil {
		return nil, err
	}

	return &betterCache[K, V]{
		lru: inner,
	}, nil
}

func (c *betterCache[K, V]) Get(key K) (V, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Get(key)
}

func (c *betterCache[K, V]) Add(key K, value V) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Add(key, value)
}

func (c *betterCache[K, V]) FilterByKeys(shouldRemove func(K) bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, key := range c.lru.Keys() {
		if shouldRemove(key) {
			c.lru.Remove(key)
		}
	}
}
