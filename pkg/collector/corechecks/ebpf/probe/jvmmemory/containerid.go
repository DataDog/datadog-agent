// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

// cgroupV1BaseController is the cgroup v1 controller whose path identifies
// the container of a process.
const cgroupV1BaseController = "memory"

// containerIDCache caches the container id of the JVMs. It is safe for
// concurrent use.
type containerIDCache struct {
	procRoot string

	mu  sync.Mutex
	ids map[uint32]string
}

func newContainerIDCache(procRoot string) *containerIDCache {
	return &containerIDCache{procRoot: procRoot, ids: make(map[uint32]string)}
}

// get returns the id of the container of pid, "" if pid does not run in a
// container or if it cannot be determined.
func (c *containerIDCache) get(pid uint32) string {
	c.mu.Lock()
	id, ok := c.ids[pid]
	c.mu.Unlock()
	if ok {
		return id
	}

	id, err := readContainerID(c.procRoot, pid)
	if err != nil {
		// not cached: the cgroup file of a running process is readable, so
		// the process most likely exited
		return ""
	}
	c.mu.Lock()
	c.ids[pid] = id
	c.mu.Unlock()
	return id
}

// prune forgets the PIDs for which alive returns false.
func (c *containerIDCache) prune(alive func(pid uint32) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for pid := range c.ids {
		if !alive(pid) {
			delete(c.ids, pid)
		}
	}
}

// readContainerID reads the id of the container of pid from
// <procRoot>/<pid>/cgroup.
func readContainerID(procRoot string, pid uint32) (string, error) {
	pidStr := strconv.FormatUint(uint64(pid), 10)
	// cgroup v2 (unified hierarchy) first, then the cgroup v1 base controller
	id, err := cgroups.IdentiferFromCgroupReferences(procRoot, pidStr, "", cgroups.ContainerFilter)
	if err != nil || id != "" {
		return id, err
	}
	return cgroups.IdentiferFromCgroupReferences(procRoot, pidStr, cgroupV1BaseController, cgroups.ContainerFilter)
}
