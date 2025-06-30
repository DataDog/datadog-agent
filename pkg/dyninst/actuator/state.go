// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
)

// state represents the state of an Actuator.
//
// This is an event-driven state machine that manages dynamic instrumentation
// for processes by coordinating IR generation, eBPF compilation, program loading,
// and process attachment. The state machine processes events sequentially in a
// dedicated goroutine to maintain consistency.
//
// The instigating event is ProcessesUpdate, which informs the actuator of the
// probes intended for processes. This triggers a pipeline of asynchronous
// operations coordinated through effects and completion events.
//
// The state machine manages processes (with states like WaitingForProgram, Attaching, Attached)
// and programs (with states like Queued, GeneratingIR, Compiling, Loading, Loaded).
//
// State transitions emit effects that execute asynchronously. The effectHandler interface
// defines all available effects that coordinate the instrumentation pipeline from IR
// generation through process attachment.
//
// Current implementation choices:
// - Each process gets its own program (no batching by executable)
// - Process updates trigger full program replacement (no incremental updates)
// - No explicit caching of object file data (relies on OS page cache)
// - Single-threaded event processing ensures consistency
type state struct {
	programIDAlloc ir.ProgramID

	processes map[ProcessID]*process
	programs  map[ir.ProgramID]*program

	queuedCompilations queue[*program, ir.ProgramID]
	currentlyCompiling *program

	// If true, the state machine is shutting down.
	shuttingDown bool
}

// isShutdown returns true if the state machine is fully shut down.
func (s *state) isShutdown() bool {
	return s.shuttingDown &&
		s.currentlyCompiling == nil &&
		s.queuedCompilations.len() == 0 &&
		len(s.processes) == 0 &&
		len(s.programs) == 0
}

func (s *state) nextProgramID() ir.ProgramID {
	s.programIDAlloc++
	return s.programIDAlloc
}

func newState() *state {
	return &state{
		programIDAlloc: 0,
		processes:      make(map[ProcessID]*process),
		programs:       make(map[ir.ProgramID]*program),
		queuedCompilations: makeQueue(func(p *program) ir.ProgramID {
			return p.id
		}),
	}
}

type program struct {
	state      programState
	id         ir.ProgramID
	config     []irgen.ProbeDefinition
	executable Executable

	// Populated after the program has been compiled.
	compiledProgram *CompiledProgram

	// Populated after the program has been loaded.
	loadedProgram *loadedProgram

	// The process with which this program is associated.
	//
	// Note: in the future when we have multiple processes per program, this
	// will be a set of process IDs.
	processID *ProcessID
}

type process struct {
	state processState

	id         ProcessID
	executable Executable
	probes     map[probeKey]irgen.ProbeDefinition

	// The currently installed program, if there is one. Will be 0 if the
	// process's program creation failed.
	currentProgram ir.ProgramID

	// The currently attached program, if there is one. It will always have the
	// same ID as the currentProgram. Will be nil if there is no program
	// attached.
	attachedProgram *attachedProgram

	// Populated after the program has failed to compile or load.
	err error
}

type probeKey struct {
	id      string
	version int
}

func (pk probeKey) cmp(other probeKey) int {
	return cmp.Or(
		cmp.Compare(pk.id, other.id),
		cmp.Compare(pk.version, other.version),
	)
}

// effectHandler defines async operations that drive the instrumentation
// pipeline. Most effects generate completion events; register/unregister are
// fire-and-forget.
type effectHandler interface {

	// Compile IR to eBPF bytecode.
	compileProgram(ir.ProgramID, Executable, []irgen.ProbeDefinition) // -> ProgramCompiled/Failed

	// Load eBPF program into kernel.
	loadProgram(*CompiledProgram)

	// Attach program to process via uprobes.
	attachToProcess(*loadedProgram, Executable, ProcessID) // -> ProgramAttached/Failed

	// Register program with event dispatcher.
	registerProgramWithDispatcher(*ir.Program)

	// Unregister program from event dispatcher.
	unregisterProgramWithDispatcher(ir.ProgramID)

	// Detach program from process.
	detachFromProcess(*attachedProgram) // -> ProgramDetached
}

