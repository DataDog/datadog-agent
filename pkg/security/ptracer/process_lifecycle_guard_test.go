// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

type lifecycleGuardTest struct {
	guard *processLifecycleGuard
	sent  []string
	root  processIdentity
	child processIdentity
}

func newLifecycleGuardTest() *lifecycleGuardTest {
	h := &lifecycleGuardTest{
		root:  processIdentity{tgid: 10, startTime: 100},
		child: processIdentity{tgid: 20, startTime: 200},
	}
	h.guard = newProcessLifecycleGuard([]processIdentity{h.root}, func(data []byte) {
		h.sent = append(h.sent, string(data))
	})
	return h
}

func (h *lifecycleGuardTest) event(owner processIdentity, kind ebpfless.SyscallType, name string) {
	h.guard.filterAndEnqueue(serializedSyscall{owner: owner, kind: kind, payload: []byte(name)})
}

func (h *lifecycleGuardTest) fork(parent, child processIdentity, name string) {
	h.guard.filterAndEnqueue(serializedSyscall{owner: parent, child: child, kind: ebpfless.SyscallTypeFork, payload: []byte(name)})
}

func TestProcessLifecycleGuardForkOrdering(t *testing.T) {
	h := newLifecycleGuardTest()
	h.fork(h.root, h.child, "fork")
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.event(h.child, ebpfless.SyscallTypeOpen, "open")
	h.event(h.child, ebpfless.SyscallTypeExit, "exit")
	assert.Equal(t, []string{"fork", "exec", "open", "exit"}, h.sent)
	assert.NotContains(t, h.guard.states, h.child)
}

func TestProcessLifecycleGuardDuplicateFork(t *testing.T) {
	h := newLifecycleGuardTest()
	h.fork(h.root, h.child, "fork")
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.fork(h.root, h.child, "duplicate fork")
	assert.Equal(t, []string{"fork", "exec"}, h.sent)
}

func TestProcessLifecycleGuardDoesNotBufferTelemetry(t *testing.T) {
	h := newLifecycleGuardTest()
	// A child allowed to run on timeout must not incur another telemetry wait.
	h.event(h.child, ebpfless.SyscallTypeExec, "child exec")
	require.Equal(t, []string{"child exec"}, h.sent)
	h.event(h.root, ebpfless.SyscallTypeOpen, "parent open")
	h.event(h.child, ebpfless.SyscallTypeOpen, "child open")
	assert.Equal(t, []string{"child exec", "parent open", "child open"}, h.sent)
	h.fork(h.root, h.child, "late fork")
	assert.Equal(t, []string{"child exec", "parent open", "child open"}, h.sent)
	assert.EqualValues(t, 1, h.guard.suppressedLateForks)
}

func TestProcessLifecycleGuardConcurrentAndNestedForks(t *testing.T) {
	h := newLifecycleGuardTest()
	other := processIdentity{tgid: 30, startTime: 300}
	grandchild := processIdentity{tgid: 40, startTime: 400}
	h.fork(h.root, h.child, "child fork")
	h.event(h.child, ebpfless.SyscallTypeExec, "child exec")
	h.fork(h.root, other, "other fork")
	h.event(other, ebpfless.SyscallTypeExec, "other exec")
	h.fork(h.child, grandchild, "grandchild fork")
	h.event(grandchild, ebpfless.SyscallTypeExec, "grandchild exec")
	assert.Equal(t, []string{
		"child fork", "child exec", "other fork", "other exec", "grandchild fork", "grandchild exec",
	}, h.sent)
}

func TestProcessLifecycleGuardExitBeforeLateFork(t *testing.T) {
	h := newLifecycleGuardTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.event(h.child, ebpfless.SyscallTypeExit, "exit")
	require.Equal(t, []string{"exec", "exit"}, h.sent)
	require.Equal(t, 1, h.guard.exitTombstones.Len())
	h.fork(h.root, h.child, "late fork")
	assert.Equal(t, []string{"exec", "exit"}, h.sent)
	assert.NotContains(t, h.guard.states, h.child)
	assert.Zero(t, h.guard.exitTombstones.Len())
}

func TestProcessLifecycleGuardPIDReuse(t *testing.T) {
	t.Run("fork before new exec", func(t *testing.T) {
		h := newLifecycleGuardTest()
		h.event(h.child, ebpfless.SyscallTypeExec, "old exec")
		h.event(h.child, ebpfless.SyscallTypeExit, "old exit")
		reused := processIdentity{tgid: h.child.tgid, startTime: h.child.startTime + 1}
		h.fork(h.root, reused, "new fork")
		h.event(reused, ebpfless.SyscallTypeExec, "new exec")
		h.fork(h.root, h.child, "old late fork")
		h.event(h.child, ebpfless.SyscallTypeOpen, "old late open")
		assert.Equal(t, []string{"old exec", "old exit", "new fork", "new exec"}, h.sent)
		assert.Equal(t, reused, h.guard.currentByTGID[reused.tgid])
	})
	t.Run("new child also resumes without fork", func(t *testing.T) {
		h := newLifecycleGuardTest()
		h.event(h.child, ebpfless.SyscallTypeExec, "old exec")
		h.event(h.child, ebpfless.SyscallTypeExit, "old exit")
		reused := processIdentity{tgid: h.child.tgid, startTime: h.child.startTime + 1}
		h.event(reused, ebpfless.SyscallTypeExec, "new exec")
		h.fork(h.root, reused, "new late fork")
		h.fork(h.root, h.child, "old late fork")
		assert.Equal(t, []string{"old exec", "old exit", "new exec"}, h.sent)
		assert.EqualValues(t, 2, h.guard.suppressedLateForks)
		assert.Equal(t, reused, h.guard.currentByTGID[reused.tgid])
	})
}

