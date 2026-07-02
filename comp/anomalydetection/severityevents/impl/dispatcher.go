// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package severityeventsimpl provides concrete helpers built on top of the
// severityevents contract.
package severityeventsimpl

import (
	"sync"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// Dispatcher owns push-based severity event subscriptions and their
// per-subscription cooldown/filter delivery state. It also remembers the
// last level fed via Advance, so a subscription added mid-stream can be told
// the current severity immediately instead of only learning about it on the
// next transition.
type Dispatcher struct {
	subsMu sync.RWMutex
	subs   []*subscription

	hasLevel bool
	level    severityeventsdef.SeverityLevel
	lastSec  int64
}

// subscription is a registered listener with its own per-subscription
// severity state machine.
type subscription struct {
	cfg severityeventsdef.SeverityEventsConfiguration

	state            severityeventsdef.SeverityLevel
	lastStateEntryTs int64
	stateInitialized bool
}

// NewDispatcher creates an empty severity event dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// SubscribeScorer registers cfg.Listener to receive severity transitions
// matching cfg.Filter. Each subscription runs its own state machine using
// cfg.CooldownSecs.
//
// If the dispatcher already knows the current severity level (i.e. at least
// one Advance call has happened since the last Reset), an initial synthetic
// event is delivered synchronously before SubscribeScorer returns, with
// FromLevel == ToLevel == the current level and Direction ==
// SeverityEventBoth. This lets a subscriber that joins mid-stream learn
// the current state immediately rather than only being told about future
// transitions relative to a silently-adopted baseline. The initial event is
// still subject to cfg.Filter, so a directional filter (escalations-only or
// de-escalations-only) will not receive it, since it is neither.
//
// The subscription is not made visible to Advance until after the initial
// event has been delivered, so a concurrent Advance call can never deliver a
// "real" transition to this listener ahead of its own initial snapshot.
//
// Returns an unsubscribe function. Safe to call concurrently. Panics if
// cfg.Listener is nil.
func (d *Dispatcher) SubscribeScorer(cfg severityeventsdef.SeverityEventsConfiguration) func() {
	if cfg.Listener == nil {
		panic("severityeventsimpl.Dispatcher.SubscribeScorer: Listener must not be nil")
	}
	sub := &subscription{cfg: cfg}

	// Compute the initial snapshot without yet publishing sub into d.subs, so
	// a concurrent Advance cannot see (and advance) this subscription before
	// its initial event has been delivered below.
	d.subsMu.RLock()
	var initialEvt severityeventsdef.SeverityEvent
	deliverInitial := false
	if d.hasLevel {
		level := clampSeverityLevel(d.level)
		sub.state = level
		sub.stateInitialized = true
		initialEvt = severityeventsdef.SeverityEvent{
			Timestamp: d.lastSec,
			FromLevel: level,
			ToLevel:   level,
			Direction: severityeventsdef.SeverityEventBoth,
		}
		// Only start the cooldown clock if the initial event is actually
		// delivered. Otherwise a filtered-out initial event (e.g. a
		// de-escalations-only subscriber joining while already Low) would
		// silently seed lastStateEntryTs and could suppress the listener's
		// very first real transition, even though it never received anything.
		if deliverInitial = eventFilterMatches(cfg.Filter, initialEvt); deliverInitial {
			sub.lastStateEntryTs = d.lastSec
		}
	}
	d.subsMu.RUnlock()

	if deliverInitial {
		cfg.Listener.OnSeverityTransition(initialEvt)
	}

	// Only now does the subscription become visible to Advance. Any level
	// change that happened between the snapshot above and this point is not
	// lost: it will simply be picked up as a normal transition on the next
	// Advance call, computed relative to the state snapshotted above.
	d.subsMu.Lock()
	d.subs = append(d.subs, sub)
	d.subsMu.Unlock()

	return func() {
		d.subsMu.Lock()
		defer d.subsMu.Unlock()
		for i, existing := range d.subs {
			if existing == sub {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
				return
			}
		}
	}
}

// Advance feeds the raw scorer severity level for one second into every
// subscription state machine and delivers any resulting events. Also records
// the level so subscriptions added later can be told the current state
// immediately (see SubscribeScorer).
func (d *Dispatcher) Advance(sec int64, level severityeventsdef.SeverityLevel) {
	d.subsMu.Lock()
	d.hasLevel = true
	d.level = level
	d.lastSec = sec
	subs := make([]*subscription, len(d.subs))
	copy(subs, d.subs)
	d.subsMu.Unlock()

	for _, sub := range subs {
		if evt, ok := sub.advance(sec, level); ok && eventFilterMatches(sub.cfg.Filter, evt) {
			sub.cfg.Listener.OnSeverityTransition(evt)
		}
	}
}

// Reset clears all per-subscription delivery state and the dispatcher's
// knowledge of the current level, so subscriptions registered before the
// next Advance call are seeded silently instead of being delivered a stale
// initial event.
func (d *Dispatcher) Reset() {
	d.subsMu.Lock()
	defer d.subsMu.Unlock()
	d.hasLevel = false
	for _, sub := range d.subs {
		sub.stateInitialized = false
		sub.lastStateEntryTs = 0
	}
}

func (sub *subscription) advance(sec int64, level severityeventsdef.SeverityLevel) (severityeventsdef.SeverityEvent, bool) {
	if !sub.stateInitialized {
		sub.state = clampSeverityLevel(level)
		sub.stateInitialized = true
		return severityeventsdef.SeverityEvent{}, false
	}

	next := clampSeverityLevel(level)
	if next == sub.state {
		return severityeventsdef.SeverityEvent{}, false
	}

	cooldown := sub.cfg.CooldownSecs
	if next < sub.state && cooldown > 0 && sec-sub.lastStateEntryTs < cooldown {
		return severityeventsdef.SeverityEvent{}, false
	}

	evt := severityeventsdef.SeverityEvent{
		Timestamp: sec,
		FromLevel: sub.state,
		ToLevel:   next,
		Direction: eventDirection(sub.state, next),
	}
	sub.state = next
	sub.lastStateEntryTs = sec
	return evt, true
}

func eventDirection(from, to severityeventsdef.SeverityLevel) severityeventsdef.SeverityEventDirection {
	if to > from {
		return severityeventsdef.SeverityEventEscalation
	}
	return severityeventsdef.SeverityEventDeescalation
}

func eventFilterMatches(f severityeventsdef.SeverityEventFilter, evt severityeventsdef.SeverityEvent) bool {
	if len(f.FromLevels) > 0 {
		found := false
		for _, l := range f.FromLevels {
			if evt.FromLevel == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.ToLevels) > 0 {
		found := false
		for _, l := range f.ToLevels {
			if evt.ToLevel == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	switch f.Direction {
	case severityeventsdef.SeverityEventEscalation:
		if evt.ToLevel <= evt.FromLevel {
			return false
		}
	case severityeventsdef.SeverityEventDeescalation:
		if evt.ToLevel >= evt.FromLevel {
			return false
		}
	}
	return true
}

func clampSeverityLevel(level severityeventsdef.SeverityLevel) severityeventsdef.SeverityLevel {
	if level < severityeventsdef.SeverityLow {
		return severityeventsdef.SeverityLow
	}
	if level > severityeventsdef.SeverityHigh {
		return severityeventsdef.SeverityHigh
	}
	return level
}
