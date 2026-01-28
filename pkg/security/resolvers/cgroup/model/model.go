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

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	lock sync.RWMutex

	// These context fields shouldn't be mutated after the cache entry is created
	cgroupContext    model.CGroupContext
	containerContext model.ContainerContext

	deleted atomic.Bool
	pids    map[uint32]bool
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(containerContext model.ContainerContext, cgroupContext model.CGroupContext, pid uint32) *CacheEntry {
	cacheEntry := CacheEntry{
		containerContext: containerContext,
		cgroupContext:    cgroupContext,
		pids:             make(map[uint32]bool, 10),
	}
	cacheEntry.pids[pid] = true

	// should not happen but added as a safe-guard to avoid overriding
	// a Releasable pointer which would cause Releasable callbacks to not be called
	if cacheEntry.containerContext.Releasable == nil {
		// we need this here because the NewCacheEntry entry will be propagated back to both the cgroup resolver
		// and the process cache entry upon context context resolution
		cacheEntry.containerContext.Releasable = &model.Releasable{}
	}

	return &cacheEntry
}

// GetCGroupContext returns the cgroup context
func (cgce *CacheEntry) GetCGroupContext() model.CGroupContext {
	return cgce.cgroupContext
}

// GetCGroupID returns the cgroup ID
func (cgce *CacheEntry) GetCGroupID() containerutils.CGroupID {
	return cgce.cgroupContext.CGroupID
}

// GetCGroupPathKey returns the cgroup path key
func (cgce *CacheEntry) GetCGroupPathKey() model.PathKey {
	return cgce.cgroupContext.CGroupPathKey
}

// GetContainerID returns the container ID
func (cgce *CacheEntry) GetContainerContext() model.ContainerContext {
	return cgce.containerContext
}

// GetContainerID returns the container ID
func (cgce *CacheEntry) GetContainerID() containerutils.ContainerID {
	return cgce.containerContext.ContainerID
}

// GetCGroupInode returns the inode of the cgroup path key
func (cgce *CacheEntry) GetCGroupInode() uint64 {
	return cgce.cgroupContext.CGroupPathKey.Inode
}

// IsCGroupContextResolved returns whether the cgroup context is resolved
func (cgce *CacheEntry) IsCGroupContextResolved() bool {
	return cgce.cgroupContext.IsResolved()
}

// IsContainerContextNull returns whether the container context is null
func (cgce *CacheEntry) IsContainerContextNull() bool {
	return cgce.containerContext.IsNull()
}

// IsCGroupContextNull returns whether the cgroup context is null
func (cgce *CacheEntry) IsCGroupContextNull() bool {
	return cgce.cgroupContext.IsNull()
}

// CallReleaseCallbacks releases the callbacks of the cache entry
func (cgce *CacheEntry) CallReleaseCallbacks() {
	if cgce.containerContext.Releasable != nil {
		cgce.containerContext.Releasable.CallReleaseCallback()
	}
	if cgce.cgroupContext.Releasable != nil {
		cgce.cgroupContext.Releasable.CallReleaseCallback()
	}
}

// GetPIDs returns the list of pids
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.lock.RLock()
	defer cgce.lock.RUnlock()

	pids := make([]uint32, len(cgce.pids))
	i := 0
	for k := range cgce.pids {
		pids[i] = k
		i++
	}

	return pids
}

// RemovePID removes the provided pid from the list of pids and return the new size
func (cgce *CacheEntry) RemovePID(pid uint32) int {
	cgce.lock.Lock()
	defer cgce.lock.Unlock()

	delete(cgce.pids, pid)

	return len(cgce.pids)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.lock.Lock()
	defer cgce.lock.Unlock()

	cgce.pids[pid] = true
}

// SetPIDs set the pids list
func (cgce *CacheEntry) SetPIDs(pids []uint32) {
	cgce.lock.Lock()
	defer cgce.lock.Unlock()

	clear(cgce.pids)
	for _, pid := range pids {
		cgce.pids[pid] = true
	}
}

// RemovePIDs removes the pids and return the new size
func (cgce *CacheEntry) RemovePIDs(pids []uint32) int {
	cgce.lock.Lock()
	defer cgce.lock.Unlock()

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
	cgce.lock.Lock()
	defer cgce.lock.Unlock()

	_, exists := cgce.pids[pid]
	return exists
}

// CGroupContextEquals returns if the cgroups are equal
func (cgce *CacheEntry) CGroupContextEquals(other *CacheEntry) bool {
	cgce.lock.RLock()
	other.lock.RLock()
	defer cgce.lock.RUnlock()
	defer other.lock.RUnlock()

	return cgce.cgroupContext.Equals(&other.cgroupContext)
}
