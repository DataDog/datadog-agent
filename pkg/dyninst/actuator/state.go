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
)

// state represents the state of an Actuator.
//
// This is an event-driven state machine that manages dynamic instrumentation
// for processes by coordinating IR generation, program loading, and process
// attachment. The state machine processes events sequentially in a dedicated
// goroutine to maintain consistency.
//
// The instigating event is ProcessesUpdate, which informs the actuator of the
// probes intended for processes. This triggers a pipeline of asynchronous
// operations coordinated through effects and events.
//
// The state machine manages processes (with states like WaitingForProgram, Attaching, Attached)
// and programs (with states like Queued, GeneratingIR, Loading, Loaded).
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

	processes map[processKey]*process
	programs  map[ir.ProgramID]*program

	queuedLoading    queue[*program, ir.ProgramID]
	currentlyLoading *program

	// If true, the state machine is shutting down.
	shuttingDown bool
}

type processKey struct {
	ProcessID
	tenantID tenantID
}

func (pk *processKey) String() string {
	if pk.tenantID == 0 {
		return pk.ProcessID.String()
	}
	return fmt.Sprintf("{PID:%v,Ten:%v}", pk.PID, pk.tenantID)
}

func (pk processKey) cmp(other processKey) int {
	return cmp.Or(
		cmp.Compare(pk.tenantID, other.tenantID),
		cmp.Compare(pk.PID, other.PID),
	)
}

// isShutdown returns true if the state machine is fully shut down.
func (s *state) isShutdown() bool {
	return s.shuttingDown &&
		s.currentlyLoading == nil &&
		s.queuedLoading.len() == 0 &&
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
		processes:      make(map[processKey]*process),
		programs:       make(map[ir.ProgramID]*program),
		queuedLoading: makeQueue(func(p *program) ir.ProgramID {
			return p.id
		}),
	}
}

type program struct {
	state      programState
	id         ir.ProgramID
	config     []ir.ProbeDefinition
	executable Executable

	// Populated after the program has been loaded.
	loaded *loadedProgram

	// The process with which this program is associated.
	//
	// Note: in the future when we have multiple processes per program, this
	// will be a set of process IDs.
	processKey
}

type process struct {
	processKey

	state processState

	executable Executable
	probes     map[probeKey]ir.ProbeDefinition

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

	// Load eBPF program into kernel.
	loadProgram(tenantID, ir.ProgramID, Executable, ProcessID, []ir.ProbeDefinition)

	// Attach program to process via uprobes.
	attachToProcess(*loadedProgram, Executable, ProcessID) // -> ProgramAttached/Failed

	// Detach program from process.
	detachFromProcess(*attachedProgram) // -> ProgramDetached

	// Unload program resources asynchronously.
	unloadProgram(*loadedProgram) // -> ProgramUnloaded
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

	case eventProgramLoaded:
		err = handleProgramLoaded(sm, effects, ev)

	case eventProgramLoadingFailed:
		err = handleProgramLoadingFailure(sm, ev.programID, ev.err)

	case eventProgramAttached:
		err = handleProgramAttached(sm, effects, ev)

	case eventProgramAttachingFailed:
		err = handleProgramAttachingFailed(sm, effects, ev)

	case eventProgramDetached:
		err = handleProgramDetached(sm, effects, ev)

	case eventProgramUnloaded:
		err = handleProgramUnloaded(sm, ev)

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
		probesAfterUpdate []ir.ProbeDefinition,
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
		key := processKey{
			ProcessID: pu.ProcessID,
			tenantID:  ev.tenantID,
		}
		p, ok := sm.processes[key]
		if !ok {
			// Process updates with no probes are like removals.
			if len(pu.Probes) == 0 {
				return nil
			}
			p = &process{
				processKey: key,
				executable: pu.Executable,
				probes:     make(map[probeKey]ir.ProbeDefinition),
			}
			sm.processes[key] = p
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
			case processStateLoadingFailed:
				if p.currentProgram != 0 {
					// We're waiting for a program we failed to attach to
					// be unloaded. When it is, we'll then go and enqueue
					// a new program.
					p.state = processStateWaitingForProgram
				} else {
					delete(sm.processes, p.processKey)
				}
			case processStateInvalid:
				delete(sm.processes, p.processKey)
			case processStateWaitingForProgram:
				// We're waiting for an aborted loading to finish.
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
			case processStateLoadingFailed:
				if p.currentProgram != 0 {
					// We're waiting for a program we failed to attach to
					// be unloaded. When it is, we'll then go and enqueue
					// a new program.
					p.state = processStateWaitingForProgram
				} else {
					if err := enqueueProgramForProcess(sm, p); err != nil {
						return err
					}
				}
			case processStateInvalid:
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
		delete(sm.processes, p.processKey)
		return nil
	}
	probes := make([]ir.ProbeDefinition, 0, len(p.probes))
	for _, probe := range p.probes {
		probes = append(probes, probe)
	}
	slices.SortFunc(probes, func(a, b ir.ProbeDefinition) int {
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
		processKey: p.processKey,
	}
	p.state = processStateWaitingForProgram
	p.currentProgram = newProgram.id
	sm.programs[newProgram.id] = newProgram
	_, havePrev := sm.queuedLoading.pushBack(newProgram)
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

	if prog.processKey != proc.processKey {
		return fmt.Errorf(
			"program %v is associated with a different process %v",
			progID, proc.processKey,
		)
	}

	switch prog.state {
	case programStateQueued:
		_, ok := sm.queuedLoading.remove(progID)
		if !ok {
			return fmt.Errorf("program %v not found in queued programs", progID)
		}
		prog.state = programStateInvalid
		delete(sm.programs, progID)
		proc.currentProgram = 0
		if proc.state != processStateWaitingForProgram {
			return fmt.Errorf(
				"process %v is in an invalid state: %v",
				proc.processKey, proc.state,
			)
		}
		proc.state = processStateInvalid
		return nil

	case programStateLoading:
		prog.state = programStateLoadingAborted
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
				"process %v is in an invalid state: %v",
				proc.processKey, proc.state,
			)
		}
		return nil

	case programStateDraining:
		return nil

	case programStateLoadingAborted:
		return nil

	case programStateUnloading:
		return nil

	default:
		return fmt.Errorf(
			"program %v in invalid state: %v", progID, prog.state,
		)
	}
}