// handleEvent updates the state given the event, triggering the relevant
// effects along the way. Any errors returned should be considered invariant
// violations.
func handleEvent(
	sm *state, effects effectHandler, ev event,
) (retErr error) {
	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf("handling %T: %w", ev, retErr)
		}
	}()

	var err error
	switch ev := ev.(type) {
	case eventProcessesUpdated:
		err = handleProcessesUpdated(sm, effects, ev)

	case eventProgramCompiled:
		err = handleProgramCompiled(sm, effects, ev)

	case eventProgramCompilationFailed:
		err = handleCompilationFailure(sm, ev.programID, ev.err)

	case eventProgramLoaded:
		err = handleProgramLoaded(sm, effects, ev)

	case eventProgramLoadingFailed:
		err = handleCompilationFailure(sm, ev.programID, ev.err)

	case eventProgramAttached:
		err = handleProgramAttached(sm, effects, ev)

	case eventProgramAttachingFailed:
		err = handleProgramAttachingFailed(sm, effects, ev)

	case eventProgramDetached:
		err = handleProgramDetached(sm, effects, ev)

	case eventShutdown:
		err = handleShutdown(sm, effects)

	default:
		return fmt.Errorf("unexpected event %T: %#v", ev, ev)
	}
	if err != nil {
		return err
	}
	if err := maybeDequeueProgram(sm, effects); err != nil {
		return fmt.Errorf("failed to dequeue program: %w", err)
	}
	return nil
}

func handleProcessesUpdated(
	sm *state,
	effects effectHandler,
	ev eventProcessesUpdated,
) error {
	if sm.shuttingDown {
		return fmt.Errorf("processes should not be updated during shutdown")
	}

	var before, after []probeKey
	anythingChanged := func(
		p *process,
		probesAfterUpdate []irgen.ProbeDefinition,
	) bool {
		before = before[:0]
		for k := range p.probes {
			before = append(before, k)
		}
		slices.SortFunc(before, probeKey.cmp)

		after = after[:0]
		for _, probe := range probesAfterUpdate {
			after = append(after, probeKey{
				id:      probe.GetID(),
				version: probe.GetVersion(),
			})
		}
		slices.SortFunc(after, probeKey.cmp)

		// Check if anything has changed.
		if len(after) != len(before) {
			return true
		}
		for i, n := 0, len(after); i < n; i++ {
			if after[i].cmp(before[i]) != 0 {
				return true
			}
		}
		return false
	}

	handleProcessUpdate := func(sm *state, pu ProcessUpdate) error {
		p, ok := sm.processes[pu.ProcessID]
		if !ok {
			// Process updates with no probes are like removals.
			if len(pu.Probes) == 0 {
				return nil
			}
			p = &process{
				id:         pu.ProcessID,
				executable: pu.Executable,
				probes:     make(map[probeKey]irgen.ProbeDefinition),
			}
			sm.processes[pu.ProcessID] = p
		}
		if !anythingChanged(p, pu.Probes) {
			return nil
		}

		if err := clearProcessProgram(sm, effects, p); err != nil {
			return fmt.Errorf("failed to clear process program: %w", err)
		}

		// The new probes will be set when the detached event is handled and
		// the new program is enqueued. For now, we just clear the old probes.
		clear(p.probes)
		for _, probe := range pu.Probes {
			k := probeKey{id: probe.GetID(), version: probe.GetVersion()}
			p.probes[k] = probe
		}
		// If now we're in an invalid state, we need to delete the process if
		// we have no probes, or enqueue the new program with the new probes.
		if len(p.probes) == 0 {
			switch p.state {
			case processStateInvalid, processStateCompilationFailed:
				delete(sm.processes, p.id)
			case processStateWaitingForProgram:
				// We're waiting for an aborted compilation to finish.
			case processStateAttached:
			case processStateAttaching:
			case processStateDetaching:
			default:
				return fmt.Errorf(
					"unexpected process state: %#v", p.state,
				)
			}
		} else {
			switch p.state {
			case processStateInvalid, processStateCompilationFailed:
				if err := enqueueProgramForProcess(sm, p); err != nil {
					return err
				}

			default:
				// In all the other cases, we're waiting for something to
				// happen to the old program.
			}
		}
		return nil
	}

	for _, pu := range ev.updated {
		if err := handleProcessUpdate(sm, pu); err != nil {
			return err
		}
	}
	for _, removal := range ev.removed {
		if err := handleProcessUpdate(sm, ProcessUpdate{
			ProcessID: removal,
		}); err != nil {
			return err
		}
	}
	if err := maybeDequeueProgram(sm, effects); err != nil {
		return err
	}
	return nil
}

