// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func newProcessContextRetryTestCtx() (*CWSPtracerCtx, *Process, *int) {
	root := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	ctx := newProcessContextTestCtx(root)
	ctx.msgDataChan = make(chan []byte, 1)
	ctx.msgDataChan <- []byte("occupied")
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx.processCache.Add(child.Pid, child)
	lookups := new(int)
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		(*lookups)++
		snapshot := testProcessContext(identity.tgid, 10, identity.startTime)
		snapshot.StartTime = identity.startTime
		return snapshot, nil
	}
	return ctx, child, lookups
}

func TestProcessContextDroppedSnapshotRetriedWithoutLookup(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial"}[partial], func(t *testing.T) {
			ctx, child, lookups := newProcessContextRetryTestCtx()
			source := testProcessContext(20, 10, 200)
			source.StartTime = 200
			if partial {
				source.Exec.Credentials = nil
			}
			ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
				(*lookups)++
				if partial {
					return source, os.ErrPermission
				}
				return source, nil
			}
			open := contextTestOpen()
			ctx.sendSyscallMsg(child, open)
			ctx.sendSyscallMsg(child, open) // repeated drops do not repeat procfs I/O
			state := ctx.lifecycleGuard.states[child.identity()]
			require.NotNil(t, state)
			require.NotNil(t, state.pendingContext)
			require.False(t, state.contextAnnounced)
			require.Equal(t, 1, *lookups)

			// Retained metadata must own its strings, slices and credentials,
			// not aliases into a reused reader result or a syscall message.
			source.Exec.Args[0] = "mutated"
			source.ContainerID = "mutated"
			if source.Exec.Credentials != nil {
				source.Exec.Credentials.UID = 999
			}
			<-ctx.msgDataChan
			thread := *child
			thread.Pid = 21
			ctx.sendSyscallMsg(&thread, open)
			message := receiveContextTestSyscall(t, ctx)
			require.NotNil(t, message.Exec)
			assert.Equal(t, ebpfless.SyscallTypeOpen, message.Type)
			assert.True(t, message.Exec.FromProcFS)
			assert.EqualValues(t, 20, message.PID)
			assert.EqualValues(t, 200, message.StartTime)
			assert.EqualValues(t, 10, message.Exec.PPID)
			assert.Equal(t, "/worker", message.Exec.File.Filename)
			assert.Equal(t, []string{"worker", "--worker"}, message.Exec.Args)
			assert.EqualValues(t, "worker-container", message.ContainerID)
			if partial {
				assert.Nil(t, message.Exec.Credentials)
			} else {
				require.NotNil(t, message.Exec.Credentials)
				assert.EqualValues(t, 33, message.Exec.Credentials.UID)
			}
			assert.Equal(t, !partial, state.contextAnnounced)
			assert.Nil(t, state.pendingContext)
			assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
			assert.Zero(t, ctx.lifecycleGuard.pendingContexts.Len())
			ctx.sendSyscallMsg(child, open)
			assert.Nil(t, receiveContextTestSyscall(t, ctx).Exec)
			ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
			assert.Empty(t, ctx.msgDataChan) // still suppress the late creation event
			assert.Equal(t, 1, *lookups)
		})
	}
}

func TestProcessContextPendingCredentialsFollowEvents(t *testing.T) {
	ctx, child, lookups := newProcessContextRetryTestCtx()
	reader := ctx.processContextReader
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		snapshot, err := reader(identity)
		snapshot.Exec.File.Credentials = &ebpfless.Credentials{UID: 7, GID: 8}
		return snapshot, err
	}
	ctx.sendSyscallMsg(child, contextTestOpen())
	for _, event := range []*ebpfless.SyscallMsg{
		{Type: ebpfless.SyscallTypeSetUID, SetUID: &ebpfless.SetUIDSyscallMsg{UID: -1, EUID: 1001, EUser: "effective-user"}},
		{Type: ebpfless.SyscallTypeSetGID, SetGID: &ebpfless.SetGIDSyscallMsg{GID: 2002, EGID: -1, Group: "group"}},
		{Type: ebpfless.SyscallTypeSetUID, SetUID: &ebpfless.SetUIDSyscallMsg{UID: 3003, EUID: -1, User: "user"}},
		{Type: ebpfless.SyscallTypeSetGID, SetGID: &ebpfless.SetGIDSyscallMsg{GID: -1, EGID: 4004, EGroup: "effective-group"}},
		{Type: ebpfless.SyscallTypeSetUID, Retval: -int64(syscall.EPERM), SetUID: &ebpfless.SetUIDSyscallMsg{UID: 999, EUID: 999}},
		{Type: ebpfless.SyscallTypeSetGID, Retval: -int64(syscall.EPERM), SetGID: &ebpfless.SetGIDSyscallMsg{GID: 999, EGID: 999}},
	} {
		ctx.sendSyscallMsg(child, event)
	}
	<-ctx.msgDataChan
	ctx.sendSyscallMsg(child, contextTestOpen())
	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.Equal(t, &ebpfless.Credentials{
		UID: 3003, EUID: 1001, GID: 2002, EGID: 4004,
		User: "user", EUser: "effective-user", Group: "group", EGroup: "effective-group",
	}, message.Exec.Credentials)
	require.NotNil(t, message.Exec.File.Credentials)
	assert.EqualValues(t, 7, message.Exec.File.Credentials.UID)
	assert.EqualValues(t, 8, message.Exec.File.Credentials.GID)
	assert.Equal(t, 1, *lookups)
	assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
}

