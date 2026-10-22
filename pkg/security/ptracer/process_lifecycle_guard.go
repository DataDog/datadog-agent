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
)

// processIdentity identifies a process generation, not an individual thread.
// All TIDs in a group use /proc/<tgid>/stat's starttime, in clock ticks since
// boot. A zero startTime means procfs was unavailable; PID reuse is then ambiguous.
// This identity stays local to the ptracer and does not change the wire protocol.
type processIdentity struct {
	tgid      uint32
	startTime uint64
}

type serializedSyscall struct {
	owner   processIdentity // emitting process, or the parent for a fork notification
	child   processIdentity // created process; only used for fork notifications
	kind    ebpfless.SyscallType
	payload []byte
}

type processLifecycleState struct {
	identity       processIdentity
	missingFork    bool // telemetry started, but its creation notification is still unseen
	exited         bool
	tombstoneEntry *list.Element
	trackingEntry  *list.Element
}

// processLifecycleGuard filters late forks and stale PID generations on the
// tracing thread. It stores only lifecycle metadata: it never buffers, sorts or
// delays telemetry, and is not accessed by the network or procfs goroutines.
//
// childCreationWaiter establishes fork-before-child ordering in the normal case.
// If that wait expires, child events flow immediately; a subsequent fork must not
// overwrite an already reported exec or resurrect the child after its exit.
type processLifecycleGuard struct {
	states         map[processIdentity]*processLifecycleState
	currentByTGID  map[uint32]processIdentity
	exitTombstones list.List
	trackingOrder  list.List
	enqueue        func([]byte)

	// Once a bounded history entry is evicted, its exact identity is lost.
	// Conservatively suppress untracked forks at or below this start-time
	// cutoff; ordinary syscall telemetry must still be forwarded.
	evictedStartTimeCutoff uint64
	hasEvictedStates       bool
	suppressedLateForks    uint64
	lastReport             time.Time
}

func newProcessLifecycleGuard(initial []processIdentity, enqueue func([]byte)) *processLifecycleGuard {
	g := &processLifecycleGuard{
		states:        make(map[processIdentity]*processLifecycleState),
		currentByTGID: make(map[uint32]processIdentity),
		enqueue:       enqueue,
	}
	for _, identity := range initial {
		if g.states[identity] == nil {
			// Wrapped/attached roots have no creation notification to expect.
			g.track(&processLifecycleState{identity: identity})
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

func (g *processLifecycleGuard) filterAndEnqueue(msg serializedSyscall) {
	if !g.acceptGeneration(msg.owner) {
		return
	}
	if g.states[msg.owner] == nil && !g.mayHaveBeenEvicted(msg.owner) {
		g.track(&processLifecycleState{identity: msg.owner, missingFork: true})
	}
	if msg.kind == ebpfless.SyscallTypeFork {
		child := g.states[msg.child]
		if child != nil {
			// Either a prior fork registered this child, or it already emitted
			// telemetry after a timeout. Do not replace its current image.
			if child.missingFork {
				g.suppressedLateForks++
				child.missingFork = false
				if child.exited {
					g.forget(child)
				}
			}
			return
		}
		if previous, ok := g.currentByTGID[msg.child.tgid]; ok && previous != msg.child && (previous.startTime == 0 || msg.child.startTime == 0) {
			// Procfs recovered (or lost) the birth identity since exec. Without
			// two known start times we cannot distinguish this from PID reuse;
			// preserve the exec rather than risk overwriting it with a fork.
			g.acceptGeneration(msg.child)
			g.suppressedLateForks++
			return
		}
		if g.mayHaveBeenEvicted(msg.child) || !g.acceptGeneration(msg.child) {
			g.suppressedLateForks++
			return
		}
		g.enqueue(msg.payload)
		g.track(&processLifecycleState{identity: msg.child})
		return
	}

	// No second wait after the ptrace deadline: even an exec without a fork
	// is enqueued immediately and can use the resolver's PPID fallback.
	g.enqueue(msg.payload)
	if msg.kind == ebpfless.SyscallTypeExit {
		if state := g.states[msg.owner]; state != nil {
			state.exited = true
			if state.missingFork {
				g.retainExitTombstone(state)
			} else {
				g.forget(state)
			}
		}
	}
}

func (g *processLifecycleGuard) forget(state *processLifecycleState) {
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

func (g *processLifecycleGuard) reportSuppressedForks() {
	if g.suppressedLateForks == 0 {
		return
	}
	now := time.Now()
	if !g.lastReport.IsZero() && now.Sub(g.lastReport) < time.Minute {
		return
	}
	logger.Logf("process message ordering: %d late forks suppressed", g.suppressedLateForks)
	g.lastReport = now
}