func enqueueProgramForProcess(sm *state, p *process) error {
	// If the process has no probes, we don't need to enqueue a program --
	// we're done with the process.
	if len(p.probes) == 0 {
		delete(sm.processes, p.id)
		return nil
	}
	probes := make([]irgen.ProbeDefinition, 0, len(p.probes))
	for _, probe := range p.probes {
		probes = append(probes, probe)
	}
	slices.SortFunc(probes, func(a, b irgen.ProbeDefinition) int {
		return cmp.Or(
			cmp.Compare(a.GetID(), b.GetID()),
			cmp.Compare(a.GetVersion(), b.GetVersion()),
		)
	})
	newProgram := &program{
		state:      programStateQueued,
		id:         sm.nextProgramID(),
		executable: p.executable,
		config:     probes,
		processID:  &p.id,
	}
	p.state = processStateWaitingForProgram
	p.currentProgram = newProgram.id
	sm.programs[newProgram.id] = newProgram
	_, havePrev := sm.queuedCompilations.pushBack(newProgram)
	if havePrev {
		return fmt.Errorf("program %v already in queue", newProgram.id)
	}
	return nil
}

func clearProcessProgram(
	sm *state, effects effectHandler, proc *process,
) error {
	progID := proc.currentProgram
	if progID == 0 { // only happens in compilation failure case
		return nil
	}

	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}

	if prog.processID != nil && *prog.processID != proc.id {
		return fmt.Errorf(
			"program %v is associated with a different process %v",
			progID, *prog.processID,
		)
	}

	switch prog.state {
	case programStateQueued:
		_, ok := sm.queuedCompilations.remove(progID)
		if !ok {
			return fmt.Errorf("program %v not found in queued programs", progID)
		}
		prog.state = programStateInvalid
		prog.processID = nil
		delete(sm.programs, progID)
		proc.currentProgram = 0
		if proc.state != processStateWaitingForProgram {
			return fmt.Errorf(
				"process %v is in an invalid state: %v", proc.id, proc.state,
			)
		}
		proc.state = processStateInvalid
		return nil

	case programStateCompiling, programStateLoading:
		prog.state = programStateCompilationAborted
		return nil

	case programStateLoaded:
		prog.state = programStateDraining
		switch proc.state {
		case processStateAttached:
			effects.detachFromProcess(proc.attachedProgram)
			proc.state = processStateDetaching
			proc.attachedProgram = nil
		case processStateDetaching:
			// Do nothing because we're waiting for the program to be detached.
		case processStateAttaching:
			// When the attached message comes in, then we need to
			// stay in detaching but send the effect to detach.
			proc.state = processStateDetaching
		default:
			return fmt.Errorf(
				"process %v is in an invalid state: %v", proc.id, proc.state,
			)
		}
		return nil

	case programStateDraining:
		return nil

	case programStateCompilationAborted:
		return nil

	default:
		return fmt.Errorf(
			"program %v in invalid state: %v", progID, prog.state,
		)
	}
}

func handleProgramCompiled(
	sm *state, effects effectHandler, ev eventProgramCompiled,
) error {
	progID := ev.programID
	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}
	switch prog.state {
	case programStateCompilationAborted:
		return handleAbortedCompilation(sm, progID)

	case programStateCompiling:
		// Nothing to do here.
		prog.state = programStateLoading
		prog.compiledProgram = ev.compiledProgram
		effects.loadProgram(ev.compiledProgram)
		return nil

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramCompiled: %v",
			progID, prog.state,
		)
	}
}

