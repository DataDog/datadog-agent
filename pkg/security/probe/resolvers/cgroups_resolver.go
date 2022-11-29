// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// CgroupsResolver defines a cgroup monitor
type CgroupsResolver struct {
	pids *lru.Cache[string, uint32]
}

// AddPID1 associates a container id and a pid which is expected to be the pid 1
func (cr *CgroupsResolver) AddPID1(id string, pid uint32) {
	// the first one wins
	if _, exists := cr.pids.Get(id); !exists {
		cr.pids.Add(id, pid)
	}
}

// GetPID1 return the registered pid1
func (cr *CgroupsResolver) GetPID1(id string) (uint32, bool) {
	return cr.pids.Get(id)
}

// DelPID1 removes the entry
func (cr *CgroupsResolver) DelPID1(id string) {
	cr.pids.Remove(id)
}

// Len return the number of entries
func (cr *CgroupsResolver) Len() int {
	return cr.pids.Len()
}

// NewCgroupsResolver returns a new cgroups monitor
func NewCgroupsResolver() (*CgroupsResolver, error) {
	pids, err := lru.New[string, uint32](1024)
	if err != nil {
		return nil, err
	}
	return &CgroupsResolver{
		pids: pids,
	}, nil
}
