// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Pool defines a pool for process entry allocations
type Pool struct {
	cacheSize *atomic.Uint64
}

// Get returns a cache entry
func (p *Pool) Get() *model.ProcessCacheEntry {
	p.cacheSize.Inc()
	return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
		if pce.Ancestor != nil {
			pce.Ancestor.Release()
		}

		p.cacheSize.Dec()
	})
}

// GetCacheSize returns the cache size of the pool
func (p *Pool) GetCacheSize() uint64 {
	return p.cacheSize.Load()
}

// NewProcessCacheEntryPool returns a new Pool
func NewProcessCacheEntryPool() *Pool {
	return &Pool{cacheSize: atomic.NewUint64(0)}
}
