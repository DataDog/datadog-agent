// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

type messageOrdererTest struct {
	orderer *syscallMessageOrderer
	now     time.Time
	sent    []string
	root    processIdentity
	child   processIdentity
}

func newMessageOrdererTest() *messageOrdererTest {
	h := &messageOrdererTest{
		now:   time.Unix(1234, 0),
		root:  processIdentity{pid: 10, startTime: 100},
		child: processIdentity{pid: 20, startTime: 200},
	}
	h.orderer = newSyscallMessageOrderer([]processIdentity{h.root}, func(data []byte) {
		h.sent = append(h.sent, string(data))
	})
	return h
}

func (h *messageOrdererTest) event(owner processIdentity, kind ebpfless.SyscallType, name string) {
	h.orderer.push(outgoingSyscall{owner: owner, kind: kind, observed: h.now, data: []byte(name)}, h.now)
}

func (h *messageOrdererTest) fork(parent, child processIdentity, name string) {
	h.orderer.push(outgoingSyscall{owner: parent, child: child, kind: ebpfless.SyscallTypeFork, observed: h.now, data: []byte(name)}, h.now)
}

func TestSyscallMessageOrdererForkOrdering(t *testing.T) {
	for _, forkFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("fork_first=%v", forkFirst), func(t *testing.T) {
			h := newMessageOrdererTest()
			if forkFirst {
				h.fork(h.root, h.child, "fork")
			}
			h.event(h.child, ebpfless.SyscallTypeExec, "exec")
			h.event(h.child, ebpfless.SyscallTypeOpen, "open")
			h.event(h.child, ebpfless.SyscallTypeExit, "exit")
			if !forkFirst {
				require.Empty(t, h.sent)
				h.fork(h.root, h.child, "fork")
			}
			assert.Equal(t, []string{"fork", "exec", "open", "exit"}, h.sent)
			assert.Empty(t, h.orderer.processes[h.child])
			assert.Zero(t, h.orderer.pendingMessages)
			assert.Zero(t, h.orderer.pendingBytes)
		})
	}
}

func TestSyscallMessageOrdererInitialTracees(t *testing.T) {
	h := newMessageOrdererTest()
	h.event(h.root, ebpfless.SyscallTypeExec, "root exec")
	assert.Equal(t, []string{"root exec"}, h.sent)
	assert.Zero(t, h.orderer.pending.Len())
}

func TestSyscallMessageOrdererConcurrentAndNestedForks(t *testing.T) {
	h := newMessageOrdererTest()
	other := processIdentity{pid: 30, startTime: 300}
	grandchild := processIdentity{pid: 40, startTime: 400}
	h.event(h.child, ebpfless.SyscallTypeExec, "child exec")
	h.event(other, ebpfless.SyscallTypeExec, "other exec")
	h.event(grandchild, ebpfless.SyscallTypeExec, "grandchild exec")
	h.fork(h.child, grandchild, "grandchild fork")
	require.Empty(t, h.sent)

	h.fork(h.root, other, "other fork")
	assert.Equal(t, []string{"other fork", "other exec"}, h.sent)
	h.fork(h.root, h.child, "child fork")
	assert.Equal(t, []string{
		"other fork", "other exec", "child fork", "child exec", "grandchild fork", "grandchild exec",
	}, h.sent)
	assert.Zero(t, h.orderer.pending.Len())
}

func TestSyscallMessageOrdererTimeout(t *testing.T) {
	h := newMessageOrdererTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.now = h.now.Add(forkMessageTimeout - time.Nanosecond)
	h.event(h.child, ebpfless.SyscallTypeOpen, "open")
	require.Empty(t, h.sent)

	h.now = h.now.Add(time.Nanosecond)
	h.orderer.expire(h.now)
	assert.Equal(t, []string{"exec", "open"}, h.sent)
	assert.EqualValues(t, 1, h.orderer.timeouts)
	h.event(h.child, ebpfless.SyscallTypeOpen, "later open")
	h.fork(h.root, h.child, "late fork")
	h.event(h.child, ebpfless.SyscallTypeExit, "exit")
	assert.Equal(t, []string{"exec", "open", "later open", "exit"}, h.sent)
	assert.EqualValues(t, 1, h.orderer.lateForks)
	assert.NotContains(t, h.orderer.processes, h.child)
}

