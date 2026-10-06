// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startup records bounded, opt-in phases within synchronous Fx startup hooks.
// It has no dependency on Fx or on a tracing transport.
package startup

import (
	"math/rand"
	"sync"
	"time"
)

const maxPhases = 128

// Event is a phase measurement. Names/resources must be bounded operation or
// component types, never configuration contents, paths, URLs, or error messages.
// Incomplete measurements end at their hook/startup boundary, not at completion.
type Event struct {
	Name       string
	Resource   string
	SpanID     uint64
	ParentID   uint64
	Start      time.Time
	Duration   time.Duration
	Failed     bool
	Incomplete bool
}

// Recorder belongs to one Fx application, not to the process. The Fx logger
// activates it for each synchronous OnStart hook and drains it at startup end.
// A nil or disabled recorder is a no-op.
type Recorder struct {
	enabled   bool
	mu        sync.Mutex
	hookID    uint64
	hookStart int
	closed    bool
	dropped   int
	events    []Event
	now       func() time.Time
}

// NewRecorder creates an application-local recorder. It does not start workers
// or perform I/O, and cannot record until the Fx logger calls BeginHook.
func NewRecorder(enabled bool) *Recorder {
	return &Recorder{enabled: enabled, now: time.Now}
}

// BeginHook sets the parent for phases created during this synchronous hook.
func (r *Recorder) BeginHook(spanID uint64) {
	if r == nil || !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.endHook()
		r.hookID = spanID
		r.hookStart = len(r.events)
	}
}

// EndHook freezes unfinished phases so they cannot leak into a subsequent hook.
func (r *Recorder) EndHook() {
	if r == nil || !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endHook()
}

// endHook requires mu. Keep incomplete parents as well as completed children:
// a timeout must not turn already measured phases into orphan spans.
func (r *Recorder) endHook() {
	if r.hookID == 0 {
		return
	}
	end := r.now()
	for i := r.hookStart; i < len(r.events); i++ {
		if r.events[i].Incomplete {
			r.events[i].Duration = end.Sub(r.events[i].Start)
		}
	}
	r.hookID = 0
}

// Phase is an in-flight measurement. Its timestamps use Go's monotonic clock.
// A nil phase and repeated Finish calls are safe.
type Phase struct {
	recorder *Recorder
	hookID   uint64
	index    int
}

// Start begins a direct child of the currently executing Fx OnStart hook.
func (r *Recorder) Start(name, resource string) *Phase {
	return r.start(name, resource, nil)
}

func (r *Recorder) start(name, resource string, parent *Phase) *Phase {
	if r == nil || !r.enabled {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.hookID == 0 {
		return nil
	}
	parentID := r.hookID
	if parent != nil {
		if parent.hookID != r.hookID || !r.events[parent.index].Incomplete {
			return nil
		}
		parentID = r.events[parent.index].SpanID
	}
	if len(r.events) >= maxPhases {
		r.dropped++
		return nil
	}
	phase := &Phase{recorder: r, hookID: r.hookID, index: len(r.events)}
	r.events = append(r.events, Event{
		Name: name, Resource: resource,
		SpanID: rand.Uint64() | 1, ParentID: parentID,
		Start: r.now(), Incomplete: true,
	})
	return phase
}

// Start begins a nested phase without changing the parent of other goroutines.
func (p *Phase) Start(name, resource string) *Phase {
	if p == nil {
		return nil
	}
	return p.recorder.start(name, resource, p)
}

// Finish records only whether an error occurred; error text is never retained.
// Completions after their hook/startup boundary are discarded, not reparented.
func (p *Phase) Finish(err error) {
	if p == nil {
		return
	}
	r := p.recorder
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.hookID != p.hookID {
		return
	}
	event := &r.events[p.index]
	if !event.Incomplete {
		return
	}
	event.Duration = r.now().Sub(event.Start)
	event.Failed = err != nil
	event.Incomplete = false
}

// Drain closes the recorder and transfers a snapshot to the Fx logger. Running
// phases are marked incomplete and timed only up to this boundary. The second
// result counts phases omitted because the per-startup cap was hit.
func (r *Recorder) Drain() ([]Event, int) {
	if r == nil || !r.enabled {
		return nil, 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endHook()
	r.closed = true
	events, dropped := r.events, r.dropped
	r.events = nil
	r.dropped = 0
	return events, dropped
}
