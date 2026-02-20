// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process holds process related files
package process

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

type fakeEBPMap struct {
	data map[string]interface{}
}

func newFakeEBPMap() *fakeEBPMap {
	return &fakeEBPMap{
		data: make(map[string]interface{}),
	}
}

func (f *fakeEBPMap) marshal(i interface{}) ([]byte, error) {
	switch value := i.(type) {
	case []byte:
		return value, nil
	case uint32:
		return binary.NativeEndian.AppendUint32(make([]byte, 0, 4), value), nil
	case uint64:
		return binary.NativeEndian.AppendUint64(make([]byte, 0, 8), value), nil
	default:
		return nil, fmt.Errorf("unsupported type %T", value)
	}
}

func (f *fakeEBPMap) LookupBytes(key interface{}) ([]byte, error) {
	keyB, err := f.marshal(key)
	if err != nil {
		return nil, err
	}

	if value, ok := f.data[string(keyB)]; ok {
		if b, ok := value.([]byte); ok {
			return b, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeEBPMap) Put(key, value interface{}) error {
	keyB, err := f.marshal(key)
	if err != nil {
		return err
	}

	f.data[string(keyB)] = value
	return nil
}

func (f *fakeEBPMap) Delete(key interface{}) error {
	keyB, err := f.marshal(key)
	if err != nil {
		return err
	}

	delete(f.data, string(keyB))
	return nil
}

func newFakeForkEvent(ppid, pid int, inode uint64, resolver *EBPFResolver) *model.Event {
	e := model.NewFakeEvent()
	e.Type = uint32(model.ForkEventType)
	e.ProcessCacheEntry = resolver.NewProcessCacheEntry(model.PIDContext{Pid: uint32(pid), Tid: uint32(pid)})
	e.PIDContext = e.ProcessCacheEntry.PIDContext
	e.ProcessContext = &e.ProcessCacheEntry.ProcessContext
	e.ProcessCacheEntry.ForkTime = time.Now()
	e.ProcessCacheEntry.PPid = uint32(ppid)
	e.ProcessCacheEntry.Pid = uint32(pid)
	e.ProcessCacheEntry.FileEvent.Inode = inode
	e.ProcessCacheEntry.CGroup.CGroupID = "FakeCgroupID"
	e.ProcessCacheEntry.CGroup.CGroupPathKey.Inode = 4242
	return e
}

func newFakeExecEvent(ppid, pid int, inode uint64, resolver *EBPFResolver) *model.Event {
	e := model.NewFakeEvent()
	e.Type = uint32(model.ExecEventType)
	e.ProcessCacheEntry = resolver.NewProcessCacheEntry(model.PIDContext{Pid: uint32(pid), Tid: uint32(pid)})
	e.PIDContext = e.ProcessCacheEntry.PIDContext
	e.ProcessContext = &e.ProcessCacheEntry.ProcessContext
	e.ProcessCacheEntry.ExecTime = time.Now()
	e.ProcessCacheEntry.PPid = uint32(ppid)
	e.ProcessCacheEntry.Pid = uint32(pid)
	e.ProcessCacheEntry.FileEvent.Inode = inode
	e.ProcessCacheEntry.CGroup.CGroupID = "FakeCgroupID"
	e.ProcessCacheEntry.CGroup.CGroupPathKey.Inode = 4242
	return e
}

func exit(event *model.Event) {
	event.Type = uint32(model.ExitEventType)
}

func newResolver() (*EBPFResolver, error) {
	timeResolver, err := ktime.NewResolver()
	if err != nil {
		return nil, err
	}

	cgroupsResolver, err := cgroup.NewResolver(nil, nil, nil)
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := usergroup.NewResolver(cgroupsResolver)
	if err != nil {
		return nil, err
	}

	resolver, err := NewEBPFResolver(nil, &config.Config{}, &statsd.NoOpClient{}, nil, nil, nil, userGroupResolver, timeResolver, &path.NoOpResolver{}, nil, nil, NewResolverOpts())
	if err != nil {
		return nil, err
	}

	return resolver, nil
}

// assertChildrenConsistency verifies that each entry's Children list is
// consistent: every child in Children has this entry as its Ancestor.
func assertChildrenConsistency(t *testing.T, resolver *EBPFResolver) {
	t.Helper()

	for _, entry := range resolver.entryCache {
		for _, child := range entry.Children {
			assert.Equal(t, entry, child.Ancestor,
				"child %d's Ancestor should be %d", child.Pid, entry.Pid)
		}
		if entry.Ancestor != nil {
			found := false
			for _, sibling := range entry.Ancestor.Children {
				if sibling == entry {
					found = true
					break
				}
			}
			assert.True(t, found,
				"entry %d should be in its Ancestor %d's Children list", entry.Pid, entry.Ancestor.Pid)
		}
	}
}

func TestFork1st(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessCacheEntry.Pid, child.ResolveEventTime())

	assert.Nil(t, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessCacheEntry.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 0, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestFork2nd(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4)
	// Note: we use DeleteEntry directly (skipping ApplyExitEntry) because this
	// test uses fake PIDs that may collide with real kernel threads. ApplyExitEntry
	// calls reparentOrphanChildren which reads /proc and would corrupt the cache.
	// Subreaper reparenting is tested separately in TestSubreaperReparenting.
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessContext.Pid, child.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 0, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestForkExec(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	exec := newFakeExecEvent(3, 4, 456, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec, model.CGroupContext{})
	assert.Equal(t, exec.ProcessCacheEntry, resolver.entryCache[exec.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessContext.Pid, child.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestResolveFromProcfs(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}
	resolver.procCacheMap = newFakeEBPMap()
	resolver.pidCacheMap = newFakeEBPMap()
	resolver.inodeFileMap = newFakeEBPMap()

	// use self pid so that the procfs entry exists and we have the permissions to read it
	pid := os.Getpid()

	t.Run("sanitize-inode", func(t *testing.T) {
		entry := resolver.resolveFromProcfs(uint32(pid), 222, 1, func(pce *model.ProcessCacheEntry, _ error) {
			assert.Equal(t, uint64(222), pce.FileEvent.Inode)
			assert.True(t, pce.IsParentMissing)
		})
		assert.NotNil(t, entry)
	})
}

func TestOrphanExec(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	exec := newFakeExecEvent(3, 4, 456, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4) --> Y(pid:4)
	resolver.AddExecEntry(exec, model.CGroupContext{})
	assert.Equal(t, exec.ProcessCacheEntry, resolver.entryCache[exec.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(exec)
	resolver.ApplyExitEntry(exec, nil)
	resolver.DeleteEntry(exec.ProcessCacheEntry.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestForkExecExec(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	exec1 := newFakeExecEvent(3, 4, 456, resolver)
	exec2 := newFakeExecEvent(3, 4, 789, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1, model.CGroupContext{})
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4) -- Z(pid:4)
	resolver.AddExecEntry(exec2, model.CGroupContext{})
	assert.Equal(t, exec2.ProcessCacheEntry, resolver.entryCache[exec2.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, exec1.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor)
	assert.Equal(t, child.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache in the entryCache
	exit(exec2)
	resolver.ApplyExitEntry(exec2, nil)
	resolver.DeleteEntry(exec1.ProcessCacheEntry.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestForkReuse(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent1 := newFakeForkEvent(0, 3, 123, resolver)
	child1 := newFakeForkEvent(3, 4, 123, resolver)
	exec1 := newFakeExecEvent(3, 4, 456, resolver)
	parent2 := newFakeForkEvent(0, 3, 123, resolver)
	child2 := newFakeForkEvent(3, 5, 123, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent1, model.CGroupContext{}, nil)
	assert.Equal(t, parent1.ProcessCacheEntry, resolver.entryCache[parent1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child1, model.CGroupContext{}, nil)
	assert.Equal(t, child1.ProcessCacheEntry, resolver.entryCache[child1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent1.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent1)
	resolver.DeleteEntry(parent1.ProcessContext.Pid, parent1.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent1.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1, model.CGroupContext{})
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child1.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent1.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4)
	//
	// Z(pid:3)
	resolver.AddForkEntry(parent2, model.CGroupContext{}, nil)
	assert.Equal(t, parent2.ProcessCacheEntry, resolver.entryCache[parent2.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4)
	//
	// Z(pid:3)
	//    |
	// T(pid:5)
	resolver.AddForkEntry(child2, model.CGroupContext{}, nil)
	assert.Equal(t, child2.ProcessCacheEntry, resolver.entryCache[child2.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, parent2.ProcessCacheEntry, child2.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// Z(pid:3)
	//    |
	// T(pid:5)
	exit(exec1)
	resolver.ApplyExitEntry(exec1, nil)
	resolver.DeleteEntry(exec1.ProcessContext.Pid, exec1.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// [Z(pid:3)]
	//    |
	// T(pid:5)
	exit(parent2)
	resolver.DeleteEntry(parent2.ProcessContext.Pid, parent2.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent2.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent2.ProcessCacheEntry, child2.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(child2)
	resolver.ApplyExitEntry(child2, nil)
	resolver.DeleteEntry(child2.ProcessCacheEntry.Pid, child2.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestForkForkExec(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	grandChild := newFakeForkEvent(4, 5, 123, resolver)
	childExec := newFakeExecEvent(3, 4, 456, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	//    |
	// X(pid:5)
	resolver.AddForkEntry(grandChild, model.CGroupContext{}, nil)
	assert.Equal(t, grandChild.ProcessCacheEntry, resolver.entryCache[grandChild.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	//    |
	// X(pid:5)
	resolver.AddExecEntry(childExec, model.CGroupContext{})
	assert.Equal(t, childExec.ProcessCacheEntry, resolver.entryCache[childExec.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, childExec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, childExec.ProcessCacheEntry.Ancestor.Ancestor)
	assert.Equal(t, child.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [parent]
	//     \ [child] -> childExec
	//          \ grandChild

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	//    |
	// X(pid:5)
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4)
	//    |
	// X(pid:5)
	exit(childExec)
	resolver.DeleteEntry(childExec.ProcessContext.Pid, childExec.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[childExec.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(grandChild)
	resolver.ApplyExitEntry(grandChild, nil)
	resolver.DeleteEntry(grandChild.ProcessContext.Pid, grandChild.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestExecBomb(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	exec1 := newFakeExecEvent(3, 4, 456, resolver)
	exec2 := newFakeExecEvent(3, 4, 456, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4)
	exit(parent)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1, model.CGroupContext{})
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assertChildrenConsistency(t, resolver)

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4) -- Y(pid:4)
	exec2Pid := exec2.ProcessCacheEntry.Pid

	resolver.AddExecEntry(exec2, model.CGroupContext{})
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec2Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// nothing in the entryCache
	exit(exec1)
	resolver.ApplyExitEntry(exec1, nil)
	resolver.DeleteEntry(exec1.ProcessContext.Pid, exec1.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}

func TestExecLostFork(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	parent.ProcessCacheEntry.FileEvent.BasenameStr = "agent"
	child := newFakeForkEvent(3, 4, 123, resolver)
	child.PIDContext.ExecInode = 123 // ExecInode == Inode Parent
	child1 := newFakeForkEvent(4, 5, 123, resolver)
	child1.ProcessCacheEntry.FileEvent.BasenameStr = "sh"
	child1.PIDContext.ExecInode = 456 // ExecInode != Inode parent

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.Equal(t, "agent", child.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.False(t, child.ProcessCacheEntry.IsParentMissing)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	//   {|}
	// X(pid:5)
	resolver.AddForkEntry(child1, model.CGroupContext{}, nil)
	assert.Equal(t, "agent", child1.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.True(t, child1.ProcessCacheEntry.IsParentMissing)
	assertChildrenConsistency(t, resolver)
}

func TestExecLostExec(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	parent.ProcessCacheEntry.FileEvent.BasenameStr = "agent"
	child1 := newFakeForkEvent(3, 4, 123, resolver)
	child1.PIDContext.ExecInode = 123 // ExecInode == Inode Parent
	child2 := newFakeExecEvent(3, 4, 456, resolver)
	child2.ProcessCacheEntry.FileEvent.BasenameStr = "sh"
	child2.PIDContext.ExecInode = 456 // ExecInode != Inode Ancestor

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child1, model.CGroupContext{}, nil)
	assert.Equal(t, child1.ProcessCacheEntry, resolver.entryCache[child1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, "agent", child1.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.False(t, child1.ProcessCacheEntry.IsParentMissing)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -**- Y(pid:4)
	resolver.AddExecEntry(child2, model.CGroupContext{})
	assert.NotEqual(t, "agent", child2.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.True(t, child2.ProcessCacheEntry.IsParentMissing)
	assertChildrenConsistency(t, resolver)
}

func TestIsExecExecRuntime(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	child2 := newFakeExecEvent(3, 4, 456, resolver)
	child3 := newFakeExecEvent(3, 4, 789, resolver)
	child4 := newFakeExecEvent(3, 4, 101112, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, model.CGroupContext{}, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(child2, model.CGroupContext{})
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4)
	resolver.AddExecEntry(child3, model.CGroupContext{})
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4) -- T(pid:4)
	resolver.AddExecEntry(child4, model.CGroupContext{})
	assertChildrenConsistency(t, resolver)

	assert.False(t, parent.ProcessCacheEntry.IsExecExec)
	assert.False(t, parent.ProcessCacheEntry.IsExec)

	assert.False(t, child.ProcessCacheEntry.IsExecExec)
	assert.False(t, child.ProcessCacheEntry.IsExec)

	assert.False(t, child2.ProcessCacheEntry.IsExecExec)
	assert.True(t, child2.ProcessCacheEntry.IsExec)

	assert.True(t, child3.ProcessCacheEntry.IsExecExec)
	assert.True(t, child3.ProcessCacheEntry.IsExec)

	assert.True(t, child4.ProcessCacheEntry.IsExecExec)
	assert.True(t, child4.ProcessCacheEntry.IsExec)

}

func TestIsExecExecSnapshot(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)
	child2 := newFakeExecEvent(3, 4, 456, resolver)
	child3 := newFakeExecEvent(3, 4, 769, resolver)

	// X(pid:3)
	resolver.insertEntry(parent.ProcessCacheEntry, model.CGroupContext{}, model.ProcessCacheEntryFromSnapshot)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)

	// X(pid:3)
	//    |
	// X(pid:4)
	child.ProcessCacheEntry.SetForkParent(parent.ProcessCacheEntry)
	resolver.insertEntry(child.ProcessCacheEntry, model.CGroupContext{}, model.ProcessCacheEntryFromSnapshot)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	assert.False(t, parent.ProcessCacheEntry.IsExecExec)
	assert.False(t, parent.ProcessCacheEntry.IsExec)

	assert.False(t, child.ProcessCacheEntry.IsExecExec)
	assert.False(t, child.ProcessCacheEntry.IsExec)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(child2, model.CGroupContext{})
	assertChildrenConsistency(t, resolver)

	assert.False(t, child2.ProcessCacheEntry.IsExecExec)
	assert.True(t, child2.ProcessCacheEntry.IsExec)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4)
	resolver.AddExecEntry(child3, model.CGroupContext{})
	assertChildrenConsistency(t, resolver)

	assert.True(t, child3.ProcessCacheEntry.IsExecExec)
	assert.True(t, child3.ProcessCacheEntry.IsExec)
}

func TestSubreaperReparenting(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	// Use real PIDs so that procfs lookups succeed during reparenting.
	// The test process has a real PID and a real PPID that exist in /proc.
	realPid := uint32(os.Getpid())
	realPPid := uint32(os.Getppid())
	fakeParentPid := uint32(99999)

	// Build tree: grandparent(realPPid) -> fakeParent(99999) -> child(realPid)
	//
	// grandparent(pid:realPPid)
	//        |
	// fakeParent(pid:99999)
	//        |
	// child(pid:realPid)
	grandparent := newFakeForkEvent(0, int(realPPid), 100, resolver)
	fakeParent := newFakeForkEvent(int(realPPid), int(fakeParentPid), 100, resolver)
	child := newFakeForkEvent(int(fakeParentPid), int(realPid), 100, resolver)

	resolver.AddForkEntry(grandparent, model.CGroupContext{}, nil)
	resolver.AddForkEntry(fakeParent, model.CGroupContext{}, nil)
	resolver.AddForkEntry(child, model.CGroupContext{}, nil)

	// Verify initial tree structure
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, fakeParentPid, child.ProcessCacheEntry.PPid)
	assert.Equal(t, fakeParent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.Equal(t, grandparent.ProcessCacheEntry, fakeParent.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// Simulate fakeParent exiting with subreaper reparenting.
	// The kernel has already reparented child(realPid) to grandparent(realPPid).
	// tryReparentChildrenFromProcfs reads /proc/realPid/status which returns realPPid,
	// matching grandparent in the cache.
	resolver.Lock()
	resolver.tryReparentChildrenFromProcfs(fakeParent.ProcessCacheEntry, metrics.ReparentCallpathDoExit)
	resolver.deleteEntry(fakeParentPid, time.Now())
	resolver.Unlock()

	// child should now be reparented to grandparent
	//
	// grandparent(pid:realPPid)
	//        |
	// child(pid:realPid)
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, realPPid, child.ProcessCacheEntry.PPid)
	assert.Equal(t, grandparent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assertChildrenConsistency(t, resolver)

	// Clean up remaining entries
	resolver.DeleteEntry(realPid, time.Now())
	assertChildrenConsistency(t, resolver)

	resolver.DeleteEntry(realPPid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assertChildrenConsistency(t, resolver)
}
