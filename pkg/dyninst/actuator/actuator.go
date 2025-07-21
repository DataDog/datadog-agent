// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"errors"
	"fmt"
	"sync"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// Actuator manages dynamic instrumentation for processes. It coordinates IR
// generation, eBPF compilation, program loading, and attachment.
//
// The Actuator is multi-tenant: regardless of the number of tenants, we want
// to have a single instance of the Actuator to coordinate resource usage.
type Actuator struct {
	mu struct {
		sync.Mutex
		maxTenantID tenantID
		tenants     map[tenantID]*Tenant
		sinks       map[ir.ProgramID]Sink
	}
	loader Loader

	// Channel for sending events to the state machine processing goroutine
	// This is send-only from the perspective of external API.
	events chan<- event

	// Shutdown controls
	wg sync.WaitGroup

	// Prevents external updates from being sent after shutdown has started.
	shuttingDown <-chan struct{}
	shutdownOnce sync.Once
	// The error that caused the shutdown, if any.
	shutdownErr error
}

// Tenant is a tenant of the Actuator.
type Tenant struct {
	name        string
	id          tenantID
	a           *Actuator
	reporter    Reporter
	irGenerator IRGenerator
}

// NewTenant creates a new tenant of the Actuator.
func (a *Actuator) NewTenant(
	name string, reporter Reporter, irGenerator IRGenerator,
) *Tenant {
	t := &Tenant{
		a:           a,
		name:        name,
		reporter:    reporter,
		irGenerator: irGenerator,
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mu.maxTenantID++
	t.id = a.mu.maxTenantID
	a.mu.tenants[t.id] = t
	return t
}

// NewActuator creates a new Actuator instance.
func NewActuator(loader Loader) *Actuator {
	shuttingDownCh := make(chan struct{})
	eventCh := make(chan event)
	a := &Actuator{
		events:       eventCh,
		shuttingDown: shuttingDownCh,
		loader:       loader,
	}
	a.mu.sinks = make(map[ir.ProgramID]Sink)
	a.mu.tenants = make(map[tenantID]*Tenant)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runEventProcessor(eventCh, shuttingDownCh)
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		_ = a.runDispatcher()
	}()

	return a
}

// runDispatcher runs in a separate goroutine and processes messages from the
// ringbuffer and to hand them to the dispatcher.
func (a *Actuator) runDispatcher() (retErr error) {
	defer func() {
		if retErr != nil {
			go a.shutdown(fmt.Errorf("error in dispatcher: %w", retErr))
		}
	}()
	reader := a.loader.OutputReader()
	inShutdown := func() bool {
		select {
		case <-a.shuttingDown:
			return true
		default:
			return false
		}
	}
	for {
		if inShutdown() {
			return nil
		}
		rec := recordPool.Get().(*ringbuf.Record)
		if err := reader.ReadInto(rec); err != nil {
			if errors.Is(err, ringbuf.ErrFlushed) {
				continue
			}
			return fmt.Errorf("error reading message: %w", err)
		}

		// TODO: Improve error handling here.
		//
		// Perhaps we want to find a way to only partially fail. Alternatively,
		// this interface should not be delivering errors at all.
		if err := a.handleMessage(Message{
			rec: rec,
		}); err != nil && !inShutdown() {
			log.Errorf("error handling message: %v", err)
			return fmt.Errorf("error handling message: %w", err)
		}
	}
}

func (a *Actuator) handleMessage(rec Message) error {
	defer rec.Release()

	ev := rec.Event()
	evHeader, err := ev.Header()
	if err != nil {
		return fmt.Errorf("error getting event header: %w", err)
	}

	progID := ir.ProgramID(evHeader.Prog_id)
	sink, ok := a.getSink(progID)
	if !ok {
		return fmt.Errorf("no sink for program %d", progID)
	}
	if err := sink.HandleEvent(ev); err != nil {
		return fmt.Errorf("error handling event: %w", err)
	}
	return nil
}

func (a *Actuator) getSink(progID ir.ProgramID) (Sink, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sink, ok := a.mu.sinks[progID]
	return sink, ok
}

// HandleUpdate processes an update to process instrumentation configuration.
// This is the single public API for updating the actuator state.
func (t *Tenant) HandleUpdate(update ProcessesUpdate) {
	if log.ShouldLog(log.TraceLvl) {
		logUpdate := update
		log.Tracef("sending update: %v", &logUpdate)
	}

	select {
	case <-t.a.shuttingDown: // prioritize shutdown
	default:
		select {
		case <-t.a.shuttingDown:
		case t.a.events <- eventProcessesUpdated{
			tenantID: t.id,
			updated:  update.Processes,
			removed:  update.Removals,
		}:
		}
	}
}

