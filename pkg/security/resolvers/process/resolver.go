// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Pool defines a pool for process entry allocations
type Pool struct {
	onRelease func()
}

// Get returns a cache entry
func (p *Pool) Get() *model.ProcessCacheEntry {
	return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
		if pce.Ancestor != nil {
			pce.Ancestor.Release()
		}

		if p.onRelease != nil {
			p.onRelease()
		}
	})
}

// NewProcessCacheEntryPool returns a new Pool
func NewProcessCacheEntryPool(onRelease func()) *Pool {
	return &Pool{onRelease: onRelease}
}
