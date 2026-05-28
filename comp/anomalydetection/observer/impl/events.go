// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
)

// engineEventKind identifies the type of engine event.
type engineEventKind int

const (
	eventAdvanceCompleted engineEventKind = iota
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
	advancedToSec int64
	reason        advanceReason
	anomalyCount  int
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

// reporterEventSink bridges engine events to the reporter group.
// When an advance completes, it populates ReportOutput with anomalies from
// the event and active correlations from the stateView, then calls Report
// on all registered reporters.
type reporterEventSink struct {
	reporters []reporterdef.Reporter
	state     *stateView // for querying current correlations on advance
	onReport  func(reporter string)
}

func (s *reporterEventSink) onEngineEvent(evt engineEvent) {
	if evt.kind == eventAdvanceCompleted {
		ac := evt.advanceCompleted
		output := reporterdef.ReportOutput{
			AdvancedToSec: ac.advancedToSec,
			NewAnomalies:  ac.anomalies,
		}
		if s.state != nil {
			// ActiveCorrelations is the live sliding-window set; reporters
			// use it to detect when a pattern has gone inactive (so it can
			// fire again on recurrence). CorrelationHistory is the accumulated
			// set including batch-detector clusters whose changepoint
			// timestamps may already be evicted from the sliding window;
			// reporters drive one-shot emission from this set.
			output.ActiveCorrelations = s.state.ActiveCorrelations()
			output.CorrelationHistory = s.state.CorrelationHistory()
		}
		for _, r := range s.reporters {
			if r.Report(output) && s.onReport != nil {
				s.onReport(r.Name())
			}
		}
	}
}