func TestProcessContextPendingExecReplacesImage(t *testing.T) {
	for _, seedOpen := range []bool{false, true} {
		t.Run(map[bool]string{false: "first event exec", true: "exec after dropped open"}[seedOpen], func(t *testing.T) {
			ctx, child, lookups := newProcessContextRetryTestCtx()
			if seedOpen {
				ctx.sendSyscallMsg(child, contextTestOpen())
			}
			child.ContainerID, child.CGroupID = "replacement-container", "replacement-cgroup"
			exec := &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{
				File: ebpfless.FileSyscallMsg{Filename: "/replacement"},
				Args: []string{"replacement", "--new"}, Envs: []string{"NEW=1"},
				Credentials: &ebpfless.Credentials{UID: 99, User: "replacement-user"},
			}}
			ctx.sendSyscallMsg(child, exec)
			assert.False(t, exec.Exec.FromProcFS)
			assert.Zero(t, exec.Exec.CreateTime)
			exec.Exec.Args[0] = "mutated"
			exec.Exec.Credentials.UID = 999
			ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{
				Type: ebpfless.SyscallTypeExec, Retval: -int64(syscall.EACCES),
				Exec: &ebpfless.ExecSyscallMsg{File: ebpfless.FileSyscallMsg{Filename: "/denied"}},
			})
			<-ctx.msgDataChan
			ctx.sendSyscallMsg(child, contextTestOpen())
			message := receiveContextTestSyscall(t, ctx)
			require.NotNil(t, message.Exec)
			assert.Equal(t, ebpfless.SyscallTypeOpen, message.Type) // no replayed or synthetic exec
			assert.True(t, message.Exec.FromProcFS)
			assert.Equal(t, "/replacement", message.Exec.File.Filename)
			assert.Equal(t, []string{"replacement", "--new"}, message.Exec.Args)
			assert.Equal(t, []string{"NEW=1"}, message.Exec.Envs)
			assert.EqualValues(t, 200, message.Exec.CreateTime)
			assert.EqualValues(t, 10, message.Exec.PPID)
			assert.EqualValues(t, "replacement-container", message.ContainerID)
			assert.EqualValues(t, "replacement-cgroup", message.CGroupID)
			require.NotNil(t, message.Exec.Credentials)
			assert.EqualValues(t, 99, message.Exec.Credentials.UID)
			assert.Equal(t, "replacement-user", message.Exec.Credentials.User)
			assert.Equal(t, 1, *lookups)
			assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
		})
	}
}

func TestProcessContextPendingSnapshotClearedByExec(t *testing.T) {
	ctx, child, lookups := newProcessContextRetryTestCtx()
	ctx.sendSyscallMsg(child, contextTestOpen())
	<-ctx.msgDataChan
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{
		File: ebpfless.FileSyscallMsg{Filename: "/replacement"},
	}})
	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	require.NotNil(t, message.Exec.Credentials)
	assert.Equal(t, ebpfless.SyscallTypeExec, message.Type)
	assert.False(t, message.Exec.FromProcFS)
	assert.Equal(t, "/replacement", message.Exec.File.Filename)
	assert.EqualValues(t, 33, message.Exec.Credentials.UID)
	assert.Nil(t, ctx.lifecycleGuard.states[child.identity()].pendingContext)
	assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
	ctx.sendSyscallMsg(child, contextTestOpen())
	assert.Nil(t, receiveContextTestSyscall(t, ctx).Exec)
	assert.Equal(t, 1, *lookups)
}

