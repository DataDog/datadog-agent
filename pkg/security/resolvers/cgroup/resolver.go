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
	"sync"

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
)

// ResolverInterface defines the interface implemented by a cgroup resolver
type ResolverInterface interface {
	Start(context.Context)
	AddPID(*model.ProcessCacheEntry)
	GetWorkload(containerutils.ContainerID) (*cgroupModel.CacheEntry, bool)
	DelPID(uint32)
	Len() int
	RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}

// Resolver defines a cgroup monitor
type Resolver struct {
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	sync.Mutex
	statsdClient       statsd.ClientInterface
	cgroups            *simplelru.LRU[model.PathKey, *model.CGroupContext]
	hostWorkloads      *simplelru.LRU[containerutils.CGroupID, *cgroupModel.CacheEntry]
	containerWorkloads *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry]
}

// NewResolver returns a new cgroups monitor
func NewResolver(statsdClient statsd.ClientInterface) (*Resolver, error) {
	cr := &Resolver{
		Notifier:     utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		statsdClient: statsdClient,
	}

	cleanup := func(value *cgroupModel.CacheEntry) {
		value.CallReleaseCallback()
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

	cr.cgroups, err = simplelru.NewLRU(maxCgroupEntries, func(_ model.PathKey, _ *model.CGroupContext) {})
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

// GetContainerWorkloads returns the container workloads
func (cr *Resolver) GetContainerWorkloads() *simplelru.LRU[containerutils.ContainerID, *cgroupModel.CacheEntry] {
	cr.Lock()
	defer cr.Unlock()
	return cr.containerWorkloads
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
