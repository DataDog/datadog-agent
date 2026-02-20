// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func validateState(s *state, reportError func(error)) {
	report := func(format string, args ...any) {
		reportError(fmt.Errorf(format, args...))
	}
	for procID, proc := range s.processes {
		if procID != proc.processID {
			report("process %v has mismatched ID field %v", procID, proc.processID)
		}
		validateProcess(proc, s, report)
	}

	for progID, prog := range s.programs {
		if progID != prog.id {
			report("program %v has mismatched ID field %v", progID, prog.id)
		}
		validateProgram(prog, s, report)
	}

	// Verify queue integrity.
	queuedPrograms := make(map[ir.ProgramID]bool)
	for progID := range s.queuedLoading.m {
		queuedPrograms[progID] = true

		prog, exists := s.programs[progID]
		if !exists {
			report("queue contains non-existent program %v", progID)
		} else if prog.state != programStateQueued {
			report(
				"queued program %v is not in Queued state: %v",
				progID, prog.state,
			)
		}
	}

	// Verify that queued programs are actually in the queue.
	for progID, prog := range s.programs {
		if prog.state == programStateQueued {
			if !queuedPrograms[progID] {
				report("program %v is in Queued state but not in queue", progID)
			}
		}
	}

	// Verify currentlyCompiling consistency.
	if s.currentlyLoading != nil {
		progID := s.currentlyLoading.id
		if prog, exists := s.programs[progID]; !exists {
			report("currentlyLoading references non-existent program %v", progID)
		} else if prog != s.currentlyLoading {
			report(
				"currentlyLoading points to program instance %v in programs map",
				progID,
			)
		}

		// Currently compiling program should not be in queue.
		if queuedPrograms[progID] {
			report("currentlyCompiling program %v is also in queue", progID)
		}

		// Should be in an active compilation state.
		switch s.currentlyLoading.state {
		case programStateLoading,
			programStateLoadingAborted:
			// Valid states for currently loading.
		default:
			report("currentlyLoading program %v is in unexpected state %v",
				progID, s.currentlyLoading.state)
		}
	}

	// Verify that Loading/LoadingAborted programs are only currentlyLoading.
	for progID, prog := range s.programs {
		switch prog.state {
		case programStateLoading, programStateLoadingAborted:
			if s.currentlyLoading == nil || s.currentlyLoading.id != progID {
				report(
					"program %v is in %v state but is not currentlyLoading",
					progID, prog.state,
				)
			}
		}
	}

	// Verify no two programs claim the same process.
	progByProcess := make(map[ProcessID]ir.ProgramID)
	for progID, prog := range s.programs {
		if prev, exists := progByProcess[prog.processID]; exists {
			report(
				"programs %v and %v both claim process %v",
				prev, progID, prog.processID,
			)
		}
		progByProcess[prog.processID] = progID
	}

	// Verify no empty inner maps in processesByService.
	for svc, pids := range s.processesByService {
		if len(pids) == 0 {
			report("processesByService[%q] is empty", svc)
		}
	}

	// Verify discoveredTypes values are sorted and deduplicated.
	{
		computedTotal := 0
		for svc, types := range s.discoveredTypes {
			if !slices.IsSorted(types) {
				report("discoveredTypes[%q] is not sorted: %v", svc, types)
			}
			if len(slices.Compact(slices.Clone(types))) != len(types) {
				report("discoveredTypes[%q] has duplicates: %v", svc, types)
			}
			computedTotal += len(types)
		}
		if computedTotal != s.totalDiscoveredTypes {
			report(
				"totalDiscoveredTypes counter %d does not match computed total %d",
				s.totalDiscoveredTypes, computedTotal,
			)
		}
	}

	// Verify that when the limit is exceeded, all discoveredTypes entries
	// belong to services with live processes.
	if s.totalDiscoveredTypes > s.discoveredTypesLimit {
		for svc := range s.discoveredTypes {
			if _, hasProcesses := s.processesByService[svc]; !hasProcesses {
				report(
					"discoveredTypes[%q] exists with no live processes while over limit (%d > %d)",
					svc, s.totalDiscoveredTypes, s.discoveredTypesLimit,
				)
			}
		}
	}

	// Verify processesByService index integrity.
	// Every PID in processesByService[svc] must exist in s.processes with matching service.
	for svc, pids := range s.processesByService {
		for pid := range pids {
			proc, exists := s.processes[pid]
			if !exists {
				report("processesByService[%q] contains non-existent process %v", svc, pid)
			} else if proc.service != svc {
				report(
					"processesByService[%q] contains process %v with service %q",
					svc, pid, proc.service,
				)
			}
		}
	}
	// Every process with a non-empty service must appear in processesByService.
	for pid, proc := range s.processes {
		if proc.service == "" {
			continue
		}
		pids, exists := s.processesByService[proc.service]
		if !exists {
			report("process %v with service %q not in processesByService", pid, proc.service)
		} else if _, ok := pids[pid]; !ok {
			report("process %v not in processesByService[%q]", pid, proc.service)
		}
	}

	// Verify process-program relationships are bidirectional.
	for procID, proc := range s.processes {
		if proc.currentProgram != 0 {
			prog, exists := s.programs[proc.currentProgram]
			if !exists {
				report(
					"process %v currentProgram %v does not exist",
					procID, proc.currentProgram,
				)
			} else if prog.processID != procID {
				report(
					"process %v currentProgram %v points to different process %v",
					procID, proc.currentProgram, prog.processID,
				)
			}
		}
	}
}

