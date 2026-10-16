// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testContainerID = "0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c"

func TestContainerIDCache(t *testing.T) {
	procfs := newFakeProcFS(t)
	// cgroup v2
	procfs.writeFile(1, "cgroup", "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234.slice/cri-containerd-"+testContainerID+".scope\n")
	// cgroup v1 (with the unified hierarchy line of the hybrid mode)
	procfs.writeFile(2, "cgroup", "12:pids:/docker/"+testContainerID+"\n11:memory:/docker/"+testContainerID+"\n0::/system.slice/containerd.service\n")
	// host process
	procfs.writeFile(3, "cgroup", "0::/user.slice/user-1000.slice/session-1.scope\n")

	cache := newContainerIDCache(procfs.root)
	assert.Equal(t, testContainerID, cache.get(1))
	assert.Equal(t, testContainerID, cache.get(2))
	assert.Empty(t, cache.get(3))
	assert.Empty(t, cache.get(4), "exited process")
	assert.Len(t, cache.ids, 3, "the ids of the exited processes are not cached")

	// cached
	procfs.removeProcess(1)
	assert.Equal(t, testContainerID, cache.get(1))

	cache.prune(func(pid uint32) bool { return pid != 1 })
	assert.Empty(t, cache.get(1))
	assert.Len(t, cache.ids, 2)
}
