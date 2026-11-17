// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func addEBPFLessTestExec(r *EBPFLessResolver, key CacheResolverKey, ppid uint32, file string, ts uint64) *model.ProcessCacheEntry {
	return r.AddExecEntry(key, ppid, file, []string{file}, false, nil, false, "", "", ts, "")
}

func TestEBPFLessExecWithoutFork(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	parentKey := CacheResolverKey{Pid: 100, NSID: 1}
	childKey := CacheResolverKey{Pid: 200, NSID: 1}
	parent := addEBPFLessTestExec(r, parentKey, 0, "/usr/sbin/php-fpm", 100)
	parent.Credentials.UID = 33
	parent.TTYName = "pts1"

	entry := addEBPFLessTestExec(r, childKey, 100, "/bin/sh", 200)
	require.Same(t, entry, r.Resolve(childKey))
	assert.Equal(t, "/bin/sh", entry.FileEvent.PathnameStr)
	assert.EqualValues(t, 100, entry.PPid)
	assert.True(t, entry.IsExec)
	assert.Equal(t, time.Unix(0, 200), entry.ExecTime)
	assert.True(t, entry.ForkTime.IsZero())
	assert.EqualValues(t, 33, entry.Credentials.UID)

	reconstructed := entry.Ancestor
	require.NotNil(t, reconstructed)
	assert.EqualValues(t, 200, reconstructed.Pid)
	assert.EqualValues(t, model.ProcessCacheEntryFromPlaceholder, reconstructed.Source)
	assert.Equal(t, "/usr/sbin/php-fpm", reconstructed.FileEvent.PathnameStr)
	assert.True(t, reconstructed.ForkTime.IsZero())
	assert.True(t, reconstructed.ExitTime.IsZero())
	require.Same(t, parent, reconstructed.Ancestor)
	assert.True(t, parent.ExitTime.IsZero())
	assert.True(t, parent.StopExecutionTime.IsZero())

	// A subsequent utility must inherit sh, not the original php-fpm image.
	utilityKey := CacheResolverKey{Pid: 300, NSID: 1}
	fork := r.AddForkEntry(utilityKey, 200, 300)
	utility := addEBPFLessTestExec(r, utilityKey, 200, "/usr/bin/realpath", 400)
	require.Same(t, fork, utility.Ancestor)
	assert.Equal(t, "/bin/sh", utility.Ancestor.FileEvent.PathnameStr)
	assert.Same(t, entry, fork.Ancestor)
}

func TestEBPFLessExecWithoutForkMissingParent(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	key := CacheResolverKey{Pid: 200, NSID: 1}
	entry := addEBPFLessTestExec(r, key, 100, "/bin/sh", 200)
	require.NotNil(t, entry.Ancestor)
	assert.EqualValues(t, 100, entry.Ancestor.PPid)
	assert.Empty(t, entry.Ancestor.FileEvent.PathnameStr)
	assert.Nil(t, entry.Ancestor.Ancestor)
	assert.Equal(t, "/bin/sh", entry.FileEvent.PathnameStr)
}

func TestEBPFLessExecWithoutForkNamespaceIsolation(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	addEBPFLessTestExec(r, CacheResolverKey{Pid: 100, NSID: 1}, 0, "/wrong/namespace", 100)
	entry := addEBPFLessTestExec(r, CacheResolverKey{Pid: 200, NSID: 2}, 100, "/bin/sh", 200)
	require.NotNil(t, entry.Ancestor)
	assert.Nil(t, entry.Ancestor.Ancestor)
	assert.Empty(t, entry.Ancestor.FileEvent.PathnameStr)
}

func TestEBPFLessExecExistingEntry(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	parentKey := CacheResolverKey{Pid: 100, NSID: 1}
	childKey := CacheResolverKey{Pid: 200, NSID: 1}
	addEBPFLessTestExec(r, parentKey, 0, "/usr/sbin/php-fpm", 100)
	fork := r.AddForkEntry(childKey, 100, 200)
	firstExec := addEBPFLessTestExec(r, childKey, 0, "/bin/sh", 300)
	secondExec := addEBPFLessTestExec(r, childKey, 100, "/bin/bash", 400)
	assert.Same(t, firstExec, secondExec.Ancestor)
	assert.Same(t, fork, firstExec.Ancestor)
	assert.EqualValues(t, 100, firstExec.PPid)
	assert.Equal(t, time.Unix(0, 200), fork.ForkTime)
	assert.True(t, fork.ExitTime.IsZero())
}

