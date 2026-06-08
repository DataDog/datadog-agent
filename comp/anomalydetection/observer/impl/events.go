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
	advancedToSec  int64
	reason         advanceReason
	anomalyCount   int
	telemetryCount int
	// anomalies are the anomalies detected in this advance cycle.
	// Included so event sinks can forward them without querying engine state.
	anomalies []observerdef.Anomaly
	// newCorrelations is the engine-computed reporter delta: patterns that are
	// new or genuinely recurred this cycle. Reporters fire on this set directly
	// with no dedup state of their own.
	newCorrelations []observerdef.ActiveCorrelation
	// totalCorrelations is the total number of unique correlation patterns ever
	// accumulated across all advance cycles, for dashboard/monitoring purposes.
	totalCorrelations int
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
// When an advance completes, it forwards the engine-computed NewCorrelations
// delta and the new anomalies to all registered reporters. Reporters are
// stateless: dedup is handled entirely by the engine's newCorrelations logic.
type reporterEventSink struct {
	reporters []reporterdef.Reporter
}

func (s *reporterEventSink) onEngineEvent(evt engineEvent) {
	if evt.kind == eventAdvanceCompleted {
		ac := evt.advanceCompleted
		output := reporterdef.ReportOutput{
			AdvancedToSec:     ac.advancedToSec,
			NewAnomalies:      ac.anomalies,
			NewCorrelations:   ac.newCorrelations,
			TotalCorrelations: ac.totalCorrelations,
		}
		for _, r := range s.reporters {
			r.Report(output)
		}
	}
}
