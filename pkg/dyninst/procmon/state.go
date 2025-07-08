// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"slices"
)

// TODO: We should add rate limiting on the actual rate at which we will
// analyze processes and the amount of time per unit time we'll spend
// doing it.

type processEventKind uint8

const (
	processEventKindExec processEventKind = iota
	processEventKindExit
)

type processEvent struct {
	kind processEventKind
	pid  uint32
}

type processAnalysis struct {
	service     string
	exe         Executable
	interesting bool
}

type analysisResult struct {
	pid uint32
	err error
	processAnalysis
}

type event interface{ event() }

func (e *processEvent) event()   {}
func (r *analysisResult) event() {}

// effects holds the side-effects the pure state machine can trigger.
// All its methods are synchronous from the point of view of Handle, so the
// heavy work (analyzeProcess) should start goroutines internally.
// None of these methods return values – new information must come back as
// events (processResult) posted through whatever channel the effects knows.

type effects interface {
	analyzeProcess(pid uint32)
	reportProcessesUpdate(update ProcessesUpdate)
}

type state struct {
	alive    map[uint32]struct{}
	queued   []uint32
	inFlight bool

	// The processes that are in the current update being built but have
	// not yet been reported.
	pending map[uint32]struct{}

	updates  []ProcessUpdate
	removals []ProcessID
}

func newState() *state {
	return &state{
		alive:   make(map[uint32]struct{}),
		pending: make(map[uint32]struct{}),
	}
}

func (s *state) handle(ev event, eff effects) {
	switch e := ev.(type) {
	case *processEvent:
		switch e.kind {
		case processEventKindExec:
			if _, ok := s.alive[e.pid]; !ok {
				s.alive[e.pid] = struct{}{}
				s.pending[e.pid] = struct{}{}
				s.queued = append(s.queued, e.pid)
			}
		case processEventKindExit:
			if _, ok := s.alive[e.pid]; ok {
				delete(s.alive, e.pid)
				if _, ok := s.pending[e.pid]; !ok {
					pid := ProcessID{PID: int32(e.pid)}
					s.removals = append(s.removals, pid)
				}
			}
		}
	case *analysisResult:
		s.inFlight = false
		if e.err != nil || !e.interesting {
			delete(s.alive, uint32(e.pid))
		} else {
			s.updates = append(s.updates, ProcessUpdate{
				ProcessID: ProcessID{
					PID:     int32(e.pid),
					Service: e.service,
				},
				Executable: e.exe,
			})
		}
	}

	// Start the next analysis if idle.
	for !s.inFlight && len(s.queued) > 0 {
		pid := s.queued[0]
		s.queued = s.queued[1:]
		if _, ok := s.alive[pid]; ok {
			s.inFlight = true
			eff.analyzeProcess(pid)
		}
	}

	shouldReport := !s.inFlight && len(s.queued) == 0 &&
		(len(s.updates) > 0 || len(s.removals) > 0)
	if !shouldReport {
		return
	}

	clear(s.pending)

	// Drop updates for processes that exited before we finished building.
	isDead := func(pid uint32) bool { _, ok := s.alive[pid]; return !ok }
	s.updates = slices.DeleteFunc(s.updates, func(u ProcessUpdate) bool {
		return isDead(uint32(u.ProcessID.PID))
	})
	if len(s.updates) > 0 || len(s.removals) > 0 {
		report := ProcessesUpdate{
			Processes: s.updates,
			Removals:  s.removals,
		}
		s.updates = nil
		s.removals = nil
		eff.reportProcessesUpdate(report)
	}
}
