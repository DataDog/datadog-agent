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
	pool     genericPool[K]
	refs     sync.Map // *K -> *atomic.Int32, including the original owner's reference
	retained atomic.Int64
}

// NewPoolManager creates a PoolManager to manage the underlying genericPool.
func NewPoolManager[K managedPoolTypes](gp genericPool[K]) *PoolManager[K] {
	return &PoolManager[K]{pool: gp}
}

// Get gets an object with one reference from the pool.
func (p *PoolManager[K]) Get() *K {
	return p.pool.Get()
}

// Retain adds a reference. The caller must hold one for the whole call and must
// not release it concurrently: this precondition is what makes Put's fast path
// safe, and violating it aliases buffers rather than leaking them.
func (p *PoolManager[K]) Retain(x *K) {
	if x == nil {
		return
	}
	// Publish the nonempty hint before the entry. The caller's ownership keeps
	// x alive until the entry is ready; unrelated objects still return normally.
	p.retained.Inc()
	refs, loaded := p.refs.LoadOrStore(x, atomic.NewInt32(2))
	if loaded {
		refs.(*atomic.Int32).Inc()
		p.retained.Dec()
	}
}

// Put releases a reference, returning the object only after its final owner.
// The hint is safe to check before the map lookup: Retain increments retained
// before publishing an entry, and the final Put removes the entry before
// decrementing, so any live entry implies retained != 0.
func (p *PoolManager[K]) Put(x *K) {
	if x == nil {
		return
	}
	// Keep the common path (no capture buffers retained) to one atomic load.
	if p.retained.Load() != 0 {
		if refs, ok := p.refs.Load(x); ok {
			if refs.(*atomic.Int32).Dec() != 0 {
				return
			}
			p.refs.Delete(x)
			p.retained.Dec()
		}
	}
	p.pool.Put(x)
}

// Count returns the number of objects with shared ownership still outstanding.
// Upper bound: a Retain of an already-tracked object inflates it until it returns.
func (p *PoolManager[K]) Count() int {
	return int(p.retained.Load())
}
