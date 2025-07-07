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
		if procID != proc.processKey {
			report("process %v has mismatched ID field %v", procID, proc.processKey)
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

	// Verify process-program relationships are bidirectional.
	for procID, proc := range s.processes {
		if proc.currentProgram != 0 {
			prog, exists := s.programs[proc.currentProgram]
			if !exists {
				report(
					"process %v currentProgram %v does not exist",
					procID, proc.currentProgram,
				)
			} else if prog.processKey != procID {
				report(
					"process %v currentProgram %v points to different process %v",
					procID, proc.currentProgram, prog.processKey,
				)
			}
		}
	}
}

func validateProcess(proc *process, s *state, report func(format string, args ...any)) {
	procKey := proc.processKey

	switch proc.state {
	case processStateWaitingForProgram:
		if proc.currentProgram == 0 {
			report(
				"process %v in WaitingForProgram state has no currentProgram",
				procKey,
			)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procKey, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil {
			report(
				"process %v in WaitingForProgram state should not have attachedProgram",
				procKey,
			)
		}

	case processStateAttaching:
		if proc.currentProgram == 0 {
			report(
				"process %v in Attaching state has no currentProgram",
				procKey,
			)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procKey, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil {
			report(
				"process %v in Attaching state should not have attachedProgram yet",
				procKey,
			)
		}

	case processStateAttached:
		if proc.currentProgram == 0 {
			report("process %v in Attached state has no currentProgram", procKey)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procKey, proc.currentProgram,
			)
		}
		if proc.attachedProgram == nil {
			report(
				"process %v in Attached state has no attachedProgram", procKey,
			)
		}
		if proc.attachedProgram != nil &&
			proc.attachedProgram.ir.ID != proc.currentProgram {
			report(
				"process %v attachedProgram ID %v does not match currentProgram %v",
				procKey, proc.attachedProgram.ir.ID, proc.currentProgram,
			)
		}
		if proc.attachedProgram != nil &&
			proc.attachedProgram.procID != procKey.ProcessID {
			report(
				"process %v attachedProgram has wrong processID %v",
				procKey, proc.attachedProgram.procID,
			)
		}

	case processStateDetaching:
		if proc.currentProgram == 0 {
			report("process %v in Detaching state has no currentProgram", procKey)
		}
		if _, exists := s.programs[proc.currentProgram]; !exists {
			report(
				"process %v references non-existent program %v",
				procKey, proc.currentProgram,
			)
		}

	case processStateLoadingFailed:
		// currentProgram may be 0 after failure.
		if proc.err == nil {
			report("process %v in LoadingFailed state has no error", procKey)
		}
		if len(proc.probes) == 0 {
			report("process %v has no probes in LoadingFailed state", procKey)
		}

	case processStateInvalid:
		// This state should not normally appear in a valid state.
		report("process %v is in Invalid state", procKey)

	default:
		report("process %v has unknown state %v", procKey, proc.state)
	}
}

func validateProgram(
	prog *program, s *state, report func(format string, args ...any),
) {
	progID := prog.id

	// Check that processID references exist and are consistent.
	proc, exists := s.processes[prog.processKey]
	if !exists {
		report(
			"program %v references non-existent process %v", progID, prog.processKey,
		)
	} else if proc.currentProgram != progID {
		report(
			"program %v is not the current program for process %v",
			progID, prog.ProcessID,
		)
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
		if prog.loaded != nil && prog.loaded.ir.ID != progID {
			report(
				"program %v has loadedProgram with mismatched ID %v",
				progID, prog.loaded.ir.ID,
			)
		}

	case programStateDraining,
		programStateLoadingAborted:
		// Transitional state, can have various field combinations.

	case programStateInvalid:
		report("program %v is in Invalid state", progID)

	default:
		report("program %v has unknown state %v", progID, prog.state)
	}
}
