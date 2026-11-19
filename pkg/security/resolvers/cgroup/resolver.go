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

	maxhostWorkloadEntries      = 3072
	maxContainerWorkloadEntries = 3072
	maxCacheEntries             = 6144
)

// FSInterface defines the interface for CGroupFS operations
type FSInterface interface {
	FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, utils.CGroupContext, string, error)
	GetCGroupPids(cgroupID string) ([]uint32, error)
}

type pendingNotification struct {
	event Event
	entry *cgroupModel.CacheEntry
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	locked                bool
	pendingNotifications  []pendingNotification
	cgroupFS              FSInterface
	statsdClient          statsd.ClientInterface
	cacheEntriesByPathKey *simplelru.LRU[uint64, *cgroupModel.CacheEntry]
	hostCacheEntries      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerCacheEntries *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
	dentryResolver        *dentry.Resolver

	// metrics
	addedCgroups    atomic.Int64
	deletedCgroups  atomic.Int64
	fallbackSucceed atomic.Int64
	fallbackFailed  atomic.Int64
	remainingPids   atomic.Int64
}

func (cr *Resolver) lock() {
	cr.Lock()
	cr.locked = true
}

func (cr *Resolver) unlock() {
	notifications := cr.pendingNotifications
	cr.pendingNotifications = nil
	cr.locked = false
	cr.Unlock()

	for _, n := range notifications {
		cr.NotifyListeners(n.event, n.entry)
	}
}

func (cr *Resolver) notify(event Event, cacheEntry *cgroupModel.CacheEntry) {
	if cr.locked {
		cr.pendingNotifications = append(cr.pendingNotifications, pendingNotification{
			event: event,
			entry: cacheEntry,
		})
		return
	}
	cr.NotifyListeners(event, cacheEntry)
}

func (cr *Resolver) finalizeCacheEntry(cacheEntry *cgroupModel.CacheEntry) bool {
	if cacheEntry == nil || cacheEntry.IsDeleted() {
		return false
	}
	cacheEntry.SetAsDeleted()
	cacheEntry.CallReleaseCallbacks()
	cr.deletedCgroups.Inc()
	cr.notify(CGroupDeleted, cacheEntry)
	return true
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

	var err error
	cr.hostCacheEntries, err = simplelru.NewLRU(maxhostWorkloadEntries, func(_ containerutils.CGroupID, value *cgroupModel.CacheEntry) {
		if value.IsDeleted() {
			return
		}
		cr.removeCacheEntry(value)
	})
	if err != nil {
		return nil, err
	}

	cr.containerCacheEntries, err = simplelru.NewLRU(maxContainerWorkloadEntries, func(containerID containerutils.ContainerID, value *cgroupModel.CacheEntry) {
		for _, entry := range cr.cacheEntriesByPathKey.Values() {
			if entry.GetContainerID() == containerID {
				cr.finalizeCacheEntry(entry)
				cr.cacheEntriesByPathKey.Remove(entry.GetCGroupPathKey().Inode)
			}
		}
		cr.finalizeCacheEntry(value)
		cr.cacheEntriesByPathKey.Remove(value.GetCGroupPathKey().Inode)
	})
	if err != nil {
		return nil, err
	}

	cr.cacheEntriesByPathKey, err = simplelru.NewLRU(maxCacheEntries, func(_ uint64, value *cgroupModel.CacheEntry) {
		if value.IsDeleted() {
			return
		}
		cr.removeCacheEntry(value)
	})
	if err != nil {
		return nil, err
	}

	return cr, nil
}

// Start starts the cgroup resolver
func (cr *Resolver) Start(_ context.Context) {
}

func (cr *Resolver) removeCacheEntry(cacheEntry *cgroupModel.CacheEntry) {
	if !cr.finalizeCacheEntry(cacheEntry) {
		return
	}

	cr.cacheEntriesByPathKey.Remove(cacheEntry.GetCGroupPathKey().Inode)
	cr.hostCacheEntries.Remove(cacheEntry.GetCGroupID())

	if !cacheEntry.IsContainerContextNull() {
		containerID := cacheEntry.GetContainerID()
		var replacement *cgroupModel.CacheEntry
		for _, entry := range cr.cacheEntriesByPathKey.Values() {
			if !entry.IsDeleted() && entry.GetContainerID() == containerID {
				replacement = entry
				break
			}
		}
		if replacement != nil {
			cr.containerCacheEntries.Add(containerID, replacement)
		} else {
			cr.containerCacheEntries.Remove(containerID)
		}
	}
}