func handleProgramLoadingFailure(
	sm *state, progID ir.ProgramID, failureError error,
) error {
	prog, ok := sm.programs[progID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", progID)
	}
	if sm.currentlyLoading == nil {
		return fmt.Errorf(
			"currentlyLoading is nil when program %v failed to load",
			progID,
		)
	}
	proc, ok := sm.processes[prog.processKey]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processKey)
	}
	switch proc.state {
	case processStateWaitingForProgram:
		// The process was already removed.
		if len(proc.probes) == 0 {
			delete(sm.processes, proc.processKey)
		} else {
			proc.state = processStateLoadingFailed
			proc.currentProgram = 0
			proc.err = failureError
		}
	default:
		return fmt.Errorf(
			"%v is in an invalid state for failure %s, expected %v",
			prog.processKey, proc.state, processStateWaitingForProgram,
		)
	}
	sm.currentlyLoading = nil
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
		prog.loaded = ev.loaded

		// Now attach to the processes and also register with the dispatcher.
		proc, ok := sm.processes[prog.processKey]
		if !ok {
			return fmt.Errorf("process %v not found in processes", prog.processKey)
		}
		if proc.state != processStateWaitingForProgram {
			return fmt.Errorf(
				"%v is in an invalid state for loading program %v, expected %v",
				proc.processKey, proc.state, processStateWaitingForProgram,
			)
		}
		proc.state = processStateAttaching
		effects.attachToProcess(ev.loaded, prog.executable, proc.processKey.ProcessID)
		sm.currentlyLoading = nil
		return nil
	case programStateLoadingAborted:
		prog.state = programStateUnloading
		effects.unloadProgram(ev.loaded)
		sm.currentlyLoading = nil
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
	// When we have more than one process per program, we'll need to
	// handle this differently.
	prog, ok := sm.programs[ev.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.programID)
	}

	proc, ok := sm.processes[prog.processKey]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processKey)
	}

	// Unload the program.
	prog.state = programStateUnloading
	effects.unloadProgram(prog.loaded)

	switch proc.state {
	case processStateDetaching:
		// What should we do here? Does it depend on what the error is?
		// For now, let's treat it as though we were in the process of
		// attaching and fail.
		fallthrough
	case processStateAttaching:
		if len(proc.probes) == 0 {
			proc.state = processStateDetaching
		} else {
			// This too is suspect, if we failed to attach, then we're
			// going to say we're in a failed state, but maybe that's
			// not the right thing to do.
			proc.state = processStateLoadingFailed
			proc.err = ev.err
		}
		return nil

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramAttachingFailed: %v",
			proc.processKey, proc.state,
		)
	}
}