// runEventProcessor runs in a separate goroutine and processes events sequentially
// to maintain state machine consistency. Only this goroutine accesses state.
func (a *Actuator) runEventProcessor(
	eventCh <-chan event, shuttingDownCh chan<- struct{},
) {
	defer a.loader.OutputReader().Flush()
	state := newState()
	for !state.isShutdown() {
		event := <-eventCh
		if _, isShutdown := event.(eventShutdown); isShutdown {
			log.Debugf("received shutdown event")
			close(shuttingDownCh)
		}
		log.Tracef("event: %v", event)
		err := handleEvent(state, (*effects)(a), event)
		if err != nil {
			log.Errorf("error handling event %T: %v", event, err)

			// Trigger shutdown on error. Cannot run directly on this goroutine
			// because it will deadlock. Note that if we're already shutting
			// down, this will be a no-op.
			go a.shutdown(fmt.Errorf("event handling error: %w", err))
		}
	}
}

// sendEvent sends an event to the state machine processor
func (a *effects) sendEvent(event event) {
	a.events <- event
}

// Implementation of effectHandler interface

type effects Actuator

var _ effectHandler = (*effects)(nil)

// loadProgram starts BPF program loading in a background goroutine.
func (a *effects) loadProgram(
	tenantID tenantID,
	programID ir.ProgramID,
	executable Executable,
	processID ProcessID,
	probes []ir.ProbeDefinition,
) {
	tenant := a.getTenant(tenantID)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ir, err := generateIR(tenant.irGenerator, programID, executable, probes)
		if err == nil && len(ir.Probes) == 0 {
			err = &NoSuccessfulProbesError{Issues: ir.Issues}
		}
		if err != nil {
			tenant.reporter.ReportIRGenFailed(processID, err, probes)
			a.sendEvent(eventProgramLoadingFailed{
				programID: programID,
				err:       err,
			})
			return
		}
		loaded, err := loadProgram(a.loader, ir)
		if err != nil {
			tenant.reporter.ReportLoadingFailed(processID, ir, err)
			a.sendEvent(eventProgramLoadingFailed{
				programID: ir.ID,
				err:       err,
			})
			return
		}
		sink, err := tenant.reporter.ReportLoaded(processID, executable, ir)
		if err != nil {
			loaded.Close()
			a.sendEvent(eventProgramLoadingFailed{
				programID: ir.ID,
				err:       err,
			})
			return
		}
		a.setSink(ir.ID, sink)
		a.sendEvent(eventProgramLoaded{
			programID: ir.ID,
			loaded: &loadedProgram{
				program:  *loaded,
				ir:       ir,
				tenantID: tenantID,
			},
		})
	}()
}

func (a *effects) getTenant(tenantID tenantID) *Tenant {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mu.tenants[tenantID]
}

func (a *effects) setSink(progID ir.ProgramID, sink Sink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mu.sinks[progID] = sink
}

// clearSink removes a sink from the map if present.
func (a *effects) clearSink(progID ir.ProgramID) {
	a.mu.Lock()
	delete(a.mu.sinks, progID)
	a.mu.Unlock()
}

// unloadProgram performs the cleanup of a loaded program asynchronously and
// notifies the state-machine once it is complete.
func (a *effects) unloadProgram(lp *loadedProgram) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		// Close kernel program & links.
		lp.program.Close()

		// TODO: We should flush the ringbuffer here to make sure
		// that there are no events that haven't been processed that
		// could possibly be affected by closing the sink.

		// Close sink and unregister it so the dispatcher stops using it.
		if lp.sink != nil {
			lp.sink.Close()
			a.clearSink(lp.ir.ID)
		}

		// Notify state-machine that unloading is finished.
		a.sendEvent(eventProgramUnloaded{programID: lp.ir.ID})
	}()
}

func generateIR(
	irGenerator IRGenerator,
	programID ir.ProgramID,
	executable Executable,
	probes []ir.ProbeDefinition,
) (*ir.Program, error) {
	elfFile, err := object.OpenElfFile(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read object file for %s: %w", executable.Path, err,
		)
	}
	defer elfFile.Close()

	ir, err := irGenerator.GenerateIR(programID, elfFile, probes)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to generate IR for %s: %w", executable.Path, err,
		)
	}

	return ir, nil
}

