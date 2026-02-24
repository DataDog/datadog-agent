// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cgroup holds cgroup related files
package cgroup

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Event defines the cgroup event type
type Event int

const (
	// CGroupDeleted is used to notify that a cgroup was deleted
	CGroupDeleted Event = iota + 1
	// CGroupCreated new croup created
	CGroupCreated

	maxhostWorkloadEntries      = 1024
	maxContainerWorkloadEntries = 1024
	maxCacheEntries             = 2048
	maxHistoryEntries           = 1024
)

// ResolverInterface defines the interface implemented by a cgroup resolver
type ResolverInterface interface {
	Start(context.Context)
	AddPID(*model.ProcessCacheEntry)
	DelPID(uint32)
	GetWorkload(containerutils.ContainerID) (*cgroupModel.CacheEntry, bool)
	GetWorkloadByCGroupID(containerutils.CGroupID) (*cgroupModel.CacheEntry, bool)
	Len() int
	RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}

// FSInterface defines the interface for CGroupFS operations
type FSInterface interface {
	FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, utils.CGroupContext, string, error)
	GetCGroupPids(cgroupID string) ([]uint32, error)
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	cgroupFS              FSInterface
	statsdClient          statsd.ClientInterface
	cacheEntriesByPathKey *simplelru.LRU[uint64, *cgroupModel.CacheEntry]
	hostCacheEntries      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerCacheEntries *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
	history               *simplelru.LRU[uint32, uint64]
	dentryResolver        *dentry.Resolver

	// metrics
	addedCgroups    atomic.Int64
	deletedCgroups  atomic.Int64
	fallbackSucceed atomic.Int64
	fallbackFailed  atomic.Int64
}