func TestProcessContextDroppedParentSnapshotRetriedOnNextFork(t *testing.T) {
	ctx, parent, lookups := newProcessContextRetryTestCtx()
	for _, pid := range []int{30, 31} {
		child := &Process{Pid: pid, Tgid: pid, startTime: uint64(pid * 10), containerContextResolved: true}
		ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 20}})
		if pid == 30 {
			require.NotNil(t, ctx.lifecycleGuard.states[parent.identity()].pendingContext)
			<-ctx.msgDataChan
		}
	}
	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.Equal(t, ebpfless.SyscallTypeFork, message.Type)
	assert.EqualValues(t, 31, message.PID) // only metadata survives; never replay the lost fork of PID 30
	assert.EqualValues(t, 310, message.StartTime)
	assert.EqualValues(t, 20, message.Fork.PPID)
	assert.EqualValues(t, 200, message.Fork.ParentStartTime)
	assert.EqualValues(t, "worker-container", message.Fork.ParentContainerID)
	assert.EqualValues(t, 10, message.Exec.PPID)
	assert.Nil(t, ctx.lifecycleGuard.states[parent.identity()].pendingContext)
	assert.Equal(t, 1, *lookups)
	assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
}

func TestProcessContextPendingSnapshotCleanup(t *testing.T) {
	for _, action := range []string{"exit", "PID reuse", "history eviction"} {
		t.Run(action, func(t *testing.T) {
			ctx, child, lookups := newProcessContextRetryTestCtx()
			ctx.sendSyscallMsg(child, contextTestOpen())
			state := ctx.lifecycleGuard.states[child.identity()]
			require.NotNil(t, state)
			require.NotNil(t, state.pendingContext)
			switch action {
			case "exit":
				ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExit, Exit: &ebpfless.ExitSyscallMsg{}})
				assert.True(t, state.exited)
			case "PID reuse":
				reused := *child
				reused.startTime++
				ctx.sendSyscallMsg(&reused, contextTestOpen())
				assert.Nil(t, state.pendingContext)
				<-ctx.msgDataChan
				ctx.sendSyscallMsg(&reused, contextTestOpen())
				message := receiveContextTestSyscall(t, ctx)
				require.NotNil(t, message.Exec)
				assert.EqualValues(t, 201, message.StartTime)
				assert.EqualValues(t, 201, message.Exec.CreateTime)
				ctx.sendSyscallMsg(child, contextTestOpen())
				assert.Empty(t, ctx.msgDataChan)
				assert.Equal(t, 2, *lookups)
			case "history eviction":
				ctx.lifecycleGuard.evict(state)
				ctx.sendSyscallMsg(child, contextTestOpen())
				assert.Equal(t, 1, *lookups) // never retry procfs after bounded history eviction
			}
			assert.Nil(t, state.pendingContext)
			assert.Zero(t, ctx.lifecycleGuard.pendingContexts.Len())
			assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
		})
	}
}

func TestProcessContextPendingCacheBound(t *testing.T) {
	guard := newProcessLifecycleGuard(nil, func([]byte) bool { return false })
	largeArg := strings.Repeat("a", ebpfless.MaxMessageSize/2)
	snapshot := testProcessContext(1000, 10, 1000)
	snapshot.StartTime, snapshot.Exec.Args = 1000, []string{largeArg}
	data, err := msgpack.Marshal(snapshot)
	require.NoError(t, err)
	count := maxPendingContextBytes/len(data) + 1
	var last processIdentity
	for i := range count {
		last = processIdentity{tgid: uint32(1000 + i), startTime: uint64(1000 + i)}
		require.True(t, guard.beginProcessContext(last))
		snapshot := testProcessContext(last.tgid, 10, last.startTime)
		snapshot.StartTime, snapshot.Exec.Args = last.startTime, []string{largeArg}
		guard.retainProcessContext(last, snapshot)
		assert.LessOrEqual(t, guard.pendingContextBytes, maxPendingContextBytes)
	}
	first := processIdentity{tgid: 1000, startTime: 1000}
	assert.Nil(t, guard.states[first].pendingContext)
	assert.False(t, guard.beginProcessContext(first)) // cache eviction must not cause another lookup
	assert.NotNil(t, guard.states[last].pendingContext)
	assert.Positive(t, guard.contextCacheEvictions)
	for guard.pendingContexts.Len() != 0 {
		guard.forget(guard.pendingContexts.Front().Value.(*processLifecycleState))
	}
	assert.Zero(t, guard.pendingContextBytes)
}

