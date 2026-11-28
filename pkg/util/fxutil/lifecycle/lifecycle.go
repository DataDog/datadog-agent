// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lifecycle records coarse startup and shutdown timings of an Agent process.
//
// It is always on and only keeps a handful of timestamps, so it can run in every process. The
// timings are published as internal telemetry (see comp/core/fxinstrumentation) and reported to
// Datadog by Agent telemetry (COAT).
//
// Shutdown timings cannot be sent by the process that is shutting down: the telemetry pipeline is
// itself stopped during shutdown, and the slowest shutdowns end with a SIGKILL. The recorder
// therefore persists a small record to disk while the process runs and stops, and the next start
// of the same process reports it.
//
// The package only depends on the standard library so that it can be used from pkg/util/fxutil.
package lifecycle

import (
	"slices"
	"sync"
	"time"
)

// Startup phases.
const (
	// PhaseInit is the time between the process start and the creation of the Fx application.
	PhaseInit = "init"
	// PhaseFxConstruct is the time spent building the Fx application (running constructors).
	PhaseFxConstruct = "fx_construct"
	// PhaseFxStart is the time spent running the Fx OnStart hooks.
	PhaseFxStart = "fx_start"
	// PhaseAppStart is the time between the end of the Fx start and MarkReady, for processes that
	// do most of their startup outside of Fx (see ExpectReadyMarker).
	PhaseAppStart = "app_start"
	// PhaseTotal is the full duration of the startup or shutdown.
	PhaseTotal = "total"
)

// Shutdown phases.
const (
	// PhaseAppStop is the time spent in the process-specific teardown that runs before the Fx
	// application is stopped.
	PhaseAppStop = "app_stop"
	// PhaseFxStop is the time spent running the Fx OnStop hooks.
	PhaseFxStop = "fx_stop"
)

// Component stages.
const (
	// StageConstruct is an Fx constructor or decorator.
	StageConstruct = "construct"
	// StageOnStart is an Fx OnStart hook.
	StageOnStart = "on_start"
	// StageOnStop is an Fx OnStop hook.
	StageOnStop = "on_stop"
	// StageModuleStart is the creation of a system-probe module.
	StageModuleStart = "module_start"
	// StageModuleStop is the closing of a system-probe module.
	StageModuleStop = "module_stop"
)

// Shutdown triggers.
const (
	TriggerSignal  = "signal"
	TriggerCommand = "stop_command"
	TriggerError   = "error"
	TriggerUnknown = "unknown"
)

const (
	// MaxSlowComponents is the number of slowest components kept for the startup and the shutdown.
	MaxSlowComponents = 5
	// DefaultSlowComponentThreshold is the minimum duration for a component to be reported as slow.
	DefaultSlowComponentThreshold = 250 * time.Millisecond
)

// Timing is the duration of a named step.
type Timing struct {
	Name    string  `json:"name"`
	Stage   string  `json:"stage,omitempty"`
	Seconds float64 `json:"seconds"`
}

// Startup is the summary of a process startup, available once the process is ready.
type Startup struct {
	// Phases maps a startup phase (Phase* constants) to its duration in seconds.
	Phases map[string]float64
	// SlowComponents are the slowest components over the threshold, slowest first.
	SlowComponents []Timing
}

// Recorder records the startup and shutdown timings of a process.
//
// All methods are safe for concurrent use. Only the first call to the one-time markers
// (FxStarted, MarkReady, BeginStop, Finish...) is taken into account, as some processes start
// several Fx applications.
type Recorder struct {
	mu  sync.Mutex
	now func() time.Time

	slowThreshold time.Duration
	processStart  time.Time

	// startup
	fxNewAt       time.Time
	fxStartedAt   time.Time
	explicitReady bool
	ready         bool
	startup       Startup
	onReady       []func(Startup)

	// shutdown
	stopStart time.Time
	finished  bool
	record    Record

	// persistence
	path    string
	onError func(error)
}

