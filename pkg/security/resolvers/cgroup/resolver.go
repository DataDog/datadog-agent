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
	cacheEntriesByPID     map[uint32]*cgroupModel.CacheEntry
	containerCGroups      map[containerutils.ContainerID][]*cgroupModel.CacheEntry
	extraPathKeys         map[*cgroupModel.CacheEntry][]uint64
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
	for _, pid := range cacheEntry.GetPIDs() {
		if cr.cacheEntriesByPID[pid] == cacheEntry {
			delete(cr.cacheEntriesByPID, pid)
		}
	}
	if cacheEntry.IsContainerContextNull() {
		cacheEntry.CallReleaseCallbacks()
	} else {
		cacheEntry.CallCGroupReleaseCallback()
	}
	cr.deletedCgroups.Inc()
	cr.notify(CGroupDeleted, cacheEntry)
	return true
}

func (cr *Resolver) addPathKey(cacheEntry *cgroupModel.CacheEntry, inode uint64) {
	if inode != 0 && inode != cacheEntry.GetCGroupInode() {
		inodes := cr.extraPathKeys[cacheEntry]
		if !slices.Contains(inodes, inode) {
			cr.extraPathKeys[cacheEntry] = append(inodes, inode)
		}
	}
	cr.cacheEntriesByPathKey.Add(inode, cacheEntry)
}

func (cr *Resolver) removePathKeys(cacheEntry *cgroupModel.CacheEntry) {
	inodes := cr.extraPathKeys[cacheEntry]
	delete(cr.extraPathKeys, cacheEntry)
	cr.cacheEntriesByPathKey.Remove(cacheEntry.GetCGroupInode())
	for _, inode := range inodes {
		cr.cacheEntriesByPathKey.Remove(inode)
	}
}

func (cr *Resolver) hasRemainingPathKey(cacheEntry *cgroupModel.CacheEntry, evictedInode uint64) bool {
	inodes, ok := cr.extraPathKeys[cacheEntry]
	if !ok {
		return false
	}
	inodes = slices.DeleteFunc(inodes, func(inode uint64) bool {
		return inode == evictedInode
	})
	if len(inodes) == 0 {
		delete(cr.extraPathKeys, cacheEntry)
	} else {
		cr.extraPathKeys[cacheEntry] = inodes
	}
	if primary := cacheEntry.GetCGroupInode(); primary != evictedInode {
		if current, ok := cr.cacheEntriesByPathKey.Peek(primary); ok && current == cacheEntry {
			return true
		}
	}
	for _, inode := range inodes {
		if current, ok := cr.cacheEntriesByPathKey.Peek(inode); ok && current == cacheEntry {
			return true
		}
	}
	return false
}

