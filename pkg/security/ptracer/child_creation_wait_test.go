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
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func initialChildStop() syscall.WaitStatus {
	return syscall.WaitStatus(uint32(syscall.SIGSTOP)<<8 | 0x7f)
}

type childCreationWaitTest struct {
	ctx      *CWSPtracerCtx
	children *childCreationWaiter
	parent   *Process
	child    *Process
	resumed  []int
	messages []*ebpfless.SyscallMsg
	steps    []string
}

func newChildCreationWaitTest(t *testing.T) *childCreationWaitTest {
	parent := NewProcess(-1)
	parent.Pid, parent.Tgid = 10, 10
	parent.startTime = 100
	parent.creationHandled = true
	parent.containerContextResolved = true
	parent.FdRes.Fd[7] = "/parent/file"
	parent.FsRes.Cwd = "/parent"
	child := NewProcess(os.Getpid())
	child.containerContextResolved = true
	h := &childCreationWaitTest{
		ctx:    &CWSPtracerCtx{opts: &Opts{}, processCache: NewProcessCache()},
		parent: parent,
		child:  child,
	}
	h.ctx.processCache.Add(parent.Pid, parent)
	h.ctx.processCache.Add(child.Pid, child)
	h.ctx.lifecycleGuard = newProcessLifecycleGuard([]processIdentity{parent.identity()}, func(data []byte) {
		var message ebpfless.Message
		require.NoError(t, msgpack.Unmarshal(data, &message))
		h.messages = append(h.messages, message.Syscall)
		h.steps = append(h.steps, "message")
	})
	h.children = newChildCreationWaiter(func(pid int) error {
		h.resumed = append(h.resumed, pid)
		h.steps = append(h.steps, "resume")
		return nil
	})
	t.Cleanup(func() { h.children.detachAll(h.children.resume) })
	return h
}

func TestChildCreationWaitOrdering(t *testing.T) {
	for _, tc := range []struct {
		name  string
		nr    int
		flags uint64
	}{
		{name: "fork", nr: ForkNr},
		{name: "vfork", nr: VforkNr},
		{name: "clone", nr: CloneNr},
		{name: "clone shared resources", nr: CloneNr, flags: unix.CLONE_FILES | unix.CLONE_FS},
		{name: "clone thread", nr: CloneNr, flags: unix.CLONE_THREAD | unix.CLONE_FILES | unix.CLONE_FS},
		{name: "clone3", nr: Clone3Nr},
		{name: "clone3 thread", nr: Clone3Nr, flags: unix.CLONE_THREAD | unix.CLONE_FILES | unix.CLONE_FS},
	} {
		for _, childFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/child_stop_first=%v", tc.name, childFirst), func(t *testing.T) {
				h := newChildCreationWaitTest(t)
				thread := tc.flags&unix.CLONE_THREAD != 0
				if thread {
					// Procfs already identifies the thread's TGID at its first stop.
					h.child.Tgid = h.parent.Tgid
					h.child.startTime = h.parent.startTime
				}
				arg := tc.flags
				if tc.nr == Clone3Nr {
					// ReadArgData uses process_vm_readv on the child. Here that is
					// our own process; pin the flags so their address stays valid.
					flags := new(uint64)
					*flags = tc.flags
					var pinner runtime.Pinner
					pinner.Pin(flags)
					defer pinner.Unpin()
					arg = uint64(uintptr(unsafe.Pointer(flags)))
				}
				if childFirst {
					require.True(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
					require.Empty(t, h.resumed)
					require.Empty(t, h.messages)
				}

				h.ctx.handleChildCreationEvent(tc.nr, h.parent.Pid, h.child.Pid, syscallTestRegs(tc.nr, arg), h.children)
				require.True(t, h.child.creationHandled)
				if !childFirst {
					require.Empty(t, h.resumed) // no restart before consuming its initial stop
					require.False(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
					require.NoError(t, h.children.resume(h.child.Pid))
				}
				assert.Equal(t, []int{h.child.Pid}, h.resumed)
				assert.Empty(t, h.children.heldByTID)
				assert.False(t, h.child.creationWaitTimedOut)
				assert.Equal(t, h.parent.FdRes.Fd, h.child.FdRes.Fd)
				assert.Equal(t, h.parent.FsRes.Cwd, h.child.FsRes.Cwd)
				if tc.flags&unix.CLONE_FILES != 0 {
					assert.Same(t, h.parent.FdRes, h.child.FdRes)
				} else {
					assert.NotSame(t, h.parent.FdRes, h.child.FdRes)
				}
				if tc.flags&unix.CLONE_FS != 0 {
					assert.Same(t, h.parent.FsRes, h.child.FsRes)
				} else {
					assert.NotSame(t, h.parent.FsRes, h.child.FsRes)
				}
				if thread {
					assert.Empty(t, h.messages) // threads do not publish process forks
					assert.Equal(t, []string{"resume"}, h.steps)
				} else {
					require.Len(t, h.messages, 1)
					assert.Equal(t, ebpfless.SyscallTypeFork, h.messages[0].Type)
					assert.EqualValues(t, h.child.Pid, h.messages[0].PID)
					assert.EqualValues(t, h.parent.Tgid, h.messages[0].Fork.PPID)
					assert.Equal(t, []string{"message", "resume"}, h.steps)
				}
			})
		}
	}
}

