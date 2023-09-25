// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Pool defines a pool for process entry allocations
type Pool struct {
	pool *sync.Pool
}

// Get returns a cache entry
func (p *Pool) Get() *model.ProcessCacheEntry {
	return p.pool.Get().(*model.ProcessCacheEntry)
}

// Put returns a cache entry
func (p *Pool) Put(pce *model.ProcessCacheEntry) {
	pce.Reset()
	p.pool.Put(pce)
}

// NewProcessCacheEntryPool returns a new Pool
func NewProcessCacheEntryPool(p *Resolver) *Pool {
	pcep := Pool{pool: &sync.Pool{}}

	pcep.pool.New = func() interface{} {
		return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
			if pce.Ancestor != nil {
				pce.Ancestor.Release()
			}

			p.cacheSize.Dec()

			pcep.Put(pce)
		})
	}

	return &pcep
}
