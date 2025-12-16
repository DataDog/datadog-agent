// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cgroup holds cgroup related files
package cgroup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"go.uber.org/atomic"

	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
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
	// CGroupMaxEvent is used cap the event ID
	CGroupMaxEvent

	maxhostWorkloadEntries      = 1024
	maxContainerWorkloadEntries = 1024
	maxCgroupEntries            = 2048
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
	GetCgroupPids(cgroupID string) ([]uint32, error)
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	cgroupFS           FSInterface
	statsdClient       statsd.ClientInterface
	cgroups            *simplelru.LRU[uint64, *model.CGroupContext]
	hostWorkloads      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerWorkloads *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
	history            *simplelru.LRU[uint32, uint64]

	// metrics
	addedCgroups        *atomic.Int64
	deletedCgroups      *atomic.Int64
	fallbackSucceed     *atomic.Int64
	fallbackFailed      *atomic.Int64
	addPidCgroupPresent *atomic.Int64
	addPidCgroupAbsent  *atomic.Int64
}

// NewResolver returns a new cgroups monitor
func NewResolver(statsdClient statsd.ClientInterface, cgroupFS FSInterface) (*Resolver, error) {
	if cgroupFS == nil {
		cgroupFS = utils.DefaultCGroupFS()
	}

	cr := &Resolver{
		Notifier:            utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		statsdClient:        statsdClient,
		cgroupFS:            cgroupFS,
		addedCgroups:        atomic.NewInt64(0),
		deletedCgroups:      atomic.NewInt64(0),
		fallbackSucceed:     atomic.NewInt64(0),
		fallbackFailed:      atomic.NewInt64(0),
		addPidCgroupPresent: atomic.NewInt64(0),
		addPidCgroupAbsent:  atomic.NewInt64(0),
	}

	cleanup := func(value *cgroupModel.CacheEntry) {
		if value.ContainerContext.Releasable != nil {
			value.ContainerContext.CallReleaseCallback()
		}
		if value.CGroupContext.Releasable != nil {
			value.CGroupContext.CallReleaseCallback()
		}
		value.Deleted.Store(true)

		cr.NotifyListeners(CGroupDeleted, value)
	}

	var err error
	cr.hostWorkloads, err = simplelru.NewLRU(maxhostWorkloadEntries, func(_ containerutils.CGroupID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.containerWorkloads, err = simplelru.NewLRU(maxContainerWorkloadEntries, func(_ containerutils.ContainerID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.cgroups, err = simplelru.NewLRU(maxCgroupEntries, func(_ uint64, _ *model.CGroupContext) {})
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

func (cr *Resolver) removeCgroup(cacheEntry *cgroupModel.CacheEntry) {
	cr.cgroups.Remove(cacheEntry.CGroupContext.CGroupFile.Inode)
	cr.hostWorkloads.Remove(cacheEntry.CGroupContext.CGroupID)
	if cacheEntry.ContainerContext.ContainerID != "" {
		cr.containerWorkloads.Remove(cacheEntry.ContainerContext.ContainerID)
	}
	cr.deletedCgroups.Inc()
}

// cgroup already locked
func (cr *Resolver) syncOrDeleteCgroup(cgroup *cgroupModel.CacheEntry, deletedPid uint32) {
	// check if the cgroup still contains pids
	pids, err := cr.cgroupFS.GetCgroupPids(string(cgroup.CGroupContext.CGroupID))
	if err != nil {
		cr.removeCgroup(cgroup)
		return
	}

	// if there is no pid left, or the only one being the one we want to delete,
	// remove the cgroup from the caches
	if len(pids) == 0 || (len(pids) == 1 && pids[0] == deletedPid) {
		cr.removeCgroup(cgroup)
		return
	}

	// otherwise sync it with new values
	pids = slices.DeleteFunc(pids, func(todel uint32) bool {
		return todel == deletedPid
	})
	for _, pid := range pids {
		cgroup.PIDs[pid] = true
	}

	// then, ensure those pids are not part of other cgroups
	cr.cleanupPidsWithMultipleCgroups(pids, cgroup)
}

// currentCgroup already locked
func (cr *Resolver) cleanupPidsWithMultipleCgroups(pids []uint32, currentCacheEntry *cgroupModel.CacheEntry) {
	cr.iterate(func(cgroup *cgroupModel.CacheEntry) bool {
		if cgroup.CGroupContext.CGroupFile.Inode == currentCacheEntry.CGroupContext.CGroupFile.Inode {
			return false
		}
		cgroup.Lock()
		defer cgroup.Unlock()
		for _, pid := range pids {
			delete(cgroup.PIDs, pid)
		}
		if len(cgroup.PIDs) == 0 {
			// No double check here to ensure that the cgroup is REALLY empty,
			// because we already are in such a double check for another cgroup.
			// No need to introduce a recursion here.
			cr.removeCgroup(cgroup)
		}
		return false
	})
}

func (cr *Resolver) pushNewCacheEntry(pce *model.ProcessCacheEntry) {
	// create new entry now
	cacheEntry := cgroupModel.NewCacheEntry(pce.ContainerContext, pce.CGroup, pce.Pid)

	// add the new CGroup to the cache
	if pce.ContainerContext.ContainerID != "" {
		cr.containerWorkloads.Add(pce.ContainerContext.ContainerID, cacheEntry)
	} else {
		cr.hostWorkloads.Add(pce.CGroup.CGroupID, cacheEntry)
	}
	// Cache a copy instead of a pointer to avoid race conditions
	cgroupCopy := pce.CGroup
	cr.cgroups.Add(pce.CGroup.CGroupFile.Inode, &cgroupCopy)

	cr.NotifyListeners(CGroupCreated, cacheEntry)
	cr.addedCgroups.Inc()
}

// returns false if the fallback failed
func (cr *Resolver) resolveFromFallback(pce *model.ProcessCacheEntry) (model.CGroupContext, model.ContainerContext, bool) {
	// it should not happen, but we have to fallback in this case
	cid, cgroup, _, err := cr.cgroupFS.FindCGroupContext(pce.Pid, pce.Pid)
	if err == nil && cgroup.CGroupID != "" {
		cgroupContext := model.CGroupContext{
			CGroupFile: model.PathKey{
				MountID: cgroup.CGroupFileMountID,
				Inode:   cgroup.CGroupFileInode,
			},
			CGroupID: cgroup.CGroupID,
		}
		containerContext := model.ContainerContext{
			ContainerID: cid,
			CreatedAt:   uint64(pce.ExecTime.UnixNano()),
		}
		return cgroupContext, containerContext, true
	}

	// fallback can fail for short lived processes, in this case we try to assign the parent cgroup
	if pce.PPid == pce.Pid || pce.PPid <= 0 {
		seclog.Infof("Failed to fallback to resolve cgroup for %d, missing parend PPID: %d", pce.Pid, pce.PPid)
		return model.CGroupContext{}, model.ContainerContext{}, false
	}

	inode, found := cr.history.Get(pce.PPid)
	if found {
		cgroup, found := cr.cgroups.Get(inode)
		if found {
			cgroupContext := model.CGroupContext{
				CGroupFile: model.PathKey{
					MountID: cgroup.CGroupFile.MountID,
					Inode:   cgroup.CGroupFile.Inode,
				},
				CGroupID: cgroup.CGroupID,
			}
			containerContext := model.ContainerContext{
				ContainerID: containerutils.FindContainerID(cgroup.CGroupID),
				CreatedAt:   uint64(pce.ExecTime.UnixNano()),
			}
			seclog.Infof("Fallback to resolve cgroup for pid %d from parent: %d", pce.Pid, pce.PPid)
			return cgroupContext, containerContext, true
		}
	}

	// last try, fallback on proc for the parent
	cid, cgroup, _, err = cr.cgroupFS.FindCGroupContext(pce.PPid, pce.PPid)
	if err == nil && cgroup.CGroupID != "" {
		cgroupContext := model.CGroupContext{
			CGroupFile: model.PathKey{
				MountID: cgroup.CGroupFileMountID,
				Inode:   cgroup.CGroupFileInode,
			},
			CGroupID: cgroup.CGroupID,
		}
		containerContext := model.ContainerContext{
			ContainerID: cid,
			CreatedAt:   uint64(pce.ExecTime.UnixNano()),
		}
		seclog.Infof("Fallback to resolve parent cgroup for ppid %d: %s", pce.PPid, cgroup.CGroupID)
		return cgroupContext, containerContext, true
	}

	if err == nil {
		err = errors.New("FindCGroupContext returned an empty cgroup")
	}
	seclog.Infof("Failed to add pid %d, error on fallback to resolve its cgroup: %v", pce.Pid, err)
	return model.CGroupContext{}, model.ContainerContext{}, false
}

// ResolveFromFallback resolves the cgroup context from the fallback mechanism
func (cr *Resolver) ResolveFromFallback(pce *model.ProcessCacheEntry) (model.CGroupContext, model.ContainerContext, bool) {
	cgroupContext, containerContext, ok := cr.resolveFromFallback(pce)
	if ok {
		cr.fallbackSucceed.Inc()
	} else {
		cr.fallbackFailed.Inc()
	}

	return cgroupContext, containerContext, ok
}

// AddPID update the cgroup cache to associates a cgroup and a pid
// Returns true if the kernel maps need to be synced (if we update somehow the process)
func (cr *Resolver) AddPID(pce *model.ProcessCacheEntry) {
	cr.Lock()
	defer cr.Unlock()

	if !pce.CGroup.IsResolved() {
		cr.addPidCgroupAbsent.Inc()
		return
	} else {
		cr.addPidCgroupPresent.Inc()
	}

	// push pid:cgroup pair to an history cache for fallbacks for short lived processes
	cr.history.Add(pce.Pid, pce.CGroup.CGroupFile.Inode)

	cacheEntryFound := false
	cr.iterate(func(cacheEntry *cgroupModel.CacheEntry) bool {
		cacheEntry.Lock()
		if cacheEntry.CGroupContext.Equals(pce.CGroup) {
			// if the cgroup context is the same, add the pid to the cache entry
			cacheEntry.PIDs[pce.Pid] = true
			cacheEntryFound = true
		} else if _, exist := cacheEntry.PIDs[pce.Pid]; exist {
			// the cgroup context is different, but the pid is already present in the cache entry, remove it.
			// it means that the process has been migrated to a different cgroup.
			delete(cacheEntry.PIDs, pce.Pid)
			if len(cacheEntry.PIDs) == 0 {
				// try to sync the cgroup with the pid in order to detect the migration.
				cr.syncOrDeleteCgroup(cacheEntry, pce.Pid)
			}
		}
		cacheEntry.Unlock()
		return false
	})

	if !cacheEntryFound {
		cr.pushNewCacheEntry(pce)
	}
}

// GetCGroupContext returns the cgroup context with the specified path key
func (cr *Resolver) GetCGroupContext(cgroupPath model.PathKey) (model.CGroupContext, bool) {
	cr.Lock()
	defer cr.Unlock()

	if cgroupContext, found := cr.cgroups.Get(cgroupPath.Inode); found {
		return *cgroupContext, true
	}
	return model.CGroupContext{}, false
}

// Iterate iterates on all cached cgroups, callback may return 'true' to break iteration
func (cr *Resolver) Iterate(cb func(*cgroupModel.CacheEntry) bool) {
	cr.Lock()
	defer cr.Unlock()
	cr.iterate(cb)
}

func (cr *Resolver) iterate(cb func(*cgroupModel.CacheEntry) bool) {
	if slices.ContainsFunc(cr.hostWorkloads.Values(), cb) {
		return
	}
	if slices.ContainsFunc(cr.containerWorkloads.Values(), cb) {
		return
	}
}

// GetContainerWorkload returns the workload referenced by the provided container ID
func (cr *Resolver) GetContainerWorkload(id containerutils.ContainerID) (*cgroupModel.CacheEntry, bool) {
	if id == "" {
		return nil, false
	}

	cr.Lock()
	defer cr.Unlock()

	return cr.containerWorkloads.Get(id)
}

// GetHostWorkload returns the workload referenced by the provided cgroup ID
func (cr *Resolver) GetHostWorkload(cgroupID containerutils.CGroupID) (*cgroupModel.CacheEntry, bool) {
	if cgroupID == "" {
		return nil, false
	}

	cr.Lock()
	defer cr.Unlock()

	return cr.hostWorkloads.Get(cgroupID)
}

// DelPID removes a PID from the cgroup resolver
func (cr *Resolver) DelPID(pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	for _, workload := range cr.containerWorkloads.Values() {
		cr.deleteWorkloadPID(pid, workload)
	}

	for _, workload := range cr.hostWorkloads.Values() {
		cr.deleteWorkloadPID(pid, workload)
	}
}

// deleteWorkloadPID removes a PID from a workload
func (cr *Resolver) deleteWorkloadPID(pid uint32, workload *cgroupModel.CacheEntry) bool {
	workload.Lock()
	defer workload.Unlock()

	if _, exist := workload.PIDs[pid]; !exist {
		return false
	}

	delete(workload.PIDs, pid)

	// check if the workload should be deleted
	if len(workload.PIDs) == 0 {
		cr.syncOrDeleteCgroup(workload, pid)
	}
	return true
}

// Len return the number of entries
func (cr *Resolver) Len() int {
	cr.Lock()
	defer cr.Unlock()

	return cr.cgroups.Len()
}

// SendStats sends stats
func (cr *Resolver) SendStats() error {
	cr.Lock()
	defer cr.Unlock()

	if val := float64(cr.containerWorkloads.Len()); val > 0 {
		if err := cr.statsdClient.Gauge(metrics.MetricCGroupResolverActiveContainerWorkloads, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricCGroupResolverActiveContainerWorkloads: %w", err)
		}
	}

	if val := float64(cr.hostWorkloads.Len()); val > 0 {
		if err := cr.statsdClient.Gauge(metrics.MetricCGroupResolverActiveHostWorkloads, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricCGroupResolverActiveHostWorkloads: %w", err)
		}
	}

	if val := float64(cr.cgroups.Len()); val > 0 {
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

	if count := cr.addPidCgroupPresent.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverAddPIDCgroupPresent, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}
	if count := cr.addPidCgroupAbsent.Swap(0); count > 0 {
		if err := cr.statsdClient.Count(metrics.MetricCGroupResolverAddPIDCgroupAbsent, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver metric: %w", err)
		}
	}

	return nil
}