// NewResolver returns a new cgroups monitor
func NewResolver(statsdClient statsd.ClientInterface, cgroupFS FSInterface, dentryResolver *dentry.Resolver) (*Resolver, error) {
	if cgroupFS == nil {
		cgroupFS = utils.DefaultCGroupFS()
	}

	cr := &Resolver{
		Notifier:          utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		statsdClient:      statsdClient,
		cgroupFS:          cgroupFS,
		dentryResolver:    dentryResolver,
		cacheEntriesByPID: make(map[uint32]*cgroupModel.CacheEntry),
		containerCGroups:  make(map[containerutils.ContainerID][]*cgroupModel.CacheEntry),
		extraPathKeys:     make(map[*cgroupModel.CacheEntry][]uint64),
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
		entries, ok := cr.containerCGroups[containerID]
		if !ok && value.IsDeleted() {
			return
		}
		delete(cr.containerCGroups, containerID)
		for _, entry := range entries {
			cr.finalizeCacheEntry(entry)
			cr.removePathKeys(entry)
		}
		cr.finalizeCacheEntry(value)
		cr.removePathKeys(value)
		value.CallContainerReleaseCallback()
	})
	if err != nil {
		return nil, err
	}

	cr.cacheEntriesByPathKey, err = simplelru.NewLRU(maxCacheEntries, func(evictedInode uint64, value *cgroupModel.CacheEntry) {
		if value.IsDeleted() {
			return
		}
		if cr.hasRemainingPathKey(value, evictedInode) {
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

func (cr *Resolver) findCacheEntryByCGroupID(containerID containerutils.ContainerID, cgroupID containerutils.CGroupID) *cgroupModel.CacheEntry {
	if cgroupID == "" {
		return nil
	}
	if containerID != "" {
		for _, entry := range cr.containerCGroups[containerID] {
			if !entry.IsDeleted() && entry.GetCGroupID() == cgroupID {
				return entry
			}
		}
		return nil
	}
	if entry, ok := cr.hostCacheEntries.Peek(cgroupID); ok && !entry.IsDeleted() {
		return entry
	}
	return nil
}

func (cr *Resolver) registerContainerCGroup(cacheEntry *cgroupModel.CacheEntry) {
	containerID := cacheEntry.GetContainerID()
	if containerID == "" {
		return
	}
	entries := cr.containerCGroups[containerID]
	if !slices.Contains(entries, cacheEntry) {
		cr.containerCGroups[containerID] = append(entries, cacheEntry)
	}
	if current, ok := cr.containerCacheEntries.Peek(containerID); !ok || current.IsDeleted() || !current.HasPIDs() || cacheEntry.HasPIDs() {
		cr.containerCacheEntries.Add(containerID, cacheEntry)
	} else {
		cr.containerCacheEntries.Get(containerID)
	}
}

func (cr *Resolver) detachPIDFromPreviousEntry(pid uint32, currentEntry *cgroupModel.CacheEntry) {
	if pid == 0 {
		return
	}
	prevEntry, ok := cr.cacheEntriesByPID[pid]
	if !ok || prevEntry == currentEntry {
		return
	}
	delete(cr.cacheEntriesByPID, pid)
	if prevEntry.IsDeleted() {
		return
	}
	if prevEntry.RemovePID(pid) == 0 {
		cr.syncOrDeleteCacheEntry(prevEntry, pid)
	}
}

func (cr *Resolver) addPIDToCacheEntry(pid uint32, cacheEntry *cgroupModel.CacheEntry) {
	if l := cacheEntry.AddPID(pid); l == 1 {
		if !cacheEntry.IsContainerContextNull() {
			containerID := cacheEntry.GetContainerID()
			if current, ok := cr.containerCacheEntries.Peek(containerID); !ok || current.IsDeleted() || !current.HasPIDs() {
				cr.containerCacheEntries.Add(containerID, cacheEntry)
			}
		}
		cr.notify(CGroupCreated, cacheEntry)
	}
	cr.detachPIDFromPreviousEntry(pid, cacheEntry)
	cr.cacheEntriesByPID[pid] = cacheEntry
}

func (cr *Resolver) removeCacheEntry(cacheEntry *cgroupModel.CacheEntry) {
	if !cr.finalizeCacheEntry(cacheEntry) {
		return
	}

	cr.removePathKeys(cacheEntry)
	cr.hostCacheEntries.Remove(cacheEntry.GetCGroupID())

	if !cacheEntry.IsContainerContextNull() {
		containerID := cacheEntry.GetContainerID()
		entries := slices.DeleteFunc(cr.containerCGroups[containerID], func(entry *cgroupModel.CacheEntry) bool {
			return entry == cacheEntry || entry.IsDeleted()
		})
		if len(entries) == 0 {
			delete(cr.containerCGroups, containerID)
			cr.containerCacheEntries.Remove(containerID)
			cacheEntry.CallContainerReleaseCallback()
			return
		}
		cr.containerCGroups[containerID] = entries

		current, ok := cr.containerCacheEntries.Peek(containerID)
		if ok && current != cacheEntry && !current.IsDeleted() && current.HasPIDs() {
			return
		}

		var replacement *cgroupModel.CacheEntry
		for _, entry := range entries {
			if replacement == nil || (!replacement.HasPIDs() && entry.HasPIDs()) {
				replacement = entry
			}
		}
		if ok && current != cacheEntry && !current.IsDeleted() && !replacement.HasPIDs() {
			return
		}
		cr.containerCacheEntries.Add(containerID, replacement)
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

	pids = slices.DeleteFunc(pids, func(pid uint32) bool {
		if pid == deletedPid {
			return true
		}
		existing, ok := cr.cacheEntriesByPID[pid]
		return ok && existing != cacheEntry && !existing.IsDeleted()
	})
	if len(pids) == 0 {
		cr.removeCacheEntry(cacheEntry)
		return
	}

	for _, oldPID := range cacheEntry.GetPIDs() {
		if cr.cacheEntriesByPID[oldPID] == cacheEntry {
			delete(cr.cacheEntriesByPID, oldPID)
		}
	}
	cacheEntry.SetPIDs(pids)
	for _, pid := range pids {
		cr.cacheEntriesByPID[pid] = cacheEntry
	}
}

func (cr *Resolver) pushNewCacheEntry(pid uint32, containerContext model.ContainerContext, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	if cgroupContext.CGroupPathKey.Inode != 0 {
		if old, ok := cr.cacheEntriesByPathKey.Peek(cgroupContext.CGroupPathKey.Inode); ok && old.GetCGroupID() != cgroupContext.CGroupID {
			cr.removeCacheEntry(old)
		}
	}

	if existing := cr.findCacheEntryByCGroupID(containerContext.ContainerID, cgroupContext.CGroupID); existing != nil {
		if cgroupContext.CGroupPathKey.Inode != 0 {
			cr.addPathKey(existing, cgroupContext.CGroupPathKey.Inode)
		}
		if !containerContext.IsNull() {
			cr.registerContainerCGroup(existing)
		} else {
			cr.hostCacheEntries.Add(cgroupContext.CGroupID, existing)
		}
		if pid != 0 {
			cr.addPIDToCacheEntry(pid, existing)
		}
		return existing
	}

	// sanity check that the releasable is not nil
	if cgroupContext.Releasable == nil {
		cgroupContext.Releasable = &model.Releasable{}
	}
	if !containerContext.IsNull() && containerContext.Releasable == nil {
		for _, entry := range cr.containerCGroups[containerContext.ContainerID] {
			if !entry.IsDeleted() {
				containerContext.Releasable = entry.GetContainerContext().Releasable
				break
			}
		}
	}

	cacheEntry := cgroupModel.NewCacheEntry(containerContext, cgroupContext, pid)

	// add the new CGroup to the cache
	if !containerContext.IsNull() {
		cr.registerContainerCGroup(cacheEntry)
	} else {
		cr.hostCacheEntries.Add(cgroupContext.CGroupID, cacheEntry)
	}

	// add the cgroup context to the cache
	cr.addPathKey(cacheEntry, cgroupContext.CGroupPathKey.Inode)

	cr.addedCgroups.Inc()

	// When the entry is created with a real pid (pid != 0), it means a process already exists
	// in this cgroup — this covers both snapshot discovery and runtime cases where no prior
	// Add() call was made. Entries pre-created with pid=0 (via Add()) will fire CGroupCreated
	// later when AddPID transitions their count from 0 to 1.
	if pid != 0 {
		cr.notify(CGroupCreated, cacheEntry)
		cr.detachPIDFromPreviousEntry(pid, cacheEntry)
		cr.cacheEntriesByPID[pid] = cacheEntry
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

			cr.addPIDToCacheEntry(pid, cacheEntry)

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

// AddPID associates a cgroup and a pid in the cgroup cache and returns the resolved
// cache entry, or nil if the cgroup cannot be resolved. The cgroup context doesn't
// have to be resolved; it will be resolved when the cgroup is created.
func (cr *Resolver) AddPID(pid uint32, cgroupContext model.CGroupContext) *cgroupModel.CacheEntry {
	cr.lock()
	defer cr.unlock()

	if !cgroupContext.IsNull() {
		if cacheEntry, ok := cr.cacheEntriesByPathKey.Get(cgroupContext.CGroupPathKey.Inode); ok && !cacheEntry.IsDeleted() {
			cr.addPIDToCacheEntry(pid, cacheEntry)
			return cacheEntry
		}

		// try to resolve the cgroup from the dentry resolver
		if cacheEntry := cr.resolveAndPushNewCacheEntry(pid, cgroupContext); cacheEntry != nil {
			return cacheEntry
		}
	}

	if cacheEntry := cr.resolveFromFallback(pid); cacheEntry != nil {
		return cacheEntry
	}

	if !cgroupContext.IsNull() {
		cr.detachPIDFromPreviousEntry(pid, nil)
	}

	return nil
}

func (cr *Resolver) iterateCacheEntries(cb func(*cgroupModel.CacheEntry) bool) {
	var seen map[*cgroupModel.CacheEntry]struct{}
	if len(cr.extraPathKeys) > 0 {
		seen = make(map[*cgroupModel.CacheEntry]struct{}, cr.cacheEntriesByPathKey.Len())
	}
	for _, cacheEntry := range cr.cacheEntriesByPathKey.Values() {
		if cacheEntry.IsDeleted() {
			continue
		}
		if seen != nil {
			if _, ok := seen[cacheEntry]; ok {
				continue
			}
			seen[cacheEntry] = struct{}{}
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

// GetContainerPIDs returns the PIDs across all live cgroups for the provided container ID,
// starting with the current representative entry.
func (cr *Resolver) GetContainerPIDs(id containerutils.ContainerID) []uint32 {
	if id == "" {
		return nil
	}

	cr.Lock()
	defer cr.Unlock()

	var pids []uint32
	rep, ok := cr.containerCacheEntries.Get(id)
	if ok && !rep.IsDeleted() {
		pids = rep.GetPIDs()
	}
	for _, entry := range cr.containerCGroups[id] {
		if entry == rep || entry.IsDeleted() {
			continue
		}
		pids = append(pids, entry.GetPIDs()...)
	}
	return pids
}

// DelPID removes a PID from the cgroup resolver
func (cr *Resolver) DelPID(pid uint32) {
	cr.lock()
	defer cr.unlock()

	cacheEntry, ok := cr.cacheEntriesByPID[pid]
	if !ok {
		return
	}
	delete(cr.cacheEntriesByPID, pid)
	if cacheEntry.IsDeleted() {
		return
	}
	cr.deleteCacheEntryPID(pid, cacheEntry)
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