func TestSyscallMessageOrdererForkHeldBehindMissingParent(t *testing.T) {
	h := newMessageOrdererTest()
	grandchild := processIdentity{pid: 30, startTime: 300}
	h.event(grandchild, ebpfless.SyscallTypeExec, "grandchild exec")
	h.now = h.now.Add(forkMessageTimeout / 2)
	h.fork(h.child, grandchild, "grandchild fork")
	require.Empty(t, h.sent)

	h.now = h.now.Add(forkMessageTimeout / 2)
	h.orderer.expire(h.now)
	require.Equal(t, []string{"grandchild exec"}, h.sent)
	h.now = h.now.Add(forkMessageTimeout / 2)
	h.orderer.expire(h.now)
	h.fork(h.root, h.child, "parent fork")
	assert.Equal(t, []string{"grandchild exec"}, h.sent)
	assert.EqualValues(t, 2, h.orderer.lateForks)
}

func TestSyscallMessageOrdererExpiredWhileQueued(t *testing.T) {
	h := newMessageOrdererTest()
	h.orderer.push(outgoingSyscall{
		owner: h.child, kind: ebpfless.SyscallTypeExec, data: []byte("exec"), observed: h.now.Add(-2 * time.Second),
	}, h.now)
	assert.Equal(t, []string{"exec"}, h.sent)
	assert.EqualValues(t, 1, h.orderer.timeouts)
}

func TestSyscallMessageOrdererExitBeforeLateFork(t *testing.T) {
	h := newMessageOrdererTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "exec")
	h.event(h.child, ebpfless.SyscallTypeExit, "exit")
	h.now = h.now.Add(forkMessageTimeout)
	h.orderer.expire(h.now)
	require.Equal(t, []string{"exec", "exit"}, h.sent)
	require.Equal(t, 1, h.orderer.tombstones.Len())

	// Even though the tracer has removed this process from its cache, its
	// late fork must not resurrect it in system-probe's resolver.
	h.fork(h.root, h.child, "late fork")
	assert.Equal(t, []string{"exec", "exit"}, h.sent)
	assert.NotContains(t, h.orderer.processes, h.child)
	assert.Zero(t, h.orderer.tombstones.Len())
}

func TestSyscallMessageOrdererPIDReuse(t *testing.T) {
	h := newMessageOrdererTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "old exec")
	h.event(h.child, ebpfless.SyscallTypeExit, "old exit")
	h.now = h.now.Add(forkMessageTimeout)
	h.orderer.expire(h.now)

	reused := processIdentity{pid: h.child.pid, startTime: h.child.startTime + 1}
	h.event(reused, ebpfless.SyscallTypeExec, "new exec")
	h.fork(h.root, reused, "new fork")
	h.fork(h.root, h.child, "old late fork")
	assert.Equal(t, []string{"old exec", "old exit", "new fork", "new exec"}, h.sent)
}

func TestSyscallMessageOrdererPIDReuseBeforeTimeout(t *testing.T) {
	for _, forkFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("fork_first=%v", forkFirst), func(t *testing.T) {
			h := newMessageOrdererTest()
			h.event(h.child, ebpfless.SyscallTypeExec, "old exec")
			h.event(h.child, ebpfless.SyscallTypeExit, "old exit")
			reused := processIdentity{pid: h.child.pid, startTime: h.child.startTime + 1}
			if forkFirst {
				h.fork(h.root, reused, "new fork")
			}
			h.event(reused, ebpfless.SyscallTypeExec, "new exec")
			if !forkFirst {
				h.fork(h.root, reused, "new fork")
			}
			h.fork(h.root, h.child, "old late fork")
			assert.Equal(t, []string{"old exec", "old exit", "new fork", "new exec"}, h.sent)
			assert.EqualValues(t, 1, h.orderer.lifecycleFlushes)
			assert.Equal(t, reused, h.orderer.current[reused.pid])
		})
	}
}

func TestSyscallMessageOrdererUnknownBirthIdentity(t *testing.T) {
	h := newMessageOrdererTest()
	unknown := processIdentity{pid: h.child.pid}
	h.event(unknown, ebpfless.SyscallTypeExec, "exec")
	h.event(unknown, ebpfless.SyscallTypeExit, "exit")
	// The fork arrives after cache removal and a successful procfs read. Its
	// recovered identity must not cause the old exec to be overwritten.
	h.fork(h.root, h.child, "ambiguous fork")
	assert.Equal(t, []string{"exec", "exit"}, h.sent)
	assert.EqualValues(t, 1, h.orderer.lateForks)
}