func TestProcessLifecycleGuardUnknownBirthIdentity(t *testing.T) {
	for _, tc := range []struct {
		name         string
		execIdentity processIdentity
		forkIdentity processIdentity
	}{
		{
			name:         "procfs recovered after exec",
			execIdentity: processIdentity{tgid: 20},
			forkIdentity: processIdentity{tgid: 20, startTime: 200},
		},
		{
			name:         "procfs unavailable after exec",
			execIdentity: processIdentity{tgid: 20, startTime: 200},
			forkIdentity: processIdentity{tgid: 20},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLifecycleGuardTest()
			h.event(tc.execIdentity, ebpfless.SyscallTypeExec, "exec")
			h.event(tc.execIdentity, ebpfless.SyscallTypeExit, "exit")
			h.fork(h.root, tc.forkIdentity, "ambiguous fork")
			assert.Equal(t, []string{"exec", "exit"}, h.sent)
			assert.EqualValues(t, 1, h.guard.suppressedLateForks)
		})
	}
}

func TestProcessLifecycleGuardShouldIgnoreCreation(t *testing.T) {
	h := newLifecycleGuardTest()
	assert.False(t, h.guard.shouldIgnoreCreation(int(h.root.tgid), h.root))
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.event(h.child, ebpfless.SyscallTypeExit, "exit")
	assert.True(t, h.guard.shouldIgnoreCreation(int(h.child.tgid), h.child))
	assert.True(t, h.guard.shouldIgnoreCreation(int(h.child.tgid), processIdentity{tgid: h.child.tgid}))
	assert.False(t, h.guard.shouldIgnoreCreation(int(h.child.tgid), processIdentity{tgid: h.child.tgid, startTime: h.child.startTime + 1}))
	assert.False(t, h.guard.shouldIgnoreCreation(30, h.child)) // another TID is not a process fork

	// An active process can predate evicted records without being obsolete.
	h.guard.hasEvictedStates = true
	h.guard.evictedStartTimeCutoff = h.root.startTime + 1
	assert.False(t, h.guard.shouldIgnoreCreation(int(h.root.tgid), h.root))
	// Even once its record is evicted, new TIDs in that process must not be
	// mistaken for late process forks: they still need shared FD/CWD setup.
	h.guard.evict(h.guard.states[h.root])
	assert.True(t, h.guard.shouldIgnoreCreation(int(h.root.tgid), h.root))
	assert.False(t, h.guard.shouldIgnoreCreation(30, h.root))
}

func TestProcessLifecycleGuardBoundedTombstones(t *testing.T) {
	h := newLifecycleGuardTest()
	for i := 0; i <= maxMissingForkTombstones; i++ {
		identity := processIdentity{tgid: uint32(i + 1000), startTime: uint64(i + 1000)}
		h.event(identity, ebpfless.SyscallTypeExec, "exec")
		h.event(identity, ebpfless.SyscallTypeExit, "exit")
	}
	assert.Equal(t, maxMissingForkTombstones, h.guard.exitTombstones.Len())
	assert.Len(t, h.guard.states, maxMissingForkTombstones+1) // initial tracee
	oldest := processIdentity{tgid: 1000, startTime: 1000}
	sent := len(h.sent)
	h.fork(h.root, oldest, "evicted late fork")
	assert.Len(t, h.sent, sent)
	h.event(oldest, ebpfless.SyscallTypeOpen, "old open")
	assert.Equal(t, "old open", h.sent[len(h.sent)-1])
	assert.NotContains(t, h.guard.states, oldest)
}

func TestProcessLifecycleGuardBoundedBookkeeping(t *testing.T) {
	h := newLifecycleGuardTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "child exec")
	for i := 0; i <= maxLifecycleStates; i++ {
		h.fork(h.root, processIdentity{tgid: uint32(i + 1000), startTime: uint64(i + 1000)}, "fork")
	}
	assert.Len(t, h.guard.states, maxLifecycleStates)
	assert.Len(t, h.guard.currentByTGID, maxLifecycleStates)
	assert.Equal(t, maxLifecycleStates, h.guard.trackingOrder.Len())
	sent := len(h.sent)
	h.fork(h.root, h.child, "evicted late fork")
	assert.Len(t, h.sent, sent)
}
