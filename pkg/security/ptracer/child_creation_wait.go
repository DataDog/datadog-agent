// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"container/list"
	"syscall"
	"time"
)

const (
	childCreationTimeout      = time.Second
	childCreationPollInterval = time.Millisecond
)

type heldChildStop struct {
	process  *Process
	deadline time.Time
}

// childCreationWaiter is owned by the tracing OS thread. A child's initial
// SIGSTOP can precede its creator's PTRACE_EVENT_FORK/CLONE/VFORK. Keep that TID
// stopped until resource inheritance and fork enqueueing are complete, or its
// one-second deadline expires. Other tracees must remain runnable meanwhile.
// Ptrace restarts, including timeout restarts, must stay on this same OS thread.
type childCreationWaiter struct {
	heldByTID        map[int]*list.Element
	deadlines        list.List
	resume           func(int) error
	timedOutChildren uint64
	lastReport       time.Time
}

func newChildCreationWaiter(resume func(int) error) *childCreationWaiter {
	return &childCreationWaiter{
		heldByTID: make(map[int]*list.Element),
		resume:    resume,
	}
}

func (w *childCreationWaiter) hold(process *Process) {
	if w.heldByTID[process.Pid] != nil {
		return // another stop must not extend the original deadline
	}
	// A fixed timeout and monotonic insertion times keep this list ordered.
	// No per-child timer goroutine may issue ptrace operations on our behalf.
	w.heldByTID[process.Pid] = w.deadlines.PushBack(heldChildStop{
		process:  process,
		deadline: time.Now().Add(childCreationTimeout),
	})
}

func (w *childCreationWaiter) forget(tid int) *Process {
	element := w.heldByTID[tid]
	if element == nil {
		return nil
	}
	delete(w.heldByTID, tid)
	w.deadlines.Remove(element)
	return element.Value.(heldChildStop).process
}

func (w *childCreationWaiter) resumeChild(tid int) {
	w.restartHeldChild(tid, w.resume)
}

func (w *childCreationWaiter) restartHeldChild(tid int, restart func(int) error) {
	if w.forget(tid) == nil {
		return
	}
	if err := restart(tid); err != nil && err != syscall.ESRCH {
		logger.Errorf("unable to restart child %d: %v", tid, err)
	}
}

func (w *childCreationWaiter) resumeExpiredChildren() {
	now := time.Now()
	for first := w.deadlines.Front(); first != nil; first = w.deadlines.Front() {
		child := first.Value.(heldChildStop)
		if now.Before(child.deadline) {
			break
		}
		// Once runnable, the child can exec or mutate its FD/CWD caches.
		// A late creation notification must never reapply inherited resources.
		child.process.creationWaitTimedOut = true
		w.timedOutChildren++
		w.resumeChild(child.process.Pid)
	}
	if w.timedOutChildren != 0 && (w.lastReport.IsZero() || now.Sub(w.lastReport) >= time.Minute) {
		logger.Logf("process creation ordering: %d children resumed after a fork timeout", w.timedOutChildren)
		w.lastReport = now
	}
}

// waitAnyTracee normally blocks in Wait4(-1). While a child is held, poll all
// tracees instead: waiting for a particular creator would block unrelated ones,
// and a blocking Wait4 could prevent an idle child's deadline from expiring.
// The syscall parameter allows testing this OS boundary with virtual time.
func (w *childCreationWaiter) waitAnyTracee(wait4 func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error)) (int, syscall.WaitStatus, error) {
	for {
		// Check before every wait, including under continuous tracee traffic.
		w.resumeExpiredChildren()
		options := 0
		if w.deadlines.Len() != 0 {
			options = syscall.WNOHANG
		}
		var status syscall.WaitStatus
		pid, err := wait4(-1, &status, options, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || pid != 0 {
			return pid, status, err
		}
		if first := w.deadlines.Front(); first != nil {
			time.Sleep(min(childCreationPollInterval, max(0, time.Until(first.Value.(heldChildStop).deadline))))
		}
	}
}

// detachAll must also run on the tracing thread, including on early trace exits.
// Detach rather than requesting another syscall stop that we will not consume.
func (w *childCreationWaiter) detachAll(detach func(int) error) {
	for first := w.deadlines.Front(); first != nil; first = w.deadlines.Front() {
		w.restartHeldChild(first.Value.(heldChildStop).process.Pid, detach)
	}
}

// holdChildAtInitialStop returns true when the caller must skip its usual
// restart. Readiness is per TID: a known TGID does not initialize a new thread's
// shared FD/CWD pointers, and initial wrapped/attached tracees are exempt.
func (ctx *CWSPtracerCtx) holdChildAtInitialStop(tid int, status syscall.WaitStatus, children *childCreationWaiter) bool {
	if status.StopSignal() != syscall.SIGSTOP {
		return false
	}
	process := ctx.processCache.Get(tid)
	if process == nil {
		process = NewProcess(tid)
		ctx.processCache.Add(tid, process)
	}
	if process.creationHandled || process.creationWaitTimedOut {
		return false
	}
	children.hold(process)
	return true
}

func (ctx *CWSPtracerCtx) handleChildCreationEvent(nr, creatorTID, childTID int, regs syscall.PtraceRegs, children *childCreationWaiter) {
	if ctx.processCache.Get(childTID) == nil && ctx.lifecycleGuard != nil {
		process := NewProcess(childTID)
		if ctx.lifecycleGuard.shouldIgnoreCreation(process.Pid, process.identity()) {
			// A timed-out child may have exited and left ProcessCache already.
			// Let the guard consume its late notification without recreating
			// inherited resources or resurrecting either process cache entry.
			ctx.sendForkMsg(process, creatorTID)
			return
		}
		ctx.processCache.Add(childTID, process)
	}
	ctx.handleHooks(CallbackPostType, nr, childTID, creatorTID, regs, nil)
	if process := ctx.processCache.Get(childTID); process != nil && process.creationHandled {
		// Hooks must finish resource inheritance and enqueue the fork before
		// this restart. If creation arrived first, there is no held stop yet;
		// holdChildAtInitialStop will recognize the ready TID when it arrives.
		children.resumeChild(childTID)
	}
}