// syncOrDeleteCacheEntry uses the cgroupFS to check if the cgroup still contains pids.
// If there is no pid left, or the only one being the one we want to delete,
// remove the cgroup from the caches.
// Otherwise, sync it with new values.
func (cr *Resolver) syncOrDeleteCacheEntry(cacheEntry *cgroupModel.CacheEntry, deletedPid uint32) {
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
	if deletedPid != 0 {
		pids = slices.DeleteFunc(pids, func(todel uint32) bool {
			return todel == deletedPid
		})
	}
	cacheEntry.SetPIDs(pids)
}

func (cr *Resolver) pushNewCacheEntry(pid uint32, containerContext model.ContainerContext, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	// sanity check that the releasable is not nil
	if cgroupContext.Releasable == nil {
		cgroupContext.Releasable = &model.Releasable{}
	}

	cacheEntry := cgroupModel.NewCacheEntry(containerContext, cgroupContext, pid)

	if cgroupContext.CGroupPathKey.Inode != 0 {
		if old, ok := cr.cacheEntriesByPathKey.Peek(cgroupContext.CGroupPathKey.Inode); ok && old != cacheEntry {
			cr.removeCacheEntry(old)
		}
	}

	// add the new CGroup to the cache
	if !containerContext.IsNull() {
		cr.containerCacheEntries.Add(containerContext.ContainerID, cacheEntry)
	} else {
		if old, ok := cr.hostCacheEntries.Peek(cgroupContext.CGroupID); ok && old != cacheEntry {
			cr.removeCacheEntry(old)
		}
		cr.hostCacheEntries.Add(cgroupContext.CGroupID, cacheEntry)
	}

	// add the cgroup context to the cache
	cr.cacheEntriesByPathKey.Add(cgroupContext.CGroupPathKey.Inode, cacheEntry)

	cr.addedCgroups.Inc()

	// When the entry is created with a real pid (pid != 0), it means a process already exists
	// in this cgroup — this covers both snapshot discovery and runtime cases where no prior
	// Add() call was made. Entries pre-created with pid=0 (via Add()) will fire CGroupCreated
	// later when AddPID transitions their count from 0 to 1.
	if pid != 0 {
		cr.notify(CGroupCreated, cacheEntry)
	}

	return cacheEntry
}

func (cr *Resolver) resolveAndPushNewCacheEntry(pid uint32, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	if cgroupContext.IsNull() {
		return nil
	}

	if !cgroupContext.IsResolved() {
		path, err := cr.dentryResolver.Resolve(cgroupContext.CGroupPathKey, false, dentry.CallerCGroup)
		if err != nil {
			seclog.Debugf("failed to fallback to resolve dentry for pid %d and path key %+v: %v", pid, cgroupContext.CGroupPathKey, err)
			return nil
		}

		cgroupContext.CGroupID = containerutils.CGroupID(path)
	}

	podUID := containerutils.FindPodUID(cgroupContext.CGroupID)
	var containerContext model.ContainerContext
	if containerID := containerutils.FindContainerID(cgroupContext.CGroupID); containerID != "" || podUID != "" {
		containerContext = model.ContainerContext{
			ContainerID:     containerID,
			PodUID:          podUID,
			ECSTaskID:       containerutils.FindECSTaskID(cgroupContext.CGroupID),
			CreatedAt:       cgroupContext.CreatedAt,
			ContainerSource: model.ContainerSourceEvent,
		}
	}

	return cr.pushNewCacheEntry(pid, containerContext, cgroupContext)
}

func (cr *Resolver) resolveFromFallback(pid uint32) *cgroupModel.CacheEntry {
	cid, cgroup, _, err := cr.cgroupFS.FindCGroupContext(pid, pid)
	if err == nil && cgroup.CGroupID != "" {
		// check if the cgroup is already in the cache
		if cacheEntry, found := cr.cacheEntriesByPathKey.Get(cgroup.CGroupFileInode); found && !cacheEntry.IsDeleted() {
			seclog.Tracef("fallback to resolve cgroup for pid %d with existing path key %+v", pid, cacheEntry.GetCGroupID())
			cr.fallbackSucceed.Inc()

			if l := cacheEntry.AddPID(pid); l == 1 {
				cr.notify(CGroupCreated, cacheEntry)
			}

			return cacheEntry
		}

		// if not, create a new cache entry
		cgroupContext := model.CGroupContext{
			CGroupPathKey: model.PathKey{
				MountID: cgroup.CGroupFileMountID,
				Inode:   cgroup.CGroupFileInode,
			},
			CGroupID:     cgroup.CGroupID,
			CGroupSource: model.CGroupSourceProcFS,
			CreatedAt:    uint64(cgroup.CreatedAt.UnixNano()),
		}
		containerContext := model.ContainerContext{
			ContainerID:     cid,
			PodUID:          containerutils.FindPodUID(cgroup.CGroupID),
			ECSTaskID:       containerutils.FindECSTaskID(cgroup.CGroupID),
			CreatedAt:       uint64(cgroup.CreatedAt.UnixNano()),
			ContainerSource: model.ContainerSourceProcFS,
		}
		seclog.Tracef("fallback to resolve cgroup for pid %d: %s", pid, cgroup.CGroupID)
		cr.fallbackSucceed.Inc()

		return cr.pushNewCacheEntry(pid, containerContext, cgroupContext)
	}

	seclog.Debugf("failed to add pid %d, error on fallback to resolve its cgroup", pid)
	cr.fallbackFailed.Inc()

	return nil
}

