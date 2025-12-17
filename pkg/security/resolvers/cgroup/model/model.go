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
	sync.RWMutex
	CGroupContext    model.CGroupContext
	ContainerContext model.ContainerContext
	Deleted          *atomic.Bool

	pids map[uint32]bool
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(containerID containerutils.ContainerID, cgroupContext *model.CGroupContext, pids ...uint32) *CacheEntry {
	newCGroup := CacheEntry{
		Deleted: atomic.NewBool(false),
		ContainerContext: model.ContainerContext{
			ContainerID: containerID,
		},
		CGroupContext: model.CGroupContext{
			Releasable: &model.Releasable{},
		},
		pids: make(map[uint32]bool, 10),
	}

	if cgroupContext != nil {
		newCGroup.CGroupContext = *cgroupContext
	}

	for _, pid := range pids {
		newCGroup.pids[pid] = true
	}
	return &newCGroup
}

// GetPIDs returns the list of pids for the current workload
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.RLock()
	defer cgce.RUnlock()

	pids := make([]uint32, len(cgce.pids))
	i := 0
	for k := range cgce.pids {
		pids[i] = k
		i++
	}

	return pids
}

// RemovePID removes the provided pid from the list of pids
func (cgce *CacheEntry) RemovePID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	delete(cgce.pids, pid)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.pids[pid] = true
}
