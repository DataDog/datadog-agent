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
)

type pid1CacheEntry struct {
	pid      uint32
	refCount int
}

// CgroupsResolver defines a cgroup monitor
type CgroupsResolver struct {
	sync.RWMutex
	pids *simplelru.LRU[string, *pid1CacheEntry]
}

// AddPID1 associates a container id and a pid which is expected to be the pid 1
func (cr *CgroupsResolver) AddPID1(id string, pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.pids.Get(id)
	if !exists {
		cr.pids.Add(id, &pid1CacheEntry{pid: pid, refCount: 1})
	} else {
		if entry.pid > pid {
			entry.pid = pid
		}
		entry.refCount++
	}
}

// GetPID1 return the registered pid1
func (cr *CgroupsResolver) GetPID1(id string) (uint32, bool) {
	cr.RLock()
	defer cr.RUnlock()

	entry, exists := cr.pids.Get(id)
	if !exists {
		return 0, false
	}

	return entry.pid, true
}

// DelByPID force removes the entry
func (cr *CgroupsResolver) DelByPID(pid uint32) {
	cr.Lock()
	defer cr.Unlock()

	for _, id := range cr.pids.Keys() {
		entry, exists := cr.pids.Get(id)
		if exists && entry.pid == pid {
			cr.pids.Remove(id)
			break
		}
	}
}

// Release decrement usage
func (cr *CgroupsResolver) Release(id string) {
	cr.Lock()
	defer cr.Unlock()

	entry, exists := cr.pids.Get(id)
	if exists {
		entry.refCount--
		if entry.refCount <= 0 {
			cr.pids.Remove(id)
		}
	}
}

// Len return the number of entries
func (cr *CgroupsResolver) Len() int {
	cr.RLock()
	defer cr.RUnlock()

	return cr.pids.Len()
}

// NewCgroupsResolver returns a new cgroups monitor
func NewCgroupsResolver() (*CgroupsResolver, error) {
	pids, err := simplelru.NewLRU[string, *pid1CacheEntry](1024, nil)
	if err != nil {
		return nil, err
	}
	return &CgroupsResolver{
		pids: pids,
	}, nil
}
