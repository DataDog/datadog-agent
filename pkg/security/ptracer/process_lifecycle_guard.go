// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"container/list"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

const (
	maxMissingForkTombstones = 4096
	maxLifecycleStates       = 16384

	// Bound the encoded size of pending metadata independently of lifecycle history.
	maxPendingContextBytes = 4 * 1024 * 1024
)

// processIdentity identifies a process generation, not an individual thread.
// All TIDs in a group use /proc/<tgid>/stat's starttime, in clock ticks since
// boot. A zero startTime means procfs was unavailable; PID reuse is then ambiguous.
// The guard uses this local key; the birth stamp also accompanies syscall frames.
type processIdentity struct {
	tgid      uint32
	startTime uint64
}

type serializedSyscall struct {
	owner      processIdentity // emitting process, or the parent for a fork notification
	child      processIdentity // created process; only used for fork notifications
	kind       ebpfless.SyscallType
	hasContext bool // payload initializes its owner's image and credentials
	payload    []byte
}

type processLifecycleState struct {
	identity         processIdentity
	missingFork      bool // telemetry started, but its creation notification is still unseen
	contextAttempted bool // at most one procfs lookup per process generation, including failures
	contextAnnounced bool // an initializing fork, exec or snapshot reached the send queue
	contextPPID      uint32
	pendingContext   *ebpfless.SyscallMsg // metadata only, retained until successfully enqueued
	pendingSize      int                  // encoded size, accounting for the pending metadata budget
	pendingEntry     *list.Element
	exited           bool
	tombstoneEntry   *list.Element
	trackingEntry    *list.Element
}

// processLifecycleGuard filters late forks and stale PID generations on the
// tracing thread. It retains lifecycle state and undelivered procfs metadata,
// never event payloads: it does not buffer, sort or delay telemetry, and is not
// accessed by the network or procfs goroutines.
//
// childCreationWaiter establishes fork-before-child ordering in the normal case.
// If that wait expires, child events flow immediately; a subsequent fork must not
// overwrite an already reported exec or resurrect the child after its exit.
type processLifecycleGuard struct {
	states         map[processIdentity]*processLifecycleState
	currentByTGID  map[uint32]processIdentity
	exitTombstones list.List
	trackingOrder  list.List
	enqueue        func([]byte) bool

	pendingContexts     list.List // least recently used process metadata first
	pendingContextBytes int

	// Once a bounded history entry is evicted, its exact identity is lost.
	// Conservatively suppress untracked forks at or below this start-time
	// cutoff; ordinary syscall telemetry must still be forwarded.
	evictedStartTimeCutoff  uint64
	hasEvictedStates        bool
	suppressedLateForks     uint64
	contextFallbacks        uint64
	contextFallbackFailures uint64
	contextCacheEvictions   uint64
	lastReport              time.Time
}

func newProcessLifecycleGuard(initial []processIdentity, enqueue func([]byte) bool) *processLifecycleGuard {
	g := &processLifecycleGuard{
		states:        make(map[processIdentity]*processLifecycleState),
		currentByTGID: make(map[uint32]processIdentity),
		enqueue:       enqueue,
	}
	for _, identity := range initial {
		if g.states[identity] == nil {
			// Wrapped/attached roots have no creation notification to expect.
			g.track(&processLifecycleState{identity: identity, contextAnnounced: true})
		}
	}
	return g
}

func (g *processLifecycleGuard) track(state *processLifecycleState) {
	g.states[state.identity] = state
	g.currentByTGID[state.identity.tgid] = state.identity
	state.trackingEntry = g.trackingOrder.PushBack(state)
	for len(g.states) > maxLifecycleStates {
		g.evict(g.trackingOrder.Front().Value.(*processLifecycleState))
	}
}

func (g *processLifecycleGuard) evict(state *processLifecycleState) {
	g.evictedStartTimeCutoff = max(g.evictedStartTimeCutoff, state.identity.startTime)
	g.hasEvictedStates = true
	g.forget(state)
}

func (g *processLifecycleGuard) mayHaveBeenEvicted(identity processIdentity) bool {
	return g.hasEvictedStates && identity.startTime <= g.evictedStartTimeCutoff
}

// acceptGeneration rejects old events once the same TGID belongs to a newer
// process. Keep an old missing-fork tombstone until its late notification arrives.
func (g *processLifecycleGuard) acceptGeneration(identity processIdentity) bool {
	previous, ok := g.currentByTGID[identity.tgid]
	if !ok || previous == identity {
		return true
	}
	if previous.startTime != 0 && identity.startTime != 0 && previous.startTime > identity.startTime {
		return false
	}
	if state := g.states[previous]; state != nil {
		g.clearPendingContext(state)
		state.exited = true
		if state.missingFork {
			g.retainExitTombstone(state)
		} else {
			g.forget(state)
		}
	}
	return true
}

// shouldIgnoreCreation also guards ProcessCache reconstruction, before any
// inherited resources are applied. Its history can outlive the cached Process.
func (g *processLifecycleGuard) shouldIgnoreCreation(tid int, identity processIdentity) bool {
	// New threads share their TGID's identity, even when that process's
	// telemetry record has been evicted. They still need initialization.
	if uint32(tid) != identity.tgid {
		return false
	}
	if state := g.states[identity]; state != nil {
		return state.exited
	}
	if previous, ok := g.currentByTGID[identity.tgid]; ok && (previous.startTime == 0 || identity.startTime == 0) {
		if state := g.states[previous]; state != nil {
			return state.exited
		}
	}
	return g.mayHaveBeenEvicted(identity)
}

