// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func testProcessContext(pid, ppid uint32, birth uint64) *ebpfless.SyscallMsg {
	return &ebpfless.SyscallMsg{
		Type: ebpfless.SyscallTypeExec, PID: pid, ContainerID: "worker-container", CGroupID: "worker-cgroup",
		Exec: &ebpfless.ExecSyscallMsg{
			FromProcFS: true, CreateTime: birth, PPID: ppid,
			File: ebpfless.FileSyscallMsg{Filename: "/worker"},
			Args: []string{"worker", "--worker"}, Envs: []string{"WORKER=1"}, TTY: "pts1",
			Credentials: &ebpfless.Credentials{UID: 33, EUID: 33, GID: 34, EGID: 34},
		},
	}
}

func newProcessContextTestCtx(roots ...*Process) *CWSPtracerCtx {
	ctx := &CWSPtracerCtx{
		opts: &Opts{StatsDisabled: true}, processCache: NewProcessCache(), msgDataChan: make(chan []byte, 16),
	}
	var initial []processIdentity
	for _, root := range roots {
		ctx.processCache.Add(root.Pid, root)
		initial = append(initial, root.identity())
	}
	ctx.lifecycleGuard = newProcessLifecycleGuard(initial, ctx.enqueueMsgData)
	return ctx
}

func receiveContextTestSyscall(t *testing.T, ctx *CWSPtracerCtx) *ebpfless.SyscallMsg {
	t.Helper()
	select {
	case data := <-ctx.msgDataChan:
		var message ebpfless.Message
		require.NoError(t, msgpack.Unmarshal(data, &message))
		require.NotNil(t, message.Syscall)
		return message.Syscall
	default:
		t.Fatal("expected an enqueued syscall message")
		return nil
	}
}

func contextTestOpen() *ebpfless.SyscallMsg {
	return &ebpfless.SyscallMsg{
		Type: ebpfless.SyscallTypeOpen,
		Open: &ebpfless.OpenSyscallMsg{FileSyscallMsg: ebpfless.FileSyscallMsg{Filename: "/first-file"}},
	}
}

func TestProcExecutableName(t *testing.T) {
	for filename, expected := range map[string]string{
		"/usr/bin/worker":                   "/usr/bin/worker",
		"/usr/bin/worker (deleted)":         "/usr/bin/worker (deleted)", // retain existing procfs path semantics
		"/memfd:worker (deleted)":           "memfd:worker",
		"/memfd:worker (deleted) (deleted)": "memfd:worker (deleted)",
	} {
		t.Run(filename, func(t *testing.T) {
			assert.Equal(t, expected, procExecutableName(filename))
		})
	}
}

func TestCollectProcessContext(t *testing.T) {
	process := NewProcess(os.Getpid())
	ctx := &CWSPtracerCtx{}
	snapshot, err := ctx.collectProcessContext(process.identity())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Exec)
	require.NotNil(t, snapshot.Exec.Credentials)
	exe, err := os.Executable()
	require.NoError(t, err)
	assert.EqualValues(t, os.Getpid(), snapshot.PID)
	assert.Equal(t, process.startTime, snapshot.StartTime)
	assert.EqualValues(t, os.Getppid(), snapshot.Exec.PPID)
	assert.Equal(t, exe, snapshot.Exec.File.Filename)
	assert.EqualValues(t, os.Getuid(), snapshot.Exec.Credentials.UID)
	assert.EqualValues(t, os.Geteuid(), snapshot.Exec.Credentials.EUID)
	assert.EqualValues(t, os.Getgid(), snapshot.Exec.Credentials.GID)
	assert.EqualValues(t, os.Getegid(), snapshot.Exec.Credentials.EGID)
	assert.NotZero(t, snapshot.Exec.CreateTime)

	// Missing cached birth metadata is recoverable, but a known mismatching
	// generation must never borrow the currently live PID's context.
	_, err = ctx.collectProcessContext(processIdentity{tgid: uint32(os.Getpid())})
	require.NoError(t, err)
	stale := process.identity()
	stale.startTime++
	snapshot, err = ctx.collectProcessContext(stale)
	require.Error(t, err)
	assert.Nil(t, snapshot)
	snapshot, err = ctx.collectProcessContext(processIdentity{tgid: 1<<31 - 1})
	require.Error(t, err)
	assert.Nil(t, snapshot)
}

