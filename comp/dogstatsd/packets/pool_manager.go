// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"sync"
	"unsafe"

	"go.uber.org/atomic"
)

type managedPoolTypes interface {
	[]byte | Packet
}

type genericPool[K managedPoolTypes] interface {
	Get() *K
	Put(x *K)
}

// PoolManager helps manage sync pools so multiple references to the same pool objects may be held.
type PoolManager[K managedPoolTypes] struct {
	pool genericPool[K]
	refs sync.Map

	passthru *atomic.Bool

	sync.RWMutex
}

// NewPoolManager creates a PoolManager to manage the underlying genericPool.
func NewPoolManager[K managedPoolTypes](gp genericPool[K]) *PoolManager[K] {
	return &PoolManager[K]{
		pool:     gp,
		passthru: atomic.NewBool(true),
	}
}

// Get gets an object from the pool.
func (p *PoolManager[K]) Get() *K {
	return p.pool.Get()
}

// Put declares intent to return an object to the pool. In passthru mode the object is immediately
// returned to the pool, otherwise we wait until the object is put by all (only 2 currently supported)
// reference holders before actually returning it to the object pool.
func (p *PoolManager[K]) Put(x *K) {

	if p.IsPassthru() {
		p.pool.Put(x)
		return
	}

	ref := unsafe.Pointer(x)

	// This lock is not to guard the map, it's here to
	// avoid adding items to the map while flushing.
	p.RLock()

	_, loaded := p.refs.LoadAndDelete(ref)
	if loaded {
		p.pool.Put(x)
	} else {
		// reference does not exist, account.
		p.refs.Store(ref, x)
	}

	// relatively hot path so not deferred
	p.RUnlock()
}

// IsPassthru returns a boolean telling us if the PoolManager is in passthru mode or not.
func (p *PoolManager[K]) IsPassthru() bool {
	return p.passthru.Load()
}

// SetPassthru sets the passthru mode to the specified value. It will flush the sccounting before
// enabling passthru mode.
func (p *PoolManager[K]) SetPassthru(b bool) {
	if b {
		p.passthru.Store(true)
		p.Flush()
	} else {
		p.passthru.Store(false)
	}
}

// Count returns the number of elements accounted by the PoolManager.
func (p *PoolManager[K]) Count() int {
	p.RLock()
	defer p.RUnlock()

	size := 0
	p.refs.Range(func(k, v interface{}) bool {
		size++
		return true
	})

	return size
}

// Flush flushes all objects back to the object pool, and stops tracking any pending objects.
func (p *PoolManager[K]) Flush() {
	p.Lock()
	defer p.Unlock()

	p.refs.Range(func(k, v any) bool {
		p.pool.Put(v.(*K))
		p.refs.Delete(k)
		return true
	})
}