func validateProcess(proc *process, s *state, report func(format string, args ...any)) {
	procID := proc.processID

	switch proc.state {
	case processStateWaitingForProgram:
		if proc.currentProgram == 0 {
			report(
				"process %v in WaitingForProgram state has no currentProgram",
				procID,
			)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procID, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil {
			report(
				"process %v in WaitingForProgram state should not have attachedProgram",
				procID,
			)
		}

	case processStateAttaching:
		if proc.currentProgram == 0 {
			report(
				"process %v in Attaching state has no currentProgram",
				procID,
			)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procID, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil {
			report(
				"process %v in Attaching state should not have attachedProgram yet",
				procID,
			)
		}

	case processStateAttached:
		if proc.currentProgram == 0 {
			report("process %v in Attached state has no currentProgram", procID)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procID, proc.currentProgram,
			)
		}
		if proc.attachedProgram == nil {
			report(
				"process %v in Attached state has no attachedProgram", procID,
			)
		}
		if proc.attachedProgram != nil &&
			proc.attachedProgram.programID != proc.currentProgram {
			report(
				"process %v attachedProgram ID %v does not match currentProgram %v",
				procID, proc.attachedProgram.programID, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil &&
			proc.attachedProgram.processID != procID {
			report(
				"process %v attachedProgram has wrong processID %v",
				procID, proc.attachedProgram.processID,
			)
		}

	case processStateDetaching:
		if proc.currentProgram == 0 {
			report("process %v in Detaching state has no currentProgram", procID)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procID, proc.currentProgram,
			)
		}

	case processStateFailed:
		if len(proc.probes) == 0 {
			report("process %v has no probes in Failed state", procID)
		}

	case processStateInvalid:
		// This state should not normally appear in a valid state.
		report("process %v is in Invalid state", procID)

	default:
		report("process %v has unknown state %v", procID, proc.state)
	}
}

func validateProgram(
	prog *program, s *state, report func(format string, args ...any),
) {
	progID := prog.id

	// Check that processID references exist and are consistent.
	proc, exists := s.processes[prog.processID]
	if !exists {
		report(
			"program %v references non-existent process %v", progID, prog.processID,
		)
	} else if proc.currentProgram != progID {
		report(
			"program %v is not the current program for process %v",
			progID, prog.processID,
		)
	}

	// Programs should always have at least one probe.
	if len(prog.config) == 0 {
		report("program %v has no probes", progID)
	}

	switch prog.state {
	case programStateQueued:
		if prog.loaded != nil {
			report("program %v in Queued state should not have loadedProgram", progID)
		}

	case programStateLoading:
		if prog.loaded != nil {
			report(
				"program %v in Loading state should not have loadedProgram yet",
				progID,
			)
		}

	case programStateLoaded:
		if prog.loaded == nil {
			report(
				"program %v in Loaded state should have loadedProgram", progID,
			)
		}
		if prog.loaded != nil && prog.loaded.programID != progID {
			report(
				"program %v has loadedProgram with mismatched ID %v",
				progID, prog.loaded.programID,
			)
		}

	case programStateDraining,
		programStateUnloading,
		programStateLoadingAborted:
		// Transitional state, can have various field combinations.

	case programStateInvalid:
		report("program %v is in Invalid state", progID)

	default:
		report("program %v has unknown state %v", progID, prog.state)
	}

	// Cross-check process and program state compatibility.
	if exists {
		switch prog.state {
		case programStateQueued, programStateLoading, programStateLoadingAborted:
			if proc.state != processStateWaitingForProgram {
				report(
					"program %v in %v state but process %v in %v state (expected WaitingForProgram)",
					progID, prog.state, proc.processID, proc.state,
				)
			}
		case programStateLoaded:
			switch proc.state {
			case processStateAttaching, processStateAttached, processStateDetaching:
				// Valid.
			default:
				report(
					"program %v in Loaded state but process %v in %v state (expected Attaching/Attached/Detaching)",
					progID, proc.processID, proc.state,
				)
			}
		case programStateDraining:
			switch proc.state {
			case processStateDetaching, processStateFailed:
				// Valid: Detaching is the normal path; Failed happens
				// when the circuit breaker triggers while attached.
			default:
				report(
					"program %v in Draining state but process %v in %v state (expected Detaching/Failed)",
					progID, proc.processID, proc.state,
				)
			}
		case programStateUnloading:
			switch proc.state {
			case processStateWaitingForProgram, processStateDetaching, processStateFailed:
				// Valid.
			default:
				report(
					"program %v in Unloading state but process %v in %v state",
					progID, proc.processID, proc.state,
				)
			}
		}
	}

	// needsRecompilation should only be set for Loading or Loaded programs.
	if prog.needsRecompilation {
		switch prog.state {
		case programStateLoading, programStateLoaded:
			// Valid.
		default:
			report(
				"program %v has needsRecompilation=true in state %v",
				progID, prog.state,
			)
		}
	}
}
