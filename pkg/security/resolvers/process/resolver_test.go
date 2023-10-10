// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process holds process related files
package process

import (
	"fmt"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

func testCacheSize(t *testing.T, resolver *Resolver) {
	err := retry.Do(
		func() error {
			if resolver.cacheSize.Load() == 0 {
				return nil
			}

			return fmt.Errorf("cache size error: %d", resolver.cacheSize.Load())
		},
	)
	assert.Nil(t, err)
}

func TestFork1st(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// parent
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[child.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// nothing
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestFork2nd(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// nothing
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkExec(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec.PPid = child.PPid
	exec.FileEvent.Inode = 123
	exec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// parent
	//     \ [child] -> exec
	resolver.AddExecEntry(exec, 0)
	assert.Equal(t, exec, resolver.entryCache[exec.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, child, exec.Ancestor)
	assert.Equal(t, parent, exec.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// [parent]
	//     \ [child] -> exec
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec.Ancestor)
	assert.Equal(t, parent, exec.Ancestor.Ancestor)

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestOrphanExec(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec.Pid = child.Pid
	exec.PPid = child.PPid
	exec.FileEvent.Inode = 123
	exec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// [parent]
	//     \ [child] -> exec
	resolver.AddExecEntry(exec, 0)
	assert.Equal(t, exec, resolver.entryCache[exec.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec.Ancestor)
	assert.Equal(t, parent, exec.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkExecExec(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec1.PPid = child.PPid
	exec1.FileEvent.Inode = 123
	exec1.ExecTime = time.Now()

	exec2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec2.Pid = child.Pid
	exec2.PPid = child.PPid
	exec2.FileEvent.Inode = 456
	exec2.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// [parent]
	//     \ [child] -> exec1
	resolver.AddExecEntry(exec1, 0)
	assert.Equal(t, exec1, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec1.Ancestor)
	assert.Equal(t, parent, exec1.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// [parent]
	//     \ [child] -> [exec1] -> exec2
	resolver.AddExecEntry(exec2, 0)
	assert.Equal(t, exec2, resolver.entryCache[exec2.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, exec1, exec2.Ancestor)
	assert.Equal(t, child, exec2.Ancestor.Ancestor)
	assert.Equal(t, parent, exec2.Ancestor.Ancestor.Ancestor)
	assert.EqualValues(t, 4, resolver.cacheSize.Load())

	// nothing
	resolver.DeleteEntry(exec2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkReuse(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent1.ForkTime = time.Now()

	child1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child1.PPid = parent1.Pid
	child1.ForkTime = time.Now()

	exec1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child1.Pid, Tid: child1.Pid})
	exec1.PPid = child1.PPid
	exec1.FileEvent.Inode = 123
	exec1.ExecTime = time.Now()

	parent2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent2.ForkTime = time.Now()

	child2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 3, Tid: 3})
	child2.PPid = parent2.Pid
	child2.ForkTime = time.Now()

	// parent1
	resolver.AddForkEntry(parent1, 0)
	assert.Equal(t, parent1, resolver.entryCache[parent1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent1
	//     \ child1
	resolver.AddForkEntry(child1, 0)
	assert.Equal(t, child1, resolver.entryCache[child1.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent1, child1.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// [parent1]
	//     \ child1
	resolver.DeleteEntry(parent1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent1, child1.Ancestor)

	// [parent1]
	//     \ [child1] -> exec1
	resolver.AddExecEntry(exec1, 0)
	assert.Equal(t, exec1, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child1, exec1.Ancestor)
	assert.Equal(t, parent1, exec1.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	resolver.AddForkEntry(parent2, 0)
	assert.Equal(t, parent2, resolver.entryCache[parent2.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.EqualValues(t, 4, resolver.cacheSize.Load())

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	//     \ child2
	resolver.AddForkEntry(child2, 0)
	assert.Equal(t, child2, resolver.entryCache[child2.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, parent2, child2.Ancestor)
	assert.EqualValues(t, 5, resolver.cacheSize.Load())

	// parent2:pid1
	//     \ child2
	resolver.DeleteEntry(exec1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))

	// [parent2:pid1]
	//     \ child2
	resolver.DeleteEntry(parent2.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent2.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent2, child2.Ancestor)

	// nothing
	resolver.DeleteEntry(child2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkForkExec(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	grandChild := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 3, Tid: 3})
	grandChild.PPid = child.Pid
	grandChild.ForkTime = time.Now()

	childExec := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	childExec.Pid = child.Pid
	childExec.PPid = child.PPid
	childExec.FileEvent.Inode = 123
	childExec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// parent
	//     \ child
	//          \ grandChild
	resolver.AddForkEntry(grandChild, 0)
	assert.Equal(t, grandChild, resolver.entryCache[grandChild.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child, grandChild.Ancestor)
	assert.Equal(t, parent, grandChild.Ancestor.Ancestor)

	// parent
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.AddExecEntry(childExec, 0)
	assert.Equal(t, childExec, resolver.entryCache[childExec.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child, childExec.Ancestor)
	assert.Equal(t, parent, childExec.Ancestor.Ancestor)
	assert.Equal(t, child, grandChild.Ancestor)
	assert.Equal(t, parent, grandChild.Ancestor.Ancestor)

	// [parent]
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))

	// [parent]
	//     \ [child]
	//          \ grandChild
	resolver.DeleteEntry(childExec.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[childExec.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// nothing
	resolver.DeleteEntry(grandChild.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestExecBomb(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec1.PPid = child.PPid
	exec1.FileEvent.Inode = 123
	exec1.ExecTime = time.Now()

	exec2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: child.Pid, Tid: child.Pid})
	exec2.Pid = child.Pid
	exec2.PPid = child.PPid
	exec2.FileEvent.Inode = 123
	exec2.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent, 0)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.cacheSize.Load())

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, resolver.cacheSize.Load())

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// [parent]
	//     \ [child] -> exec1
	resolver.AddExecEntry(exec1, 0)
	assert.Equal(t, exec1, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec1.Ancestor)
	assert.Equal(t, parent, exec1.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// [parent]
	//     \ [child] -> [exec1] -> exec2
	resolver.AddExecEntry(exec2, 0)
	assert.Equal(t, exec1, resolver.entryCache[exec2.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, exec1.ExecTime, exec2.ExecTime)
	assert.EqualValues(t, 3, resolver.cacheSize.Load())

	// nothing
	resolver.DeleteEntry(exec1.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestExecLostFork(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 11, Tid: 11})
	parent.FileEvent.BasenameStr = "agent"
	parent.ForkTime = time.Now()
	parent.FileEvent.Inode = 1
	parent.ExecInode = 1

	// parent
	resolver.AddForkEntry(parent, 0)

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 22, Tid: 22})
	child.PPid = parent.Pid
	child.FileEvent.Inode = 1

	// parent
	//     \ child
	resolver.AddForkEntry(child, parent.ExecInode)

	assert.Equal(t, "agent", child.FileEvent.BasenameStr)
	assert.False(t, child.IsParentMissing)

	// exec loss with inode 2

	child1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 33, Tid: 33})
	child1.FileEvent.BasenameStr = "sh"
	child1.PPid = child.Pid
	child1.ExecInode = 2

	// parent
	//     \ child
	//		\ child1
	resolver.AddForkEntry(child1, child1.ExecInode)

	assert.Equal(t, "agent", child1.FileEvent.BasenameStr)
	assert.True(t, child1.IsParentMissing)
}

func TestExecLostExec(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 11, Tid: 11})
	parent.FileEvent.BasenameStr = "agent"
	parent.ForkTime = time.Now()
	parent.FileEvent.Inode = 1
	parent.ExecInode = 1

	// parent
	resolver.AddForkEntry(parent, 0)

	child1 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 22, Tid: 22})
	child1.PPid = parent.Pid
	child1.FileEvent.Inode = 1
	child1.ExecInode = 1

	// parent
	//     \ child1
	resolver.AddForkEntry(child1, parent.ExecInode)

	assert.Equal(t, "agent", child1.FileEvent.BasenameStr)
	assert.False(t, child1.IsParentMissing)

	// exec loss with inode 2 and pid 22

	child2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 33, Tid: 33})
	child2.FileEvent.BasenameStr = "sh"
	child2.PPid = child1.Pid
	child2.ExecInode = 2

	// parent
	//     \ child1
	//		\ child2
	resolver.AddForkEntry(child2, child2.ExecInode)

	assert.Equal(t, "agent", child2.FileEvent.BasenameStr)
	assert.True(t, child2.IsParentMissing)
}

func TestIsExecChildRuntime(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()
	parent.FileEvent.Inode = 1

	// parent
	resolver.AddForkEntry(parent, 0)

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.FileEvent.Inode = 1

	// parent
	//     \ child
	resolver.AddForkEntry(child, 0)

	// parent
	//     \ child
	//      \ child2

	child2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child2.FileEvent.Inode = 2
	child2.PPid = child.Pid
	resolver.AddExecEntry(child2, 0)

	// parent
	//     \ child
	//      \ child2
	//       \ child3

	child3 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child3.FileEvent.Inode = 3
	child3.PPid = child2.Pid
	resolver.AddExecEntry(child3, 0)

	assert.False(t, parent.IsExecChild)
	assert.False(t, parent.IsThread) // root node, no fork

	assert.False(t, child.IsExecChild)
	assert.True(t, child.IsThread)

	assert.False(t, child2.IsExecChild)
	assert.False(t, child2.IsThread)

	assert.True(t, child3.IsExecChild)
	assert.False(t, child3.IsThread)

	child4 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child4.FileEvent.Inode = 3
	child4.PPid = child3.Pid
	resolver.AddExecEntry(child4, 0)

	assert.True(t, child3.IsExecChild)
	assert.False(t, child3.IsThread)
}

func TestIsExecChildSnapshot(t *testing.T) {
	resolver, err := NewResolver(nil, nil, &statsd.NoOpClient{}, nil, nil, nil, nil, nil, nil, nil, NewResolverOpts())
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 1, Tid: 1})
	parent.ForkTime = time.Now()
	parent.FileEvent.Inode = 1
	parent.IsThread = true

	// parent
	resolver.insertEntry(parent, nil, model.ProcessCacheEntryFromSnapshot)

	child := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child.PPid = parent.Pid
	child.FileEvent.Inode = 2
	child.IsThread = true

	// parent
	//     \ child

	resolver.setAncestor(child)
	resolver.insertEntry(child, nil, model.ProcessCacheEntryFromSnapshot)

	assert.False(t, parent.IsExecChild)
	assert.True(t, parent.IsThread) // root node, no fork

	assert.False(t, child.IsExecChild)
	assert.True(t, child.IsThread)

	// parent
	//     \ child
	//      \ child2

	child2 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child2.FileEvent.Inode = 3
	child2.PPid = child.Pid
	resolver.AddExecEntry(child2, 0)

	assert.False(t, child2.IsExecChild)
	assert.False(t, child2.IsThread)

	child3 := resolver.NewProcessCacheEntry(model.PIDContext{Pid: 2, Tid: 2})
	child3.FileEvent.Inode = 4
	child3.PPid = child2.Pid
	resolver.AddExecEntry(child3, 0)

	assert.True(t, child3.IsExecChild)
	assert.False(t, child3.IsThread)
}