func TestEBPFLessExecWithoutForkPIDReuse(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	key := CacheResolverKey{Pid: 200, NSID: 1}
	old := addEBPFLessTestExec(r, key, 100, "/old/image", 100)
	r.DeleteEntry(key, time.Unix(0, 200))
	fork := r.AddForkEntry(key, 300, 300)
	entry := addEBPFLessTestExec(r, key, 300, "/new/image", 400)
	assert.Same(t, fork, entry.Ancestor)
	assert.Equal(t, "/new/image", entry.FileEvent.PathnameStr)
	assert.EqualValues(t, 300, entry.PPid)
	assert.Equal(t, time.Unix(0, 200), old.ExitTime)
}

func newEBPFLessTestContext(r *EBPFLessResolver, key CacheResolverKey, ppid uint32, file string, birth uint64) *model.ProcessCacheEntry {
	entry := r.NewEntry(key, ppid, file, []string{"worker", "--worker"}, false,
		[]string{"WORKER=1"}, false, "child-container", "child-cgroup", birth, "pts1", model.ProcessCacheEntryFromProcFS)
	entry.Credentials.UID = 33
	return entry
}

func TestEBPFLessContextWithoutExec(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	parent := addEBPFLessTestExec(r, CacheResolverKey{Pid: 100, NSID: 1}, 0, "/parent", 100)
	parent.Credentials.UID = 99
	key := CacheResolverKey{Pid: 200, NSID: 1}
	entry := newEBPFLessTestContext(r, key, 100, "/worker", 200)
	require.Same(t, entry, r.AddContextEntry(key, entry, 200))
	assert.Same(t, entry, r.Resolve(key))
	assert.Same(t, parent, entry.Ancestor)
	assert.Equal(t, "/worker", entry.FileEvent.PathnameStr)
	assert.Equal(t, []string{"worker", "--worker"}, entry.ArgsEntry.Values)
	assert.Equal(t, []string{"WORKER=1"}, entry.EnvsEntry.Values)
	assert.EqualValues(t, 33, entry.Credentials.UID)
	assert.EqualValues(t, "child-container", entry.ContainerContext.ContainerID)
	assert.EqualValues(t, "child-cgroup", entry.CGroup.CGroupID)
	assert.Equal(t, time.Unix(0, 200), entry.ForkTime)
	assert.True(t, entry.ExecTime.IsZero())
	assert.False(t, entry.IsExec)
	assert.True(t, parent.StopExecutionTime.IsZero())

	// A later real exec replaces the observed image, not a guessed copy of
	// the parent's image. The first open did not need to wait for that exec.
	exec := addEBPFLessTestExec(r, key, 0, "/replacement", 300)
	assert.Same(t, entry, exec.Ancestor)
	assert.EqualValues(t, 100, exec.PPid)
	assert.EqualValues(t, 33, exec.Credentials.UID)
	assert.Equal(t, time.Unix(0, 300), entry.StopExecutionTime)
	assert.True(t, entry.ExitTime.IsZero())
	assert.True(t, parent.StopExecutionTime.IsZero())
}

func TestEBPFLessContextDoesNotGuessParentContainer(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	parent := addEBPFLessTestExec(r, CacheResolverKey{Pid: 100, NSID: 1}, 0, "/parent", 100)
	parent.ContainerContext.ContainerID = "parent-container"
	key := CacheResolverKey{Pid: 200, NSID: 1}
	entry := newEBPFLessTestContext(r, key, 100, "/worker", 200)
	entry.ContainerContext.ContainerID = ""
	r.AddContextEntry(key, entry, 200)
	assert.Same(t, parent, entry.Ancestor)
	assert.Empty(t, entry.ContainerContext.ContainerID)
}

