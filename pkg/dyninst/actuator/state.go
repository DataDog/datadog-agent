// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	procinfo "github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	processes          map[ProcessID]*process
	processesByService map[string]map[ProcessID]struct{}
	programs           map[ir.ProgramID]*program

	queuedLoading    queue[*program, ir.ProgramID]
	currentlyLoading *program

	// If true, the state machine is shutting down.
	shuttingDown bool

	breakerCfg    CircuitBreakerConfig
	lastHeartbeat time.Time

	// discoveredTypes tracks type names discovered at runtime via interface
	// decoding, keyed by service name. Each value is a sorted, deduplicated
	// slice of type names.
	discoveredTypes map[string][]string

	// recompilationRateLimit is the rate limit in recompilations/second.
	// Negative disables recompilation entirely. Zero disables rate limiting.
	recompilationRateLimit float64
	// recompilationRateBurst is the max burst (token cap).
	recompilationRateBurst int
	// recompilationAllowance is the current token count. Replenished by
	// the heartbeat, consumed by recompilations.
	recompilationAllowance float64

	// discoveredTypesLimit caps the total number of discovered type names
	// tracked across all services. When exceeded, entries for services with
	// no live processes are evicted.
	discoveredTypesLimit int
	// totalDiscoveredTypes is the running total of type names across all
	// entries in discoveredTypes.
	totalDiscoveredTypes int

	counters struct {
		loaded                      uint64
		loadFailed                  uint64
		attached                    uint64
		attachFailed                uint64
		detached                    uint64
		unloaded                    uint64
		typeRecompilationsTriggered uint64
	}
}

var unexpectedProgramStateLogLimiter = rate.NewLimiter(rate.Every(time.Minute), 1)

func (s *state) Metrics() Metrics {
	var numWaiting uint64
	var numAttached uint64
	var numAttaching uint64
	var numFailed uint64
	var numDetaching uint64
	for _, p := range s.processes {
		switch p.state {
		case processStateAttached:
			numAttached++
		case processStateAttaching:
			numAttaching++
		case processStateWaitingForProgram:
			numWaiting++
		case processStateFailed:
			numFailed++
		case processStateDetaching:
			numDetaching++
		case processStateInvalid:
			// no-op
		default:
			if unexpectedProgramStateLogLimiter.Allow() {
				log.Errorf("unexpected actuator.processState: %v", p.state)
			}
		}
	}

	return Metrics{
		Loaded:                      s.counters.loaded,
		LoadFailed:                  s.counters.loadFailed,
		Attached:                    s.counters.attached,
		AttachFailed:                s.counters.attachFailed,
		Detached:                    s.counters.detached,
		Unloaded:                    s.counters.unloaded,
		TypeRecompilationsTriggered: s.counters.typeRecompilationsTriggered,

		NumWaitingForProgram: uint64(numWaiting),
		NumAttached:          numAttached,
		NumAttaching:         numAttaching,
		NumFailed:            numFailed,
		NumDetaching:         numDetaching,

		NumProcesses: uint64(len(s.processes)),
		NumPrograms:  uint64(len(s.programs)),
	}
}

// Metrics is used to report metrics about the state machine.
type Metrics struct {
	// Counters

	// Loaded is the total number of programs that have been loaded
	// successfully.
	Loaded uint64
	// LoadFailed is the total number of programs that have failed to load.
	LoadFailed uint64
	// Attached is the total number of programs that have been attached to a
	// process.
	Attached uint64
	// AttachFailed is the total number of programs that have failed to attach
	// to a process.
	AttachFailed uint64
	// Detached is the total number of programs that have been detached from a
	// process.
	Detached uint64
	// Unloaded is the total number of programs that have been unloaded.
	Unloaded uint64
	// TypeRecompilationsTriggered is the total number of times a program was
	// recompiled due to missing type information discovered at runtime.
	TypeRecompilationsTriggered uint64

	// Gauges

	// NumWaitingForProgram is the number of processes waiting for a program to
	// be compiled and loaded.
	NumWaitingForProgram uint64
	// NumAttached is the number of processes attached to a program.
	NumAttached uint64
	// NumAttaching is the number of processes attaching to a program.
	NumAttaching uint64
	// NumFailed is the number of processes with programs that have failed.
	NumFailed uint64
	// NumDetaching is the number of programs that are detaching.
	NumDetaching uint64

	// NumProcesses is the number of processes in the state machine.
	NumProcesses uint64
	// NumPrograms is the number of programs in the state machine.
	NumPrograms uint64
}