// Add registers a new cgroup entry
func (cr *Resolver) Add(cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	cr.lock()
	defer cr.unlock()

	if cacheEntry, found := cr.cacheEntriesByPathKey.Get(cgroupContext.CGroupPathKey.Inode); found && !cacheEntry.IsDeleted() {
		return cacheEntry
	}

	seclog.Tracef("add a new empty cgroup : %d", cgroupContext.CGroupPathKey.Inode)

	return cr.resolveAndPushNewCacheEntry(0, cgroupContext)
}

// Delete removes the cgroup associated with the given inode
func (cr *Resolver) Delete(inode uint64) {
	cr.lock()
	defer cr.unlock()

	cacheEntry, ok := cr.cacheEntriesByPathKey.Get(inode)
	if !ok {
		return
	}

	seclog.Tracef("received a cgroup delete : %d", inode)

	// try to resync remaining pids
	if pids := cacheEntry.GetPIDs(); len(pids) > 0 {
		cr.remainingPids.Inc()

		for _, pid := range pids {
			cr.resolveFromFallback(pid)
		}
	}

	cr.removeCacheEntry(cacheEntry)
}

// AddPID update the cgroup cache to associates a cgroup and a pid
// Returns true if the kernel maps need to be synced (if we update somehow the process)
// the cgroup context doesn't have to be resolved, it will be resolved when the cgroup is created.
func (cr *Resolver) AddPID(pid uint32, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	cr.lock()
	defer cr.unlock()

	if !cgroupContext.IsNull() {
		var cacheEntryFound *cgroupModel.CacheEntry

		var cgroupsToClean []*cgroupModel.CacheEntry
		cr.iterateCacheEntries(func(cacheEntry *cgroupModel.CacheEntry) bool {
			if cc := cacheEntry.GetCGroupContext(); cc.Equals(&cgroupContext) {
				// if the cgroup context is the same, add the pid to the cache entry
				if l := cacheEntry.AddPID(pid); l == 1 {
					cr.notify(CGroupCreated, cacheEntry)
				}

				cacheEntryFound = cacheEntry
			} else if cacheEntry.ContainsPID(pid) {
				cgroupsToClean = append(cgroupsToClean, cacheEntry)
			}
			return false
		})
		for _, cacheEntry := range cgroupsToClean {
			// the cgroup context is different, but the pid is already present in the cache entry, remove it.
			// it means that the process has been migrated to a different cgroup.
			if cacheEntry.RemovePID(pid) == 0 {
				// try to sync the cgroup with the pid in order to detect the migration.
				cr.syncOrDeleteCacheEntry(cacheEntry, pid)
			}
		}

		// found the cache entry
		if cacheEntryFound != nil {
			return cacheEntryFound
		}

		// try to resolve the cgroup from the dentry resolver
		if cacheEntry := cr.resolveAndPushNewCacheEntry(pid, cgroupContext); cacheEntry != nil {
			return cacheEntry
		}
	}

	return cr.resolveFromFallback(pid)
}

func (cr *Resolver) iterateCacheEntries(cb func(*cgroupModel.CacheEntry) bool) {
	for _, cacheEntry := range cr.cacheEntriesByPathKey.Values() {
		if cacheEntry.IsDeleted() {
			continue
		}
		if cb(cacheEntry) {
			return
		}
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
	if !ok || cacheEntry.IsDeleted() {
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
	if !ok || cacheEntry.IsDeleted() {
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
	if !ok || cacheEntry.IsDeleted() {
		return nil
	}
	return cacheEntry
}

// DelPID removes a PID from the cgroup resolver
func (cr *Resolver) DelPID(pid uint32) {
	cr.lock()
	defer cr.unlock()

	for _, cacheEntry := range cr.cacheEntriesByPathKey.Values() {
		cr.deleteCacheEntryPID(pid, cacheEntry)
	}
}

// deleteWorkloadPID removes a PID from a cacheEntry
func (cr *Resolver) deleteCacheEntryPID(pid uint32, cacheEntry *cgroupModel.CacheEntry) {
	if !cacheEntry.ContainsPID(pid) {
		return
	}

	// check if the cacheEntry should be deleted
	if cacheEntry.RemovePID(pid) == 0 {
		cr.syncOrDeleteCacheEntry(cacheEntry, pid)
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
	if count := cr.remainingPids.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverRemainingPids, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}

	return nil
}
