// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type ebpflessContextEventHandler func(*model.Event)

func (h ebpflessContextEventHandler) HandleEvent(event *model.Event) { h(event) }

func newEBPFLessContextTestProbe(t *testing.T) (*EBPFLessProbe, *client) {
	t.Helper()
	resolver, err := process.NewEBPFLessResolver(nil, nil, nil, process.NewResolverOpts())
	require.NoError(t, err)
	r := &resolvers.EBPFLessResolvers{ProcessResolver: resolver}
	fh := &EBPFLessFieldHandlers{BaseFieldHandlers: &BaseFieldHandlers{}, resolvers: r}
	return &EBPFLessProbe{
		Resolvers: r, fieldHandlers: fh, event: NewEBPFLessEvent(fh),
		probe: &Probe{}, processKiller: &ProcessKiller{}, fileHasher: &FileHasher{},
	}, &client{nsID: 1}
}

func ebpflessTestSnapshot(ppid uint32) *ebpfless.ExecSyscallMsg {
	return &ebpfless.ExecSyscallMsg{
		FromProcFS: true, CreateTime: 200, PPID: ppid,
		File: ebpfless.FileSyscallMsg{Filename: "/worker", Inode: 123, Mode: 0755},
		Args: []string{"worker", "--worker"}, Envs: []string{"WORKER=1"}, TTY: "pts1",
		Credentials: &ebpfless.Credentials{UID: 33, EUID: 33, GID: 34, EGID: 34},
	}
}

// Exercise the actual wire decoder and dispatch path, not just cache insertion.
func handleEBPFLessContextTestMessage(t *testing.T, p *EBPFLessProbe, cl *client, syscall *ebpfless.SyscallMsg) {
	t.Helper()
	data, err := msgpack.Marshal(&ebpfless.Message{Type: ebpfless.MessageTypeSyscall, Syscall: syscall})
	require.NoError(t, err)
	var message ebpfless.Message
	require.NoError(t, msgpack.Unmarshal(data, &message))
	p.handleClientMsg(cl, &message)
}

func TestEBPFLessContextBeforeFirstOpenWithoutExec(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	parent := p.Resolvers.ProcessResolver.AddExecEntry(process.CacheResolverKey{Pid: 100, NSID: 1},
		0, "/parent", []string{"parent"}, false, []string{"PARENT=1"}, false, "", "", 100, "")
	parent.Credentials.UID = 99
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		calls++
		require.Equal(t, model.FileOpenEventType, event.GetEventType())
		entry := event.ProcessCacheEntry
		require.NotNil(t, entry)
		assert.EqualValues(t, 200, entry.Pid)
		assert.EqualValues(t, 100, entry.PPid)
		assert.Equal(t, "/worker", entry.FileEvent.PathnameStr)
		assert.EqualValues(t, 123, entry.FileEvent.Inode)
		assert.EqualValues(t, 0755, entry.FileEvent.Mode)
		assert.EqualValues(t, 33, entry.Credentials.UID)
		assert.EqualValues(t, 34, entry.Credentials.GID)
		require.NotNil(t, entry.ArgsEntry)
		require.NotNil(t, entry.EnvsEntry)
		assert.Equal(t, []string{"worker", "--worker"}, entry.ArgsEntry.Values)
		assert.Equal(t, []string{"WORKER=1"}, entry.EnvsEntry.Values)
		assert.EqualValues(t, "worker-container", entry.ContainerContext.ContainerID)
		assert.EqualValues(t, "worker-cgroup", entry.CGroup.CGroupID)
		assert.Same(t, parent, entry.Ancestor)
		assert.False(t, entry.IsExec)
		assert.True(t, entry.ExecTime.IsZero())
		assert.Equal(t, time.Unix(0, 200), entry.ForkTime)
		assert.EqualValues(t, model.ProcessCacheEntryFromProcFS, entry.Source)
		assert.Equal(t, "/opened-file", event.Open.File.PathnameStr)
		// These are the same accessors used by rules evaluating this first open.
		for field, expected := range map[string]any{
			"process.file.path": "/worker", "process.file.name": "worker",
			"process.uid": 33, "process.created_at": 200,
		} {
			value, err := event.GetFieldValue(field)
			require.NoError(t, err)
			assert.EqualValues(t, expected, value, "field %s", field)
		}
	})))
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 200, Timestamp: 300, Type: ebpfless.SyscallTypeOpen,
		ContainerID: "worker-container", CGroupID: "worker-cgroup", Exec: ebpflessTestSnapshot(100),
		Open: &ebpfless.OpenSyscallMsg{FileSyscallMsg: ebpfless.FileSyscallMsg{Filename: "/opened-file"}},
	})
	assert.Equal(t, 1, calls) // Exec metadata did not dispatch an extra exec or fork
	assert.True(t, parent.ExitTime.IsZero())
	assert.True(t, parent.StopExecutionTime.IsZero())
}