func TestChildCreationWaitInitialTraceeAndOtherSignals(t *testing.T) {
	h := newChildCreationWaitTest(t)
	assert.False(t, h.ctx.holdChildAtInitialStop(h.parent.Pid, initialChildStop(), h.children))
	trap := syscall.WaitStatus(uint32(syscall.SIGTRAP)<<8 | 0x7f)
	assert.False(t, h.ctx.holdChildAtInitialStop(h.child.Pid, trap, h.children))
	assert.Empty(t, h.children.heldByTID)
}

func TestChildCreationWaitUnknownChild(t *testing.T) {
	h := newChildCreationWaitTest(t)
	require.True(t, h.ctx.holdChildAtInitialStop(-2, initialChildStop(), h.children))
	child := h.ctx.processCache.Get(-2)
	require.NotNil(t, child)
	assert.False(t, child.creationHandled)
	assert.False(t, child.creationWaitTimedOut)
	assert.Contains(t, h.children.heldByTID, -2)
	assert.Empty(t, h.resumed)
}

func TestChildCreationWaitDoesNotBlockOtherProcesses(t *testing.T) {
	h := newChildCreationWaitTest(t)
	other := &Process{Pid: 30}
	h.children.hold(h.child)
	h.children.hold(other)
	pid, status, err := h.children.waitAnyTracee(func(pid int, status *syscall.WaitStatus, options int, _ *syscall.Rusage) (int, error) {
		assert.Equal(t, -1, pid)
		assert.Equal(t, syscall.WNOHANG, options)
		*status = initialChildStop()
		return h.parent.Pid, nil
	})
	require.NoError(t, err)
	assert.Equal(t, h.parent.Pid, pid)
	assert.Equal(t, initialChildStop(), status)
	assert.Empty(t, h.resumed)
	assert.Len(t, h.children.heldByTID, 2)
	h.ctx.sendSyscallMsg(h.parent, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeOpen})
	require.Len(t, h.messages, 1)
	assert.Equal(t, ebpfless.SyscallTypeOpen, h.messages[0].Type)
}

func TestChildCreationWaitTimeoutWithoutFurtherEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resumed := make(chan int, 1)
		w := newChildCreationWaiter(func(pid int) error { resumed <- pid; return nil })
		defer w.detachAll(w.resume)
		process := &Process{Pid: 20}
		w.hold(process)
		unblock := make(chan struct{})
		done := make(chan struct{})
		var lastOptions atomic.Int64
		go func() {
			defer close(done)
			_, _, err := w.waitAnyTracee(func(_ int, _ *syscall.WaitStatus, options int, _ *syscall.Rusage) (int, error) {
				lastOptions.Store(int64(options))
				if options&syscall.WNOHANG != 0 {
					return 0, nil
				}
				<-unblock
				return -1, syscall.ECHILD
			})
			assert.ErrorIs(t, err, syscall.ECHILD)
		}()
		synctest.Wait()
		require.Equal(t, syscall.WNOHANG, int(lastOptions.Load()))
		time.Sleep(childCreationTimeout - time.Nanosecond)
		synctest.Wait()
		require.Empty(t, resumed)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.Equal(t, 20, <-resumed)
		assert.True(t, process.creationWaitTimedOut)
		assert.Empty(t, w.heldByTID)
		assert.Zero(t, lastOptions.Load()) // return to blocking Wait4 when no children wait
		close(unblock)
		<-done
	})
}