func TestSyscallMessageOrdererLimits(t *testing.T) {
	t.Run("per-process count", func(t *testing.T) {
		h := newMessageOrdererTest()
		for range maxProcessPendingMessages {
			h.event(h.child, ebpfless.SyscallTypeOpen, "open")
		}
		assert.Len(t, h.sent, maxProcessPendingMessages)
		assert.EqualValues(t, 1, h.orderer.pressureFlushes)
		h.fork(h.root, h.child, "late fork")
		assert.Len(t, h.sent, maxProcessPendingMessages)
	})
	t.Run("per-process bytes", func(t *testing.T) {
		h := newMessageOrdererTest()
		h.event(h.child, ebpfless.SyscallTypeExec, string(make([]byte, maxProcessPendingBytes)))
		assert.Len(t, h.sent, 1)
		assert.EqualValues(t, 1, h.orderer.pressureFlushes)
		assert.Zero(t, h.orderer.pendingBytes)
	})
	t.Run("global count", func(t *testing.T) {
		h := newMessageOrdererTest()
		for i := range maxPendingMessages + 1 {
			h.event(processIdentity{pid: uint32(i + 1000), startTime: uint64(i + 1000)}, ebpfless.SyscallTypeOpen, "open")
		}
		assert.Len(t, h.sent, 1)
		assert.Equal(t, maxPendingMessages, h.orderer.pendingMessages)
		h.orderer.flush()
		assert.Zero(t, h.orderer.pendingBytes)
	})
	t.Run("global bytes", func(t *testing.T) {
		h := newMessageOrdererTest()
		payload := string(make([]byte, maxProcessPendingBytes/2))
		for i := range maxPendingBytes/(maxProcessPendingBytes/2) + 1 {
			h.event(processIdentity{pid: uint32(i + 1000), startTime: uint64(i + 1000)}, ebpfless.SyscallTypeExec, payload)
		}
		assert.Len(t, h.sent, 1)
		assert.LessOrEqual(t, h.orderer.pendingBytes, maxPendingBytes)
	})
}

func TestSyscallMessageOrdererBoundedTombstones(t *testing.T) {
	h := newMessageOrdererTest()
	for i := range maxMissingForkTombstones + 1 {
		identity := processIdentity{pid: uint32(i + 1000), startTime: uint64(i + 1000)}
		h.event(identity, ebpfless.SyscallTypeExec, "exec")
		h.event(identity, ebpfless.SyscallTypeExit, "exit")
		h.now = h.now.Add(forkMessageTimeout)
		h.orderer.expire(h.now)
	}
	require.Equal(t, maxMissingForkTombstones, h.orderer.tombstones.Len())
	require.Len(t, h.orderer.processes, maxMissingForkTombstones+1) // includes root
	sent := len(h.sent)
	oldest := processIdentity{pid: 1000, startTime: 1000}
	h.fork(h.root, oldest, "evicted late fork")
	assert.Len(t, h.sent, sent)

	// Unknown old processes bypass waiting after bookkeeping is retired.
	h.event(oldest, ebpfless.SyscallTypeExec, "untracked old exec")
	assert.Len(t, h.sent, sent+1)
	assert.Zero(t, h.orderer.pending.Len())
}

func TestSyscallMessageOrdererBoundedBookkeeping(t *testing.T) {
	h := newMessageOrdererTest()
	h.event(h.child, ebpfless.SyscallTypeExec, "held exec")
	// Simulate missing exit notifications: even known processes must not
	// accumulate without limit in a long-running tracer.
	for i := range maxTrackedProcesses + 1 {
		h.fork(h.root, processIdentity{pid: uint32(i + 1000), startTime: uint64(i + 1000)}, "fork")
	}
	assert.Len(t, h.orderer.processes, maxTrackedProcesses)
	assert.Len(t, h.orderer.current, maxTrackedProcesses)
	assert.Equal(t, maxTrackedProcesses, h.orderer.tracked.Len())
	assert.Contains(t, h.sent, "held exec")
	assert.Zero(t, h.orderer.pendingMessages)
	sent := len(h.sent)
	h.fork(h.root, h.child, "evicted late fork")
	assert.Len(t, h.sent, sent)
}

func TestSyscallMessageBufferTimerWithoutMoreEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		output := make(chan string, 8)
		b := newSyscallMessageBuffer(nil, func(data []byte) { output <- string(data) })
		defer b.close()
		require.True(t, b.enqueue(outgoingSyscall{
			owner: processIdentity{pid: 20, startTime: 200}, kind: ebpfless.SyscallTypeExec, observed: time.Now(), data: []byte("exec"),
		}))
		synctest.Wait()
		require.Empty(t, output)
		time.Sleep(forkMessageTimeout - time.Nanosecond)
		synctest.Wait()
		require.Empty(t, output)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.Equal(t, "exec", <-output)
	})
}