func TestEBPFLessContextFirstExecDoesNotInventReplacementAncestor(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	parent := p.Resolvers.ProcessResolver.AddExecEntry(process.CacheResolverKey{Pid: 100, NSID: 1},
		0, "/parent", []string{"parent"}, false, nil, false, "", "", 100, "")
	parent.Credentials.UID = 99
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		calls++
		require.Equal(t, model.ExecEventType, event.GetEventType())
		entry := event.ProcessCacheEntry
		require.NotNil(t, entry)
		assert.Equal(t, "/replacement", entry.FileEvent.PathnameStr)
		assert.EqualValues(t, 100, entry.PPid)
		assert.EqualValues(t, 33, entry.Credentials.UID)
		assert.True(t, entry.IsExec)
		assert.Equal(t, time.Unix(0, 300), entry.ExecTime)
		require.NotNil(t, entry.Ancestor)
		assert.EqualValues(t, 200, entry.Ancestor.Pid)
		assert.Equal(t, "/parent", entry.Ancestor.FileEvent.PathnameStr)
		assert.EqualValues(t, model.ProcessCacheEntryFromPlaceholder, entry.Ancestor.Source)
		assert.True(t, entry.Ancestor.ForkTime.IsZero())
		assert.Same(t, parent, entry.Ancestor.Ancestor)
	})))
	// The producer enriches the real exec directly; there is no second image.
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 200, Timestamp: 300, Type: ebpfless.SyscallTypeExec,
		Exec: &ebpfless.ExecSyscallMsg{
			PPID: 100, File: ebpfless.FileSyscallMsg{Filename: "/replacement"}, Args: []string{"replacement"},
			Credentials: &ebpfless.Credentials{UID: 33, EUID: 33, GID: 34, EGID: 34},
		},
	})
	assert.Equal(t, 1, calls)
	assert.True(t, parent.ExitTime.IsZero())
	assert.True(t, parent.StopExecutionTime.IsZero())
}

func TestEBPFLessContextFirstForkInitializesItsParent(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	grandparent := p.Resolvers.ProcessResolver.AddExecEntry(process.CacheResolverKey{Pid: 50, NSID: 1},
		0, "/grandparent", []string{"grandparent"}, false, nil, false, "", "", 100, "")
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		calls++
		require.Equal(t, model.ForkEventType, event.GetEventType())
		entry := event.ProcessCacheEntry
		require.NotNil(t, entry)
		assert.EqualValues(t, 200, entry.Pid)
		assert.EqualValues(t, 100, entry.PPid)
		assert.Equal(t, "/worker", entry.FileEvent.PathnameStr)
		assert.EqualValues(t, 33, entry.Credentials.UID)
		assert.EqualValues(t, "child-container", entry.ContainerContext.ContainerID)
		assert.EqualValues(t, "child-cgroup", entry.CGroup.CGroupID)
		require.NotNil(t, entry.Ancestor)
		assert.EqualValues(t, 100, entry.Ancestor.Pid)
		assert.Equal(t, "/worker", entry.Ancestor.FileEvent.PathnameStr)
		assert.EqualValues(t, "worker-container", entry.Ancestor.ContainerContext.ContainerID)
		assert.EqualValues(t, "worker-cgroup", entry.Ancestor.CGroup.CGroupID)
		assert.Same(t, grandparent, entry.Ancestor.Ancestor)
	})))
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 300, Timestamp: 300, Type: ebpfless.SyscallTypeFork,
		ContainerID: "child-container", CGroupID: "child-cgroup", Exec: ebpflessTestSnapshot(50),
		Fork: &ebpfless.ForkSyscallMsg{
			PPID: 100, ParentStartTime: 200, ParentContainerID: "worker-container", ParentCGroupID: "worker-cgroup",
		},
	})
	assert.Equal(t, 1, calls)
}

func TestEBPFLessContextStaleForkParentDoesNotReplaceCurrent(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	key := process.CacheResolverKey{Pid: 100, NSID: cl.nsID}
	require.True(t, p.Resolvers.ProcessResolver.PrepareProcessGeneration(key, 201, false))
	parent := p.Resolvers.ProcessResolver.AddExecEntry(key, 50, "/current", nil, false, nil, false, "", "", 300, "")
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(*model.Event) { calls++ })))
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 400, Timestamp: 400, Type: ebpfless.SyscallTypeFork,
		Exec: ebpflessTestSnapshot(50), Fork: &ebpfless.ForkSyscallMsg{PPID: 100, ParentStartTime: 200},
	})
	assert.Zero(t, calls)
	assert.Same(t, parent, p.Resolvers.ProcessResolver.Resolve(key))
	assert.Nil(t, p.Resolvers.ProcessResolver.Resolve(process.CacheResolverKey{Pid: 200, NSID: cl.nsID}))
}

