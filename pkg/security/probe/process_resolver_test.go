// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/stretchr/testify/assert"
)

func testCacheSize(t *testing.T, resolver *ProcessResolver) {
	err := retry.Do(
		func() error {
			if atomic.LoadInt64(&resolver.cacheSize) == 0 {
				return nil
			}

			return fmt.Errorf("cache size error: %d", atomic.LoadInt64(&resolver.cacheSize))
		},
	)
	assert.Nil(t, err)
}

func TestFork1st(t *testing.T) {
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

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
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

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
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec := resolver.NewProcessCacheEntry()
	exec.Pid = child.Pid
	exec.PPid = child.PPid
	exec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ [child] -> exec
	resolver.AddExecEntry(exec)
	assert.Equal(t, exec, resolver.entryCache[exec.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, child, exec.Ancestor)
	assert.Equal(t, parent, exec.Ancestor.Ancestor)
	assert.EqualValues(t, 3, atomic.LoadInt64(&resolver.cacheSize))

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
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec := resolver.NewProcessCacheEntry()
	exec.Pid = child.Pid
	exec.PPid = child.PPid
	exec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// [parent]
	//     \ [child] -> exec
	resolver.AddExecEntry(exec)
	assert.Equal(t, exec, resolver.entryCache[exec.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec.Ancestor)
	assert.Equal(t, parent, exec.Ancestor.Ancestor)
	assert.EqualValues(t, 3, atomic.LoadInt64(&resolver.cacheSize))

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkExecExec(t *testing.T) {
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	exec1 := resolver.NewProcessCacheEntry()
	exec1.Pid = child.Pid
	exec1.PPid = child.PPid
	exec1.ExecTime = time.Now()

	exec2 := resolver.NewProcessCacheEntry()
	exec2.Pid = child.Pid
	exec2.PPid = child.PPid
	exec2.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// [parent]
	//     \ [child] -> exec1
	resolver.AddExecEntry(exec1)
	assert.Equal(t, exec1, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child, exec1.Ancestor)
	assert.Equal(t, parent, exec1.Ancestor.Ancestor)
	assert.EqualValues(t, 3, atomic.LoadInt64(&resolver.cacheSize))

	// [parent]
	//     \ [child] -> [exec1] -> exec2
	resolver.AddExecEntry(exec2)
	assert.Equal(t, exec2, resolver.entryCache[exec2.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, exec1, exec2.Ancestor)
	assert.Equal(t, child, exec2.Ancestor.Ancestor)
	assert.Equal(t, parent, exec2.Ancestor.Ancestor.Ancestor)
	assert.EqualValues(t, 4, atomic.LoadInt64(&resolver.cacheSize))

	// nothing
	resolver.DeleteEntry(exec2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkReuse(t *testing.T) {
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent1 := resolver.NewProcessCacheEntry()
	parent1.Pid = 1
	parent1.ForkTime = time.Now()

	child1 := resolver.NewProcessCacheEntry()
	child1.Pid = 2
	child1.PPid = parent1.Pid
	child1.ForkTime = time.Now()

	exec1 := resolver.NewProcessCacheEntry()
	exec1.Pid = child1.Pid
	exec1.PPid = child1.PPid
	exec1.ExecTime = time.Now()

	parent2 := resolver.NewProcessCacheEntry()
	parent2.Pid = 1
	parent2.ForkTime = time.Now()

	child2 := resolver.NewProcessCacheEntry()
	child2.Pid = 3
	child2.PPid = parent2.Pid
	child2.ForkTime = time.Now()

	// parent1
	resolver.AddForkEntry(parent1)
	assert.Equal(t, parent1, resolver.entryCache[parent1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, atomic.LoadInt64(&resolver.cacheSize))

	// parent1
	//     \ child1
	resolver.AddForkEntry(child1)
	assert.Equal(t, child1, resolver.entryCache[child1.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent1, child1.Ancestor)
	assert.EqualValues(t, 2, atomic.LoadInt64(&resolver.cacheSize))

	// [parent1]
	//     \ child1
	resolver.DeleteEntry(parent1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent1, child1.Ancestor)

	// [parent1]
	//     \ [child1] -> exec1
	resolver.AddExecEntry(exec1)
	assert.Equal(t, exec1, resolver.entryCache[exec1.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child1, exec1.Ancestor)
	assert.Equal(t, parent1, exec1.Ancestor.Ancestor)
	assert.EqualValues(t, 3, atomic.LoadInt64(&resolver.cacheSize))

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	resolver.AddForkEntry(parent2)
	assert.Equal(t, parent2, resolver.entryCache[parent2.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.EqualValues(t, 4, atomic.LoadInt64(&resolver.cacheSize))

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	//     \ child2
	resolver.AddForkEntry(child2)
	assert.Equal(t, child2, resolver.entryCache[child2.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, parent2, child2.Ancestor)
	assert.EqualValues(t, 5, atomic.LoadInt64(&resolver.cacheSize))

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
	resolver, err := NewProcessResolver(nil, nil, NewProcessResolverOpts(10000))
	if err != nil {
		t.Fatal(err)
	}

	parent := resolver.NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTime = time.Now()

	child := resolver.NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTime = time.Now()

	grandChild := resolver.NewProcessCacheEntry()
	grandChild.Pid = 3
	grandChild.PPid = child.Pid
	grandChild.ForkTime = time.Now()

	childExec := resolver.NewProcessCacheEntry()
	childExec.Pid = child.Pid
	childExec.PPid = child.PPid
	childExec.ExecTime = time.Now()

	// parent
	resolver.AddForkEntry(parent)
	assert.Equal(t, parent, resolver.entryCache[parent.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// parent
	//     \ child
	resolver.AddForkEntry(child)
	assert.Equal(t, child, resolver.entryCache[child.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent, child.Ancestor)

	// parent
	//     \ child
	//          \ grandChild
	resolver.AddForkEntry(grandChild)
	assert.Equal(t, grandChild, resolver.entryCache[grandChild.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child, grandChild.Ancestor)
	assert.Equal(t, parent, grandChild.Ancestor.Ancestor)

	// parent
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.AddExecEntry(childExec)
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
