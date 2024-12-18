// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cgroup holds cgroup related files
package cgroup

import (
	"context"
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"

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
)

// ResolverInterface defines the interface implemented by a cgroup resolver
type ResolverInterface interface {
	Start(context.Context)
	AddPID(*model.ProcessCacheEntry)
	GetWorkload(containerutils.ContainerID) (*cgroupModel.CacheEntry, bool)
	DelPID(uint32)
	DelPIDWithID(containerutils.ContainerID, uint32)
	Len() int
	RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	cgroups            *simplelru.LRU[model.PathKey, *model.CGroupContext]
	hostWorkloads      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerWorkloads *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
}

// NewResolver returns a new cgroups monitor
func NewResolver() (*Resolver, error) {
	cr := &Resolver{
		Notifier: utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
	}

	cleanup := func(value *cgroupModel.CacheEntry) {
		value.CallReleaseCallback()
		value.Deleted.Store(true)

		cr.NotifyListeners(CGroupDeleted, value)
	}

	var err error
	cr.hostWorkloads, err = simplelru.NewLRU(1024, func(_ containerutils.CGroupID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.containerWorkloads, err = simplelru.NewLRU(1024, func(_ containerutils.ContainerID, value *cgroupModel.CacheEntry) {
		cleanup(value)
	})
	if err != nil {
		return nil, err
	}

	cr.cgroups, err = simplelru.NewLRU(2048, func(_ model.PathKey, _ *model.CGroupContext) {})
	if err != nil {
		return nil, err
	}

	return cr, nil
}

// Start starts the goroutine of the SBOM resolver
func (cr *Resolver) Start(_ context.Context) {
}

// AddPID associates a container id and a pid which is expected to be the pid 1
func (cr *Resolver) AddPID(process *model.ProcessCacheEntry) {
	cr.Lock()
	defer cr.Unlock()

	if process.ContainerID != "" {
		entry, exists := cr.containerWorkloads.Get(process.ContainerID)
		if exists {
			entry.AddPID(process.Pid)
			return
		}
	}

	entry, exists := cr.hostWorkloads.Get(process.CGroup.CGroupID)
	if exists {
		entry.AddPID(process.Pid)
		return
	}

	var err error
	// create new entry now
	newCGroup, err := cgroupModel.NewCacheEntry(process.ContainerID, &process.CGroup, process.Pid)
	if err != nil {
		seclog.Errorf("couldn't create new cgroup_resolver cache entry: %v", err)
		return
	}
	newCGroup.CreatedAt = uint64(process.ProcessContext.ExecTime.UnixNano())

	// add the new CGroup to the cache
	if process.ContainerID != "" {
		cr.containerWorkloads.Add(process.ContainerID, newCGroup)
	} else {
		cr.hostWorkloads.Add(process.CGroup.CGroupID, newCGroup)
	}
	cr.cgroups.Add(process.CGroup.CGroupFile, &process.CGroup)

	cr.NotifyListeners(CGroupCreated, newCGroup)
}

// GetCGroupContext returns the cgroup context with the specified path key
func (cr *Resolver) GetCGroupContext(cgroupPath model.PathKey) (*model.CGroupContext, bool) {
	cr.Lock()
	defer cr.Unlock()

	return cr.cgroups.Get(cgroupPath)
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

// DelPIDWithID removes a PID from the cgroup cache entry referenced by the provided ID
func (cr *Resolver) DelPIDWithID(id containerutils.ContainerID, pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.containerWorkloads.Get(id)
	if exists {
		cr.deleteWorkloadPID(pid, entry)
	}
}

// deleteWorkloadPID removes a PID from a workload
func (cr *Resolver) deleteWorkloadPID(pid uint32, workload *cgroupModel.CacheEntry) {
	workload.Lock()
	defer workload.Unlock()

	delete(workload.PIDs, pid)

	// check if the workload should be deleted
	if len(workload.PIDs) <= 0 {
		cr.cgroups.Remove(workload.CGroupFile)
		cr.hostWorkloads.Remove(workload.CGroupID)
		if workload.ContainerID != "" {
			cr.containerWorkloads.Remove(workload.ContainerID)
		}
	}
}

// Len return the number of entries
func (cr *Resolver) Len() int {
	cr.Lock()
	defer cr.Unlock()

	return cr.cgroups.Len()
}
