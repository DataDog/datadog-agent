// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityeventsimpl

import (
	"sync"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// Dispatcher owns push-based severity event subscriptions and their
// per-subscription cooldown/filter delivery state.
type Dispatcher struct {
	subsMu sync.RWMutex
	subs   []*subscription
}

// subscription is a registered listener with its own per-subscription
// severity state machine.
type subscription struct {
	cfg severityeventsdef.AnomalyScorerConfiguration

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
// cfg.CooldownSecs. Returns an unsubscribe function. Safe to call concurrently.
// Panics if cfg.Listener is nil.
func (d *Dispatcher) SubscribeScorer(cfg severityeventsdef.AnomalyScorerConfiguration) func() {
	if cfg.Listener == nil {
		panic("severityeventsimpl.Dispatcher.SubscribeScorer: Listener must not be nil")
	}
	sub := &subscription{cfg: cfg}

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
// subscription state machine and delivers any resulting events.
func (d *Dispatcher) Advance(sec int64, level severityeventsdef.SeverityLevel) {
	d.subsMu.RLock()
	subs := make([]*subscription, len(d.subs))
	copy(subs, d.subs)
	d.subsMu.RUnlock()

	for _, sub := range subs {
		if evt, ok := sub.advance(sec, level); ok && eventFilterMatches(sub.cfg.Filter, evt) {
			sub.cfg.Listener.OnSeverityTransition(evt)
		}
	}
}

// Reset clears all per-subscription delivery state while preserving the
// registered listeners themselves.
func (d *Dispatcher) Reset() {
	d.subsMu.Lock()
	defer d.subsMu.Unlock()
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

func eventDirection(from, to severityeventsdef.SeverityLevel) severityeventsdef.AnomalyScorerEventDirection {
	if to > from {
		return severityeventsdef.AnomalyScorerEventEscalation
	}
	return severityeventsdef.AnomalyScorerEventDeescalation
}

func eventFilterMatches(f severityeventsdef.AnomalyScorerEventFilter, evt severityeventsdef.SeverityEvent) bool {
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
	case severityeventsdef.AnomalyScorerEventEscalation:
		if evt.ToLevel <= evt.FromLevel {
			return false
		}
	case severityeventsdef.AnomalyScorerEventDeescalation:
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