// AsStats converts the Metrics to a map[string]any for use by the system-probe.
func (m Metrics) AsStats() map[string]any {
	return map[string]any{
		"loaded":                      m.Loaded,
		"loadFailed":                  m.LoadFailed,
		"attached":                    m.Attached,
		"attachFailed":                m.AttachFailed,
		"detached":                    m.Detached,
		"unloaded":                    m.Unloaded,
		"typeRecompilationsTriggered": m.TypeRecompilationsTriggered,

		"numWaitingForProgram": m.NumWaitingForProgram,
		"numAttached":          m.NumAttached,
		"numAttaching":         m.NumAttaching,
		"numFailed":            m.NumFailed,
		"numDetaching":         m.NumDetaching,

		"numProcesses": m.NumProcesses,
		"numPrograms":  m.NumPrograms,
	}
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

func newState(cfg Config) *state {
	return &state{
		programIDAlloc:     0,
		processes:          make(map[ProcessID]*process),
		processesByService: make(map[string]map[ProcessID]struct{}),
		programs:           make(map[ir.ProgramID]*program),
		queuedLoading: makeQueue(func(p *program) ir.ProgramID {
			return p.id
		}),
		breakerCfg:             cfg.CircuitBreakerConfig,
		lastHeartbeat:          time.Now(),
		discoveredTypes:        make(map[string][]string),
		discoveredTypesLimit:   cfg.DiscoveredTypesLimit,
		recompilationRateLimit: cfg.RecompilationRateLimit,
		recompilationRateBurst: cfg.RecompilationRateBurst,
		recompilationAllowance: float64(cfg.RecompilationRateBurst),
	}
}

type program struct {
	state      programState
	id         ir.ProgramID
	config     []ir.ProbeDefinition
	executable Executable

	// Populated after the program has been loaded.
	loaded *loadedProgram

	// Stats collected from the last heartbeat, indexed by core.
	lastRuntimeStats []loader.RuntimeStats

	// The process with which this program is associated.
	//
	// Note: in the future when we have multiple processes per program, this
	// will be a set of process IDs.
	processID ProcessID

	// needsRecompilation is set when new types have been discovered for the
	// service since this program was compiled. When the pipeline is idle,
	// maybeTriggerTypeRecompilation will clear the program and re-enqueue it.
	needsRecompilation bool
}

type process struct {
	processID ProcessID

	state processState

	executable Executable
	service    string
	probes     map[probeKey]ir.ProbeDefinition

	// The currently installed program, if there is one. Will be 0 if the
	// process's program creation failed.
	currentProgram ir.ProgramID

	// The currently attached program, if there is one. It will always have the
	// same ID as the currentProgram. Will be nil if there is no program
	// attached.
	attachedProgram *attachedProgram
}

func (s *state) addProcessToServiceIndex(proc *process) {
	if proc.service == "" {
		return
	}
	pids, ok := s.processesByService[proc.service]
	if !ok {
		pids = make(map[ProcessID]struct{})
		s.processesByService[proc.service] = pids
	}
	pids[proc.processID] = struct{}{}
}

func (s *state) removeProcessFromServiceIndex(proc *process) {
	if proc.service == "" {
		return
	}
	pids := s.processesByService[proc.service]
	delete(pids, proc.processID)
	if len(pids) == 0 {
		delete(s.processesByService, proc.service)
	}
}

func (s *state) deleteProcess(pid ProcessID) {
	proc, ok := s.processes[pid]
	if !ok {
		return
	}
	s.removeProcessFromServiceIndex(proc)
	delete(s.processes, pid)
	s.evictOrphanedDiscoveredTypes()
}

// evictOrphanedDiscoveredTypes removes discovered type entries for services
// that no longer have any live processes, when the total number of
// discovered types exceeds the configured limit.
func (s *state) evictOrphanedDiscoveredTypes() {
	if s.totalDiscoveredTypes <= s.discoveredTypesLimit {
		return
	}
	for service, types := range s.discoveredTypes {
		if _, hasProcesses := s.processesByService[service]; !hasProcesses {
			s.totalDiscoveredTypes -= len(types)
			delete(s.discoveredTypes, service)
		}
	}
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
	loadProgram(ir.ProgramID, Executable, ProcessID, []ir.ProbeDefinition, LoadOptions)

	// Attach program to process via uprobes.
	attachToProcess(*loadedProgram, Executable, ProcessID) // -> ProgramAttached/Failed

	// Detach program from process. An optional error indicates an extraordinary
	// failure that should be reported to the diagnostics system.
	detachFromProcess(*attachedProgram, error) // -> ProgramDetached

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
	case eventGetMetrics:
		ev.metricsChan <- sm.Metrics()
		return nil

	case eventHeartbeatCheck:
		handleHeartbeatCheck(sm, effects)

	case eventProcessesUpdated:
		err = handleProcessesUpdated(sm, effects, ev)

	case eventProgramLoaded:
		sm.counters.loaded++
		err = handleProgramLoaded(sm, effects, ev)

	case eventMissingTypesReported:
		err = handleMissingTypesReported(sm, effects, ev)

	case eventProgramLoadingFailed:
		sm.counters.loadFailed++
		err = handleProgramLoadingFailure(sm, ev.programID)

	case eventProgramAttached:
		sm.counters.attached++
		err = handleProgramAttached(sm, effects, ev)

	case eventProgramAttachingFailed:
		sm.counters.attachFailed++
		err = handleProgramAttachingFailed(sm, effects, ev)

	case eventProgramDetached:
		sm.counters.detached++
		err = handleProgramDetached(sm, effects, ev)

	case eventProgramUnloaded:
		sm.counters.unloaded++
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
	if err := maybeTriggerTypeRecompilation(sm, effects); err != nil {
		return fmt.Errorf("failed to trigger type recompilation: %w", err)
	}
	return nil
}

func handleProcessesUpdated(
	sm *state,
	effects effectHandler,
	ev eventProcessesUpdated,
) error {
	if sm.shuttingDown {
		return errors.New("processes should not be updated during shutdown")
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
		pid := pu.ProcessID
		p, ok := sm.processes[pid]
		if !ok {
			// Process updates with no probes are like removals.
			if len(pu.Probes) == 0 {
				return nil
			}
			p = &process{
				processID:  pid,
				executable: pu.Executable,
				service:    pu.Info.Service,
				probes:     make(map[probeKey]ir.ProbeDefinition),
			}
			sm.processes[pid] = p
			sm.addProcessToServiceIndex(p)
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
			case processStateFailed:
				if p.currentProgram != 0 {
					// We're waiting for a program that failed to be unloaded.
					// When it is, we'll then go and enqueue a new program.
					p.state = processStateWaitingForProgram
				} else {
					sm.deleteProcess(p.processID)
				}
			case processStateInvalid:
				sm.deleteProcess(p.processID)
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
			case processStateFailed:
				if p.currentProgram != 0 {
					// We're waiting for a program that failed to be unloaded.
					// When it is, we'll then go and enqueue a new program.
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
			Info: procinfo.Info{
				ProcessID: removal,
			},
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
		sm.deleteProcess(p.processID)
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
		processID:  p.processID,
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

	if prog.processID != proc.processID {
		return fmt.Errorf(
			"program %v is associated with a different process %v",
			progID, proc.processID,
		)
	}

	prog.needsRecompilation = false

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
				proc.processID, proc.state,
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
			effects.detachFromProcess(proc.attachedProgram, nil)
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
				proc.processID, proc.state,
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

// mergeIntoSorted merges src into dst, maintaining a sorted, deduplicated slice.
// src must be sorted; duplicates in src are tolerated.
func mergeIntoSorted(dst, src []string) (_ []string, changed bool) {
	var i int
	for _, name := range src {
		j, found := slices.BinarySearch(dst[i:], name)
		if found {
			continue
		}
		i += j
		dst = slices.Insert(dst, i, name)
		changed = true
	}
	return dst, changed
}

func handleMissingTypesReported(
	sm *state, _ effectHandler, ev eventMissingTypesReported,
) error {
	if sm.shuttingDown {
		return nil
	}
	proc, ok := sm.processes[ev.processID]
	if !ok {
		// Process may have been removed since the event was emitted.
		return nil
	}
	service := proc.service
	if service == "" {
		return nil
	}

	// Merge reported type names into the per-service discovered set,
	// maintaining a sorted, deduplicated slice.
	slices.Sort(ev.typeNames)
	before := sm.discoveredTypes[service]
	after, changed := mergeIntoSorted(before, ev.typeNames)
	if !changed {
		return nil
	}
	sm.discoveredTypes[service] = after
	sm.totalDiscoveredTypes += len(after) - len(before)
	sm.evictOrphanedDiscoveredTypes()

	// Mark all programs for processes of this service that need
	// recompilation. Only Loading and Loaded programs are marked: Queued
	// programs will pick up the latest types at dequeue time, and programs
	// in teardown states (Draining/Unloading/LoadingAborted) are already
	// being replaced.
	for pid := range sm.processesByService[service] {
		p := sm.processes[pid]
		if p.currentProgram == 0 {
			continue
		}
		prog, ok := sm.programs[p.currentProgram]
		if !ok {
			continue
		}
		switch prog.state {
		case programStateLoading, programStateLoaded:
			prog.needsRecompilation = true
		}
	}

	// Actual recompilation is triggered by maybeTriggerTypeRecompilation,
	// which runs after every event and acts when the pipeline is idle.
	return nil
}

// maybeTriggerTypeRecompilation finds a program flagged with
// needsRecompilation and clears it to trigger re-enqueue. This handles both
// the case where missing types were reported while the pipeline was busy and
// the per-service fan-out where multiple programs need recompilation.
//
// Only one recompilation is triggered per call to avoid cascading effects.
func maybeTriggerTypeRecompilation(sm *state, effects effectHandler) error {
	// Negative rate limit disables recompilation entirely.
	if sm.shuttingDown || sm.recompilationRateLimit < 0 {
		return nil
	}
	// Only act when the pipeline is completely idle.
	if sm.currentlyLoading != nil || sm.queuedLoading.len() > 0 {
		return nil
	}
	// Rate-limit recompilations.
	if sm.recompilationRateLimit > 0 && sm.recompilationAllowance < 1.0 {
		return nil
	}

	// Find the flagged program with the minimum ID for determinism.
	var minProg *program
	for _, prog := range sm.programs {
		if !prog.needsRecompilation {
			continue
		}
		if minProg == nil || prog.id < minProg.id {
			minProg = prog
		}
	}
	if minProg == nil {
		return nil
	}

	proc, ok := sm.processes[minProg.processID]
	if !ok {
		return fmt.Errorf("process %v not found for program %v", minProg.processID, minProg.id)
	}
	if err := clearProcessProgram(sm, effects, proc); err != nil {
		return fmt.Errorf("failed to clear process program for type recompilation: %w", err)
	}
	sm.counters.typeRecompilationsTriggered++
	if sm.recompilationRateLimit > 0 {
		sm.recompilationAllowance--
	}
	// Only trigger one recompilation per call; the next event cycle
	// will pick up additional ones if needed.
	return nil
}

func handleProgramLoadingFailure(
	sm *state, progID ir.ProgramID,
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
	proc, ok := sm.processes[prog.processID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processID)
	}
	switch proc.state {
	case processStateWaitingForProgram:
		// The process was already removed.
		if len(proc.probes) == 0 {
			sm.deleteProcess(proc.processID)
		} else {
			proc.state = processStateFailed
			proc.currentProgram = 0
		}
	default:
		return fmt.Errorf(
			"%v is in an invalid state for failure %s, expected %v",
			prog.processID, proc.state, processStateWaitingForProgram,
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
		proc, ok := sm.processes[prog.processID]
		if !ok {
			return fmt.Errorf("process %v not found in processes", prog.processID)
		}
		if proc.state != processStateWaitingForProgram {
			return fmt.Errorf(
				"%v is in an invalid state for loading program %v, expected %v",
				proc.processID, proc.state, processStateWaitingForProgram,
			)
		}
		proc.state = processStateAttaching
		effects.attachToProcess(ev.loaded, prog.executable, proc.processID)
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

	proc, ok := sm.processes[prog.processID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processID)
	}

	// Unload the program.
	prog.state = programStateUnloading
	prog.needsRecompilation = false
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
			proc.state = processStateFailed
		}
		return nil

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramAttachingFailed: %v",
			proc.processID, proc.state,
		)
	}
}

func handleProgramAttached(
	sm *state, effects effectHandler, ev eventProgramAttached,
) error {
	_, ok := sm.programs[ev.program.programID]
	if !ok {
		return fmt.Errorf("program %v not found in programs", ev.program.programID)
	}
	pid := ev.program.processID
	proc, ok := sm.processes[pid]
	if !ok {
		return fmt.Errorf("process %v not found in processes", pid)
	}
	switch proc.state {
	case processStateAttaching:
		proc.state = processStateAttached
		proc.attachedProgram = ev.program
	case processStateDetaching:
		effects.detachFromProcess(ev.program, nil)

	default:
		return fmt.Errorf(
			"%v is in an invalid state for eventProgramAttached: %v",
			proc.processID, proc.state,
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
	proc, ok := sm.processes[ev.processID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", ev.processID)
	}

	switch proc.state {
	case processStateFailed:
	case processStateDetaching:
	case processStateWaitingForProgram:
	default:
		return fmt.Errorf(
			"%v is in an invalid state %v",
			proc.processID, proc.state,
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

	proc, ok := sm.processes[prog.processID]
	if !ok {
		return fmt.Errorf("process %v not found in processes", prog.processID)
	}
	if proc.currentProgram != prog.id {
		return fmt.Errorf(
			"process %v has program %v, expected %v",
			proc.processID, proc.currentProgram, prog.id,
		)
	}
	proc.currentProgram = 0

	switch proc.state {
	case processStateFailed:
	case processStateWaitingForProgram,
		processStateDetaching:
		if len(proc.probes) == 0 {
			sm.deleteProcess(proc.processID)
		} else {
			proc.currentProgram = 0
			if err := enqueueProgramForProcess(sm, proc); err != nil {
				return fmt.Errorf("failed to enqueue program for process: %w", err)
			}
		}
	default:
		return fmt.Errorf("process %v is in an invalid state %v", proc.processID, proc.state)
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
	p.needsRecompilation = false // defensive: should not be set for Queued programs
	// Look up discovered types for the process's service at dequeue time,
	// so we always use the latest set.
	var additionalTypes []string
	if proc, ok := sm.processes[p.processID]; ok && proc.service != "" {
		additionalTypes = slices.Clone(sm.discoveredTypes[proc.service])
	}
	effects.loadProgram(p.id, p.executable, p.processID, p.config, LoadOptions{
		AdditionalTypes: additionalTypes,
	})
	return nil
}

func handleShutdown(sm *state, effects effectHandler) error {
	if sm.shuttingDown {
		return errors.New("state machine is already shutting down")
	}
	sm.shuttingDown = true

	// 1. Detach all attached processes and move all processes to Removed state.
	// Note that we bounce through this sorting to make the event processing
	// deterministic.
	procs := make([]ProcessID, 0, len(sm.processes))
	for _, proc := range sm.processes {
		procs = append(procs, proc.processID)
	}
	slices.SortFunc(procs, func(a, b ProcessID) int {
		return cmp.Compare(a.PID, b.PID)
	})
	for _, pid := range procs {
		proc := sm.processes[pid]
		clear(proc.probes) // clear probes for all processes
		switch proc.state {
		case processStateAttached:
			effects.detachFromProcess(proc.attachedProgram, nil)
			fallthrough
		case processStateAttaching:
			prog := sm.programs[proc.currentProgram]
			prog.state = programStateDraining
			prog.needsRecompilation = false
			proc.state = processStateDetaching
		case processStateFailed:
			// Otherwise we're still waiting for the program to be unloaded.
			if proc.currentProgram == 0 {
				sm.deleteProcess(proc.processID)
			} else {
				proc.state = processStateDetaching
			}
		}
	}

	// 2. Abort currently loading program, if any.
	if sm.currentlyLoading != nil {
		sm.currentlyLoading.state = programStateLoadingAborted
		sm.currentlyLoading.needsRecompilation = false
	}

	// 3. Clear the loading queue.
	pop := sm.queuedLoading.popFront
	for prog, ok := pop(); ok; prog, ok = pop() {
		proc, ok := sm.processes[prog.processID]
		if !ok {
			return fmt.Errorf("process %v not found in processes", prog.processID)
		}
		sm.deleteProcess(proc.processID)
		delete(sm.programs, prog.id)
	}
	return nil
}

func handleHeartbeatCheck(sm *state, effects effectHandler) {
	now := time.Now()
	interval := now.Sub(sm.lastHeartbeat)
	sm.lastHeartbeat = now

	checkCosts(sm, interval, effects)
	replenishRecompilationAllowance(sm, interval)
}

func checkCosts(sm *state, interval time.Duration, effects effectHandler) {
	// Validate budget on every core independently.
	var totalCostSPS []float64
	var maxCostSPS []float64
	var maxProg []*program
	detachedAny := false
	for _, prog := range sm.programs {
		if prog.state != programStateLoaded {
			continue
		}
		proc, ok := sm.processes[prog.processID]
		if !ok || proc.state != processStateAttached {
			// Not attached.
			continue
		}
		perCoreStats := prog.loaded.loaded.RuntimeStats()
		if len(prog.lastRuntimeStats) < len(perCoreStats) {
			lastRuntimeStats := make([]loader.RuntimeStats, len(perCoreStats))
			copy(lastRuntimeStats, prog.lastRuntimeStats)
			prog.lastRuntimeStats = lastRuntimeStats
		}
		for len(totalCostSPS) < len(perCoreStats) {
			totalCostSPS = append(totalCostSPS, 0)
			maxCostSPS = append(maxCostSPS, -1)
			maxProg = append(maxProg, nil)
		}
		for core, stats := range perCoreStats {
			lastStats := prog.lastRuntimeStats[core]
			hits := stats.HitCnt - lastStats.HitCnt
			execCost := stats.CPU - lastStats.CPU
			interruptCost := sm.breakerCfg.InterruptOverhead * time.Duration(hits)
			prog.lastRuntimeStats[core] = stats

			costSPS := (execCost + interruptCost).Seconds() / interval.Seconds()
			totalCostSPS[core] += costSPS
			if costSPS > maxCostSPS[core] {
				maxCostSPS[core] = costSPS
				maxProg[core] = prog
			}
			if costSPS > sm.breakerCfg.PerProbeCPULimit && proc.state == processStateAttached {
				// Circuit breaker triggered for this probe, detach it.
				prog.state = programStateDraining
				prog.needsRecompilation = false
				proc.state = processStateFailed
				err := fmt.Errorf(
					"probe exceeded CPU limit of %fcpus/s using %fcpus/s = %fcpus/s (exec) + %fcpus/s (%d interrupts) over %fs on core %d",
					sm.breakerCfg.PerProbeCPULimit,
					costSPS,
					execCost.Seconds()/interval.Seconds(),
					interruptCost.Seconds()/interval.Seconds(),
					hits,
					interval.Seconds(),
					core,
				)
				effects.detachFromProcess(proc.attachedProgram, err)
				proc.attachedProgram = nil
				detachedAny = true
				break
			}
		}
	}

	// Check if any core exceeded the total budget across all probes.
	// If so, pick the most expensive probe on a core with highest total cost.
	if len(totalCostSPS) == 0 {
		return
	}
	{
		maxCore := 0
		for core, cost := range totalCostSPS {
			if cost > totalCostSPS[maxCore] {
				maxCore = core
			}
		}
		if !detachedAny && maxProg[maxCore] != nil && totalCostSPS[maxCore] > sm.breakerCfg.AllProbesCPULimit {
			prog := maxProg[maxCore]
			proc := sm.processes[prog.processID]
			prog.state = programStateDraining
			prog.needsRecompilation = false
			proc.state = processStateFailed
			err := fmt.Errorf(
				"probes exceeded total CPU limit of %fcpus/s using %fcpus/s on core %d; detaching most expensive probe, that used %fcpus/s (mean over %fs)",
				sm.breakerCfg.AllProbesCPULimit,
				totalCostSPS[maxCore],
				maxCore,
				maxCostSPS[maxCore],
				interval.Seconds(),
			)
			effects.detachFromProcess(proc.attachedProgram, err)
		}
	}
}

func replenishRecompilationAllowance(sm *state, interval time.Duration) {
	if sm.recompilationRateLimit > 0 {
		increase := interval.Seconds() * sm.recompilationRateLimit
		sm.recompilationAllowance = min(
			sm.recompilationAllowance+increase,
			float64(sm.recompilationRateBurst),
		)
	}
}
