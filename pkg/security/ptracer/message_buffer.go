// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"container/list"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

const (
	forkMessageTimeout        = time.Second
	maxPendingMessages        = 4096
	maxPendingBytes           = 8 << 20
	maxProcessPendingMessages = 64
	maxProcessPendingBytes    = 1 << 20
	maxMissingForkTombstones  = 4096
	maxTrackedProcesses       = 16384
)

// startTime is /proc/<tgid>/stat's starttime, in clock ticks since boot. It
// distinguishes PID reuse without changing the wire protocol.
type processIdentity struct {
	pid       uint32
	startTime uint64
}

// data is serialized on the tracing thread: syscall handlers may subsequently
// modify their messages, so pointers to those messages must not cross threads.
type outgoingSyscall struct {
	owner    processIdentity
	child    processIdentity // only used for fork messages
	kind     ebpfless.SyscallType
	observed time.Time
	data     []byte
}

type bufferedProcess struct {
	identity    processIdentity
	ready       bool
	missingFork bool
	exited      bool
	deadline    time.Time
	messages    []outgoingSyscall
	bytes       int
	pending     *list.Element
	tombstone   *list.Element
	tracked     *list.Element
}

// syscallMessageOrderer is owned exclusively by the emission goroutine. It
// delays telemetry, never ptrace stops. Forks belong to the parent's stream:
// this also orders nested forks behind an as-yet unreported parent.
type syscallMessageOrderer struct {
	processes       map[processIdentity]*bufferedProcess
	current         map[uint32]processIdentity
	pending         list.List
	tombstones      list.List
	tracked         list.List
	pendingMessages int
	pendingBytes    int
	emit            func([]byte)

	// Once a missing-fork tombstone is evicted, conservatively release unknown
	// older processes without waiting, and suppress their forks. This bounds
	// bookkeeping without allowing an arbitrarily late fork to overwrite exec.
	retiredBefore uint64
	hasRetired    bool

	timeouts         uint64
	pressureFlushes  uint64
	lifecycleFlushes uint64
	lateForks        uint64
	lastReport       time.Time
}

func newSyscallMessageOrderer(initial []processIdentity, emit func([]byte)) *syscallMessageOrderer {
	r := &syscallMessageOrderer{
		processes: make(map[processIdentity]*bufferedProcess),
		current:   make(map[uint32]processIdentity),
		emit:      emit,
	}
	for _, identity := range initial {
		if r.processes[identity] == nil {
			r.track(&bufferedProcess{identity: identity, ready: true})
		}
	}
	return r
}

func (r *syscallMessageOrderer) track(state *bufferedProcess) {
	r.processes[state.identity] = state
	r.current[state.identity.pid] = state.identity
	state.tracked = r.tracked.PushBack(state)
	// Exit notifications can also be lost under pressure. Bound all lifecycle
	// bookkeeping, not just payloads and missing-fork tombstones.
	for len(r.processes) > maxTrackedProcesses {
		oldest := r.tracked.Front().Value.(*bufferedProcess)
		if !oldest.ready {
			r.pressureFlushes++
			r.release(oldest, true)
		}
		r.retire(oldest)
	}
}

func (r *syscallMessageOrderer) retire(state *bufferedProcess) {
	r.retiredBefore = max(r.retiredBefore, state.identity.startTime)
	r.hasRetired = true
	r.forget(state)
}

func (r *syscallMessageOrderer) retired(identity processIdentity) bool {
	return r.hasRetired && identity.startTime <= r.retiredBefore
}

// The receiver keys its cache by PID, not starttime. Finish an older instance's
// stream before publishing the next one, even when reuse occurs within 1 s.
func (r *syscallMessageOrderer) activate(identity processIdentity) bool {
	previous, ok := r.current[identity.pid]
	if !ok || previous == identity {
		return true
	}
	if previous.startTime != 0 && identity.startTime != 0 && previous.startTime > identity.startTime {
		return false
	}
	if state := r.processes[previous]; state != nil {
		if !state.ready {
			r.lifecycleFlushes++
			r.release(state, true)
		}
		state.exited = true
		if state.missingFork {
			r.retainTombstone(state)
		} else {
			r.forget(state)
		}
	}
	return true
}

func (r *syscallMessageOrderer) push(msg outgoingSyscall, now time.Time) {
	r.expire(now)
	if !r.activate(msg.owner) {
		return
	}
	state := r.processes[msg.owner]
	if state == nil {
		if r.retired(msg.owner) {
			r.deliver(msg)
			return
		}
		state = &bufferedProcess{
			identity: msg.owner,
			deadline: msg.observed.Add(forkMessageTimeout),
		}
		state.pending = r.pending.PushBack(state)
		r.track(state)
	}
	if state.ready {
		r.deliver(msg)
		return
	}

	state.messages = append(state.messages, msg)
	state.bytes += len(msg.data)
	r.pendingMessages++
	r.pendingBytes += len(msg.data)
	if len(state.messages) >= maxProcessPendingMessages || state.bytes >= maxProcessPendingBytes {
		r.pressureFlushes++
		r.release(state, true)
	}
	for r.pendingMessages > maxPendingMessages || r.pendingBytes > maxPendingBytes {
		r.pressureFlushes++
		r.release(r.pending.Front().Value.(*bufferedProcess), true)
	}
	// Queueing must count against the one-second budget too.
	r.expire(now)
}

