// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroup

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// Resolver defines a cgroup monitor
type Resolver struct {
	sync.RWMutex
	workloads    *simplelru.LRU[string, *cgroupModel.CacheEntry]
	sbomResolver *sbom.Resolver
}

// AddPID associates a container id and a pid which is expected to be the pid 1
func (cr *Resolver) AddPID(process *model.ProcessCacheEntry) {
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
	newCGroup, err := cgroupModel.NewCacheEntry(process.ContainerID, process.Pid)
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
func (cr *Resolver) Len() int {
	cr.RLock()
	defer cr.RUnlock()

	return cr.workloads.Len()
}

// NewResolver returns a new cgroups monitor
func NewResolver(sbomResolver *sbom.Resolver) (*Resolver, error) {
	workloads, err := simplelru.NewLRU[string, *cgroupModel.CacheEntry](1024, func(key string, value *cgroupModel.CacheEntry) {
		// notify the SBOM resolver that the CGroupCacheEntry was ejected
		sbomResolver.Delete(key)
	})
	if err != nil {
		return nil, err
	}
	return &Resolver{
		workloads:    workloads,
		sbomResolver: sbomResolver,
	}, nil
}
