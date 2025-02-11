// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

type Resolver struct {
	sync.RWMutex
	entryCache map[uint32]*model.ProcessCacheEntry
}

// Pool defines a pool for process entry allocations
type Pool struct {
	pool *ddsync.TypedPool[model.ProcessCacheEntry]
}

// Get returns a cache entry
func (p *Pool) Get() *model.ProcessCacheEntry {
	return p.pool.Get()
}

// Put returns a cache entry
func (p *Pool) Put(pce *model.ProcessCacheEntry) {
	pce.Reset()
	p.pool.Put(pce)
}

// Walk iterates through the entire tree and call the provided callback on each entry
func (p *Resolver) Walk(callback func(entry *model.ProcessCacheEntry)) {
	p.RLock()
	defer p.RUnlock()

	for _, entry := range p.entryCache {
		callback(entry)
	}
}

// NewProcessCacheEntryPool returns a new Pool
func NewProcessCacheEntryPool(onRelease func()) *Pool {
	pcep := Pool{}
	pcep.pool = ddsync.NewTypedPool(func() *model.ProcessCacheEntry {
		return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
			if pce.Ancestor != nil {
				pce.Ancestor.Release()
			}

			onRelease()

			pcep.Put(pce)
		})
	})

	return &pcep
}
