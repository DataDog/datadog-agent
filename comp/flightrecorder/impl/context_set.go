// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"sync/atomic"
)

const numShards = 64

// contextSet is a sharded set of uint64 context keys that tracks which metric
// contexts have already been sent with full name+tags definitions. It replaces
// sync.Map to reduce per-entry memory overhead (~16 bytes vs ~120 bytes) and
// GC pressure (flat uint64 keys vs boxed interface{} pointers).
//
// 64 shards with RWMutex provide low contention at 10+ goroutines: the hot
// path (known key) takes only a shared RLock, and the low bits of the hash key
// distribute evenly across shards.
type contextSet struct {
	shards [numShards]contextShard
	cap    int          // max total entries; 0 = unlimited
	count  atomic.Int64 // approximate total (not exact under race, OK)
}

type contextShard struct {
	mu sync.RWMutex
	m  map[uint64]struct{}
}

// newContextSet creates a context set with the given capacity cap.
// If cap <= 0, no cap is enforced.
func newContextSet(cap int) *contextSet {
	cs := &contextSet{cap: cap}
	for i := range cs.shards {
		cs.shards[i].m = make(map[uint64]struct{})
	}
	return cs
}

// IsKnown returns true if the key was already in the set. If not, it inserts
// the key and returns false. This is the hot path called at 100K+/s.
func (cs *contextSet) IsKnown(key uint64) bool {
	s := &cs.shards[key&(numShards-1)]

	// Fast path: read lock for the common case (key already seen).
	s.mu.RLock()
	_, ok := s.m[key]
	s.mu.RUnlock()
	if ok {
		return true
	}

	// Slow path: upgrade to write lock for insertion.
	s.mu.Lock()
	if _, ok := s.m[key]; ok {
		s.mu.Unlock()
		return true // another goroutine inserted between RUnlock and Lock
	}
	s.m[key] = struct{}{}
	s.mu.Unlock()
	cs.count.Add(1)
	return false
}

// Remove deletes a key from the set, making it "unknown" again. Called when
// a context definition is evicted from the def ring before being flushed.
func (cs *contextSet) Remove(key uint64) {
	s := &cs.shards[key&(numShards-1)]
	s.mu.Lock()
	if _, ok := s.m[key]; ok {
		delete(s.m, key)
		cs.count.Add(-1)
	}
	s.mu.Unlock()
}

// Reset clears all entries, forcing all context definitions to be re-sent.
func (cs *contextSet) Reset() {
	for i := range cs.shards {
		s := &cs.shards[i]
		s.mu.Lock()
		s.m = make(map[uint64]struct{})
		s.mu.Unlock()
	}
	cs.count.Store(0)
}

// CheckCap checks whether the set has exceeded its capacity. If so, it resets
// the set and returns true. Called periodically (e.g., on flush).
func (cs *contextSet) CheckCap() bool {
	if cs.cap > 0 && cs.count.Load() > int64(cs.cap) {
		cs.Reset()
		return true
	}
	return false
}

// Len returns the approximate number of entries in the set.
func (cs *contextSet) Len() int64 {
	return cs.count.Load()
}