func TestEBPFLessContextDoesNotRollBackCurrentImage(t *testing.T) {
	for _, kind := range []string{"fork", "exec", "context", "unknown birth"} {
		t.Run(kind, func(t *testing.T) {
			r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
			addEBPFLessTestExec(r, CacheResolverKey{Pid: 100, NSID: 1}, 0, "/parent", 100)
			key := CacheResolverKey{Pid: 200, NSID: 1}
			var previous *model.ProcessCacheEntry
			switch kind {
			case "fork":
				previous = r.AddForkEntry(key, 100, 200)
			case "context":
				previous = r.AddContextEntry(key, newEBPFLessTestContext(r, key, 100, "/current", 200), 200)
			default:
				previous = addEBPFLessTestExec(r, key, 100, "/current", 300)
			}
			birth := uint64(200)
			if kind == "unknown birth" {
				birth = 0
			}
			entry := newEBPFLessTestContext(r, key, 100, "/stale", birth)
			assert.Same(t, previous, r.AddContextEntry(key, entry, birth))
			assert.Same(t, previous, r.Resolve(key))
			assert.NotEqual(t, "/stale", previous.FileEvent.PathnameStr)
			assert.True(t, previous.ExitTime.IsZero())
		})
	}
}

func TestEBPFLessContextPIDReuseAndNamespaceIsolation(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	addEBPFLessTestExec(r, CacheResolverKey{Pid: 300, NSID: 2}, 0, "/wrong-namespace", 100)
	key := CacheResolverKey{Pid: 200, NSID: 1}
	require.True(t, r.PrepareProcessGeneration(key, 100, false))
	old := addEBPFLessTestExec(r, key, 100, "/old", 1000)
	require.True(t, r.PrepareProcessGeneration(key, 300, true))
	// A coarse procfs birth date can precede the old exec's wall timestamp.
	// Only the kernel birth stamp, not that date, proves this is a new PID.
	entry := newEBPFLessTestContext(r, key, 300, "/new", 50)
	assert.Same(t, entry, r.AddContextEntry(key, entry, 50))
	assert.Same(t, entry, r.Resolve(key))
	assert.Equal(t, "/new", entry.FileEvent.PathnameStr)
	assert.Nil(t, entry.Ancestor) // neither the old PID generation nor another PID namespace
	assert.True(t, old.ExitTime.IsZero())
	assert.True(t, old.StopExecutionTime.IsZero())
}

func TestEBPFLessGenerationBookkeeping(t *testing.T) {
	r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
	key := CacheResolverKey{Pid: 200, NSID: 1}
	require.True(t, r.PrepareProcessGeneration(key, 200, false))
	entry := addEBPFLessTestExec(r, key, 100, "/current", 300)
	assert.True(t, r.PrepareProcessGeneration(key, 200, true))
	assert.Same(t, entry, r.Resolve(key))
	assert.False(t, r.PrepareProcessGeneration(key, 100, true))
	assert.Same(t, entry, r.Resolve(key))
	assert.True(t, r.PrepareProcessGeneration(key, 0, false)) // legacy sender
	assert.Same(t, entry, r.Resolve(key))
	r.DeleteEntry(key, time.Unix(0, 400))
	assert.NotContains(t, r.processStartTimes, key)

	// Even a failed procfs lookup can leave only a generation record, without
	// a cached image. Its exit must release that bookkeeping too.
	require.True(t, r.PrepareProcessGeneration(key, 500, false))
	require.Nil(t, r.Resolve(key))
	r.DeleteEntry(key, time.Unix(0, 600))
	assert.Empty(t, r.processStartTimes)
}

func TestEBPFLessExecWithoutForkInvalidParent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pid, ppid uint32
	}{
		{name: "unknown parent", pid: 200},
		{name: "self parent", pid: 200, ppid: 200},
		{name: "root without parent", pid: 1},
		{name: "root with apparent parent", pid: 1, ppid: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
			entry := addEBPFLessTestExec(r, CacheResolverKey{Pid: tc.pid, NSID: 1}, tc.ppid, "/bin/sh", 200)
			assert.Nil(t, entry.Ancestor)
		})
	}
}
