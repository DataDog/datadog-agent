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

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
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

	cgroupsResolver, err := cgroup.NewResolver(nil)
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := usergroup.NewResolver(cgroupsResolver)
	if err != nil {
		return nil, err
	}

	containerResolver := container.New()

	resolver, err := NewEBPFResolver(nil, &config.Config{}, &statsd.NoOpClient{}, nil, containerResolver, nil, cgroupsResolver, userGroupResolver, timeResolver, &path.NoOpResolver{}, nil, NewResolverOpts())
	if err != nil {
		return nil, err
	}

	return resolver, nil
}

func TestFork1st(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessCacheEntry.Pid, child.ResolveEventTime())

	assert.Nil(t, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessCacheEntry.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 0, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
}

func TestFork2nd(t *testing.T) {

	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	parent := newFakeForkEvent(0, 3, 123, resolver)
	child := newFakeForkEvent(3, 4, 123, resolver)

	// X(pid:3)
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessContext.Pid, child.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 0, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec)
	assert.Equal(t, exec.ProcessCacheEntry, resolver.entryCache[exec.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(child)
	resolver.ApplyExitEntry(child, nil)
	resolver.DeleteEntry(child.ProcessContext.Pid, child.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4) --> Y(pid:4)
	resolver.AddExecEntry(exec)
	assert.Equal(t, exec.ProcessCacheEntry, resolver.entryCache[exec.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(exec)
	resolver.ApplyExitEntry(exec, nil)
	resolver.DeleteEntry(exec.ProcessCacheEntry.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1)
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4) -- Z(pid:4)
	resolver.AddExecEntry(exec2)
	assert.Equal(t, exec2.ProcessCacheEntry, resolver.entryCache[exec2.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, exec1.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor)
	assert.Equal(t, child.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec2.ProcessCacheEntry.Ancestor.Ancestor.Ancestor)
	assert.EqualValues(t, 4, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache in the entryCache
	exit(exec2)
	resolver.ApplyExitEntry(exec2, nil)
	resolver.DeleteEntry(exec1.ProcessCacheEntry.Pid, time.Now())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent1, nil)
	assert.Equal(t, parent1.ProcessCacheEntry, resolver.entryCache[parent1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child1, nil)
	assert.Equal(t, child1.ProcessCacheEntry, resolver.entryCache[child1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent1.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4)
	exit(parent1)
	resolver.ApplyExitEntry(parent1, nil)
	resolver.DeleteEntry(parent1.ProcessContext.Pid, parent1.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent1.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1)
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child1.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent1.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4)
	//
	// Z(pid:3)
	resolver.AddForkEntry(parent2, nil)
	assert.Equal(t, parent2.ProcessCacheEntry, resolver.entryCache[parent2.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.EqualValues(t, 4, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	//  X(pid:4) -- Y(pid:4)
	//
	// Z(pid:3)
	//    |
	// T(pid:5)
	resolver.AddForkEntry(child2, nil)
	assert.Equal(t, child2.ProcessCacheEntry, resolver.entryCache[child2.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, parent2.ProcessCacheEntry, child2.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 5, resolver.processCacheEntryCount.Load())

	// Z(pid:3)
	//    |
	// T(pid:5)
	exit(exec1)
	resolver.ApplyExitEntry(exec1, nil)
	resolver.DeleteEntry(exec1.ProcessContext.Pid, exec1.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [Z(pid:3)]
	//    |
	// T(pid:5)
	exit(parent2)
	resolver.ApplyExitEntry(parent2, nil)
	resolver.DeleteEntry(parent2.ProcessContext.Pid, parent2.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent2.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent2.ProcessCacheEntry, child2.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(child2)
	resolver.ApplyExitEntry(child2, nil)
	resolver.DeleteEntry(child2.ProcessCacheEntry.Pid, child2.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	//    |
	// X(pid:5)
	resolver.AddForkEntry(grandChild, nil)
	assert.Equal(t, grandChild.ProcessCacheEntry, resolver.entryCache[grandChild.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	//    |
	// X(pid:5)
	resolver.AddExecEntry(childExec)
	assert.Equal(t, childExec.ProcessCacheEntry, resolver.entryCache[childExec.ProcessCacheEntry.Pid])
	assert.Equal(t, 3, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, childExec.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, childExec.ProcessCacheEntry.Ancestor.Ancestor)
	assert.Equal(t, child.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, grandChild.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 4, resolver.processCacheEntryCount.Load())

	// [parent]
	//     \ [child] -> childExec
	//          \ grandChild

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	//    |
	// X(pid:5)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.EqualValues(t, 4, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4)
	//    |
	// X(pid:5)
	exit(childExec)
	resolver.ApplyExitEntry(childExec, nil)
	resolver.DeleteEntry(childExec.ProcessContext.Pid, childExec.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[childExec.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(grandChild)
	resolver.ApplyExitEntry(grandChild, nil)
	resolver.DeleteEntry(grandChild.ProcessContext.Pid, grandChild.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 1, resolver.processCacheEntryCount.Load())

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4)
	exit(parent)
	resolver.ApplyExitEntry(parent, nil)
	resolver.DeleteEntry(parent.ProcessContext.Pid, parent.ResolveEventTime())
	assert.Nil(t, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.EqualValues(t, 2, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(exec1)
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec1.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.Equal(t, child.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, parent.ProcessCacheEntry, exec1.ProcessCacheEntry.Ancestor.Ancestor)
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// [X(pid:3)]
	//    |
	// X(pid:4) -- Y(pid:4) -- Y(pid:4)
	exec2Pid := exec2.ProcessCacheEntry.Pid

	resolver.AddExecEntry(exec2)
	assert.Equal(t, exec1.ProcessCacheEntry, resolver.entryCache[exec2Pid])
	assert.Equal(t, 1, len(resolver.entryCache))
	assert.EqualValues(t, 3, resolver.processCacheEntryCount.Load())

	// nothing in the entryCache
	exit(exec1)
	resolver.ApplyExitEntry(exec1, nil)
	resolver.DeleteEntry(exec1.ProcessContext.Pid, exec1.ResolveEventTime())
	assert.Zero(t, len(resolver.entryCache))
	assert.Zero(t, resolver.processCacheEntryCount.Load())
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)
	assert.Equal(t, "agent", child.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.False(t, child.ProcessCacheEntry.IsParentMissing)

	// X(pid:3)
	//    |
	// X(pid:4)
	//   {|}
	// X(pid:5)
	resolver.AddForkEntry(child1, nil)
	assert.Equal(t, "agent", child1.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.True(t, child1.ProcessCacheEntry.IsParentMissing)
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child1, nil)
	assert.Equal(t, child1.ProcessCacheEntry, resolver.entryCache[child1.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child1.ProcessCacheEntry.Ancestor)
	assert.Equal(t, "agent", child1.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.False(t, child1.ProcessCacheEntry.IsParentMissing)

	// X(pid:3)
	//    |
	// X(pid:4) -**- Y(pid:4)
	resolver.AddExecEntry(child2)
	assert.NotEqual(t, "agent", child2.ProcessCacheEntry.FileEvent.BasenameStr)
	assert.True(t, child2.ProcessCacheEntry.IsParentMissing)
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
	resolver.AddForkEntry(parent, nil)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.AddForkEntry(child, nil)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(child2)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4)
	resolver.AddExecEntry(child3)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4) -- T(pid:4)
	resolver.AddExecEntry(child4)

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
	resolver.insertEntry(parent.ProcessCacheEntry, model.ProcessCacheEntryFromSnapshot)
	assert.Equal(t, parent.ProcessCacheEntry, resolver.entryCache[parent.ProcessCacheEntry.Pid])
	assert.Equal(t, 1, len(resolver.entryCache))

	// X(pid:3)
	//    |
	// X(pid:4)
	resolver.setAncestor(child.ProcessCacheEntry)
	resolver.insertEntry(child.ProcessCacheEntry, model.ProcessCacheEntryFromSnapshot)
	assert.Equal(t, child.ProcessCacheEntry, resolver.entryCache[child.ProcessCacheEntry.Pid])
	assert.Equal(t, 2, len(resolver.entryCache))
	assert.Equal(t, parent.ProcessCacheEntry, child.ProcessCacheEntry.Ancestor)

	assert.False(t, parent.ProcessCacheEntry.IsExecExec)
	assert.False(t, parent.ProcessCacheEntry.IsExec)

	assert.False(t, child.ProcessCacheEntry.IsExecExec)
	assert.False(t, child.ProcessCacheEntry.IsExec)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)
	resolver.AddExecEntry(child2)

	assert.False(t, child2.ProcessCacheEntry.IsExecExec)
	assert.True(t, child2.ProcessCacheEntry.IsExec)

	// X(pid:3)
	//    |
	// X(pid:4) -- Y(pid:4)  -- Z(pid:4)
	resolver.AddExecEntry(child3)

	assert.True(t, child3.ProcessCacheEntry.IsExecExec)
	assert.True(t, child3.ProcessCacheEntry.IsExec)
}

func TestCGroupContext(t *testing.T) {
	resolver, err := newResolver()
	if err != nil {
		t.Fatal()
	}

	t.Run("unknown-container-runtime", func(t *testing.T) {
		node := newFakeForkEvent(0, 3, 123, resolver)
		resolver.insertEntry(node.ProcessCacheEntry, model.ProcessCacheEntryFromEvent)

		const (
			containerID = containerutils.ContainerID("09d3f62464b8761e9106350bacc609deb0dc639403888bdf2112033cb30e1bf6")
			cgroupID    = containerutils.CGroupID("/kubepods/besteffort/pod8bbdd97b-f902-4e16-8235-4ac18307cef6/" + string(containerID))
		)

		resolver.UpdateProcessCGroupContext(node.ProcessCacheEntry.Pid, &model.CGroupContext{
			CGroupID:    cgroupID,
			CGroupFlags: 0,
		}, nil)

		assert.Equal(t, cgroupID, node.ProcessCacheEntry.CGroup.CGroupID)
		assert.Equal(t, containerutils.CGroupManagerCRI, node.ProcessCacheEntry.CGroup.CGroupFlags.GetCGroupManager())
		assert.Equal(t, containerID, node.ProcessCacheEntry.ContainerID)
	})
}
