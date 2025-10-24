// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
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
	service       string
	version       string
	environment   string
	exe           process.Executable
	interesting   bool
	gitInfo       process.GitInfo
	containerInfo process.ContainerInfo
}

type analysisResult struct {
	pid uint32
	err error
	processAnalysis
}

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
	// The set of processes that we have reported as alive, or that we are
	// currently analyzing.
	alive map[uint32]struct{}
	// The set of processes that we have queued for analysis (excluding
	// processes that are currently being analyzed).
	queued []uint32

	// True if we are currently analyzing a process.
	inFlight bool
	// True if the current process has issued an exec while it was being analyzed.
	needsReanalysis bool
	// The process that is currently being analyzed.
	currentProcess uint32 // 0 if no process is being analyzed

	// The processes that are in the current update being built but have
	// not yet been reported. This map should only ever contain processes
	// that are queued for analysis or in the update that is currently
	// being built.
	pending map[uint32]struct{}

	updates  []process.Info
	removals []process.ID
}

func makeState() state {
	return state{
		alive:   make(map[uint32]struct{}),
		pending: make(map[uint32]struct{}),
	}
}

func (s *state) handleAnalysisResult(e analysisResult) {
	s.inFlight, s.currentProcess = false, 0

	// If the current process has issued an exec while it was being analyzed,
	// we need to enqueue it again for analysis.
	if s.needsReanalysis {
		s.needsReanalysis = false
		if _, ok := s.alive[e.pid]; ok {
			s.queued = append(s.queued, e.pid)
		}
		return
	}
	if e.err != nil || !e.interesting {
		delete(s.alive, uint32(e.pid))
		delete(s.pending, uint32(e.pid))
	} else if _, ok := s.alive[e.pid]; ok {
		delete(s.pending, e.pid)
		s.updates = append(s.updates, process.Info{
			ProcessID: process.ID{
				PID: int32(e.pid),
			},
			Executable:  e.exe,
			Service:     e.service,
			Version:     e.version,
			Environment: e.environment,
			GitInfo:     e.gitInfo,
			Container:   e.containerInfo,
		})
	}
}

func (s *state) handleProcessEvent(e processEvent) {
	switch e.kind {
	case processEventKindExec:
		if _, ok := s.alive[e.pid]; !ok {
			s.alive[e.pid] = struct{}{}
			s.pending[e.pid] = struct{}{}
			s.queued = append(s.queued, e.pid)
		} else if s.inFlight && s.currentProcess == e.pid {
			s.needsReanalysis = true
		}
	case processEventKindExit:
		if _, ok := s.alive[e.pid]; ok {
			delete(s.alive, e.pid)
			if _, ok := s.pending[e.pid]; !ok {
				pid := process.ID{PID: int32(e.pid)}
				s.removals = append(s.removals, pid)
			}
		}
		delete(s.pending, e.pid)
	}
}

func (s *state) analyzeOrReport(eff effects) {
	for !s.inFlight && len(s.queued) > 0 {
		pid := s.queued[0]
		s.queued = s.queued[1:]
		if _, ok := s.alive[pid]; ok {
			s.inFlight = true
			s.currentProcess = pid
			eff.analyzeProcess(pid)
		}
	}

	shouldReport := !s.inFlight && len(s.queued) == 0 &&
		(len(s.updates) > 0 || len(s.removals) > 0)
	if !shouldReport {
		return
	}

	// Drop updates for processes that exited before we finished building.
	isDead := func(pid uint32) bool { _, ok := s.alive[pid]; return !ok }
	s.updates = slices.DeleteFunc(s.updates, func(u process.Info) bool {
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