func (r *syscallMessageOrderer) deliver(msg outgoingSyscall) {
	if msg.kind == ebpfless.SyscallTypeFork {
		child := r.processes[msg.child]
		if child != nil && child.ready {
			// In particular, never send a fork after releasing its child's exec
			// on timeout: the resolver would replace the executing image.
			if child.missingFork {
				r.lateForks++
				child.missingFork = false
				if child.exited {
					r.forget(child)
				}
			}
			return
		}
		if previous, ok := r.current[msg.child.pid]; ok && previous != msg.child && (previous.startTime == 0 || msg.child.startTime == 0) {
			// If procfs identity collection failed, a changed key might be a
			// recovered identity rather than PID reuse. Preserve already
			// collected telemetry, but don't risk replacing its exec by fork.
			r.activate(msg.child)
			r.lateForks++
			return
		}
		if (child == nil && r.retired(msg.child)) || !r.activate(msg.child) {
			r.lateForks++
			return
		}
		r.emit(msg.data)
		if child == nil {
			r.track(&bufferedProcess{identity: msg.child, ready: true})
		} else {
			r.release(child, false)
		}
		return
	}

	r.emit(msg.data)
	if msg.kind == ebpfless.SyscallTypeExit {
		if state := r.processes[msg.owner]; state != nil {
			state.exited = true
			if state.missingFork {
				r.retainTombstone(state)
			} else {
				r.forget(state)
			}
		}
	}
}

func (r *syscallMessageOrderer) release(state *bufferedProcess, missingFork bool) {
	state.ready = true
	state.missingFork = missingFork
	if state.pending != nil {
		r.pending.Remove(state.pending)
		state.pending = nil
	}
	messages := state.messages
	r.pendingMessages -= len(messages)
	r.pendingBytes -= state.bytes
	state.messages = nil
	state.bytes = 0
	for _, msg := range messages {
		r.deliver(msg)
	}
}

func (r *syscallMessageOrderer) forget(state *bufferedProcess) {
	delete(r.processes, state.identity)
	if r.current[state.identity.pid] == state.identity {
		delete(r.current, state.identity.pid)
	}
	if state.tombstone != nil {
		r.tombstones.Remove(state.tombstone)
		state.tombstone = nil
	}
	if state.tracked != nil {
		r.tracked.Remove(state.tracked)
		state.tracked = nil
	}
}

func (r *syscallMessageOrderer) retainTombstone(state *bufferedProcess) {
	if state.tombstone == nil {
		state.tombstone = r.tombstones.PushBack(state)
	}
	for r.tombstones.Len() > maxMissingForkTombstones {
		oldest := r.tombstones.Front().Value.(*bufferedProcess)
		r.retire(oldest)
	}
}

func (r *syscallMessageOrderer) expire(now time.Time) {
	for r.pending.Len() > 0 {
		state := r.pending.Front().Value.(*bufferedProcess)
		if now.Before(state.deadline) {
			break
		}
		r.timeouts++
		r.release(state, true)
	}
}

func (r *syscallMessageOrderer) flush() {
	for r.pending.Len() > 0 {
		r.release(r.pending.Front().Value.(*bufferedProcess), true)
	}
}

func (r *syscallMessageOrderer) report(now time.Time) {
	if r.timeouts+r.pressureFlushes+r.lifecycleFlushes+r.lateForks == 0 || (!r.lastReport.IsZero() && now.Sub(r.lastReport) < time.Minute) {
		return
	}
	logger.Logf("process message ordering: %d fork timeouts, %d capacity flushes, %d PID reuse flushes, %d late forks suppressed", r.timeouts, r.pressureFlushes, r.lifecycleFlushes, r.lateForks)
	r.lastReport = now
}

// The timer runs independently of Wait4 and of network I/O. Emission is a
// nonblocking enqueue into the existing network queue, not a socket write.
type syscallMessageBuffer struct {
	input     chan outgoingSyscall
	done      chan struct{}
	closeOnce sync.Once
}

func newSyscallMessageBuffer(initial []processIdentity, emit func([]byte)) *syscallMessageBuffer {
	b := &syscallMessageBuffer{
		input: make(chan outgoingSyscall, 1024),
		done:  make(chan struct{}),
	}
	go b.run(newSyscallMessageOrderer(initial, emit))
	return b
}

func (b *syscallMessageBuffer) enqueue(msg outgoingSyscall) bool {
	select {
	case b.input <- msg:
		return true
	default:
		return false
	}
}

func (b *syscallMessageBuffer) run(r *syscallMessageOrderer) {
	defer close(b.done)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		var deadline <-chan time.Time
		if first := r.pending.Front(); first != nil {
			timer.Reset(time.Until(first.Value.(*bufferedProcess).deadline))
			deadline = timer.C
		} else {
			timer.Stop()
		}
		select {
		case msg, ok := <-b.input:
			if !ok {
				r.flush()
				r.report(time.Now())
				return
			}
			r.push(msg, time.Now())
		case <-deadline:
			r.expire(time.Now())
		}
		r.report(time.Now())
	}
}

func (b *syscallMessageBuffer) close() {
	b.closeOnce.Do(func() { close(b.input) })
	<-b.done
}
