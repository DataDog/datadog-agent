// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	sync.RWMutex
	CGroupContext    model.CGroupContext
	ContainerContext model.ContainerContext

	deleted *atomic.Bool
	pids    map[uint32]bool
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(containerContext model.ContainerContext, cgroupContext model.CGroupContext, pid uint32) *CacheEntry {
	cacheEntry := CacheEntry{
		deleted:          atomic.NewBool(false),
		ContainerContext: containerContext,
		CGroupContext:    cgroupContext,
		pids:             make(map[uint32]bool, 10),
	}
	cacheEntry.pids[pid] = true

	// should not happen but added as a safe-guard to avoid overriding
	// a Releasable pointer which would cause Releasable callbacks to not be called
	if cacheEntry.ContainerContext.Releasable == nil {
		// we need this here because the newCGroup entry will be propagated back to both the cgroup resolver
		// and the process cache entry upon context context resolution
		cacheEntry.ContainerContext.Releasable = &model.Releasable{}
	}

	return &cacheEntry
}

// GetPIDs returns the list of pids for the current workload
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.RLock()
	defer cgce.RUnlock()

	pids := make([]uint32, len(cgce.pids))
	i := 0
	for k := range cgce.pids {
		pids[i] = k
		i++
	}

	return pids
}

// RemovePID removes the provided pid from the list of pids
func (cgce *CacheEntry) RemovePID(pid uint32) int {
	cgce.Lock()
	defer cgce.Unlock()

	delete(cgce.pids, pid)

	return len(cgce.pids)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.pids[pid] = true
}

// SetPIDs set the pids list
func (cgce *CacheEntry) SetPIDs(pids []uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	clear(cgce.pids)
	for _, pid := range pids {
		cgce.pids[pid] = true
	}
}

// RemovePIDs removes the pids and return the new size
func (cgce *CacheEntry) RemovePIDs(pids []uint32) int {
	cgce.Lock()
	defer cgce.Unlock()

	for _, pid := range pids {
		delete(cgce.pids, pid)
	}

	return len(cgce.pids)
}

// IsDeleted returns whether the cache entry is deleted
func (cgce *CacheEntry) IsDeleted() bool {
	return cgce.deleted.Load()
}

// SetAsDeleted sets the cache entry as deleted
func (cgce *CacheEntry) SetAsDeleted() {
	cgce.deleted.Store(true)
}

// ContainsPID returns whether the cache entry contains the pid
func (cgce *CacheEntry) ContainsPID(pid uint32) bool {
	cgce.Lock()
	defer cgce.Unlock()

	_, exists := cgce.pids[pid]
	return exists
}

// CGroupContextEquals returns if the cgroups are equal
func (cgce *CacheEntry) CGroupContextEquals(other *CacheEntry) bool {
	cgce.RLock()
	other.RLock()
	defer cgce.RUnlock()
	defer other.RUnlock()

	return cgce.CGroupContext.Equals(&other.CGroupContext)
}
