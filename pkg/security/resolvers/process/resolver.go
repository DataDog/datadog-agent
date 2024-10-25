// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// Pool defines a pool for process entry allocations
type Pool struct {
	cacheSize atomic.Int64
	pool      *ddsync.TypedPool[model.ProcessCacheEntry]
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

// IncCacheSize increments the cache size
func (p *Pool) IncCacheSize() {
	p.cacheSize.Add(1)
}

// GetCacheSize returns the current cache size
func (p *Pool) GetCacheSize() int64 {
	return p.cacheSize.Load()
}

// NewProcessCacheEntryPool returns a new Pool
func NewProcessCacheEntryPool() *Pool {
	pcep := Pool{}
	pcep.pool = ddsync.NewTypedPool(func() *model.ProcessCacheEntry {
		return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
			if pce.Ancestor != nil {
				pce.Ancestor.Release()
			}

			pcep.cacheSize.Add(-1)

			pcep.Put(pce)
		})
	})

	return &pcep
}