// beginProcessContext reserves the first-event procfs lookup for an unknown
// process generation. A new TID in an already known process needs no snapshot.
// Failed lookups are not retried on every syscall, and evicted history stays
// conservative rather than turning into repeated procfs reads on the hot path.
func (g *processLifecycleGuard) beginProcessContext(identity processIdentity) bool {
	if !g.acceptGeneration(identity) {
		return false
	}
	state := g.states[identity]
	if state == nil {
		if g.mayHaveBeenEvicted(identity) {
			return false
		}
		state = &processLifecycleState{identity: identity, missingFork: true}
		g.track(state)
	}
	if state.contextAnnounced || state.contextAttempted || state.exited {
		return false
	}
	state.contextAttempted = true
	return true
}

func (g *processLifecycleGuard) processPPID(identity processIdentity) uint32 {
	if state := g.states[identity]; state != nil {
		return state.contextPPID
	}
	return 0
}

func (g *processLifecycleGuard) filterAndEnqueue(msg serializedSyscall) {
	if g.prepareSyscall(msg) {
		g.enqueuePrepared(msg)
	}
}

// prepareSyscall filters lifecycle notifications before context collection or
// serialization. Suppressed forks must not consume their owner's first snapshot.
func (g *processLifecycleGuard) prepareSyscall(msg serializedSyscall) bool {
	if !g.acceptGeneration(msg.owner) {
		return false
	}
	if g.states[msg.owner] == nil && !g.mayHaveBeenEvicted(msg.owner) {
		g.track(&processLifecycleState{identity: msg.owner, missingFork: true})
	}
	if msg.kind != ebpfless.SyscallTypeFork {
		return true
	}
	if child := g.states[msg.child]; child != nil {
		// A snapshot can establish context without exec. Even if procfs
		// failed, do not overwrite changes already reported by ordinary
		// events (for example setuid), or resurrect an exited process.
		if child.missingFork {
			g.suppressedLateForks++
			child.missingFork = false
			if child.exited {
				g.forget(child)
			}
		}
		return false
	}
	if previous, ok := g.currentByTGID[msg.child.tgid]; ok && previous != msg.child && (previous.startTime == 0 || msg.child.startTime == 0) {
		// Procfs recovered (or lost) the birth identity since exec. Without
		// two known start times we cannot distinguish this from PID reuse;
		// preserve the exec rather than risk overwriting it with a fork.
		g.acceptGeneration(msg.child)
		g.suppressedLateForks++
		return false
	}
	if g.mayHaveBeenEvicted(msg.child) || !g.acceptGeneration(msg.child) {
		g.suppressedLateForks++
		return false
	}
	return true
}

// enqueuePrepared never waits after the ptrace deadline. Only successful queue
// insertions announce context; lifecycle history still forbids late forks even
// when the queue dropped an event's frame. The result lets the caller retain
// collected metadata without repeating the procfs lookup or buffering an event.
func (g *processLifecycleGuard) enqueuePrepared(msg serializedSyscall) bool {
	enqueued := g.enqueue(msg.payload)
	state := g.states[msg.owner]
	if state != nil && enqueued && (msg.hasContext || msg.kind == ebpfless.SyscallTypeExec) {
		state.contextAnnounced = true
		g.clearPendingContext(state)
	}
	if msg.kind == ebpfless.SyscallTypeFork {
		child := g.states[msg.child]
		if child == nil {
			child = &processLifecycleState{identity: msg.child}
			g.track(child)
		}
		child.missingFork = false
		child.contextAnnounced = enqueued
		return enqueued
	}
	if msg.kind == ebpfless.SyscallTypeExit && state != nil {
		g.clearPendingContext(state)
		state.exited = true
		if state.missingFork {
			g.retainExitTombstone(state)
		} else {
			g.forget(state)
		}
	}
	return enqueued
}

func (g *processLifecycleGuard) forget(state *processLifecycleState) {
	g.clearPendingContext(state)
	delete(g.states, state.identity)
	if g.currentByTGID[state.identity.tgid] == state.identity {
		delete(g.currentByTGID, state.identity.tgid)
	}
	if state.tombstoneEntry != nil {
		g.exitTombstones.Remove(state.tombstoneEntry)
		state.tombstoneEntry = nil
	}
	if state.trackingEntry != nil {
		g.trackingOrder.Remove(state.trackingEntry)
		state.trackingEntry = nil
	}
}

func (g *processLifecycleGuard) retainExitTombstone(state *processLifecycleState) {
	// ProcessCache removes exited children, but the creator can still report
	// their fork later. Retain metadata only, with a separate bound for exits.
	if state.tombstoneEntry == nil {
		state.tombstoneEntry = g.exitTombstones.PushBack(state)
	}
	for g.exitTombstones.Len() > maxMissingForkTombstones {
		g.evict(g.exitTombstones.Front().Value.(*processLifecycleState))
	}
}

func (g *processLifecycleGuard) reportStats() {
	if g.suppressedLateForks == 0 && g.contextFallbacks == 0 && g.contextFallbackFailures == 0 && g.contextCacheEvictions == 0 {
		return
	}
	now := time.Now()
	if !g.lastReport.IsZero() && now.Sub(g.lastReport) < time.Minute {
		return
	}
	logger.Logf("process message ordering: %d late forks suppressed, %d procfs contexts collected, %d context lookup failures, %d pending contexts evicted",
		g.suppressedLateForks, g.contextFallbacks, g.contextFallbackFailures, g.contextCacheEvictions)
	g.lastReport = now
}