func TestChildCreationWaitIndividualDeadlines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var resumed []int
		w := newChildCreationWaiter(func(pid int) error { resumed = append(resumed, pid); return nil })
		defer w.detachAll(w.resume)
		first := &Process{Pid: 20}
		second := &Process{Pid: 30}
		w.hold(first)
		time.Sleep(childCreationTimeout / 2)
		w.hold(first) // duplicate must not restart its timeout
		w.hold(second)
		time.Sleep(childCreationTimeout / 2)
		w.resumeExpiredChildren()
		assert.Equal(t, []int{20}, resumed)
		assert.True(t, first.creationWaitTimedOut)
		assert.False(t, second.creationWaitTimedOut)
		assert.Contains(t, w.heldByTID, 30)
		time.Sleep(childCreationTimeout / 2)
		w.resumeExpiredChildren()
		assert.Equal(t, []int{20, 30}, resumed)
		assert.True(t, second.creationWaitTimedOut)
		assert.Empty(t, w.heldByTID)
	})
}

func TestChildCreationWaitTimeoutCheckedDuringTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var resumed []int
		w := newChildCreationWaiter(func(pid int) error { resumed = append(resumed, pid); return nil })
		defer w.detachAll(w.resume)
		w.hold(&Process{Pid: 20})
		time.Sleep(childCreationTimeout)
		pid, _, err := w.waitAnyTracee(func(_ int, _ *syscall.WaitStatus, options int, _ *syscall.Rusage) (int, error) {
			assert.Zero(t, options)
			return 10, nil // another tracee has a ready event, even at the deadline
		})
		require.NoError(t, err)
		assert.Equal(t, 10, pid)
		assert.Equal(t, []int{20}, resumed)
	})
}

func TestChildCreationWaitDeathAndPIDReuse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var resumed []int
		w := newChildCreationWaiter(func(pid int) error { resumed = append(resumed, pid); return nil })
		defer w.detachAll(w.resume)
		old := &Process{Pid: 20, startTime: 100}
		w.hold(old)
		time.Sleep(childCreationTimeout / 2)
		assert.Same(t, old, w.forget(old.Pid)) // death while stopped: do not restart it
		reused := &Process{Pid: old.Pid, startTime: 200}
		w.hold(reused)
		time.Sleep(childCreationTimeout / 2)
		w.resumeExpiredChildren()
		assert.Empty(t, resumed)
		assert.False(t, old.creationWaitTimedOut)
		assert.False(t, reused.creationWaitTimedOut)
		time.Sleep(childCreationTimeout / 2)
		w.resumeExpiredChildren()
		assert.Equal(t, []int{20}, resumed)
		assert.True(t, reused.creationWaitTimedOut)
	})
}

