// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func processStatFixture(comm, ppid, startTime string) string {
	fields := make([]string, 20)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S"
	fields[1] = ppid
	fields[19] = startTime
	return fmt.Sprintf("123 (%s) %s", comm, strings.Join(fields, " "))
}

func TestParseProcessStat(t *testing.T) {
	for _, comm := range []string{"sh", "with spaces", "with ) parentheses (", "with\nnewlines"} {
		t.Run(comm, func(t *testing.T) {
			ppid, startTime, err := parseProcessStat(processStatFixture(comm, "42", "123456789"))
			require.NoError(t, err)
			assert.EqualValues(t, 42, ppid)
			assert.EqualValues(t, 123456789, startTime)
		})
	}
	for name, data := range map[string]string{
		"empty":             "",
		"missing comm":      "123 S 42",
		"truncated":         "123 (sh) S 42",
		"invalid ppid":      processStatFixture("sh", "invalid", "123"),
		"negative ppid":     processStatFixture("sh", "-1", "123"),
		"overflowing ppid":  processStatFixture("sh", "4294967296", "123"),
		"invalid starttime": processStatFixture("sh", "42", "invalid"),
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseProcessStat(data)
			assert.Error(t, err)
		})
	}
}

func TestReadProcessStat(t *testing.T) {
	ppid, startTime, err := readProcessStat(os.Getpid())
	require.NoError(t, err)
	assert.EqualValues(t, os.Getppid(), ppid)
	assert.NotZero(t, startTime)
	assert.EqualValues(t, os.Getppid(), getPidPPID(os.Getpid()))
	assert.EqualValues(t, 0, getPidPPID(-1))
	assert.Equal(t, startTime, NewProcess(os.Getpid()).startTime)
}

func TestProcessIdentityFromProc(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid := unix.Gettid()
	thread := NewProcess(tid)
	assert.Equal(t, os.Getpid(), thread.Tgid)
	assert.Equal(t, NewProcess(os.Getpid()).identity(), thread.identity())
	assert.Equal(t, -1, getPidTGID(-1))
}

func TestProcessIdentityAcrossThreadExec(t *testing.T) {
	cache := NewProcessCache()
	parent := NewProcess(os.Getpid())
	thread := NewProcess(-1)
	cache.Add(parent.Pid, parent)
	cache.shareResources(thread, parent.Pid, unix.CLONE_THREAD)
	assert.Equal(t, parent.identity(), thread.identity())
	thread.Pid = thread.Tgid // exec changes a nonleader's TID to the TGID
	assert.Equal(t, parent.identity(), thread.identity())
}

func TestExecPPID(t *testing.T) {
	t.Run("fork already handled", func(t *testing.T) {
		// There is no procfs entry for this PID: the known fork parent must
		// take the fast path rather than be replaced by a procfs lookup.
		process := &Process{Pid: -1, forkPPID: 42}
		assert.EqualValues(t, 42, process.execPPID())
	})
	t.Run("fork not yet handled", func(t *testing.T) {
		process := &Process{Pid: os.Getpid()}
		assert.EqualValues(t, os.Getppid(), process.execPPID())
		// A procfs observation is not a fork notification. Until that arrives,
		// keep resolving at exec instead of caching a potentially stale PPID.
		assert.Zero(t, process.forkPPID)
	})
	t.Run("procfs unavailable", func(t *testing.T) {
		process := &Process{Pid: -1}
		assert.Zero(t, process.execPPID())
	})
}

func TestForkPPID(t *testing.T) {
	for _, nr := range []int{ForkNr, VforkNr} {
		t.Run(fmt.Sprintf("syscall=%d", nr), func(t *testing.T) {
			ctx := &CWSPtracerCtx{processCache: NewProcessCache()}
			parent := &Process{Pid: 11, Tgid: 10}
			child := &Process{Pid: -1, Tgid: -1, containerContextResolved: true}
			ctx.processCache.Add(parent.Pid, parent)
			ctx.handlePostHooks(nr, parent.Pid, syscall.PtraceRegs{}, child, syscallHandler{})
			assert.EqualValues(t, parent.Tgid, child.forkPPID)
			assert.EqualValues(t, parent.Tgid, child.execPPID())
		})
	}
}

func TestClonePPID(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      uint64
		parentPPID uint32
		wantPPID   uint32
	}{
		{name: "new process", parentPPID: 42, wantPPID: 10},
		{name: "shared memory", flags: unix.CLONE_VM, parentPPID: 42, wantPPID: 10},
		{name: "thread", flags: unix.CLONE_THREAD, parentPPID: 42, wantPPID: 42},
		{name: "same parent requires procfs", flags: unix.CLONE_PARENT, parentPPID: 42},
		{name: "thread with same parent", flags: unix.CLONE_THREAD | unix.CLONE_PARENT, parentPPID: 42, wantPPID: 42},
		{name: "unknown thread parent", flags: unix.CLONE_THREAD},
		{name: "unknown shared parent", flags: unix.CLONE_PARENT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewProcessCache()
			parent := NewProcess(-1)
			parent.Tgid = 10
			parent.forkPPID = tc.parentPPID
			child := NewProcess(-2)
			cache.Add(parent.Pid, parent)
			cache.shareResources(child, parent.Pid, tc.flags)
			assert.Equal(t, tc.wantPPID, child.forkPPID)
			assert.Equal(t, tc.wantPPID, child.execPPID())
		})
	}
}
