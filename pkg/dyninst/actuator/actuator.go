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

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// Actuator manages dynamic instrumentation for processes.  It coordinates IR
// generation, eBPF compilation, program loading, and attachment.
type Actuator struct {
	configuration

	// Channel for sending events to the state machine processing goroutine
	// This is send-only from the perspective of external API.
	events chan<- event

	// Pre-created ringbuffer for collecting probe output
	ringbufMap    *ebpf.Map
	ringbufReader *ringbuf.Reader

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
	settings := defaultSettings
	for _, option := range options {
		option.apply(&settings)
	}

	// Pre-create the ringbuffer that will be shared across all BPF programs.
	ringbufMap, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       compiler.RingbufMapName,
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(settings.ringBufSize),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ringbuffer map: %w", err)
	}
	ringbufReader, err := ringbuf.NewReader(ringbufMap)
	if err != nil {
		if mapCloseErr := ringbufMap.Close(); mapCloseErr != nil {
			err = fmt.Errorf(
				"%w (also failed to close ringbuffer map %w)", err, mapCloseErr,
			)
		}
		return nil, fmt.Errorf("failed to create ringbuffer reader: %w", err)
	}

	shutdownCh := make(chan struct{})
	eventCh := make(chan event)
	a := &Actuator{
		configuration: settings,
		events:        eventCh,
		ringbufMap:    ringbufMap,
		ringbufReader: ringbufReader,
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
		if err := a.ringbufReader.ReadInto(rec); err != nil {
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
	log.Debugf("Sending update: %#v", update)

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
	defer a.ringbufReader.Flush()
	state := newState()
	for !state.isShutdown() {
		event := <-eventCh
		if _, isShutdown := event.(eventShutdown); isShutdown {
			log.Debugf("Received shutdown event")
			close(shuttingDownCh)
		}
		log.Debugf("event: %#v", event)
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

// compileProgram starts eBPF compilation for an IR program in a background
// goroutine.
func (a *effects) compileProgram(
	programID ir.ProgramID,
	executable Executable,
	probes []config.Probe,
) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		compiled, err := compileProgram(
			programID, executable, probes, a.configuration.codegenWriter,
		)
		if err != nil {
			err = fmt.Errorf("failed to compile eBPF program: %w", err)
			a.sendEvent(eventProgramCompilationFailed{
				programID: programID,
				err:       err,
			})
			return
		}
		a.compiledCallback(compiled)
		a.sendEvent(eventProgramCompiled{
			programID:       programID,
			compiledProgram: compiled,
		})
	}()
}

func compileProgram(
	programID ir.ProgramID,
	exe Executable,
	probes []config.Probe,
	cwf CodegenWriterFactory,
) (*CompiledProgram, error) {

	elfFile, err := safeelf.Open(exe.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", exe.Path, err,
		)
	}
	defer elfFile.Close()

	objFile, err := object.NewElfObject(elfFile)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create object interface for %s: %w", exe.Path, err,
		)
	}

	irProgram, err := irgen.GenerateIR(programID, objFile, probes)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to generate IR for %s: %w", exe.Path, err,
		)
	}

	// Compile the IR to eBPF
	extraCodeSink := cwf(irProgram)
	compiled, err := compiler.CompileBPFProgram(
		irProgram, extraCodeSink,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to compile eBPF program: %w", err,
		)
	}

	return &CompiledProgram{
		IR:          irProgram,
		Probes:      probes,
		CompiledBPF: compiled,
	}, nil
}

// loadProgram starts BPF program loading in a background goroutine.
func (a *effects) loadProgram(compiled *CompiledProgram) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		loaded, err := loadProgram(a.ringbufMap, compiled)
		if err != nil {
			a.sendEvent(eventProgramCompilationFailed{
				programID: compiled.IR.ID,
				err:       fmt.Errorf("failed to load collection spec: %w", err),
			})
		} else {
			a.sendEvent(eventProgramLoaded{
				programID:     compiled.IR.ID,
				loadedProgram: loaded,
			})
		}
	}()
}

func loadProgram(
	sharedRingbufMap *ebpf.Map,
	compiled *CompiledProgram,
) (*loadedProgram, error) {
	spec, err := ebpf.LoadCollectionSpecFromReader(compiled.CompiledBPF.Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to load collection spec: %w", err)
	}

	ringbufMapSpec, ok := spec.Maps[compiler.RingbufMapName]
	if !ok {
		return nil, fmt.Errorf("ringbuffer map not found in collection spec")
	}
	ringbufMapSpec.MaxEntries = defaultRingbufSize

	mapReplacements := map[string]*ebpf.Map{
		compiler.RingbufMapName: sharedRingbufMap,
	}

	opts := ebpf.CollectionOptions{
		MapReplacements: mapReplacements,
	}
	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create BPF collection: %w", err)
	}

	bpfProg, ok := bpfCollection.Programs[compiled.CompiledBPF.ProgramName]
	if !ok {
		return nil, fmt.Errorf("BPF program %s not found in collection", compiled.CompiledBPF.ProgramName)
	}

	return &loadedProgram{
		id:           compiled.IR.ID,
		collection:   bpfCollection,
		program:      bpfProg,
		attachpoints: compiled.CompiledBPF.Attachpoints,
	}, nil
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
			a.sendEvent(eventProgramAttachingFailed{
				programID: loaded.id,
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

	attached := make([]link.Link, 0, len(loaded.attachpoints))
	for _, attachpoint := range loaded.attachpoints {
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		l, err := linkExe.Uprobe(
			"",
			loaded.program,
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
	reporter.ReportAttached(processID, loaded.probes)

	return &attachedProgram{
		progID:         loaded.id,
		procID:         processID,
		executableLink: linkExe,
		attachedLinks:  attached,
		probes:         loaded.probes,
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
			a.reporter.ReportDetached(ap.procID, ap.probes)
		}

		a.sendEvent(eventProgramDetached{
			programID: ap.progID,
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
		if err := a.ringbufReader.Close(); err != nil {
			log.Errorf("Error closing ringbuffer reader: %v", err)
		}
		if err := a.ringbufMap.Close(); err != nil {
			log.Errorf("Error closing ringbuffer map: %v", err)
		}
	})
}
