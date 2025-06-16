// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func validateState(s *state, reportError func(error)) {
	report := func(format string, args ...any) {
		reportError(fmt.Errorf(format, args...))
	}
	for procID, proc := range s.processes {
		if procID != proc.id {
			report("process %v has mismatched ID field %v", procID, proc.id)
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
	for progID := range s.queuedCompilations.m {
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
	if s.currentlyCompiling != nil {
		progID := s.currentlyCompiling.id
		if prog, exists := s.programs[progID]; !exists {
			report("currentlyCompiling references non-existent program %v", progID)
		} else if prog != s.currentlyCompiling {
			report(
				"currentlyCompiling points to program instance %v in programs map",
				progID,
			)
		}

		// Currently compiling program should not be in queue.
		if queuedPrograms[progID] {
			report("currentlyCompiling program %v is also in queue", progID)
		}

		// Should be in an active compilation state.
		switch s.currentlyCompiling.state {
		case programStateCompiling, programStateLoading,
			programStateCompilationAborted:
			// Valid states for currently compiling.
		default:
			report("currentlyCompiling program %v is in unexpected state %v",
				progID, s.currentlyCompiling.state)
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
			} else if prog.processID == nil {
				report(
					"process %v currentProgram %v has nil processID",
					procID, proc.currentProgram,
				)
			} else if *prog.processID != procID {
				report(
					"process %v currentProgram %v points to different process %v",
					procID, proc.currentProgram, *prog.processID,
				)
			}
		}
	}
}

func validateProcess(proc *process, s *state, report func(format string, args ...any)) {
	procID := proc.id

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
			proc.attachedProgram.progID != proc.currentProgram {
			report(
				"process %v attachedProgram ID %v does not match currentProgram %v",
				procID, proc.attachedProgram.progID, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil &&
			proc.attachedProgram.procID != procID {
			report(
				"process %v attachedProgram has wrong processID %v",
				procID, proc.attachedProgram.procID,
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

	case processStateCompilationFailed:
		// currentProgram may be 0 after failure.
		if proc.err == nil {
			report("process %v in CompilationFailed state has no error", procID)
		}
		if len(proc.probes) == 0 {
			report("process %v has no probes in CompilationFailed state", procID)
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
	if procID := prog.processID; procID != nil {
		proc, exists := s.processes[*procID]
		if !exists {
			report(
				"program %v references non-existent process %v", progID, *procID,
			)
		} else if proc.currentProgram != progID {
			report(
				"program %v is not the current program for process %v",
				progID, *procID,
			)
		}
	} else {
		switch prog.state {
		case programStateCompilationAborted, programStateDraining:
			// Valid states for programs with no process.
		default:
			report(
				"program %v has no process but is not in a cleanup state (%v)",
				progID, prog.state,
			)
		}
	}

	switch prog.state {
	case programStateQueued:
		if prog.compiledProgram != nil {
			report("program %v in Queued state should not have IR", progID)
		}
		if prog.loadedProgram != nil {
			report("program %v in Queued state should not have loadedProgram", progID)
		}

	case programStateCompiling:
		if prog.compiledProgram != nil {
			report("program %v in Compiling state should not have compiledProgram", progID)
		}
		if prog.loadedProgram != nil {
			report(
				"program %v in Compiling state should not have loadedProgram yet",
				progID,
			)
		}

	case programStateLoading:
		if prog.compiledProgram == nil {
			report("program %v in Loading state should have IR", progID)
		}
		if prog.loadedProgram != nil {
			report(
				"program %v in Loading state should not have loadedProgram yet",
				progID,
			)
		}

	case programStateLoaded:
		if prog.compiledProgram == nil {
			report("program %v in Loaded state should have IR", progID)
		}
		if prog.loadedProgram == nil {
			report(
				"program %v in Loaded state should have loadedProgram", progID,
			)
		}
		if prog.loadedProgram != nil && prog.loadedProgram.id != progID {
			report(
				"program %v has loadedProgram with mismatched ID %v",
				progID,
				prog.loadedProgram.id,
			)
		}

	case programStateDraining,
		programStateCompilationAborted:
		// Transitional state, can have various field combinations.

	case programStateInvalid:
		report("program %v is in Invalid state", progID)

	default:
		report("program %v has unknown state %v", progID, prog.state)
	}
}
