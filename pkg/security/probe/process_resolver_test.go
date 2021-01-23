// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func testCacheSize(t *testing.T, resolver *ProcessResolver) {
	err := retry.Do(
		func() error {
			runtime.GC()
			if atomic.LoadInt64(&resolver.cacheSize) == 0 {
				return nil
			}

			return errors.New("cache size error")
		},
	)
	assert.Nil(t, err)
}

func TestFork1st(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// parent
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[child.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)

	// nothing
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestFork2nd(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Ancestor, parent)

	// nothing
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	exec := NewProcessCacheEntry()
	exec.Pid = child.Pid
	exec.PPid = child.PPid
	exec.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// parent
	//     \ [child] -> exec
	resolver.AddExecEntry(exec.Pid, exec)
	assert.Equal(t, resolver.entryCache[exec.Pid], exec)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, exec.Ancestor, child)
	assert.Equal(t, exec.Ancestor.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 3)

	// [parent]
	//     \ [child] -> exec
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec.Ancestor, child)
	assert.Equal(t, exec.Ancestor.Ancestor, parent)

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestOrphanExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	exec := NewProcessCacheEntry()
	exec.Pid = child.Pid
	exec.PPid = child.PPid
	exec.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Ancestor, parent)

	// [parent]
	//     \ [child] -> exec
	resolver.AddExecEntry(exec.Pid, exec)
	assert.Equal(t, resolver.entryCache[exec.Pid], exec)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec.Ancestor, child)
	assert.Equal(t, exec.Ancestor.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 3)

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkExecExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	exec1 := NewProcessCacheEntry()
	exec1.Pid = child.Pid
	exec1.PPid = child.PPid
	exec1.ExecTimestamp = time.Now()

	exec2 := NewProcessCacheEntry()
	exec2.Pid = child.Pid
	exec2.PPid = child.PPid
	exec2.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Ancestor, parent)

	// [parent]
	//     \ [child] -> exec1
	resolver.AddExecEntry(exec1.Pid, exec1)
	assert.Equal(t, resolver.entryCache[exec1.Pid], exec1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec1.Ancestor, child)
	assert.Equal(t, exec1.Ancestor.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 3)

	// [parent]
	//     \ [child] -> [exec1] -> exec2
	resolver.AddExecEntry(exec2.Pid, exec2)
	assert.Equal(t, resolver.entryCache[exec2.Pid], exec2)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec2.Ancestor, exec1)
	assert.Equal(t, exec2.Ancestor.Ancestor, child)
	assert.Equal(t, exec2.Ancestor.Ancestor.Ancestor, parent)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 4)

	// nothing
	resolver.DeleteEntry(exec2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkReuse(t *testing.T) {
	parent1 := NewProcessCacheEntry()
	parent1.Pid = 1
	parent1.ForkTimestamp = time.Now()

	child1 := NewProcessCacheEntry()
	child1.Pid = 2
	child1.PPid = parent1.Pid
	child1.ForkTimestamp = time.Now()

	exec1 := NewProcessCacheEntry()
	exec1.Pid = child1.Pid
	exec1.PPid = child1.PPid
	exec1.ExecTimestamp = time.Now()

	parent2 := NewProcessCacheEntry()
	parent2.Pid = 1
	parent2.ForkTimestamp = time.Now()

	child2 := NewProcessCacheEntry()
	child2.Pid = 3
	child2.PPid = parent2.Pid
	child2.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent1
	resolver.AddForkEntry(parent1.Pid, parent1)
	assert.Equal(t, resolver.entryCache[parent1.Pid], parent1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 1)

	// parent1
	//     \ child1
	resolver.AddForkEntry(child1.Pid, child1)
	assert.Equal(t, resolver.entryCache[child1.Pid], child1)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child1.Ancestor, parent1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 2)

	// [parent1]
	//     \ child1
	resolver.DeleteEntry(parent1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent1.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child1.Ancestor, parent1)

	// [parent1]
	//     \ [child1] -> exec1
	resolver.AddExecEntry(exec1.Pid, exec1)
	assert.Equal(t, resolver.entryCache[exec1.Pid], exec1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec1.Ancestor, child1)
	assert.Equal(t, exec1.Ancestor.Ancestor, parent1)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 3)

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	resolver.AddForkEntry(parent2.Pid, parent2)
	assert.Equal(t, resolver.entryCache[parent2.Pid], parent2)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 4)

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	//     \ child2
	resolver.AddForkEntry(child2.Pid, child2)
	assert.Equal(t, resolver.entryCache[child2.Pid], child2)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, child2.Ancestor, parent2)
	assert.EqualValues(t, atomic.LoadInt64(&resolver.cacheSize), 5)

	// parent2:pid1
	//     \ child2
	resolver.DeleteEntry(exec1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[exec1.Pid])
	assert.Equal(t, len(resolver.entryCache), 2)

	// [parent2:pid1]
	//     \ child2
	resolver.DeleteEntry(parent2.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent2.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child2.Ancestor, parent2)

	// nothing
	resolver.DeleteEntry(child2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}

func TestForkForkExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = parent.Pid
	child.ForkTimestamp = time.Now()

	grandChild := NewProcessCacheEntry()
	grandChild.Pid = 3
	grandChild.PPid = child.Pid
	grandChild.ForkTimestamp = time.Now()

	childExec := NewProcessCacheEntry()
	childExec.Pid = child.Pid
	childExec.PPid = child.PPid
	childExec.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil, NewProcessResolverOpts(true, 10000))
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddForkEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)

	// parent
	//     \ child
	resolver.AddForkEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Ancestor, parent)

	// parent
	//     \ child
	//          \ grandChild
	resolver.AddForkEntry(grandChild.Pid, grandChild)
	assert.Equal(t, resolver.entryCache[grandChild.Pid], grandChild)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, grandChild.Ancestor, child)
	assert.Equal(t, grandChild.Ancestor.Ancestor, parent)

	// parent
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.AddExecEntry(childExec.Pid, childExec)
	assert.Equal(t, resolver.entryCache[childExec.Pid], childExec)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, childExec.Ancestor, child)
	assert.Equal(t, childExec.Ancestor.Ancestor, parent)
	assert.Equal(t, grandChild.Ancestor, child)
	assert.Equal(t, grandChild.Ancestor.Ancestor, parent)

	// [parent]
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 2)

	// [parent]
	//     \ [child]
	//          \ grandChild
	resolver.DeleteEntry(childExec.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[childExec.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)

	// nothing
	resolver.DeleteEntry(grandChild.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))

	testCacheSize(t, resolver)
}