func TestProcessContextFirstEventFromThread(t *testing.T) {
	ctx := newProcessContextTestCtx()
	first := &Process{Pid: 21, Tgid: 20, startTime: 200, containerContextResolved: true}
	second := &Process{Pid: 22, Tgid: 20, startTime: 200, containerContextResolved: true}
	snapshot := testProcessContext(20, 10, 200)
	lookups := 0
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		lookups++
		require.Equal(t, first.identity(), identity)
		return snapshot, nil
	}
	open := contextTestOpen()
	ctx.sendSyscallMsg(first, open)
	// The optional Exec is metadata, not an exec event, and is frozen with open.
	snapshot.Exec.Args[0] = "mutated"
	open.Open.Filename = "/second-file"
	ctx.sendSyscallMsg(second, open)
	ctx.sendSyscallMsg(second, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{}})

	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.True(t, message.Exec.FromProcFS)
	assert.EqualValues(t, 20, message.PID)
	assert.EqualValues(t, 200, message.StartTime)
	assert.EqualValues(t, 10, message.Exec.PPID)
	assert.Equal(t, []string{"worker", "--worker"}, message.Exec.Args)
	assert.EqualValues(t, "worker-container", message.ContainerID)
	assert.EqualValues(t, "worker-cgroup", message.CGroupID)
	assert.Equal(t, "/first-file", message.Open.Filename)
	message = receiveContextTestSyscall(t, ctx)
	assert.Nil(t, message.Exec)
	assert.Equal(t, "/second-file", message.Open.Filename)
	message = receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.False(t, message.Exec.FromProcFS)
	assert.EqualValues(t, 10, message.Exec.PPID) // remembered by TGID, not the first TID
	assert.Equal(t, 1, lookups)
	assert.Len(t, ctx.lifecycleGuard.states, 1)
	assert.Empty(t, ctx.msgDataChan) // no extra exec or separate snapshot event
}

func TestProcessContextKnownForkAndThreadFastPath(t *testing.T) {
	root := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	ctx := newProcessContextTestCtx(root)
	ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
		t.Fatal("known process context must not require procfs")
		return nil, nil
	}
	thread := &Process{Pid: 11, Tgid: 10, startTime: 100, containerContextResolved: true}
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx.sendSyscallMsg(thread, contextTestOpen())
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	ctx.sendSyscallMsg(child, contextTestOpen())
	for range 3 {
		assert.Nil(t, receiveContextTestSyscall(t, ctx).Exec)
	}
	assert.Empty(t, ctx.msgDataChan)
}

func TestProcessContextFirstExecEnrichesExistingPayload(t *testing.T) {
	ctx := newProcessContextTestCtx()
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
		return testProcessContext(20, 10, 200), nil
	}
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{
		Type: ebpfless.SyscallTypeExec,
		Exec: &ebpfless.ExecSyscallMsg{File: ebpfless.FileSyscallMsg{Filename: "/replacement"}, Args: []string{"replacement"}},
	})
	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	require.NotNil(t, message.Exec.Credentials)
	assert.EqualValues(t, 10, message.Exec.PPID)
	assert.EqualValues(t, 33, message.Exec.Credentials.UID)
	assert.False(t, message.Exec.FromProcFS)
	assert.Zero(t, message.Exec.CreateTime)
	assert.Equal(t, "/replacement", message.Exec.File.Filename)
	assert.Equal(t, []string{"replacement"}, message.Exec.Args)
	assert.Empty(t, ctx.msgDataChan)
}

