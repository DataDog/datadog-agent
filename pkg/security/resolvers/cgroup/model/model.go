// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package model

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	sync.RWMutex
	ID   string
	PIDs *simplelru.LRU[uint32, int8]
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(id string, pids ...uint32) (*CacheEntry, error) {
	pidsLRU, err := simplelru.NewLRU[uint32, int8](1000, nil)
	if err != nil {
		return nil, err
	}

	newCGroup := CacheEntry{
		ID:   id,
		PIDs: pidsLRU,
	}

	for _, pid := range pids {
		newCGroup.PIDs.Add(pid, 0)
	}
	return &newCGroup, nil
}

// GetRootPIDs returns the list of root pids for the current workload
func (cgce *CacheEntry) GetRootPIDs() []uint32 {
	cgce.Lock()
	defer cgce.Unlock()

	return cgce.PIDs.Keys()
}

// RemoveRootPID removes the provided root pid from the list of pids
func (cgce *CacheEntry) RemoveRootPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs.Remove(pid)
}
