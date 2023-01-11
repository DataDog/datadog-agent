// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// CGroupCacheEntry cgroup resolver cache entry
type CGroupCacheEntry struct {
	sync.RWMutex
	ID   string
	PIDs *simplelru.LRU[uint32, int8]
}

// NewCGroupCacheEntry returns a new instance of a CGroupCacheEntry
func NewCGroupCacheEntry(id string, pids ...uint32) (*CGroupCacheEntry, error) {
	pidsLRU, err := simplelru.NewLRU[uint32, int8](1000, nil)
	if err != nil {
		return nil, err
	}

	newCGroup := CGroupCacheEntry{
		ID:   id,
		PIDs: pidsLRU,
	}

	for _, pid := range pids {
		newCGroup.PIDs.Add(pid, 0)
	}
	return &newCGroup, nil
}

// GetRootPIDs returns the list of root pids for the current workload
func (cgce *CGroupCacheEntry) GetRootPIDs() []uint32 {
	cgce.Lock()
	defer cgce.Unlock()

	return cgce.PIDs.Keys()
}

// RemoveRootPID removes the provided root pid from the list of pids
func (cgce *CGroupCacheEntry) RemoveRootPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs.Remove(pid)
}

// CGroupsResolver defines a cgroup monitor
type CGroupsResolver struct {
	sync.RWMutex
	workloads    *simplelru.LRU[string, *CGroupCacheEntry]
	sbomResolver *SBOMResolver
}

// AddPID associates a container id and a pid which is expected to be the pid 1
func (cr *CGroupsResolver) AddPID(process *model.ProcessCacheEntry) {
	cr.Lock()
	defer cr.Unlock()

	// if !process.IsContainerRoot() {
	// 	return
	// }

	entry, exists := cr.workloads.Get(process.ContainerID)
	if exists {
		entry.PIDs.Add(process.Pid, 0)
		return
	}

	var err error
	// create new entry now
	newCGroup, err := NewCGroupCacheEntry(process.ContainerID, process.Pid)
	if err != nil {
		seclog.Errorf("couldn't create new cgroup_resolver cache entry: %v", err)
		return
	}

	// add the new CGroup to the cache
	cr.workloads.Add(process.ContainerID, newCGroup)

	if cr.sbomResolver != nil {
		// a new entry was created, notify the SBOM resolver that it should create a new entry too
		cr.sbomResolver.Retain(process.ContainerID, newCGroup)
	}
}

// GetWorkload returns the workload referenced by the provided ID
func (cr *CGroupsResolver) GetWorkload(id string) (*CGroupCacheEntry, bool) {
	cr.RLock()
	defer cr.RUnlock()

	return cr.workloads.Get(id)
}

// DelPID removes a PID from the cgroup resolver
func (cr *CGroupsResolver) DelPID(pid uint32) {
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
func (cr *CGroupsResolver) DelPIDWithID(id string, pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.workloads.Get(id)
	if exists {
		cr.deleteWorkloadPID(pid, entry)
	}
}

// deleteWorkloadPID removes a PID from a workload
func (cr *CGroupsResolver) deleteWorkloadPID(pid uint32, workload *CGroupCacheEntry) {
	workload.Lock()
	defer workload.Unlock()

	for _, workloadPID := range workload.PIDs.Keys() {
		if pid == workloadPID {
			workload.PIDs.Remove(pid)
			break
		}
	}

	// check if the workload should be deleted
	if workload.PIDs.Len() <= 0 {
		cr.workloads.Remove(workload.ID)
	}
}

// Len return the number of entries
func (cr *CGroupsResolver) Len() int {
	cr.RLock()
	defer cr.RUnlock()

	return cr.workloads.Len()
}

// NewCGroupsResolver returns a new cgroups monitor
func NewCGroupsResolver(sbomResolver *SBOMResolver) (*CGroupsResolver, error) {
	workloads, err := simplelru.NewLRU[string, *CGroupCacheEntry](1024, func(key string, value *CGroupCacheEntry) {
		// notify the SBOM resolver that the CGroupCacheEntry was ejected
		sbomResolver.Release(key)
	})
	if err != nil {
		return nil, err
	}
	return &CGroupsResolver{
		workloads:    workloads,
		sbomResolver: sbomResolver,
	}, nil
}