func TestSyscallMessageBufferShutdown(t *testing.T) {
	var sent []string
	b := newSyscallMessageBuffer(nil, func(data []byte) { sent = append(sent, string(data)) })
	require.True(t, b.enqueue(outgoingSyscall{
		owner: processIdentity{pid: 20, startTime: 200}, kind: ebpfless.SyscallTypeExec, observed: time.Now(), data: []byte("exec"),
	}))
	b.close()
	b.close() // cleanup is also called on tracing errors
	assert.Equal(t, []string{"exec"}, sent)
}

func TestSendSyscallMsgSnapshotAndOrdering(t *testing.T) {
	parent := &Process{Pid: 10, Tgid: 10, startTime: 100, containerContextResolved: true}
	child := &Process{Pid: 20, Tgid: 20, startTime: 200, containerContextResolved: true}
	ctx := &CWSPtracerCtx{processCache: NewProcessCache(), msgDataChan: make(chan []byte, 8)}
	ctx.processCache.Add(parent.Pid, parent)
	ctx.processCache.Add(child.Pid, child)
	ctx.syscallMessages = newSyscallMessageBuffer([]processIdentity{parent.identity()}, ctx.enqueueMsgData)
	defer ctx.syscallMessages.close()

	exec := &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{PPID: 10, Args: []string{"sh"}}}
	ctx.sendSyscallMsg(child, exec)
	timestamp := exec.Timestamp
	exec.Exec.Args[0] = "mutated after enqueue"
	ctx.sendSyscallMsg(child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeFork, Fork: &ebpfless.ForkSyscallMsg{PPID: 10}})
	ctx.syscallMessages.close()

	var forkMsg, execMsg ebpfless.Message
	require.NoError(t, msgpack.Unmarshal(<-ctx.msgDataChan, &forkMsg))
	require.NoError(t, msgpack.Unmarshal(<-ctx.msgDataChan, &execMsg))
	assert.Equal(t, ebpfless.SyscallTypeFork, forkMsg.Syscall.Type)
	assert.Equal(t, ebpfless.SyscallTypeExec, execMsg.Syscall.Type)
	assert.EqualValues(t, 20, execMsg.Syscall.PID)
	assert.EqualValues(t, 10, execMsg.Syscall.Exec.PPID)
	assert.Equal(t, []string{"sh"}, execMsg.Syscall.Exec.Args)
	assert.Equal(t, timestamp, execMsg.Syscall.Timestamp)
}

type messageTestConn struct {
	net.Conn
	write func([]byte) (int, error)
}

func (c *messageTestConn) Write(data []byte) (int, error) {
	return c.write(data)
}

func TestSendMessagesLoopReconnectOrdering(t *testing.T) {
	ctx := &CWSPtracerCtx{msgDataChan: make(chan []byte, 2)}
	var cancel context.CancelFunc
	ctx.cancel, cancel = context.WithCancel(context.Background())
	defer cancel()
	ctx.msgDataChan <- []byte("fork")
	ctx.msgDataChan <- []byte("exec")
	ctx.client = &messageTestConn{write: func([]byte) (int, error) {
		return 0, errors.New("connection lost")
	}}
	require.NoError(t, ctx.sendMessagesLoop()) // requests a reconnect
	assert.Equal(t, "fork", string(ctx.pendingMsgData))
	require.Len(t, ctx.msgDataChan, 1)

	var sent []string
	writes := 0
	ctx.client = &messageTestConn{write: func(data []byte) (int, error) {
		writes++
		if writes%2 == 0 { // every payload is preceded by its size
			sent = append(sent, string(data))
			if len(sent) == 2 {
				cancel()
			}
		}
		return len(data), nil
	}}
	require.Error(t, ctx.sendMessagesLoop()) // canceled after sending both
	assert.Equal(t, []string{"fork", "exec"}, sent)
	assert.Nil(t, ctx.pendingMsgData)
}

func TestSyscallMessageBufferQueueFull(t *testing.T) {
	b := &syscallMessageBuffer{input: make(chan outgoingSyscall, 1)}
	require.True(t, b.enqueue(outgoingSyscall{}))
	assert.False(t, b.enqueue(outgoingSyscall{}))
}
