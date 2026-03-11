// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// engineEventKind identifies the type of engine event.
type engineEventKind int

const (
	eventAdvanceCompleted  engineEventKind = iota
	eventAnomalyCreated
	eventCorrelationUpdated
)

// engineEvent represents a meaningful state transition in the engine.
// Events are lightweight notifications; consumers should query stateView for
// full details when needed.
type engineEvent struct {
	kind      engineEventKind
	timestamp int64 // event time (data time, not wall clock)

	// Exactly one of these is non-nil based on kind.
	advanceCompleted   *advanceCompletedEvent
	anomalyCreated     *anomalyCreatedEvent
	correlationUpdated *correlationUpdatedEvent
}

// advanceCompletedEvent is emitted after the engine finishes an Advance call.
type advanceCompletedEvent struct {
	advancedToSec  int64
	reason         advanceReason
	anomalyCount   int
	telemetryCount int
	// anomalies are the anomalies detected in this advance cycle.
	// Included so event sinks can forward them without querying engine state.
	anomalies []observerdef.Anomaly
}

// anomalyCreatedEvent is emitted for each anomaly detected during Advance.
type anomalyCreatedEvent struct {
	anomaly observerdef.Anomaly
}

// correlationUpdatedEvent is emitted after correlators flush and their state
// may have changed.
type correlationUpdatedEvent struct {
	correlatorName string
}

// eventSink receives engine events.
type eventSink interface {
	onEngineEvent(engineEvent)
}

// reporterEventSink bridges engine events to the existing Reporter interface.
// When an advance completes, it populates ReportOutput with anomalies from
// the event and active correlations from the stateView, then calls Report
// on all registered reporters.
type reporterEventSink struct {
	reporters []observerdef.Reporter
	state     *stateView // for querying current correlations on advance
}

func (s *reporterEventSink) onEngineEvent(evt engineEvent) {
	if evt.kind == eventAdvanceCompleted {
		ac := evt.advanceCompleted
		output := observerdef.ReportOutput{
			AdvancedToSec: ac.advancedToSec,
			NewAnomalies:  ac.anomalies,
		}
		if s.state != nil {
			output.ActiveCorrelations = s.state.ActiveCorrelations()
		}
		for _, r := range s.reporters {
			r.Report(output)
		}
	}
}