func TestProcessContextPendingCacheBoundsSnapshot(t *testing.T) {
	for _, field := range []string{"args", "filename"} {
		t.Run(field, func(t *testing.T) {
			guard := newProcessLifecycleGuard(nil, func([]byte) bool { return false })
			identity := processIdentity{tgid: 20, startTime: 200}
			require.True(t, guard.beginProcessContext(identity))
			snapshot := testProcessContext(20, 10, 200)
			guard.retainProcessContext(identity, snapshot)
			require.NotNil(t, guard.states[identity].pendingContext)
			if field == "args" {
				snapshot.Exec.Args = []string{strings.Repeat("x", ebpfless.MaxMessageSize)}
			} else {
				snapshot.Exec.File.Filename = strings.Repeat("x", ebpfless.MaxMessageSize)
			}
			guard.retainProcessContext(identity, snapshot)
			retained := guard.states[identity].pendingContext
			if field == "args" {
				require.NotNil(t, retained)
				assert.Nil(t, retained.Exec.Args)
				assert.Nil(t, retained.Exec.Envs)
				assert.True(t, retained.Exec.ArgsTruncated)
				assert.True(t, retained.Exec.EnvsTruncated)
				assert.LessOrEqual(t, guard.pendingContextBytes, ebpfless.MaxMessageSize)
			} else {
				assert.Nil(t, retained) // never retain the old image if its replacement cannot fit
				assert.Zero(t, guard.pendingContextBytes)
			}
			assert.False(t, guard.beginProcessContext(identity))
		})
	}
}

func TestProcessContextUnchangedCredentialsReuseSnapshot(t *testing.T) {
	snapshot := testProcessContext(20, 10, 200)
	for _, event := range []*ebpfless.SyscallMsg{
		{Type: ebpfless.SyscallTypeSetUID, SetUID: &ebpfless.SetUIDSyscallMsg{UID: -1, EUID: -1}},
		{Type: ebpfless.SyscallTypeSetUID, SetUID: &ebpfless.SetUIDSyscallMsg{UID: 33, EUID: 33}},
		{Type: ebpfless.SyscallTypeSetGID, SetGID: &ebpfless.SetGIDSyscallMsg{GID: -1, EGID: -1}},
		{Type: ebpfless.SyscallTypeSetGID, SetGID: &ebpfless.SetGIDSyscallMsg{GID: 34, EGID: 34}},
	} {
		assert.Same(t, snapshot, processContextAfterEvent(snapshot, event))
	}
}

func TestProcessContextRetainsSnapshotWhenFrameOmitsMetadata(t *testing.T) {
	ctx, child, lookups := newProcessContextRetryTestCtx()
	ctx.sendSyscallMsg(child, contextTestOpen())
	<-ctx.msgDataChan
	open := contextTestOpen()
	open.PID, open.StartTime, open.Timestamp = 20, 200, ^uint64(0)
	open.ContainerID, open.CGroupID = "worker-container", "worker-cgroup"
	open.Open.Filename = strings.Repeat("x", ebpfless.MaxMessageSize)
	base := &ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: open}
	data, err := msgpack.Marshal(base)
	require.NoError(t, err)
	open.Open.Filename = open.Open.Filename[:len(open.Open.Filename)-(len(data)-ebpfless.MaxMessageSize)]
	data, err = msgpack.Marshal(base)
	require.NoError(t, err)
	require.Len(t, data, ebpfless.MaxMessageSize)
	ctx.sendSyscallMsg(child, open)
	message := receiveContextTestSyscall(t, ctx)
	assert.Nil(t, message.Exec) // only the base event fit, despite successful enqueueing
	assert.False(t, ctx.lifecycleGuard.states[child.identity()].contextAnnounced)
	require.NotNil(t, ctx.lifecycleGuard.states[child.identity()].pendingContext)
	ctx.sendSyscallMsg(child, contextTestOpen())
	message = receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.Equal(t, []string{"worker", "--worker"}, message.Exec.Args)
	assert.Equal(t, 1, *lookups)
	assert.Zero(t, ctx.lifecycleGuard.pendingContextBytes)
}
