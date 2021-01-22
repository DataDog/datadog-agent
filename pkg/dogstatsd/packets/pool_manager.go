// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"sync"
	"sync/atomic"
)

type genericPool interface {
	Get() interface{}
	Put(x interface{})
}

type PoolManager struct {
	pool genericPool
	refs sync.Map

	passthru int32

	sync.RWMutex
}

func NewPoolManager(gp genericPool) *PoolManager {
	return &PoolManager{
		pool:     gp,
		passthru: int32(1),
	}
}

func (p *PoolManager) Get() interface{} {
	return p.pool.Get()
}

func (p *PoolManager) Put(x interface{}) {

	if p.IsPassthru() {
		p.pool.Put(x)
		return
	}

	// This lock is not to guard the map, it's here to
	// avoid adding items to the map while flushing.
	p.RLock()

	// TODO: use LoadAndDelete when go 1.15 is introduced

	_, loaded := p.refs.Load(x)
	if loaded {
		// reference exists, put back.
		p.refs.Delete(x)
		p.pool.Put(x)
	} else {
		// reference does not exist, account.
		p.refs.Load(x)
	}

	// relatively hot-path so not deferred
	p.RUnlock()
}

func (p *PoolManager) IsPassthru() bool {
	return atomic.LoadInt32(&(p.passthru)) != 0
}

func (p *PoolManager) SetPassthru(b bool) {
	if b {
		atomic.StoreInt32(&(p.passthru), 1)
	} else {
		atomic.StoreInt32(&(p.passthru), 0)
	}
}

func (p *PoolManager) Flush() {
	p.Lock()
	defer p.Unlock()

	p.refs.Range(func(k, v interface{}) bool {
		p.pool.Put(k)
		return true
	})

}