func TestProcessContextFailureDoesNotRetryOrBlockTelemetry(t *testing.T) {
	for _, partial := range []bool{false, true} {
		ctx := newProcessContextTestCtx()
		child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
		lookups := 0
		ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
			lookups++
			if partial {
				return &ebpfless.SyscallMsg{PID: 20, Exec: &ebpfless.ExecSyscallMsg{PPID: 10, FromProcFS: true}}, os.ErrPermission
			}
			return nil, os.ErrNotExist
		}
		ctx.sendSyscallMsg(child, contextTestOpen())
		ctx.sendSyscallMsg(child, contextTestOpen())
		first := receiveContextTestSyscall(t, ctx)
		assert.Equal(t, partial, first.Exec != nil)
		if partial {
			require.NotNil(t, first.Exec)
			assert.EqualValues(t, 10, first.Exec.PPID)
		}
		assert.Nil(t, receiveContextTestSyscall(t, ctx).Exec)
		assert.Equal(t, 1, lookups)
		assert.EqualValues(t, 1, ctx.lifecycleGuard.contextFallbackFailures)
		assert.False(t, ctx.lifecycleGuard.states[child.identity()].contextAnnounced)
	}
}

func TestProcessContextFirstEventIsExit(t *testing.T) {
	root := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	ctx := newProcessContextTestCtx(root)
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	lookups := 0
	ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
		lookups++
		return nil, os.ErrNotExist // wait4 has already reaped the process
	}
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExit, Exit: &ebpfless.ExitSyscallMsg{}})
	message := receiveContextTestSyscall(t, ctx)
	assert.Equal(t, ebpfless.SyscallTypeExit, message.Type)
	assert.Nil(t, message.Exec)
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	assert.Empty(t, ctx.msgDataChan)
	assert.Equal(t, 1, lookups)
}

func TestProcessContextFirstEventIsFork(t *testing.T) {
	ctx := newProcessContextTestCtx()
	parent := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	child := &Process{Pid: 30, Tgid: 30, startTime: 300, containerContextResolved: true, ContainerID: "child-container", CGroupID: "child-cgroup"}
	ctx.processCache.Add(parent.Pid, parent)
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		require.Equal(t, parent.identity(), identity)
		snapshot := testProcessContext(20, 10, 200)
		snapshot.StartTime = 200
		return snapshot, nil
	}
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 20}})
	message := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, message.Exec)
	assert.EqualValues(t, 30, message.PID)
	assert.EqualValues(t, 300, message.StartTime)
	assert.EqualValues(t, 20, message.Fork.PPID) // Exec describes this parent, not PID 30
	assert.EqualValues(t, 200, message.Fork.ParentStartTime)
	assert.EqualValues(t, 10, message.Exec.PPID)
	assert.EqualValues(t, "child-container", message.ContainerID)
	assert.EqualValues(t, "child-cgroup", message.CGroupID)
	assert.EqualValues(t, "worker-container", message.Fork.ParentContainerID)
	assert.EqualValues(t, "worker-cgroup", message.Fork.ParentCGroupID)
	ctx.sendSyscallMsg(child, contextTestOpen())
	assert.Nil(t, receiveContextTestSyscall(t, ctx).Exec)
}

func TestProcessContextSuppressedForkDoesNotConsumeLookup(t *testing.T) {
	knownChild := &Process{Pid: 30, Tgid: 30, startTime: 300, containerContextResolved: true}
	parent := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx := newProcessContextTestCtx(knownChild)
	ctx.processCache.Add(parent.Pid, parent)
	lookups := 0
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		lookups++
		require.Equal(t, parent.identity(), identity)
		return testProcessContext(20, 10, 200), nil
	}
	ctx.sendSyscallMsg(knownChild, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 20}})
	assert.Empty(t, ctx.msgDataChan)
	assert.Zero(t, lookups)
	ctx.sendSyscallMsg(parent, contextTestOpen())
	require.NotNil(t, receiveContextTestSyscall(t, ctx).Exec)
	assert.Equal(t, 1, lookups)
}

func TestProcessContextPIDReuseAndExit(t *testing.T) {
	root := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	ctx := newProcessContextTestCtx(root)
	old := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	reused := &Process{Pid: 20, Tgid: 20, startTime: 201, containerContextResolved: true}
	lookups := 0
	ctx.processContextReader = func(identity processIdentity) (*ebpfless.SyscallMsg, error) {
		lookups++
		return testProcessContext(identity.tgid, 10, identity.startTime), nil
	}
	ctx.sendSyscallMsg(old, contextTestOpen())
	ctx.sendSyscallMsg(old, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExit, Exit: &ebpfless.ExitSyscallMsg{}})
	ctx.sendSyscallMsg(old, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	assert.Len(t, ctx.msgDataChan, 2) // no resurrection after exit
	ctx.sendSyscallMsg(reused, contextTestOpen())
	ctx.sendSyscallMsg(old, contextTestOpen())
	require.Len(t, ctx.msgDataChan, 3) // no snapshot of an obsolete generation
	require.NotNil(t, receiveContextTestSyscall(t, ctx).Exec)
	assert.Equal(t, ebpfless.SyscallTypeExit, receiveContextTestSyscall(t, ctx).Type)
	current := receiveContextTestSyscall(t, ctx)
	require.NotNil(t, current.Exec)
	assert.EqualValues(t, 201, current.Exec.CreateTime)
	assert.Equal(t, 2, lookups)
}