func TestEBPFLessContextReplayDoesNotRollBackExec(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	var kinds []model.EventType
	var images []string
	var uids []uint32
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		kinds = append(kinds, event.GetEventType())
		images = append(images, event.ProcessCacheEntry.FileEvent.PathnameStr)
		uids = append(uids, event.ProcessCacheEntry.Credentials.UID)
	})))
	open := &ebpfless.SyscallMsg{
		PID: 200, StartTime: 200, Timestamp: 250, Type: ebpfless.SyscallTypeOpen,
		Exec: ebpflessTestSnapshot(100), Open: &ebpfless.OpenSyscallMsg{},
	}
	handleEBPFLessContextTestMessage(t, p, cl, open)
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 200, Timestamp: 300, Type: ebpfless.SyscallTypeExec,
		Exec: &ebpfless.ExecSyscallMsg{
			File: ebpfless.FileSyscallMsg{Filename: "/replacement"}, Credentials: &ebpfless.Credentials{User: "root"},
		},
	})
	handleEBPFLessContextTestMessage(t, p, cl, open)
	assert.Equal(t, []model.EventType{model.FileOpenEventType, model.ExecEventType, model.FileOpenEventType}, kinds)
	assert.Equal(t, []string{"/worker", "/replacement", "/replacement"}, images)
	assert.Equal(t, []uint32{33, 0, 0}, uids)
}

func TestEBPFLessContextPIDReuseRejectsStaleFrames(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	key := process.CacheResolverKey{Pid: 200, NSID: cl.nsID}
	require.True(t, p.Resolvers.ProcessResolver.PrepareProcessGeneration(key, 100, false))
	p.Resolvers.ProcessResolver.AddExecEntry(key, 100, "/old", []string{"old"}, false, nil, false, "", "", 1000, "")
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		calls++
		require.Equal(t, model.FileOpenEventType, event.GetEventType())
		assert.Equal(t, "/new", event.ProcessCacheEntry.FileEvent.PathnameStr)
	})))
	snapshot := ebpflessTestSnapshot(100)
	snapshot.CreateTime = 50 // creation dates are not generation identities
	snapshot.File.Filename = "/new"
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 201, Timestamp: 1100, Type: ebpfless.SyscallTypeOpen, Exec: snapshot,
		Open: &ebpfless.OpenSyscallMsg{FileSyscallMsg: ebpfless.FileSyscallMsg{Filename: "/opened-file"}},
	})
	current := p.Resolvers.ProcessResolver.Resolve(key)
	require.NotNil(t, current)
	for _, stale := range []*ebpfless.SyscallMsg{
		{PID: 200, StartTime: 100, Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{File: ebpfless.FileSyscallMsg{Filename: "/stale"}}},
		{PID: 200, StartTime: 100, Type: ebpfless.SyscallTypeExit, Exit: &ebpfless.ExitSyscallMsg{}},
		{PID: 200, StartTime: 200, Type: ebpfless.SyscallTypeOpen, Exec: ebpflessTestSnapshot(100), Open: &ebpfless.OpenSyscallMsg{}},
	} {
		handleEBPFLessContextTestMessage(t, p, cl, stale)
	}
	assert.Equal(t, 1, calls)
	assert.Same(t, current, p.Resolvers.ProcessResolver.Resolve(key))
	assert.True(t, current.ExitTime.IsZero())
}

func TestEBPFLessPartialContextBeforeCredentialChange(t *testing.T) {
	p, cl := newEBPFLessContextTestProbe(t)
	calls := 0
	require.NoError(t, p.probe.AddEventHandler(ebpflessContextEventHandler(func(event *model.Event) {
		calls++
		require.Equal(t, model.SetuidEventType, event.GetEventType())
		entry := event.ProcessCacheEntry
		require.NotNil(t, entry)
		assert.EqualValues(t, 100, entry.PPid)
		assert.Empty(t, entry.FileEvent.PathnameStr)
		assert.Empty(t, entry.FileEvent.BasenameStr)
		assert.EqualValues(t, 33, entry.Credentials.UID)
		assert.EqualValues(t, 34, entry.Credentials.EUID)
		assert.Equal(t, "worker", entry.Credentials.User)
		assert.Equal(t, "effective-worker", entry.Credentials.EUser)
	})))
	handleEBPFLessContextTestMessage(t, p, cl, &ebpfless.SyscallMsg{
		PID: 200, StartTime: 200, Timestamp: 300, Type: ebpfless.SyscallTypeSetUID,
		Exec:   &ebpfless.ExecSyscallMsg{PPID: 100, FromProcFS: true},
		SetUID: &ebpfless.SetUIDSyscallMsg{UID: 33, EUID: 34, User: "worker", EUser: "effective-worker"},
	})
	assert.Equal(t, 1, calls)
}