var defaultRecorder = NewRecorder(time.Now(), time.Now)

// Default returns the recorder of the current process.
func Default() *Recorder {
	return defaultRecorder
}

// NewRecorder returns a recorder for a process started at processStart.
func NewRecorder(processStart time.Time, now func() time.Time) *Recorder {
	return &Recorder{
		now:           now,
		slowThreshold: DefaultSlowComponentThreshold,
		processStart:  processStart,
		startup:       Startup{Phases: map[string]float64{}},
		record: Record{
			Version:        recordVersion,
			State:          StateRunning,
			ProcessStartAt: processStart,
			Phases:         map[string]float64{},
		},
	}
}

// SetSlowComponentThreshold sets the minimum duration for a component to be reported as slow.
func (r *Recorder) SetSlowComponentThreshold(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slowThreshold = d
}

// ExpectReadyMarker indicates that the process calls MarkReady itself, because a significant part
// of its startup happens after the Fx application has started. Without it, the process is
// considered ready as soon as the Fx application has started.
//
// It must be called before the Fx application is started.
func (r *Recorder) ExpectReadyMarker() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.explicitReady = true
}

// BeforeFxNew must be called right before the Fx application is created.
func (r *Recorder) BeforeFxNew() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.fxNewAt.IsZero() {
		return
	}
	r.fxNewAt = r.now()
	r.startup.Phases[PhaseInit] = r.fxNewAt.Sub(r.processStart).Seconds()
}

// FxConstructed must be called right after the Fx application is created.
func (r *Recorder) FxConstructed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fxNewAt.IsZero() {
		return
	}
	if _, ok := r.startup.Phases[PhaseFxConstruct]; ok {
		return
	}
	r.startup.Phases[PhaseFxConstruct] = r.now().Sub(r.fxNewAt).Seconds()
}

// FxStarted must be called once the Fx application has started, with the time the start began.
func (r *Recorder) FxStarted(startBegin time.Time) {
	r.mu.Lock()
	if !r.fxStartedAt.IsZero() {
		r.mu.Unlock()
		return
	}
	r.fxStartedAt = r.now()
	r.startup.Phases[PhaseFxStart] = r.fxStartedAt.Sub(startBegin).Seconds()
	explicit := r.explicitReady
	r.mu.Unlock()

	if !explicit {
		r.MarkReady()
	}
}

// MarkReady marks the process as ready: the startup is complete.
func (r *Recorder) MarkReady() {
	r.mu.Lock()
	if r.ready {
		r.mu.Unlock()
		return
	}
	r.ready = true
	now := r.now()
	if r.explicitReady && !r.fxStartedAt.IsZero() {
		r.startup.Phases[PhaseAppStart] = now.Sub(r.fxStartedAt).Seconds()
	}
	r.startup.Phases[PhaseTotal] = now.Sub(r.processStart).Seconds()
	r.record.ReadyAt = now
	r.persistLocked()

	startup := r.startupLocked()
	callbacks := r.onReady
	r.onReady = nil
	r.mu.Unlock()

	for _, cb := range callbacks {
		cb(startup)
	}
}

// OnReady registers a callback called once the process is ready, or right away if it already is.
func (r *Recorder) OnReady(cb func(Startup)) {
	r.mu.Lock()
	if !r.ready {
		r.onReady = append(r.onReady, cb)
		r.mu.Unlock()
		return
	}
	startup := r.startupLocked()
	r.mu.Unlock()
	cb(startup)
}

// RecordComponent records the duration of a component for the given stage (Stage* constants).
// Only the slowest components over the threshold are kept.
func (r *Recorder) RecordComponent(stage, name string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	shutdown := stage == StageOnStop || stage == StageModuleStop
	if shutdown && r.finished {
		// A hook that kept running after a failed shutdown.
		return
	}
	if shutdown && r.record.CurrentComponent == name {
		r.record.CurrentComponent = ""
	}
	if d < r.slowThreshold {
		return
	}
	t := Timing{Name: name, Stage: stage, Seconds: d.Seconds()}
	if shutdown {
		r.record.SlowComponents = insertSlow(r.record.SlowComponents, t)
	} else if !r.ready {
		r.startup.SlowComponents = insertSlow(r.startup.SlowComponents, t)
	}
}

