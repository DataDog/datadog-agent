// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CircuitBreakerConfig configures the circuit breaker for enforcing probe CPU limits.
type CircuitBreakerConfig struct {
	// Interval is the interval at which probe CPU usage is checked.
	Interval time.Duration
	// PerProbeCPULimit is the limit on mean CPUs/s usage per core per probe within the interval.
	PerProbeCPULimit float64
	// AllProbesCPULimit is the limit on mean CPUs/s usage per core for all probes within the interval.
	AllProbesCPULimit float64
	// InterruptOverhead is the estimate of the cost of an interrupt incurred on every probe hit.
	InterruptOverhead time.Duration
}

// Actuator manages dynamic instrumentation for processes. It coordinates IR
// generation, eBPF compilation, program loading, and attachment.
type Actuator struct {
	// Runtime handles program loading and attachment.
	// Set via SetRuntime() method.
	runtime atomic.Pointer[Runtime]

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

// Stats returns the current stats of the Actuator.
func (a *Actuator) Stats() map[string]any {
	metricsChan := make(chan Metrics, 1)
	select {
	case <-a.shuttingDown:
		return nil
	case a.events <- eventGetMetrics{metricsChan: metricsChan}:
		select {
		case <-a.shuttingDown:
			return nil
		case metrics := <-metricsChan:
			return metrics.AsStats()
		}
	}
}

// NewActuator creates a new Actuator instance.
func NewActuator(breakerCfg CircuitBreakerConfig) *Actuator {
	shuttingDownCh := make(chan struct{})
	eventCh := make(chan event)
	a := &Actuator{
		events:       eventCh,
		shuttingDown: shuttingDownCh,
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runEventProcessor(breakerCfg, eventCh, shuttingDownCh)
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.heartbeatLoop(breakerCfg.Interval)
	}()
	return a
}

// SetRuntime initializes the actuator with a runtime and makes it ready to use.
func (a *Actuator) SetRuntime(runtime Runtime) {
	a.runtime.Store(&runtime)
}

// HandleUpdate processes an update to process instrumentation configuration.
// This is the single public API for updating the actuator state.
func (a *Actuator) HandleUpdate(update ProcessesUpdate) {
	if log.ShouldLog(log.TraceLvl) {
		logUpdate := update
		log.Tracef("sending update: %v", &logUpdate)
	}

	select {
	case <-a.shuttingDown: // prioritize shutdown
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
func (a *Actuator) runEventProcessor(
	breakerCfg CircuitBreakerConfig,
	eventCh <-chan event,
	shuttingDownCh chan<- struct{},
) {
	state := newState(breakerCfg)
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

func (a *Actuator) heartbeatLoop(interval time.Duration) {
	heartbeat := time.NewTicker(interval)
	defer heartbeat.Stop()
	for {
		select {
		case <-a.shuttingDown:
			return
		case <-heartbeat.C:
			select {
			case <-a.shuttingDown:
				return
			case a.events <- eventHeartbeatCheck{}:
			}
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
	processID ProcessID,
	probes []ir.ProbeDefinition,
) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		runtimePtr := a.runtime.Load()
		if runtimePtr == nil {
			a.sendEvent(eventProgramLoadingFailed{
				programID: programID,
			})
			return
		}
		runtime := *runtimePtr
		loaded, err := runtime.Load(
			programID, executable, processID, probes,
		)
		if err != nil {
			a.sendEvent(eventProgramLoadingFailed{
				programID: programID,
			})
			return
		}
		a.sendEvent(eventProgramLoaded{
			programID: programID,
			loaded: &loadedProgram{
				programID: programID,
				loaded:    loaded,
			},
		})
	}()
}

// unloadProgram performs the cleanup of a loaded program asynchronously and
// notifies the state-machine once it is complete.
func (a *effects) unloadProgram(lp *loadedProgram) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		lp.loaded.Close()
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
			})
			return
		}

		a.sendEvent(eventProgramAttached{
			program: &attachedProgram{
				loadedProgram:   loaded,
				processID:       processID,
				attachedProgram: attached,
			},
		})
	}()
}

var detachLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

// detachFromProcess detaches a program from a process.
func (a *effects) detachFromProcess(ap *attachedProgram, failure error) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := ap.attachedProgram.Detach(failure); err != nil {
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