// NewResolver returns a new cgroups monitor
func NewResolver(statsdClient statsd.ClientInterface, cgroupFS FSInterface, dentryResolver *dentry.Resolver) (*Resolver, error) {
	if cgroupFS == nil {
		cgroupFS = utils.DefaultCGroupFS()
	}

	cr := &Resolver{
		Notifier:       utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		statsdClient:   statsdClient,
		cgroupFS:       cgroupFS,
		dentryResolver: dentryResolver,
	}

	cleanup := func(cacheEntry *cgroupModel.CacheEntry) {
		cacheEntry.CallReleaseCallbacks()
		cacheEntry.SetAsDeleted()

		cr.NotifyListeners(CGroupDeleted, cacheEntry)
	}

	var err error
	cr.hostCacheEntries, err = simplelru.NewLRU(maxhostWorkloadEntries, func(_ containerutils.CGroupID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.containerCacheEntries, err = simplelru.NewLRU(maxContainerWorkloadEntries, func(_ containerutils.ContainerID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.cacheEntriesByPathKey, err = simplelru.NewLRU(maxCacheEntries, func(_ uint64, _ *cgroupModel.CacheEntry) {})
	if err != nil {
		return nil, err
	}

	cr.history, err = simplelru.NewLRU(maxHistoryEntries, func(_ uint32, _ uint64) {})
	if err != nil {
		return nil, err
	}

	return cr, nil
}

// Start starts the cgroup resolver
func (cr *Resolver) Start(_ context.Context) {
}

func (cr *Resolver) removeCacheEntry(cacheEntry *cgroupModel.CacheEntry) {
	cr.hostCacheEntries.Remove(cacheEntry.GetCGroupID())
	if !cacheEntry.IsContainerContextNull() {
		cr.containerCacheEntries.Remove(cacheEntry.GetContainerID())
	}

	cr.cacheEntriesByPathKey.Remove(cacheEntry.GetCGroupPathKey().Inode)

	cr.deletedCgroups.Inc()
}

// syncOrDeleteCaheEntry uses the cgroupFS to check if the cgroup still contains pids.
// If there is no pid left, or the only one being the one we want to delete,
// remove the cgroup from the caches.
// Otherwise, sync it with new values.
func (cr *Resolver) syncOrDeleteCaheEntry(cacheEntry *cgroupModel.CacheEntry, deletedPid uint32) {
	// check if the cgroup still contains pids
	pids, err := cr.cgroupFS.GetCGroupPids(string(cacheEntry.GetCGroupID()))
	if err != nil {
		cr.removeCacheEntry(cacheEntry)
		return
	}

	// if there is no pid left, or the only one being the one we want to delete,
	// remove the cgroup from the caches
	if len(pids) == 0 || (len(pids) == 1 && pids[0] == deletedPid) {
		cr.removeCacheEntry(cacheEntry)
		return
	}

	// otherwise sync it with new values
	pids = slices.DeleteFunc(pids, func(todel uint32) bool {
		return todel == deletedPid
	})
	cacheEntry.SetPIDs(pids)

	// then, ensure those pids are not part of other cgroups
	cr.cleanupPidsWithMultipleCgroups(pids, cacheEntry)
}

// cleanupPidsWithMultipleCgroups removes the pids from the other cache entries.
// A pid can't be part of multiple cgroups, so if a pid is part of another cgroup.
func (cr *Resolver) cleanupPidsWithMultipleCgroups(pids []uint32, currentCacheEntry *cgroupModel.CacheEntry) {
	cr.iterateCacheEntries(func(cacheEntry *cgroupModel.CacheEntry) bool {
		if cacheEntry.CGroupContextEquals(currentCacheEntry) {
			return false
		}

		if cacheEntry.RemovePIDs(pids) == 0 {
			// No double check here to ensure that the cgroup is REALLY empty,
			// because we already are in such a double check for another cgroup.
			// No need to introduce a recursion here.
			cr.removeCacheEntry(cacheEntry)
		}
		return false
	})
}

func (cr *Resolver) pushNewCacheEntry(pid uint32, containerContext model.ContainerContext, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	// sanity check that the releasable is not nil
	if cgroupContext.Releasable == nil {
		cgroupContext.Releasable = &model.Releasable{}
	}

	cacheEntry := cgroupModel.NewCacheEntry(containerContext, cgroupContext, pid)

	// add the new CGroup to the cache
	if !containerContext.IsNull() {
		cr.containerCacheEntries.Add(containerContext.ContainerID, cacheEntry)
	} else {
		cr.hostCacheEntries.Add(cgroupContext.CGroupID, cacheEntry)
	}

	// add the cgroup context to the cache
	cr.cacheEntriesByPathKey.Add(cgroupContext.CGroupPathKey.Inode, cacheEntry)

	// push pid:PathKey pair to an history cache for fallbacks for short lived processes
	cr.history.Add(pid, cgroupContext.CGroupPathKey.Inode)

	cr.NotifyListeners(CGroupCreated, cacheEntry)
	cr.addedCgroups.Inc()

	return cacheEntry
}

func (cr *Resolver) resolveAndPushNewCacheEntry(pid uint32, cgroupContext model.CGroupContext, createdAt time.Time) *cgroupModel.CacheEntry {
	if cgroupContext.IsNull() {
		return nil
	}

	if !cgroupContext.IsResolved() {
		path, err := cr.dentryResolver.Resolve(cgroupContext.CGroupPathKey, false)
		if err != nil {
			seclog.Debugf("fallback to resolve dentry for pid %d and path key %v", pid, cgroupContext.CGroupPathKey)
			return nil
		}

		cgroupContext.CGroupID = containerutils.CGroupID(path)
	}

	var containerContext model.ContainerContext
	if containerID := containerutils.FindContainerID(cgroupContext.CGroupID); containerID != "" {
		containerContext = model.ContainerContext{
			ContainerID: containerID,
			CreatedAt:   uint64(createdAt.UnixNano()),
		}
	}

	return cr.pushNewCacheEntry(pid, containerContext, cgroupContext)
}

func (cr *Resolver) resolveFromFallback(pid uint32, ppid uint32, createdAt time.Time) *cgroupModel.CacheEntry {
	cid, cgroup, _, err := cr.cgroupFS.FindCGroupContext(pid, pid)
	if err == nil && cgroup.CGroupID != "" {
		// check if the cgroup is already in the cache
		if cacheEntry, found := cr.cacheEntriesByPathKey.Get(cgroup.CGroupFileInode); found {
			seclog.Debugf("fallback to resolve cgroup for pid %d with existing path key %+v", pid, cacheEntry.GetCGroupID())
			cr.fallbackSucceed.Inc()

			cacheEntry.AddPID(pid)

			return cacheEntry
		}

		// if not, create a new cache entry
		cgroupContext := model.CGroupContext{
			CGroupPathKey: model.PathKey{
				MountID: cgroup.CGroupFileMountID,
				Inode:   cgroup.CGroupFileInode,
			},
			CGroupID: cgroup.CGroupID,
		}
		containerContext := model.ContainerContext{
			ContainerID: cid,
			CreatedAt:   uint64(createdAt.UnixNano()),
		}
		seclog.Debugf("fallback to resolve cgroup for pid %d: %s", pid, cgroup.CGroupID)
		cr.fallbackSucceed.Inc()

		return cr.pushNewCacheEntry(pid, containerContext, cgroupContext)
	}

	// fallback can fail for short lived processes, in this case we try to assign the parent cgroup
	if ppid == pid || ppid <= 0 {
		seclog.Debugf("failed to fallback to resolve cgroup for %d, missing parend PPID: %d", pid, ppid)
		return nil
	}

	if pathKey, found := cr.history.Get(ppid); found {
		if cacheEntry, found := cr.cacheEntriesByPathKey.Get(pathKey); found {
			seclog.Debugf("fallback to resolve cgroup for pid %d from parent: %d", pid, ppid)
			cr.fallbackSucceed.Inc()

			return cr.pushNewCacheEntry(pid, cacheEntry.GetContainerContext(), cacheEntry.GetCGroupContext())
		}
	}

	// last try, fallback on proc for the parent
	cid, cgroup, _, err = cr.cgroupFS.FindCGroupContext(ppid, ppid)
	if err == nil && cgroup.CGroupID != "" {
		cgroupContext := model.CGroupContext{
			CGroupPathKey: model.PathKey{
				MountID: cgroup.CGroupFileMountID,
				Inode:   cgroup.CGroupFileInode,
			},
			CGroupID: cgroup.CGroupID,
		}
		containerContext := model.ContainerContext{
			ContainerID: cid,
			CreatedAt:   uint64(createdAt.UnixNano()),
		}
		seclog.Debugf("fallback to resolve parent cgroup for ppid %d: %s", ppid, cgroup.CGroupID)
		cr.fallbackSucceed.Inc()

		return cr.pushNewCacheEntry(pid, containerContext, cgroupContext)
	}

	seclog.Debugf("failed to add pid %d, error on fallback to resolve its cgroup", pid)
	cr.fallbackFailed.Inc()

	return nil
}

// AddPID update the cgroup cache to associates a cgroup and a pid
// Returns true if the kernel maps need to be synced (if we update somehow the process)
// the cgroup context doesn't have to be resolved, it will be resolved when the cgroup is created.
func (cr *Resolver) AddPID(pid uint32, ppid uint32, createdAt time.Time, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	cr.Lock()
	defer cr.Unlock()

	if !cgroupContext.IsNull() {
		var cacheEntryFound *cgroupModel.CacheEntry

		cr.iterateCacheEntries(func(cacheEntry *cgroupModel.CacheEntry) bool {
			if cc := cacheEntry.GetCGroupContext(); cc.Equals(&cgroupContext) {
				// if the cgroup context is the same, add the pid to the cache entry
				cacheEntry.AddPID(pid)
				cacheEntryFound = cacheEntry
			} else if cacheEntry.ContainsPID(pid) {
				// the cgroup context is different, but the pid is already present in the cache entry, remove it.
				// it means that the process has been migrated to a different cgroup.
				if cacheEntry.RemovePID(pid) == 0 {
					// try to sync the cgroup with the pid in order to detect the migration.
					cr.syncOrDeleteCaheEntry(cacheEntry, pid)
				}
			}
			return false
		})

		// found the cache entry
		if cacheEntryFound != nil {
			return cacheEntryFound
		}

		// try to resolve the cgroup from the dentry resolver
		if cacheEntry := cr.resolveAndPushNewCacheEntry(pid, cgroupContext, createdAt); cacheEntry != nil {
			return cacheEntry
		}
	}

	return cr.resolveFromFallback(pid, ppid, createdAt)
}

func (cr *Resolver) iterateCacheEntries(cb func(*cgroupModel.CacheEntry) bool) {
	if slices.ContainsFunc(cr.hostCacheEntries.Values(), cb) {
		return
	}

	if slices.ContainsFunc(cr.containerCacheEntries.Values(), cb) {
		return
	}
}

// IterateCacheEntries iterates over the cache entries
func (cr *Resolver) IterateCacheEntries(cb func(*cgroupModel.CacheEntry) bool) {
	cr.Lock()
	defer cr.Unlock()

	cr.iterateCacheEntries(cb)
}

// GetCacheEntryContainerID returns the cache entry by the provided container ID
func (cr *Resolver) GetCacheEntryContainerID(id containerutils.ContainerID) *cgroupModel.CacheEntry {
	if id == "" {
		return nil
	}

	// simplelru.LRU.Get() is a mutating operation — it calls MoveToFront() to update the LRU ordering.
	// So we need the take a write-lock to avoid concurrent modifications on the LRU.
	cr.Lock()
	defer cr.Unlock()

	cacheEntry, ok := cr.containerCacheEntries.Get(id)
	if !ok {
		return nil
	}
	return cacheEntry
}

// GetCacheEntryByCgroupID returns the cache entry referenced by the provided cgroup ID
func (cr *Resolver) GetCacheEntryByCgroupID(cgroupID containerutils.CGroupID) *cgroupModel.CacheEntry {
	if cgroupID == "" {
		return nil
	}

	// simplelru.LRU.Get() is a mutating operation — it calls MoveToFront() to update the LRU ordering.
	// So we need the take a write-lock to avoid concurrent modifications on the LRU.
	cr.Lock()
	defer cr.Unlock()

	cacheEntry, ok := cr.hostCacheEntries.Get(cgroupID)
	if !ok {
		return nil
	}
	return cacheEntry
}

// GetCacheEntryByInode returns the cache entry referenced by the provided cgroup inode
func (cr *Resolver) GetCacheEntryByInode(inode uint64) *cgroupModel.CacheEntry {
	// simplelru.LRU.Get() is a mutating operation — it calls MoveToFront() to update the LRU ordering.
	// So we need the take a write-lock to avoid concurrent modifications on the LRU.
	cr.Lock()
	defer cr.Unlock()

	cacheEntry, ok := cr.cacheEntriesByPathKey.Get(inode)
	if !ok {
		return nil
	}
	return cacheEntry
}

// DelPID removes a PID from the cgroup resolver
func (cr *Resolver) DelPID(pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	for _, workload := range cr.containerCacheEntries.Values() {
		cr.deleteCacheEntryPID(pid, workload)
	}

	for _, workload := range cr.hostCacheEntries.Values() {
		cr.deleteCacheEntryPID(pid, workload)
	}
}

// deleteWorkloadPID removes a PID from a workload
func (cr *Resolver) deleteCacheEntryPID(pid uint32, cacheEntry *cgroupModel.CacheEntry) {
	if !cacheEntry.ContainsPID(pid) {
		return
	}

	// check if the workload should be deleted
	if cacheEntry.RemovePID(pid) == 0 {
		cr.syncOrDeleteCaheEntry(cacheEntry, pid)
	}
}

// Len return the number of entries
func (cr *Resolver) Len() int {
	cr.Lock()
	defer cr.Unlock()

	return cr.cacheEntriesByPathKey.Len()
}

// SendStats sends stats
func (cr *Resolver) SendStats() error {
	cr.Lock()
	defer cr.Unlock()

	if val := float64(cr.containerCacheEntries.Len()); val > 0 {
		if err := cr.statsdClient.Gauge(metrics.MetricCGroupResolverActiveContainerWorkloads, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricCGroupResolverActiveContainerWorkloads: %w", err)
		}
	}

	if val := float64(cr.hostCacheEntries.Len()); val > 0 {
		if err := cr.statsdClient.Gauge(metrics.MetricCGroupResolverActiveHostWorkloads, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricCGroupResolverActiveHostWorkloads: %w", err)
		}
	}

	if val := float64(cr.cacheEntriesByPathKey.Len()); val > 0 {
		if err := cr.statsdClient.Gauge(metrics.MetricCGroupResolverActiveCGroups, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricCGroupResolverActiveCGroups: %w", err)
		}
	}

	if count := cr.addedCgroups.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverAddedCgroups, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}
	if count := cr.deletedCgroups.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverDeletedCgroups, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}

	if count := cr.fallbackSucceed.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverFallbackSucceed, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}
	if count := cr.fallbackFailed.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverFallbackFailed, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}

	return nil
}