// ComponentStopping records that a component started to stop, so that a process killed during its
// shutdown reports the component it was stuck on.
func (r *Recorder) ComponentStopping(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopStart.IsZero() || r.finished {
		return
	}
	r.record.CurrentComponent = name
	r.persistLocked()
}

// BeginStop marks the beginning of the shutdown. trigger is one of the Trigger* constants.
func (r *Recorder) BeginStop(trigger string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopStart.IsZero() {
		return
	}
	r.stopStart = r.now()
	r.record.State = StateStopping
	r.record.Trigger = trigger
	r.record.StopStartAt = r.stopStart
	r.persistLocked()
}

// StartShutdownPhase marks the beginning of a shutdown phase (PhaseAppStop, PhaseFxStop) and returns
// a function to call when the phase is complete. It implicitly begins the shutdown if needed.
func (r *Recorder) StartShutdownPhase(phase string) func() {
	r.BeginStop(TriggerUnknown)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return func() {}
	}
	start := r.now()
	r.record.CurrentPhase = phase
	r.persistLocked()

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.finished {
			return
		}
		r.record.Phases[phase] += r.now().Sub(start).Seconds()
		if r.record.CurrentPhase == phase {
			r.record.CurrentPhase = ""
		}
		r.persistLocked()
	}
}

// Finish marks the end of the shutdown, with the error the shutdown returned, if any. It does nothing
// if the shutdown has not begun, for instance for short-lived commands.
//
// When err is not nil, the shutdown is recorded as failed and the phase and component in progress
// are kept: after a timed out Fx stop, they are the hook that did not return. Shutdown phases that
// have not completed are not recorded.
func (r *Recorder) Finish(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopStart.IsZero() || r.finished {
		return
	}
	r.finished = true
	if err != nil {
		r.record.State = StateStopFailed
	} else {
		r.record.State = StateStopped
		r.record.CurrentPhase = ""
		r.record.CurrentComponent = ""
	}
	r.record.Phases[PhaseTotal] = r.now().Sub(r.stopStart).Seconds()
	r.persistLocked()
}

// Persist enables persisting the lifecycle record at path. The previous record at that path, if
// any, must be loaded with LoadPrevious before calling Persist. onError is called with the errors
// encountered when writing the record; it must not call the recorder.
func (r *Recorder) Persist(path string, onError func(error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = path
	r.onError = onError
	r.persistLocked()
}

func (r *Recorder) persistLocked() {
	if r.path == "" {
		return
	}
	r.record.UpdatedAt = r.now()
	if err := writeRecord(r.path, r.record); err != nil && r.onError != nil {
		r.onError(err)
	}
}

func (r *Recorder) startupLocked() Startup {
	s := Startup{
		Phases:         make(map[string]float64, len(r.startup.Phases)),
		SlowComponents: slices.Clone(r.startup.SlowComponents),
	}
	for k, v := range r.startup.Phases {
		s.Phases[k] = v
	}
	return s
}

// insertSlow inserts t in the list sorted by decreasing duration, keeping MaxSlowComponents entries.
func insertSlow(list []Timing, t Timing) []Timing {
	i, _ := slices.BinarySearchFunc(list, t, func(e, t Timing) int {
		switch {
		case e.Seconds > t.Seconds:
			return -1
		case e.Seconds < t.Seconds:
			return 1
		}
		return 0
	})
	if i >= MaxSlowComponents {
		return list
	}
	list = slices.Insert(list, i, t)
	if len(list) > MaxSlowComponents {
		list = list[:MaxSlowComponents]
	}
	return list
}