func handleProgramAttached(
	sm *state, effects effectHandler, ev eventProgramAttached,
) error {
	prog, ok := sm.programs[ev.program.ir.ID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.program.ir.ID)
	}
	key := processKey{
		ProcessID: ev.program.procID,
		tenantID:  prog.tenantID,
	}
	proc, ok := sm.processes[key]
	if !ok {
		return fmt.Errorf("process %v not found in processes", key)
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
			proc.processKey, proc.state,
		)
	}

	return nil
}

func handleProgramDetached(
	sm *state, effects effectHandler, ev eventProgramDetached,
) error {
	prog, ok := sm.programs[ev.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.programID)
	}
	key := processKey{
		ProcessID: ev.processID,
		tenantID:  prog.tenantID,
	}
	proc, ok := sm.processes[key]
	if !ok {
		return fmt.Errorf("process %v not found in processes", key)
	}

	switch proc.state {
	case processStateDetaching:
	case processStateWaitingForProgram:
	default:
		return fmt.Errorf(
			"%v is in an invalid state %v",
			proc.processKey, proc.state,
		)
	}
	switch prog.state {
	case programStateDraining:
		prog.state = programStateUnloading
		effects.unloadProgram(prog.loaded)
		return nil
	default:
		return fmt.Errorf(
			"program %v is in an invalid state: %v",
			ev.programID, prog.state,
		)
	}
}

// handleProgramUnloaded finalises the removal of a program after its resources
// have been released by the asynchronous effect.
func handleProgramUnloaded(sm *state, ev eventProgramUnloaded) error {
	prog, ok := sm.programs[ev.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.programID)
	}
	if prog.state != programStateUnloading {
		return fmt.Errorf("program %v is in invalid state %v for unload", ev.programID, prog.state)
	}

	proc, ok := sm.processes[prog.processKey]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processKey)
	}
	if proc.currentProgram != prog.id {
		return fmt.Errorf(
			"process %v has program %v, expected %v",
			proc.processKey, proc.currentProgram, prog.id,
		)
	}
	proc.currentProgram = 0

	switch proc.state {
	case processStateLoadingFailed:
		// Do nothing.
	case processStateWaitingForProgram,
		processStateDetaching:
		if len(proc.probes) == 0 {
			delete(sm.processes, proc.processKey)
		} else {
			proc.currentProgram = 0
			if err := enqueueProgramForProcess(sm, proc); err != nil {
				return fmt.Errorf("failed to enqueue program for process: %w", err)
			}
		}
	default:
		return fmt.Errorf("process %v is in an invalid state %v", proc.processKey, proc.state)
	}

	prog.state = programStateInvalid
	delete(sm.programs, ev.programID)
	return nil
}

func maybeDequeueProgram(sm *state, effects effectHandler) error {
	if sm.currentlyLoading != nil {
		return nil
	}
	p, ok := sm.queuedLoading.popFront()
	if !ok {
		return nil
	}
	sm.currentlyLoading = p
	if p.state != programStateQueued {
		return fmt.Errorf("program %v in invalid state: %v", p.id, p.state)
	}
	p.state = programStateLoading
	effects.loadProgram(p.tenantID, p.id, p.executable, p.ProcessID, p.config)
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
	procs := make([]processKey, 0, len(sm.processes))
	for procKey := range sm.processes {
		procs = append(procs, procKey)
	}
	slices.SortFunc(procs, processKey.cmp)
	for _, procKey := range procs {
		proc := sm.processes[procKey]
		clear(proc.probes) // clear probes for all processes
		switch proc.state {
		case processStateAttached:
			effects.detachFromProcess(proc.attachedProgram)
			fallthrough
		case processStateAttaching:
			prog := sm.programs[proc.currentProgram]
			prog.state = programStateDraining
			proc.state = processStateDetaching
		case processStateLoadingFailed:
			// Otherwise we're still waiting for the program to be unloaded.
			if proc.currentProgram == 0 {
				delete(sm.processes, proc.processKey)
			} else {
				proc.state = processStateDetaching
			}
		}
	}

	// 2. Abort currently loading program, if any.
	if sm.currentlyLoading != nil {
		sm.currentlyLoading.state = programStateLoadingAborted
	}

	// 3. Clear the loading queue.
	for {
		prog, ok := sm.queuedLoading.popFront()
		if !ok {
			break
		}
		proc, ok := sm.processes[prog.processKey]
		if !ok {
			return fmt.Errorf("process %v not found in processes", prog.processKey)
		}
		delete(sm.processes, proc.processKey)
		delete(sm.programs, prog.id)
	}
	return nil
}
