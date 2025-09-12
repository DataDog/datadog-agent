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

	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
)

// ResolverInterface defines the interface implemented by a cgroup resolver
type ResolverInterface interface {
	Start(context.Context)
	AddPID(*model.ProcessCacheEntry)
	DelPID(uint32)
	GetWorkload(containerutils.ContainerID) (*cgroupModel.CacheEntry, bool)
	Len() int
	RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	cgroupFS           *utils.CGroupFS
	statsdClient       statsd.ClientInterface
	cgroups            *simplelru.LRU[uint64, *model.CGroupContext]
	hostWorkloads      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerWorkloads *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
}

// NewResolver returns a new cgroups monitor
func NewResolver(statsdClient statsd.ClientInterface) (*Resolver, error) {
	cr := &Resolver{
		Notifier:     utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		statsdClient: statsdClient,
		cgroupFS:     utils.DefaultCGroupFS(),
	}

	cleanup := func(value *cgroupModel.CacheEntry) {

		if value.ContainerContext.Resolved && value.ContainerContext.ContainerID != "" {
			value.ContainerContext.CallReleaseCallback()
		}
		value.CGroupContext.CallReleaseCallback()
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

	return cr, nil
}

// Start starts the goroutine of the SBOM resolver
func (cr *Resolver) Start(_ context.Context) {
}

func (cr *Resolver) removeCgroup(cgroup *cgroupModel.CacheEntry) {
	cr.cgroups.Remove(cgroup.CGroupFile.Inode)
	cr.hostWorkloads.Remove(cgroup.CGroupID)
	if cgroup.ContainerID != "" {
		cr.containerWorkloads.Remove(cgroup.ContainerID)
	}
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
func (cr *Resolver) cleanupPidsWithMultipleCgroups(pids []uint32, currentCgroup *cgroupModel.CacheEntry) {
	for _, cgroup := range cr.containerWorkloads.Values() {
		if cgroup.CGroupFile == currentCgroup.CGroupFile {
			continue
		}
		cgroup.Lock()
		for _, pid := range pids {
			delete(cgroup.PIDs, pid)
		}
		if len(cgroup.PIDs) == 0 {
			// No double check here to ensure that the cgroup is REALLY empty,
			// because we already are in such a double check for another cgroup.
			// No need to introduce a recursion here.
			cr.removeCgroup(cgroup)
		}
		cgroup.Unlock()
	}

	for _, cgroup := range cr.hostWorkloads.Values() {
		if cgroup.CGroupFile == currentCgroup.CGroupFile {
			continue
		}
		cgroup.Lock()
		for _, pid := range pids {
			delete(cgroup.PIDs, pid)
		}
		if len(cgroup.PIDs) == 0 {
			// No double check here to ensure that the cgroup is REALLY empty,
			// because we already are in such a double check for another cgroup.
			// No need to introduce a recursion here.
			cr.removeCgroup(cgroup)
		}
		cgroup.Unlock()
	}
}

func (cr *Resolver) pushNewCacheEntry(process *model.ProcessCacheEntry) {
	// create new entry now
	newCGroup := cgroupModel.NewCacheEntry(process.ContainerID, &process.CGroup, process.Pid)
	newCGroup.CreatedAt = uint64(process.ProcessContext.ExecTime.UnixNano())

	// add the new CGroup to the cache
	if process.ContainerID != "" {
		cr.containerWorkloads.Add(process.ContainerID, newCGroup)
	} else {
		cr.hostWorkloads.Add(process.CGroup.CGroupID, newCGroup)
	}
	// Cache a copy instead of a pointer to avoid race conditions
	cgroupCopy := process.CGroup
	cr.cgroups.Add(process.CGroup.CGroupFile.Inode, &cgroupCopy)

	cr.NotifyListeners(CGroupCreated, newCGroup)
}

// AddPID update the cgroup cache to associates a cgroup and a pid
func (cr *Resolver) AddPID(process *model.ProcessCacheEntry) {
	cr.Lock()
	defer cr.Unlock()

	found := false

	for _, cgroup := range cr.hostWorkloads.Values() {
		cgroup.Lock()
		if cgroup.CGroupFile == process.CGroup.CGroupFile {
			cgroup.PIDs[process.Pid] = true
			found = true
		} else if _, exist := cgroup.PIDs[process.Pid]; exist {
			delete(cgroup.PIDs, process.Pid)
			if len(cgroup.PIDs) == 0 {
				cr.syncOrDeleteCgroup(cgroup, process.Pid)
			}
		}
		cgroup.Unlock()
	}

	for _, cgroup := range cr.containerWorkloads.Values() {
		cgroup.Lock()
		if cgroup.CGroupFile == process.CGroup.CGroupFile {
			cgroup.PIDs[process.Pid] = true
			found = true
		} else if _, exist := cgroup.PIDs[process.Pid]; exist {
			delete(cgroup.PIDs, process.Pid)
			if len(cgroup.PIDs) == 0 {
				cr.syncOrDeleteCgroup(cgroup, process.Pid)
			}
		}
		cgroup.Unlock()
	}

	if !found {
		cr.pushNewCacheEntry(process)
	}
}

// GetCGroupContext returns the cgroup context with the specified path key
func (cr *Resolver) GetCGroupContext(cgroupPath model.PathKey) (*model.CGroupContext, bool) {
	cr.Lock()
	defer cr.Unlock()

	if cgroupContext, found := cr.cgroups.Get(cgroupPath.Inode); found {
		// Return a copy to avoid race conditions when dereferencing the shared pointer
		cgroupContextCopy := *cgroupContext
		return &cgroupContextCopy, true
	}
	return nil, false
}

// Iterate iterates on all cached cgroups
func (cr *Resolver) Iterate(cb func(*cgroupModel.CacheEntry)) {
	cr.Lock()
	defer cr.Unlock()

	for _, cgroup := range cr.hostWorkloads.Values() {
		cb(cgroup)
	}
	for _, cgroup := range cr.containerWorkloads.Values() {
		cb(cgroup)
	}
}

// GetWorkload returns the workload referenced by the provided ID
func (cr *Resolver) GetWorkload(id containerutils.ContainerID) (*cgroupModel.CacheEntry, bool) {
	if id == "" {
		return nil, false
	}

	cr.Lock()
	defer cr.Unlock()

	return cr.containerWorkloads.Get(id)
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

	return nil
}