func TestProcessContextDroppedForkStillNeedsInitialization(t *testing.T) {
	root := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	ctx := newProcessContextTestCtx(root)
	ctx.msgDataChan = make(chan []byte, 1)
	ctx.msgDataChan <- []byte("occupied")
	ctx.processContextReader = func(processIdentity) (*ebpfless.SyscallMsg, error) {
		return testProcessContext(20, 10, 200), nil
	}
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	require.False(t, ctx.lifecycleGuard.states[child.identity()].contextAnnounced)
	<-ctx.msgDataChan
	ctx.sendSyscallMsg(child, contextTestOpen())
	require.NotNil(t, receiveContextTestSyscall(t, ctx).Exec)
	assert.True(t, ctx.lifecycleGuard.states[child.identity()].contextAnnounced)
}

func TestProcessContextWireCompatibility(t *testing.T) {
	message := &ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: contextTestOpen()}
	message.Syscall.PID, message.Syscall.StartTime = 20, 200
	message.Syscall.Exec = testProcessContext(20, 10, 200).Exec
	data, err := marshalSyscallMessage(message)
	require.NoError(t, err)
	// An older probe already understands Exec, but dispatches only Type=Open.
	var legacy struct {
		Type    ebpfless.MessageType
		Syscall struct {
			Type ebpfless.SyscallType
			PID  uint32
			Open *ebpfless.OpenSyscallMsg
			Exec struct {
				PPID       uint32
				FromProcFS bool
			}
		}
	}
	require.NoError(t, msgpack.Unmarshal(data, &legacy))
	require.NotNil(t, legacy.Syscall.Open)
	assert.Equal(t, ebpfless.MessageTypeSyscall, legacy.Type)
	assert.Equal(t, ebpfless.SyscallTypeOpen, legacy.Syscall.Type)
	assert.EqualValues(t, 20, legacy.Syscall.PID)
	assert.Equal(t, "/first-file", legacy.Syscall.Open.Filename)
	assert.EqualValues(t, 10, legacy.Syscall.Exec.PPID)
	assert.True(t, legacy.Syscall.Exec.FromProcFS)
}

func TestProcessContextFrameSizeLimit(t *testing.T) {
	for _, metadataFits := range []bool{false, true} {
		snapshot := testProcessContext(20, 10, 200)
		snapshot.Exec.Args = []string{strings.Repeat("a", ebpfless.MaxMessageSize)}
		snapshot.Exec.Envs = []string{strings.Repeat("e", ebpfless.MaxMessageSize)}
		if !metadataFits {
			snapshot.Exec.File.Filename = strings.Repeat("x", ebpfless.MaxMessageSize)
		}
		message := &ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: contextTestOpen()}
		message.Syscall.Exec = snapshot.Exec
		data, err := marshalSyscallMessage(message)
		require.NoError(t, err)
		require.LessOrEqual(t, len(data), ebpfless.MaxMessageSize)
		var decoded ebpfless.Message
		require.NoError(t, msgpack.Unmarshal(data, &decoded))
		require.NotNil(t, decoded.Syscall)
		assert.Equal(t, ebpfless.SyscallTypeOpen, decoded.Syscall.Type)
		assert.Equal(t, metadataFits, decoded.Syscall.Exec != nil)
		if metadataFits {
			image := decoded.Syscall.Exec
			require.NotNil(t, image)
			assert.True(t, image.ArgsTruncated)
			assert.True(t, image.EnvsTruncated)
			assert.Equal(t, "/worker", image.File.Filename)
		}
	}
}
