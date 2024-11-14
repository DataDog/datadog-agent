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
	GetWorkload(string) (*cgroupModel.CacheEntry, bool)
	DelPID(uint32)
	DelPIDWithID(string, uint32)
	Len() int
	RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.RWMutex
	workloads *simplelru.LRU[string, *cgroupModel.CacheEntry]
}

// NewResolver returns a new cgroups monitor
func NewResolver() (*Resolver, error) {
	cr := &Resolver{
		Notifier: utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
	}
	workloads, err := simplelru.NewLRU(1024, func(_ string, value *cgroupModel.CacheEntry) {
		value.CallReleaseCallback()
		value.Deleted.Store(true)

		cr.NotifyListeners(CGroupDeleted, value)
	})
	if err != nil {
		return nil, err
	}
	cr.workloads = workloads
	return cr, nil
}

// Start starts the goroutine of the SBOM resolver
func (cr *Resolver) Start(_ context.Context) {
}

// AddPID associates a container id and a pid which is expected to be the pid 1
func (cr *Resolver) AddPID(process *model.ProcessCacheEntry) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.workloads.Get(string(process.ContainerID))
	if exists {
		entry.AddPID(process.Pid)
		return
	}

	var err error
	// create new entry now
	newCGroup, err := cgroupModel.NewCacheEntry(string(process.ContainerID), uint64(process.CGroup.CGroupFlags), process.Pid)
	if err != nil {
		seclog.Errorf("couldn't create new cgroup_resolver cache entry: %v", err)
		return
	}
	newCGroup.CreatedAt = uint64(process.ProcessContext.ExecTime.UnixNano())

	// add the new CGroup to the cache
	cr.workloads.Add(string(process.ContainerID), newCGroup)

	cr.NotifyListeners(CGroupCreated, newCGroup)
}

// GetWorkload returns the workload referenced by the provided ID
func (cr *Resolver) GetWorkload(id string) (*cgroupModel.CacheEntry, bool) {
	cr.RLock()
	defer cr.RUnlock()

	return cr.workloads.Get(id)
}

// DelPID removes a PID from the cgroup resolver
func (cr *Resolver) DelPID(pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	for _, id := range cr.workloads.Keys() {
		entry, exists := cr.workloads.Get(id)
		if exists {
			cr.deleteWorkloadPID(pid, entry)
		}
	}
}

// DelPIDWithID removes a PID from the cgroup cache entry referenced by the provided ID
func (cr *Resolver) DelPIDWithID(id string, pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.workloads.Get(id)
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
		cr.workloads.Remove(string(workload.ContainerID))
	}
}

// Len return the number of entries
func (cr *Resolver) Len() int {
	cr.RLock()
	defer cr.RUnlock()

	return cr.workloads.Len()
}
