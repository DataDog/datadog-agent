// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package cache

import (
	"container/list"
	"sync"
)

// maxKeyCacheSize specifies the maximum allowed size of the key cache.
// 10MB desired size divided by size of one item (uint64 = 8 bytes + int16 = 2 bytes)
const maxKeyCacheSize = (10 * 1024 * 1024) / (2 + 8)

// keyCache keeps track of a limited number of keyCacheItem, dropping the oldest one added
// if the size limit is reached.
type keyCache struct {
	maxLen int // maximum number of items allowed

	mu   sync.RWMutex
	ll   *list.List
	keys map[uint64]*list.Element
}

type keyCacheItem struct {
	key    uint64
	reason EvictReason
}

func newKeyCache(max int) *keyCache {
	return &keyCache{
		maxLen: max,
		ll:     list.New(),
		keys:   make(map[uint64]*list.Element),
	}
}

func (kc *keyCache) seen(key uint64) (r EvictReason, ok bool) {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	el, ok := kc.keys[key]
	if ok {
		return el.Value.(*keyCacheItem).reason, true
	}
	return EvictReason(0), false
}

func (kc *keyCache) len() int {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.ll.Len()
}

// Mark marks that the given key was evicted for the given reason. It reports whether the key was seen
// before and returns the original reason for eviction.
func (kc *keyCache) Mark(key uint64, reason EvictReason) (r EvictReason, seen bool) {
	if v, ok := kc.seen(key); ok {
		return v, true
	}
	kc.mu.Lock()
	defer kc.mu.Unlock()
	kc.keys[key] = kc.ll.PushFront(&keyCacheItem{
		reason: reason,
		key:    key,
	})
	for kc.ll.Len() > kc.maxLen {
		el := kc.ll.Back()
		if el == nil {
			break
		}
		kc.ll.Remove(el)
		delete(kc.keys, el.Value.(*keyCacheItem).key)
	}
	return reason, false
}