func handleCompilationFailure(
	sm *state, progID ir.ProgramID, failureError error,
) error {
	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}
	if sm.currentlyCompiling == nil {
		return fmt.Errorf(
			"currentlyCompiling is nil when program %v failed to compile",
			progID,
		)
	}
	if procID := prog.processID; procID != nil {
		proc, ok := sm.processes[*procID]
		if !ok {
			return fmt.Errorf("process %v not found in processes", procID)
		}
		switch proc.state {
		case processStateWaitingForProgram:
			// The process was already removed.
			if len(proc.probes) == 0 {
				delete(sm.processes, proc.id)
			} else {
				proc.state = processStateCompilationFailed
				proc.currentProgram = 0
				proc.err = failureError
			}
		default:
			return fmt.Errorf(
				"%v is in an invalid state for failure %s, expected %v",
				procID, proc.state, processStateWaitingForProgram,
			)
		}
	}
	sm.currentlyCompiling = nil
	prog.state = programStateInvalid
	delete(sm.programs, progID)
	return nil
}

func handleProgramLoaded(
	sm *state, effects effectHandler, ev eventProgramLoaded,
) error {
	progID := ev.programID
	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}
	switch prog.state {
	case programStateLoading:
		prog.state = programStateLoaded
		prog.loadedProgram = ev.loadedProgram

		// Tell the dispatcher about the program.
		effects.registerProgramWithDispatcher(prog.compiledProgram.IR)

		// Now attach to the processes and also register with the dispatcher.
		if procID := prog.processID; procID != nil {
			proc, ok := sm.processes[*procID]
			if !ok {
				return fmt.Errorf("process %v not found in processes", procID)
			}
			if proc.state != processStateWaitingForProgram {
				return fmt.Errorf(
					"%v is in an invalid state for loading program %v, expected %v",
					procID, proc.state, processStateWaitingForProgram,
				)
			}
			proc.state = processStateAttaching
			effects.attachToProcess(ev.loadedProgram, prog.executable, proc.id)
		}
		sm.currentlyCompiling = nil
		return nil
	case programStateCompilationAborted:
		ev.loadedProgram.close()
		sm.currentlyCompiling = nil
		delete(sm.programs, progID)

		if procID := prog.processID; procID != nil {
			proc, ok := sm.processes[*procID]
			if !ok {
				return fmt.Errorf("process %v not found in processes", procID)
			}
			switch proc.state {
			case processStateWaitingForProgram:
				if err := enqueueProgramForProcess(sm, proc); err != nil {
					return err
				}
			}
		}
		return nil

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramLoaded: %v",
			progID, prog.state,
		)
	}
}

func handleProgramAttachingFailed(
	sm *state, effects effectHandler, ev eventProgramAttachingFailed,
) error {
	procID := ev.processID
	proc, ok := sm.processes[procID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", procID)
	}
	// When we have more than one process per program, we'll need to
	// handle this differently.
	prog, ok := sm.programs[ev.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.programID)
	}
	prog.state = programStateInvalid

	// TODO: Who unloads the program? This should also be done
	// asynchronously -- right?
	prog.loadedProgram.close()
	effects.unregisterProgramWithDispatcher(ev.programID)
	prog.loadedProgram = nil
	delete(sm.programs, ev.programID)
	switch proc.state {
	case processStateDetaching:
		// What should we do here? Does it depend on what the error is?
		// For now, let's treat it as though we were in the process of
		// attaching and fail.
		fallthrough
	case processStateAttaching:
		// This too is suspect, if we failed to attach, then we're
		// going to say we're in a failed state, but maybe that's
		// not the right thing to do.
		if len(proc.probes) == 0 {
			delete(sm.processes, proc.id)
		} else {
			proc.state = processStateCompilationFailed
			proc.currentProgram = 0
			proc.err = ev.err
		}
		return nil

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramAttachingFailed: %v",
			procID, proc.state,
		)
	}
}

func handleProgramAttached(
	sm *state, effects effectHandler, ev eventProgramAttached,
) error {
	procID := ev.program.procID
	proc, ok := sm.processes[procID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", procID)
	}
	switch proc.state {
	case processStateAttaching:
		proc.state = processStateAttached
		proc.attachedProgram = ev.program
	case processStateDetaching:
		effects.detachFromProcess(ev.program)

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramAttached: %v",
			procID, proc.state,
		)
	}

	return nil
}

