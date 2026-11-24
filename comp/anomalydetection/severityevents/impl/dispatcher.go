// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package severityeventsimpl provides concrete helpers built on top of the
// severityevents contract.
package severityeventsimpl

import severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"

// Dispatcher owns one listener plus one fixed filter/cooldown state machine.
type Dispatcher struct {
	listener     severityeventsdef.SeverityEventListener
	filter       severityeventsdef.SeverityEventFilter
	cooldownSecs int64

	level   severityeventsdef.SeverityLevel
	lastSec int64

	cooldownStartedAt int64
}

// NewDispatcher creates a dispatcher delivering to listener, filtered/cooled
// down per cfg.
func NewDispatcher(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) *Dispatcher {
	return &Dispatcher{
		listener:     listener,
		filter:       cfg.Filter,
		cooldownSecs: cfg.CooldownSecs,
		level:        severityeventsdef.SeverityLow,
	}
}

// DeliverInitial seeds the dispatcher from the current level and may deliver a
// synthetic snapshot event.
func (d *Dispatcher) DeliverInitial(sec int64, level severityeventsdef.SeverityLevel) {
	level = clampSeverityLevel(level)
	d.level = level
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
	if eventFilterMatches(d.filter, evt) {
		d.cooldownStartedAt = sec
		d.listener.OnSeverityTransition(evt)
	}
}

// Advance feeds one raw severity level into the dispatcher state machine.
func (d *Dispatcher) Advance(sec int64, level severityeventsdef.SeverityLevel) {
	d.AdvanceWithDiscovery(sec, level, severityeventsdef.SeverityLow, 0)
}

// AdvanceWithDiscovery combines raw severity with an optional discovered
// period. The stronger level wins. A matching discovery renews the hold.
// Medium cannot override High during its cooldown.
func (d *Dispatcher) AdvanceWithDiscovery(
	sec int64,
	rawLevel severityeventsdef.SeverityLevel,
	discoveryLevel severityeventsdef.SeverityLevel,
	crossingSec int64,
) {
	raw := clampSeverityLevel(rawLevel)
	discovery := clampSeverityLevel(discoveryLevel)
	next := maxSeverityLevel(raw, discovery)
	hasDiscovery := discovery != severityeventsdef.SeverityLow && next == discovery
	historicalSec := int64(0)
	if hasDiscovery {
		historicalSec = crossingSec
	}

	if next == d.level {
		if hasDiscovery && next > severityeventsdef.SeverityLow {
			d.lastSec = sec
			d.cooldownStartedAt = sec
			return
		}
		d.lastSec = sec
		return
	}

	if next < d.level && d.cooldownSecs > 0 && sec-d.cooldownStartedAt < d.cooldownSecs {
		d.lastSec = sec
		return
	}
	d.transition(sec, next, historicalSec)
}

func (d *Dispatcher) transition(sec int64, next severityeventsdef.SeverityLevel, historicalSec int64) {
	evt := severityeventsdef.SeverityEvent{
		Timestamp:           sec,
		HistoricalTimestamp: historicalSec,
		FromLevel:           d.level,
		ToLevel:             next,
		Direction:           eventDirection(d.level, next),
	}
	d.level = next
	d.lastSec = sec
	d.cooldownStartedAt = sec

	if eventFilterMatches(d.filter, evt) {
		d.listener.OnSeverityTransition(evt)
	}
}

// Reset clears delivery state and known level.
func (d *Dispatcher) Reset() {
	d.level = severityeventsdef.SeverityLow
	d.lastSec = 0
	d.cooldownStartedAt = 0
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

func maxSeverityLevel(left, right severityeventsdef.SeverityLevel) severityeventsdef.SeverityLevel {
	if left > right {
		return left
	}
	return right
}