func loadProgram(
	loader Loader,
	ir *ir.Program,
) (*loader.Program, error) {
	program, err := compiler.GenerateProgram(ir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate program: %w", err)
	}
	loaded, err := loader.Load(program)
	if err != nil {
		return nil, fmt.Errorf("failed to load program: %w", err)
	}
	return loaded, nil
}

// attachToProcess attaches a loaded program to a specific process.
func (a *effects) attachToProcess(
	loaded *loadedProgram, executable Executable, processID ProcessID,
) {
	tenant := a.getTenant(loaded.tenantID)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		attached, err := attachToProcess(
			tenant.id, loaded, executable, processID,
		)
		if err != nil {
			tenant.reporter.ReportAttachingFailed(processID, loaded.ir, err)
			a.sendEvent(eventProgramAttachingFailed{
				programID: loaded.ir.ID,
				err:       fmt.Errorf("failed to attach to process: %w", err),
			})
		} else {
			// Tell the reporter we've attached the probes.
			//
			// Note: there's something perhaps off about performing this call
			// here is that the state machine will not have been updated yet to
			// reflect that the probes are attached.
			//
			// At the time of writing, that external state was not exposed
			// anywhere, and does not have any visible impact on the API, but
			// it's still a bit of a smell. It's not clear, however, that this
			// callback ought to be called on the state machine goroutine.
			tenant.reporter.ReportAttached(processID, loaded.ir)
			a.sendEvent(eventProgramAttached{
				program: attached,
			})
		}
	}()
}

func attachToProcess(
	tenantID tenantID,
	loaded *loadedProgram,
	executable Executable,
	processID ProcessID,
) (*attachedProgram, error) {

	// A silly thing here is that it's going to call, under the hood,
	// safeelf.Open twice: once for the link package and once for finding the
	// text section offset to translate the attachpoints.
	linkExe, err := link.OpenExecutable(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", executable.Path, err,
		)
	}
	elfFile, err := safeelf.Open(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", executable.Path, err,
		)
	}
	defer elfFile.Close()

	textSection, err := object.FindTextSectionHeader(elfFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get text section: %w", err)
	}

	attached := make([]link.Link, 0, len(loaded.program.Attachpoints))
	for _, attachpoint := range loaded.program.Attachpoints {
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		l, err := linkExe.Uprobe(
			"",
			loaded.program.BpfProgram,
			&link.UprobeOptions{
				PID:     int(processID.PID),
				Address: addr,
				Offset:  0,
				Cookie:  attachpoint.Cookie,
			},
		)
		if err != nil {
			// Clean up any previously attached probes.
			for _, prev := range attached {
				prev.Close()
			}
			return nil, fmt.Errorf(
				"failed to attach uprobe at 0x%x: %w", addr, err,
			)
		}
		attached = append(attached, l)
	}

	return &attachedProgram{
		tenantID:       tenantID,
		procID:         processID,
		ir:             loaded.ir,
		executableLink: linkExe,
		attachedLinks:  attached,
	}, nil
}

// detachFromProcess detaches a program from a process.
func (a *effects) detachFromProcess(ap *attachedProgram) {
	tenant := a.getTenant(ap.tenantID)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for _, link := range ap.attachedLinks {
			if err := link.Close(); err != nil {
				// What else is there to do if this fails?
				//
				// TODO: Rate limit this log line -- there could be a lot of
				// these.
				log.Errorf("Error closing link: %v", err)
			}
		}

		tenant.reporter.ReportDetached(ap.procID, ap.ir)

		a.sendEvent(eventProgramDetached{
			programID: ap.ir.ID,
			processID: ap.procID,
		})
	}()
}

// Shutdown initiates a clean shutdown of the actuator.
//
// It returns the error that caused the shutdown, if any.
func (a *Actuator) Shutdown() error {
	a.shutdown(nil)
	return a.shutdownErr
}

func (a *Actuator) shutdown(err error) {
	a.shutdownOnce.Do(func() {
		defer log.Debugf("actuator shut down")
		if err != nil {
			log.Warnf("shutting down actuator due to error: %v", err)
		} else {
			log.Debugf("shutting down actuator")
		}
		a.events <- eventShutdown{}
		a.shutdownErr = err

		// Wait for all goroutines to complete
		a.wg.Wait()

		// Close resources
		if err := a.loader.Close(); err != nil {
			log.Errorf("Error closing ringbuffer reader: %v", err)
		}
	})
}
