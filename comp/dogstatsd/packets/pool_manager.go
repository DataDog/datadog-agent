// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"sync"
	"unsafe"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type genericPool interface {
	Get() interface{}
	Put(x interface{})
}

// PoolManager helps manage sync pools so multiple references to the same pool objects may be held.
type PoolManager struct {
	pool genericPool
	refs sync.Map

	passthru *atomic.Bool

	sync.RWMutex
}

// NewPoolManager creates a PoolManager to manage the underlying genericPool.
func NewPoolManager(gp genericPool) *PoolManager {
	return &PoolManager{
		pool:     gp,
		passthru: atomic.NewBool(true),
	}
}

// Get gets an object from the pool.
func (p *PoolManager) Get() interface{} {
	return p.pool.Get()
}

// Put declares intent to return an object to the pool. In passthru mode the object is immediately
// returned to the pool, otherwise we wait until the object is put by all (only 2 currently supported)
// reference holders before actually returning it to the object pool.
func (p *PoolManager) Put(x interface{}) {

	if p.IsPassthru() {
		p.pool.Put(x)
		return
	}

	var ref unsafe.Pointer
	switch t := x.(type) {
	case *[]byte:
		ref = unsafe.Pointer(t)
	case *Packet:
		ref = unsafe.Pointer(t)
	default:
		log.Debugf("Unsupported type by pool manager")
		return
	}

	// This lock is not to guard the map, it's here to
	// avoid adding items to the map while flushing.
	p.RLock()

	// TODO: use LoadAndDelete when go 1.15 is introduced
	_, loaded := p.refs.Load(ref)
	if loaded {
		// reference exists, put back.
		p.refs.Delete(ref)
		p.pool.Put(x)
	} else {
		// reference does not exist, account.
		p.refs.Store(ref, x)
	}

	// relatively hot path so not deferred
	p.RUnlock()
}

// IsPassthru returns a boolean telling us if the PoolManager is in passthru mode or not.
func (p *PoolManager) IsPassthru() bool {
	return p.passthru.Load()
}

// SetPassthru sets the passthru mode to the specified value. It will flush the sccounting before
// enabling passthru mode.
func (p *PoolManager) SetPassthru(b bool) {
	panic("not called")
}

// Count returns the number of elements accounted by the PoolManager.
func (p *PoolManager) Count() int {
	panic("not called")
}

// Flush flushes all objects back to the object pool, and stops tracking any pending objects.
func (p *PoolManager) Flush() {
	panic("not called")
}