func handleProgramDetached(
	sm *state, effects effectHandler, ev eventProgramDetached,
) error {
	procID := ev.processID
	proc, ok := sm.processes[procID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", procID)
	}
	prog, ok := sm.programs[ev.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.programID)
	}

	switch proc.state {
	case processStateDetaching:
		if err := enqueueProgramForProcess(sm, proc); err != nil {
			return err
		}
	case processStateWaitingForProgram:
	default:
		return fmt.Errorf(
			"%v is in an invalid state %v",
			procID, proc.state,
		)
	}
	switch prog.state {
	case programStateDraining:
		prog.state = programStateInvalid
		delete(sm.programs, ev.programID)
		effects.unregisterProgramWithDispatcher(ev.programID)
		prog.loadedProgram.close()
		return nil
	default:
		return fmt.Errorf(
			"program %v is in an invalid state: %v",
			ev.programID, prog.state,
		)
	}
}

func handleAbortedCompilation(
	sm *state,
	progID ir.ProgramID,
) error {
	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}
	if sm.currentlyCompiling == nil {
		return fmt.Errorf(
			"currentlyCompiling is nil when program %v failed to compile",
			progID,
		)
	}
	if sm.currentlyCompiling.id != progID {
		return fmt.Errorf(
			"currentlyCompiling is %v when program %v failed to compile",
			sm.currentlyCompiling.id, progID,
		)
	}
	sm.currentlyCompiling = nil
	prog.loadedProgram = nil
	prog.state = programStateInvalid
	if procID := prog.processID; procID != nil {
		proc, ok := sm.processes[*procID]
		if !ok {
			return fmt.Errorf("process %v not found in processes", procID)
		}
		switch proc.state {
		case processStateWaitingForProgram:
			if err := enqueueProgramForProcess(sm, proc); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"%v is in an invalid state for aborted compilation: %v",
				procID, proc.state,
			)
		}
	}
	delete(sm.programs, progID)
	return nil
}

func maybeDequeueProgram(sm *state, effects effectHandler) error {
	if sm.currentlyCompiling != nil {
		return nil
	}
	p, ok := sm.queuedCompilations.popFront()
	if !ok {
		return nil
	}
	sm.currentlyCompiling = p
	if p.state != programStateQueued {
		return fmt.Errorf("program %v in invalid state: %v", p.id, p.state)
	}
	p.state = programStateCompiling
	effects.compileProgram(p.id, p.executable, p.config)
	return nil
}

func handleShutdown(sm *state, effects effectHandler) error {
	if sm.shuttingDown {
		return fmt.Errorf("state machine is already shutting down")
	}
	sm.shuttingDown = true

	// 1. Detach all attached processes and move all processes to Removed state.
	// Note that we bounce through this sorting to make the event processing
	// deterministic.
	procs := make([]ProcessID, 0, len(sm.processes))
	for procID := range sm.processes {
		procs = append(procs, procID)
	}
	slices.SortFunc(procs, func(a, b ProcessID) int {
		return cmp.Compare(a.PID, b.PID)
	})
	for _, procID := range procs {
		proc := sm.processes[procID]
		clear(proc.probes) // clear probes for all processes
		switch proc.state {
		case processStateAttached:
			effects.detachFromProcess(proc.attachedProgram)
			fallthrough
		case processStateAttaching:
			prog := sm.programs[proc.currentProgram]
			prog.state = programStateDraining
			proc.state = processStateDetaching
		case processStateCompilationFailed:
			delete(sm.processes, proc.id)
		}
	}

	// 2. Abort currently compiling program, if any.
	if sm.currentlyCompiling != nil {
		sm.currentlyCompiling.state = programStateCompilationAborted
	}

	// 3. Clear the compilation queue.
	for {
		prog, ok := sm.queuedCompilations.popFront()
		if !ok {
			break
		}
		if procID := prog.processID; procID != nil {
			proc, ok := sm.processes[*procID]
			if !ok {
				return fmt.Errorf("process %v not found in processes", procID)
			}
			delete(sm.processes, proc.id)
		}
		delete(sm.programs, prog.id)
	}
	return nil
}
