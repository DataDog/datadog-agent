// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"sync"

	"go.uber.org/atomic"
)

type managedPoolTypes interface {
	[]byte | Packet
}

type genericPool[K managedPoolTypes] interface {
	Get() *K
	Put(x *K)
}

// PoolManager returns objects to their pool after all owners release them.
// Objects have one owner by default; Retain explicitly adds another owner.
type PoolManager[K managedPoolTypes] struct {
	pool genericPool[K]

	mu       sync.Mutex
	refs     map[*K]int32 // retained objects -> outstanding references
	retained atomic.Int64 // len(refs), so Put skips the lock when nothing is retained
}

// NewPoolManager creates a PoolManager to manage the underlying genericPool.
func NewPoolManager[K managedPoolTypes](gp genericPool[K]) *PoolManager[K] {
	return &PoolManager[K]{pool: gp, refs: make(map[*K]int32)}
}

// Get gets an object with one reference from the pool.
func (p *PoolManager[K]) Get() *K {
	return p.pool.Get()
}

// Retain adds a reference. The caller must hold one for the whole call.
func (p *PoolManager[K]) Retain(x *K) {
	if x == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.refs[x]
	if !ok {
		n = 1 // the caller's reference
	}
	p.refs[x] = n + 1
	p.retained.Store(int64(len(p.refs)))
}

// Put releases a reference, returning the object only after its final owner.
func (p *PoolManager[K]) Put(x *K) {
	if x == nil {
		return
	}
	// The unlocked hint is safe: every Put of x that must see an entry is ordered
	// after the Retain that created it.
	if p.retained.Load() != 0 && p.release(x) {
		return
	}
	p.pool.Put(x)
}

// release drops a reference to a retained object, reporting whether others remain.
func (p *PoolManager[K]) release(x *K) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.refs[x]
	if !ok {
		return false
	}
	if n > 1 {
		p.refs[x] = n - 1
		return true
	}
	delete(p.refs, x)
	p.retained.Store(int64(len(p.refs)))
	return false
}

// Count returns the number of retained objects not yet returned to the pool.
func (p *PoolManager[K]) Count() int {
	return int(p.retained.Load())
}