func TestChildCreationWaitLateForkDoesNotOverwriteResourcesOrExec(t *testing.T) {
	for _, nr := range []int{ForkNr, VforkNr, CloneNr, Clone3Nr} {
		t.Run(fmt.Sprintf("syscall=%d", nr), func(t *testing.T) {
			h := newChildCreationWaitTest(t)
			synctest.Test(t, func(t *testing.T) {
				require.True(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
				time.Sleep(childCreationTimeout)
				h.children.resumeExpiredChildren()
				require.Equal(t, []int{h.child.Pid}, h.resumed)
				h.child.FsRes.Cwd = "/child/new-cwd"
				h.child.FdRes.Fd[7] = "/child/new-file"
				h.ctx.sendSyscallMsg(h.child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{PPID: 10}})
				require.Len(t, h.messages, 1)
				// In particular, late clone3 must not dereference flags from a
				// child that may have exec'd away the original argument memory.
				h.ctx.handleChildCreationEvent(nr, h.parent.Pid, h.child.Pid, syscallTestRegs(nr, 0), h.children)
				assert.Equal(t, "/child/new-cwd", h.child.FsRes.Cwd)
				assert.Equal(t, "/child/new-file", h.child.FdRes.Fd[7])
				assert.Equal(t, "/parent", h.parent.FsRes.Cwd)
				assert.Equal(t, "/parent/file", h.parent.FdRes.Fd[7])
				assert.Len(t, h.messages, 1)
				assert.Equal(t, ebpfless.SyscallTypeExec, h.messages[0].Type)
				assert.EqualValues(t, 1, h.ctx.lifecycleGuard.suppressedLateForks)
				assert.False(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
			})
		})
	}
}

func TestChildCreationWaitLateForkBeforeTelemetry(t *testing.T) {
	h := newChildCreationWaitTest(t)
	synctest.Test(t, func(t *testing.T) {
		require.True(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
		time.Sleep(childCreationTimeout)
		h.children.resumeExpiredChildren()
		h.child.FsRes.Cwd = "/child/new-cwd"
		h.ctx.handleChildCreationEvent(ForkNr, h.parent.Pid, h.child.Pid, syscallTestRegs(ForkNr, 0), h.children)
		// Without prior child telemetry the late fork remains useful, but it
		// must neither reset resources nor restart an already running child.
		require.Len(t, h.messages, 1)
		assert.Equal(t, ebpfless.SyscallTypeFork, h.messages[0].Type)
		assert.Equal(t, "/child/new-cwd", h.child.FsRes.Cwd)
		assert.Equal(t, []int{h.child.Pid}, h.resumed)
		assert.Equal(t, []string{"resume", "message"}, h.steps)
		assert.True(t, h.child.creationHandled)
		assert.Zero(t, h.ctx.lifecycleGuard.suppressedLateForks)
	})
}

func TestChildCreationWaitLateForkAfterExit(t *testing.T) {
	h := newChildCreationWaitTest(t)
	h.child.creationWaitTimedOut = true
	h.ctx.sendSyscallMsg(h.child, &ebpfless.SyscallMsg{Type: ebpfless.SyscallTypeExec, Exec: &ebpfless.ExecSyscallMsg{PPID: 10}})
	status := syscall.WaitStatus(0)
	h.ctx.handleExit(h.child, &status)
	require.Nil(t, h.ctx.processCache.Get(h.child.Pid))
	require.Len(t, h.messages, 2)
	h.ctx.handleChildCreationEvent(ForkNr, h.parent.Pid, h.child.Pid, syscallTestRegs(ForkNr, 0), h.children)
	assert.Nil(t, h.ctx.processCache.Get(h.child.Pid))
	assert.Len(t, h.messages, 2)
	assert.EqualValues(t, 1, h.ctx.lifecycleGuard.suppressedLateForks)
}

func TestChildCreationWaitFailedCreationBeforeInitialStop(t *testing.T) {
	h := newChildCreationWaitTest(t)
	// Malformed/unreadable clone3 arguments must not make the child ready.
	h.ctx.handleChildCreationEvent(Clone3Nr, h.parent.Pid, h.child.Pid, syscallTestRegs(Clone3Nr, 0), h.children)
	assert.False(t, h.child.creationHandled)
	assert.Empty(t, h.messages)
	assert.Empty(t, h.resumed)
	assert.True(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
}

func TestChildCreationWaitFailedCreationStillTimesOut(t *testing.T) {
	h := newChildCreationWaitTest(t)
	synctest.Test(t, func(t *testing.T) {
		require.True(t, h.ctx.holdChildAtInitialStop(h.child.Pid, initialChildStop(), h.children))
		h.ctx.handleChildCreationEvent(ForkNr, -3, h.child.Pid, syscallTestRegs(ForkNr, 0), h.children)
		require.False(t, h.child.creationHandled)
		require.Empty(t, h.resumed)
		time.Sleep(childCreationTimeout)
		h.children.resumeExpiredChildren()
		assert.Equal(t, []int{h.child.Pid}, h.resumed)
		assert.True(t, h.child.creationWaitTimedOut)
	})
}

func TestChildCreationWaitDetachAllAndErrors(t *testing.T) {
	for _, detachErr := range []error{nil, syscall.ESRCH, syscall.EIO} {
		t.Run(fmt.Sprint(detachErr), func(t *testing.T) {
			w := newChildCreationWaiter(func(int) error { t.Fatal("unexpected syscall restart during shutdown"); return nil })
			var detached []int
			detach := func(pid int) error { detached = append(detached, pid); return detachErr }
			w.hold(&Process{Pid: 20})
			w.hold(&Process{Pid: 30})
			w.detachAll(detach)
			w.detachAll(detach)
			assert.Equal(t, []int{20, 30}, detached)
			assert.Empty(t, w.heldByTID)
			assert.Zero(t, w.deadlines.Len())
		})
	}
}

func TestChildCreationWaitResumeErrors(t *testing.T) {
	for _, resumeErr := range []error{nil, syscall.ESRCH, syscall.EIO} {
		t.Run(fmt.Sprint(resumeErr), func(t *testing.T) {
			var resumed []int
			w := newChildCreationWaiter(func(pid int) error { resumed = append(resumed, pid); return resumeErr })
			w.hold(&Process{Pid: 20})
			w.resumeChild(20)
			w.resumeChild(20)
			assert.Equal(t, []int{20}, resumed)
			assert.Empty(t, w.heldByTID)
		})
	}
}

func TestChildCreationWaitInterruptedWait(t *testing.T) {
	w := newChildCreationWaiter(func(int) error { t.Fatal("unexpected restart"); return nil })
	calls := 0
	pid, _, err := w.waitAnyTracee(func(_ int, _ *syscall.WaitStatus, options int, _ *syscall.Rusage) (int, error) {
		assert.Zero(t, options)
		calls++
		if calls == 1 {
			return -1, syscall.EINTR
		}
		return 10, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 10, pid)
	assert.Equal(t, 2, calls)
}
