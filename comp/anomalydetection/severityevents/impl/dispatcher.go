// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package severityeventsimpl provides concrete helpers built on top of the
// severityevents contract.
package severityeventsimpl

import (
	"sync/atomic"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// Dispatcher owns one listener plus one fixed filter/cooldown state machine.
type Dispatcher struct {
	listener     severityeventsdef.SeverityEventListener
	filter       severityeventsdef.SeverityEventFilter
	cooldownSecs int64

	level   atomic.Int32
	lastSec int64

	cooldownStartedAt int64
}

// NewDispatcher creates a dispatcher delivering to listener, filtered/cooled
// down per cfg.
func NewDispatcher(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) *Dispatcher {
	d := &Dispatcher{
		listener:     listener,
		filter:       cfg.Filter,
		cooldownSecs: cfg.CooldownSecs,
	}
	d.level.Store(int32(severityeventsdef.SeverityLow))
	return d
}

// Level returns the dispatcher's current internal severity level.
func (d *Dispatcher) Level() severityeventsdef.SeverityLevel {
	return severityeventsdef.SeverityLevel(d.level.Load())
}

// DeliverInitial seeds the dispatcher from the current level and may deliver a
// synthetic snapshot event.
func (d *Dispatcher) DeliverInitial(sec int64, level severityeventsdef.SeverityLevel) {
	level = clampSeverityLevel(level)
	d.level.Store(int32(level))
	d.lastSec = sec
	if level == severityeventsdef.SeverityLow {
		return
	}

	evt := severityeventsdef.SeverityEvent{
		Timestamp: sec,
		FromLevel: severityeventsdef.SeverityLow,
		ToLevel:   level,
		Direction: severityeventsdef.SeverityEventEscalation,
	}
	if EventFilterMatches(d.filter, evt) {
		d.cooldownStartedAt = sec
		d.listener.OnSeverityTransition(evt)
	}
}

// Advance feeds one raw severity level into the dispatcher state machine.
func (d *Dispatcher) Advance(sec int64, level severityeventsdef.SeverityLevel) {
	next := clampSeverityLevel(level)
	current := d.Level()
	if next == current {
		d.lastSec = sec
		return
	}

	if next < current && d.cooldownSecs > 0 && sec-d.cooldownStartedAt < d.cooldownSecs {
		d.lastSec = sec
		return
	}
	d.transition(sec, next, 0)
}

// DiscoverHigh starts or renews this subscriber's hold for a distinct raw High.
// crossingSec describes the evidence; only triggerSec affects the cooldown.
func (d *Dispatcher) DiscoverHigh(triggerSec, crossingSec int64) {
	if d.Level() == severityeventsdef.SeverityHigh {
		d.lastSec = triggerSec
		d.cooldownStartedAt = triggerSec
		return
	}
	d.transition(triggerSec, severityeventsdef.SeverityHigh, crossingSec)
}

func (d *Dispatcher) transition(sec int64, next severityeventsdef.SeverityLevel, historicalSec int64) {
	current := d.Level()
	evt := severityeventsdef.SeverityEvent{
		Timestamp:           sec,
		HistoricalTimestamp: historicalSec,
		FromLevel:           current,
		ToLevel:             next,
		Direction:           eventDirection(current, next),
	}
	d.level.Store(int32(next))
	d.lastSec = sec
	d.cooldownStartedAt = sec

	if EventFilterMatches(d.filter, evt) {
		d.listener.OnSeverityTransition(evt)
	}
}

// Reset clears delivery state and known level.
func (d *Dispatcher) Reset() {
	d.level.Store(int32(severityeventsdef.SeverityLow))
	d.lastSec = 0
	d.cooldownStartedAt = 0
}

func eventDirection(from, to severityeventsdef.SeverityLevel) severityeventsdef.SeverityEventDirection {
	if to > from {
		return severityeventsdef.SeverityEventEscalation
	}
	return severityeventsdef.SeverityEventDeescalation
}

// EventFilterMatches reports whether evt matches filter.
func EventFilterMatches(f severityeventsdef.SeverityEventFilter, evt severityeventsdef.SeverityEvent) bool {
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
