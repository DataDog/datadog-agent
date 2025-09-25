// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	}

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
	name    string
	id      tenantID
	a       *Actuator
	runtime Runtime
}

// NewTenant creates a new tenant of the Actuator.
func (a *Actuator) NewTenant(
	name string, runtime Runtime,
) *Tenant {
	t := &Tenant{
		a:       a,
		name:    name,
		runtime: runtime,
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mu.maxTenantID++
	t.id = a.mu.maxTenantID
	a.mu.tenants[t.id] = t
	return t
}

// NewActuator creates a new Actuator instance.
func NewActuator() *Actuator {
	shuttingDownCh := make(chan struct{})
	eventCh := make(chan event)
	a := &Actuator{
		events:       eventCh,
		shuttingDown: shuttingDownCh,
	}
	a.mu.tenants = make(map[tenantID]*Tenant)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runEventProcessor(eventCh, shuttingDownCh)
	}()

	return a
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
		loaded, err := tenant.runtime.Load(
			programID, executable, processID, probes,
		)
		if err != nil {
			a.sendEvent(eventProgramLoadingFailed{
				programID: programID,
				err:       err,
			})
			return
		}
		a.sendEvent(eventProgramLoaded{
			programID: programID,
			loaded: &loadedProgram{
				programID: programID,
				loaded:    loaded,
				tenantID:  tenantID,
			},
		})
	}()
}

func (a *effects) getTenant(tenantID tenantID) *Tenant {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mu.tenants[tenantID]
}

// unloadProgram performs the cleanup of a loaded program asynchronously and
// notifies the state-machine once it is complete.
func (a *effects) unloadProgram(lp *loadedProgram) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		// Close kernel program & links.
		lp.loaded.Close()

		// Dataplane/sink lifecycle is owned by loader/runtime now.

		// Notify state-machine that unloading is finished.
		a.sendEvent(eventProgramUnloaded{programID: lp.programID})
	}()
}

// attachToProcess attaches a loaded program to a specific process.
func (a *effects) attachToProcess(
	loaded *loadedProgram, executable Executable, processID ProcessID,
) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		attached, err := loaded.loaded.Attach(processID, executable)
		if err != nil {
			a.sendEvent(eventProgramAttachingFailed{
				programID: loaded.programID,
				err:       fmt.Errorf("failed to attach to process: %w", err),
			})
			return
		}

		program := &attachedProgram{
			loadedProgram:   loaded,
			processID:       processID,
			attachedProgram: attached,
		}
		a.sendEvent(eventProgramAttached{
			program: program,
		})
	}()
}

var detachLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

// detachFromProcess detaches a program from a process.
func (a *effects) detachFromProcess(ap *attachedProgram) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := ap.attachedProgram.Detach(); err != nil {
			if detachLogLimiter.Allow() {
				log.Errorf(
					"failed to detach program %v from process %v: %v",
					ap.programID, ap.processID, err,
				)
			}
		}
		a.sendEvent(eventProgramDetached{
			programID: ap.programID,
			processID: ap.processID,
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
	})
}
