// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFork1st(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)
	assert.Equal(t, child.Parent, parent)

	// parent
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[child.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// nothing
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestFork2nd(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, len(parent.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// nothing
	resolver.DeleteEntry(child.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestForkExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	exec := NewProcessCacheEntry()
	exec.Pid = 2
	exec.PPid = 1
	exec.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// parent
	//     \ [child] -> exec
	resolver.AddEntry(exec.Pid, exec)
	assert.Equal(t, resolver.entryCache[exec.Pid], exec)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, exec.Parent, child)
	assert.Equal(t, exec.Parent.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// [parent]
	//     \ [child] -> exec
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, len(parent.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestOrphanExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	exec := NewProcessCacheEntry()
	exec.Pid = 2
	exec.PPid = 1
	exec.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, len(parent.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent]
	//     \ [child] -> exec
	resolver.AddEntry(exec.Pid, exec)
	assert.Equal(t, resolver.entryCache[exec.Pid], exec)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec.Parent, child)
	assert.Equal(t, exec.Parent.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// nothing
	resolver.DeleteEntry(exec.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestForkExecExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	exec1 := NewProcessCacheEntry()
	exec1.Pid = 2
	exec1.PPid = 1
	exec1.ExecTimestamp = time.Now()

	exec2 := NewProcessCacheEntry()
	exec2.Pid = 2
	exec2.PPid = 1
	exec2.ExecTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent]
	//     \ child
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, len(parent.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent]
	//     \ [child] -> exec1
	resolver.AddEntry(exec1.Pid, exec1)
	assert.Equal(t, resolver.entryCache[exec1.Pid], exec1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec1.Parent, child)
	assert.Equal(t, exec1.Parent.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// [parent]
	//     \ [child] -> [exec1] -> exec2
	resolver.AddEntry(exec2.Pid, exec2)
	assert.Equal(t, resolver.entryCache[exec2.Pid], exec2)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec2.Parent, exec1)
	assert.Equal(t, exec2.Parent.Parent, child)
	assert.Equal(t, exec2.Parent.Parent.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 4.0)

	// nothing
	resolver.DeleteEntry(exec2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestForkReuse(t *testing.T) {
	parent1 := NewProcessCacheEntry()
	parent1.Pid = 1
	parent1.ForkTimestamp = time.Now()

	child1 := NewProcessCacheEntry()
	child1.Pid = 2
	child1.PPid = 1
	child1.ForkTimestamp = time.Now()

	exec1 := NewProcessCacheEntry()
	exec1.Pid = 2
	exec1.PPid = 1
	exec1.ExecTimestamp = time.Now()

	parent2 := NewProcessCacheEntry()
	parent2.Pid = 1
	parent2.ForkTimestamp = time.Now()

	child2 := NewProcessCacheEntry()
	child2.Pid = 3
	child2.PPid = 1
	child2.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent1
	resolver.AddEntry(parent1.Pid, parent1)
	assert.Equal(t, resolver.entryCache[parent1.Pid], parent1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent1
	//     \ child1
	resolver.AddEntry(child1.Pid, child1)
	assert.Equal(t, resolver.entryCache[child1.Pid], child1)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child1.Parent, parent1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent1]
	//     \ child1
	resolver.DeleteEntry(parent1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent1.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child1.Parent, parent1)
	assert.Equal(t, len(parent1.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent1]
	//     \ [child1] -> exec1
	resolver.AddEntry(exec1.Pid, exec1)
	assert.Equal(t, resolver.entryCache[exec1.Pid], exec1)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, exec1.Parent, child1)
	assert.Equal(t, exec1.Parent.Parent, parent1)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	resolver.AddEntry(parent2.Pid, parent2)
	assert.Equal(t, resolver.entryCache[parent2.Pid], parent2)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, resolver.GetCacheSize(), 4.0)

	// [parent1:pid1]
	//     \ [child1] -> exec1
	//
	// parent2:pid1
	//     \ child2
	resolver.AddEntry(child2.Pid, child2)
	assert.Equal(t, resolver.entryCache[child2.Pid], child2)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, child2.Parent, parent2)
	assert.Equal(t, resolver.GetCacheSize(), 5.0)

	// parent2:pid1
	//     \ child2
	resolver.DeleteEntry(exec1.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[exec1.Pid])
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// [parent2:pid1]
	//     \ child2
	resolver.DeleteEntry(parent2.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent2.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, child2.Parent, parent2)
	assert.Equal(t, len(parent2.Children), 1)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// nothing
	resolver.DeleteEntry(child2.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}

func TestForkForkExec(t *testing.T) {
	parent := NewProcessCacheEntry()
	parent.Pid = 1
	parent.ForkTimestamp = time.Now()

	child := NewProcessCacheEntry()
	child.Pid = 2
	child.PPid = 1
	child.ForkTimestamp = time.Now()

	grandChild := NewProcessCacheEntry()
	grandChild.Pid = 3
	grandChild.PPid = child.Pid
	grandChild.ForkTimestamp = time.Now()

	childExec := NewProcessCacheEntry()
	childExec.Pid = child.Pid
	childExec.PPid = child.PPid
	childExec.ExecTimestamp = time.Now()

	grandChildExec := NewProcessCacheEntry()
	grandChildExec.Pid = grandChild.Pid
	grandChildExec.PPid = grandChild.PPid
	grandChildExec.ForkTimestamp = time.Now()

	resolver, err := NewProcessResolver(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// parent
	resolver.AddEntry(parent.Pid, parent)
	assert.Equal(t, resolver.entryCache[parent.Pid], parent)
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 1.0)

	// parent
	//     \ child
	resolver.AddEntry(child.Pid, child)
	assert.Equal(t, resolver.entryCache[child.Pid], child)
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, child.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 2.0)

	// parent
	//     \ child
	//          \ grandChild
	resolver.AddEntry(grandChild.Pid, grandChild)
	assert.Equal(t, resolver.entryCache[grandChild.Pid], grandChild)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, grandChild.Parent, child)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// parent
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.AddEntry(childExec.Pid, childExec)
	assert.Equal(t, resolver.entryCache[childExec.Pid], childExec)
	assert.Equal(t, len(resolver.entryCache), 3)
	assert.Equal(t, childExec.Parent, child)
	assert.Equal(t, childExec.Parent.Parent, parent)
	assert.Equal(t, resolver.GetCacheSize(), 4.0)

	// [parent]
	//     \ [child] -> childExec
	//          \ grandChild
	resolver.DeleteEntry(parent.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[parent.Pid])
	assert.Equal(t, len(resolver.entryCache), 2)
	assert.Equal(t, resolver.GetCacheSize(), 4.0)

	// [parent]
	//     \ [child]
	//          \ grandChild
	resolver.DeleteEntry(childExec.Pid, time.Now())
	assert.Nil(t, resolver.entryCache[childExec.Pid])
	assert.Equal(t, len(resolver.entryCache), 1)
	assert.Equal(t, resolver.GetCacheSize(), 3.0)

	// nothing
	resolver.DeleteEntry(grandChild.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Equal(t, resolver.GetCacheSize(), 0.0)
}
