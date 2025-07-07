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
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// Actuator manages dynamic instrumentation for processes. It coordinates IR
// generation, eBPF compilation, program loading, and attachment.
type Actuator struct {
	configuration

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

// NewActuator creates a new Actuator instance.
func NewActuator(
	options ...Option,
) (*Actuator, error) {
	cfg, err := makeConfiguration(options...)
	if err != nil {
		return nil, err
	}

	shutdownCh := make(chan struct{})
	eventCh := make(chan event)
	a := &Actuator{
		configuration: cfg,
		events:        eventCh,
		shuttingDown:  shutdownCh,
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runEventProcessor(eventCh, shutdownCh)
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		_ = a.runDispatcher()
	}()

	return a, nil
}

// runDispatcher runs in a separate goroutine and processes messages from the
// ringbuffer and to hand them to the dispatcher.
func (a *Actuator) runDispatcher() (err error) {
	defer func() {
		if err != nil {
			go a.shutdown(fmt.Errorf("error in dispatcher: %w", err))
		}
	}()
	for {
		select {
		case <-a.shuttingDown:
			return
		default:
		}

		rec := recordPool.Get().(*ringbuf.Record)
		if err := a.loader.OutputReader().ReadInto(rec); err != nil {
			if errors.Is(err, ringbuf.ErrFlushed) {
				continue
			}
			return err
		}
		message := Message{rec: rec}
		err = a.sink.HandleMessage(message)
		// TODO: Improve error handling here.
		//
		// Perhaps we want to find a way to only partially fail. Alternatively,
		// this interface should not be delivering errors at all.
		if err != nil {
			select {
			case <-a.shuttingDown:
				recordPool.Put(rec)
				return
			default:
				log.Errorf("Error handling message: %v", err)
				go a.shutdown(fmt.Errorf("error handling message: %w", err))
				return
			}
		}
	}
}

// HandleUpdate processes an update to process instrumentation configuration.
// This is the single public API for updating the actuator state.
func (a *Actuator) HandleUpdate(update ProcessesUpdate) {
	log.Debugf("sending update: %v", update)

	// Make sure we don't send the update event if we're shutting down.
	select {
	case <-a.shuttingDown:
	default:
		select {
		case <-a.shuttingDown:
		case a.events <- eventProcessesUpdated{
			updated: update.Processes,
			removed: update.Removals,
		}:
		}
	}
}

// runEventProcessor runs in a separate goroutine and processes events sequentially
// to maintain state machine consistency. Only this goroutine accesses state.
func (a *Actuator) runEventProcessor(eventCh <-chan event, shuttingDownCh chan<- struct{}) {
	defer a.loader.OutputReader().Flush()
	state := newState()
	for !state.isShutdown() {
		event := <-eventCh
		if _, isShutdown := event.(eventShutdown); isShutdown {
			log.Debugf("Received shutdown event")
			close(shuttingDownCh)
		}
		log.Debugf("event: %v", event)
		err := handleEvent(state, (*effects)(a), event)
		if err != nil {
			log.Errorf("Error handling event %T: %v", event, err)

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
	programID ir.ProgramID,
	executable Executable,
	probes []ir.ProbeDefinition,
) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ir, err := generateIR(programID, executable, probes)
		if err != nil {
			a.reporter.ReportIRGenFailed(programID, err, probes)
			a.sendEvent(eventProgramLoadingFailed{
				programID: ir.ID,
				err:       err,
			})
			return
		}
		loaded, err := loadProgram(a.loader, ir)
		if err != nil {
			a.reporter.ReportLoadingFailed(ir, err)
			a.sendEvent(eventProgramLoadingFailed{
				programID: ir.ID,
				err:       err,
			})
			return
		}
		a.sendEvent(eventProgramLoaded{
			programID: ir.ID,
			loaded: &loadedProgram{
				program: *loaded,
				ir:      ir,
			},
		})
	}()
}

func generateIR(
	programID ir.ProgramID,
	executable Executable,
	probes []ir.ProbeDefinition,
) (*ir.Program, error) {
	elfFile, err := safeelf.Open(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", executable.Path, err,
		)
	}
	defer elfFile.Close()

	objFile, err := object.NewElfObject(elfFile)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create object interface for %s: %w", executable.Path, err,
		)
	}

	ir, err := irgen.GenerateIR(programID, objFile, probes)
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
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		attached, err := attachToProcess(
			loaded, executable, processID, a.reporter,
		)
		if err != nil {
			a.reporter.ReportAttachingFailed(processID, loaded.ir, err)
			a.sendEvent(eventProgramAttachingFailed{
				programID: loaded.ir.ID,
				err:       fmt.Errorf("failed to attach to process: %w", err),
			})
		} else {
			a.sendEvent(eventProgramAttached{
				program: attached,
			})
		}
	}()
}

func attachToProcess(
	loaded *loadedProgram,
	executable Executable,
	processID ProcessID,
	reporter Reporter,
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

	// Tell the reporter we've attached the probes.
	//
	// Note: there's something perhaps off about performing this call here is
	// that the state machine will not have been updated yet to reflect that the
	// probes are attached.
	//
	// At the time of writing, that external state was not exposed anywhere, and
	// does not have any visible impact on the API, but it's still a bit of a
	// smell. It's not, however, clear that this callback ought to be called on
	// the state machine goroutine.
	reporter.ReportAttached(processID, loaded.ir)

	return &attachedProgram{
		ir:             loaded.ir,
		procID:         processID,
		executableLink: linkExe,
		attachedLinks:  attached,
	}, nil
}

// registerProgramWithDispatcher registers a program with the event dispatcher.
func (a *effects) registerProgramWithDispatcher(irProgram *ir.Program) {
	a.sink.RegisterProgram(irProgram)
}

// unregisterProgramWithDispatcher unregisters a program from the event dispatcher.
//
// TODO: We need to do something to make sure all messages that have already
// been produced are processed before the program is unregistered. This will
// involve coordinating some flushing with the dispatcher goroutine.
func (a *effects) unregisterProgramWithDispatcher(programID ir.ProgramID) {
	a.sink.UnregisterProgram(programID)
}

// detachFromProcess detaches a program from a process.
func (a *effects) detachFromProcess(ap *attachedProgram) {
	go func() {
		for _, link := range ap.attachedLinks {
			if err := link.Close(); err != nil {
				// What else is there to do if this fails?
				//
				// TODO: Rate limit this log line -- there could be a lot of
				// these.
				log.Errorf("Error closing link: %v", err)
			}
		}

		if a.reporter != nil {
			a.reporter.ReportDetached(ap.procID, ap.ir)
		}

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
