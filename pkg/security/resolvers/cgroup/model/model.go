// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	model.CGroupContext
	model.ContainerContext
	sync.RWMutex
	Deleted *atomic.Bool
	PIDs    map[uint32]bool
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(containerID containerutils.ContainerID, cgroupContext *model.CGroupContext, pids ...uint32) (*CacheEntry, error) {
	newCGroup := CacheEntry{
		Deleted: atomic.NewBool(false),
		ContainerContext: model.ContainerContext{
			ContainerID: containerID,
		},
		PIDs: make(map[uint32]bool, 10),
	}

	if cgroupContext != nil {
		newCGroup.CGroupContext = *cgroupContext
	}

	for _, pid := range pids {
		newCGroup.PIDs[pid] = true
	}
	return &newCGroup, nil
}

// GetPIDs returns the list of pids for the current workload
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.RLock()
	defer cgce.RUnlock()

	pids := make([]uint32, len(cgce.PIDs))
	i := 0
	for k := range cgce.PIDs {
		pids[i] = k
		i++
	}

	return pids
}

// RemovePID removes the provided pid from the list of pids
func (cgce *CacheEntry) RemovePID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	delete(cgce.PIDs, pid)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs[pid] = true
}
