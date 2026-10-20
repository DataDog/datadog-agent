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

func TestEBPFLessExecWithoutForkInvalidParent(t *testing.T) {
	for _, tc := range []struct{ pid, ppid uint32 }{{200, 0}, {200, 200}, {1, 0}} {
		r := &EBPFLessResolver{entryCache: make(map[CacheResolverKey]*model.ProcessCacheEntry)}
		entry := addEBPFLessTestExec(r, CacheResolverKey{Pid: tc.pid, NSID: 1}, tc.ppid, "/bin/sh", 200)
		assert.Nil(t, entry.Ancestor)
	}
}
