// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// correlationEmitter tracks first-seen / recurrence state for a correlator and
// produces CorrelationDetected events via PendingEvents.
//
// Usage pattern (call from Advance, after computing the current active set but
// BEFORE evicting old entries so same-cycle batch clusters are not missed):
//
//	e.emitter.observe(active, dataTime)
//
// Then return e.emitter.drain() from PendingEvents() and call e.emitter.reset()
// from Reset().
//
// Thread-safety: not safe for concurrent use. Each correlator owns one emitter
// and only touches it from within its own Advance/PendingEvents/Reset calls,
// which the engine invokes sequentially.
type correlationEmitter struct {
	correlatorName string
	// seen holds patterns emitted at least once; entry is deleted when the
	// pattern leaves the active set, enabling re-fire on recurrence.
	seen map[string]bool
	// activeBefore holds patterns that were active on the previous advance.
	// Used to detect when a pattern leaves the active set for recurrence cleanup.
	activeBefore map[string]bool
	pending      []observer.CorrelatorEvent
}

func newCorrelationEmitter(name string) correlationEmitter {
	return correlationEmitter{
		correlatorName: name,
		seen:           make(map[string]bool),
		activeBefore:   make(map[string]bool),
	}
}

// observe computes new CorrelationDetected events from the current active
// set and handles recurrence cleanup.
//
// For each pattern not yet in seen: enqueue a CorrelationDetected event.
// For each pattern that was in activeBefore but is no longer active: delete
// from seen and activeBefore so it can re-fire when it comes back.
func (e *correlationEmitter) observe(active []observer.ActiveCorrelation, dataTime int64) {
	currentlyActive := make(map[string]bool, len(active))
	for _, ac := range active {
		currentlyActive[ac.Pattern] = true
	}

	// Emit for newly-seen patterns.
	for _, ac := range active {
		if !e.seen[ac.Pattern] {
			e.pending = append(e.pending, observer.CorrelatorEvent{
				Kind:           observer.CorrelatorEventCorrelationDetected,
				CorrelatorName: e.correlatorName,
				Timestamp:      dataTime,
				Correlation:    ac,
			})
			e.seen[ac.Pattern] = true
		}
	}

	// Recurrence cleanup: patterns that left the active set can re-fire later.
	for pattern := range e.activeBefore {
		if !currentlyActive[pattern] {
			delete(e.seen, pattern)
			delete(e.activeBefore, pattern)
		}
	}
	for pattern := range currentlyActive {
		e.activeBefore[pattern] = true
	}
}

// drain returns and clears all pending events accumulated since the last drain.
func (e *correlationEmitter) drain() []observer.CorrelatorEvent {
	if len(e.pending) == 0 {
		return nil
	}
	evts := e.pending
	e.pending = nil
	return evts
}

// reset clears all emitter state, mirroring the correlator's Reset().
func (e *correlationEmitter) reset() {
	e.seen = make(map[string]bool)
	e.activeBefore = make(map[string]bool)
	e.pending = nil
}
