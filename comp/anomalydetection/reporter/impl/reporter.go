// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporterimpl provides the live reporter implementations:
// a stdout reporter (always active) and an optional Datadog event reporter
// (active when anomaly_detection.reporting.enabled=true).
package reporterimpl

import (
	"time"

	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
)

const (
	// telemetryReportsOngoing counts advances where at least one already-seen correlation was still active.
	telemetryReportsOngoing = "observer.reports.ongoing"
	// telemetryReportsEmitted counts new correlation patterns seen for the first time (would-have-been event reports).
	telemetryReportsEmitted = "observer.reports.emitted"
)

// Requires defines the dependencies for the live reporter component.
type Requires struct {
	Config        config.Component
	Log           log.Component
	Telemetry     telemetryComp.Component
	EventPlatform eventplatform.Component
	Hostname      hostname.Component
}

// Provides defines the output of the live reporter component.
// Reporters are provided via the anomalydetection_reporters Fx group so the
// observer can subscribe multiple reporters independently.
type Provides struct {
	Reporters []reporterdef.Reporter `group:"anomalydetection_reporters,flatten"`
}

// NewComponent creates the live reporter component. It always provides a
// stdoutReporter and, when anomaly_detection.reporting.enabled=true and the
// event-platform forwarder is available, also provides an EventReporter that
// posts Datadog change events through the event-management intake pipeline.
func NewComponent(req Requires) (Provides, error) {
	ongoingCounter := req.Telemetry.NewCounter(
		"observer",
		telemetryReportsOngoing,
		nil,
		"Number of advances with at least one ongoing (already-seen) active anomaly correlation",
	)
	emittedCounter := req.Telemetry.NewCounter(
		"observer",
		telemetryReportsEmitted,
		nil,
		"Number of new anomaly correlation patterns detected for the first time",
	)

	reporters := []reporterdef.Reporter{&stdoutReporter{
		logger:          req.Log,
		ongoingCounter:  ongoingCounter,
		emittedCounter:  emittedCounter,
		seenCorrelation: make(map[string]bool),
		activeBefore:    make(map[string]bool),
	}}

	if req.Config.GetBool("anomaly_detection.reporting.enabled") {
		forwarder, ok := req.EventPlatform.Get()
		if !ok {
			req.Log.Warnf("[reporter] event_reporter disabled: event-platform forwarder is not running")
		} else {
			sender, err := newEventSender(forwarder, req.Log, nil, req.Hostname)
			if err != nil {
				req.Log.Warnf("[reporter] event_reporter disabled: %v", err)
			} else {
				reporters = append(reporters, &EventReporter{sender: sender, logger: req.Log})
			}
		}
	}

	return Provides{Reporters: reporters}, nil
}

type stdoutReporter struct {
	logger         log.Component
	ongoingCounter telemetryComp.Counter
	emittedCounter telemetryComp.Counter
	// seenCorrelation tracks patterns reported at info level (first-seen). Mirrors
	// EventReporter.seenCorrelations: driven by CorrelationHistory, cleaned up when
	// a pattern leaves ActiveCorrelations.
	seenCorrelation map[string]bool
	// activeBefore holds patterns that were in ActiveCorrelations last advance,
	// used to detect when a pattern goes inactive for recurrence cleanup.
	activeBefore map[string]bool
}

func (r *stdoutReporter) Name() string { return "stdout_reporter" }

func (r *stdoutReporter) Report(output reporterdef.ReportOutput) bool {
	currentlyActive := make(map[string]bool, len(output.ActiveCorrelations))
	for _, ac := range output.ActiveCorrelations {
		currentlyActive[ac.Pattern] = true
	}

	// Info log for new correlations (first time seen, mirrors EventReporter semantics).
	newlyEmitted := make(map[string]bool)
	for _, ac := range output.CorrelationHistory {
		if !r.seenCorrelation[ac.Pattern] {
			r.logger.Infof("[observer] anomaly detection report: pattern=%s title=%q members=%d",
				ac.Pattern, ac.Title, len(ac.Members))
			r.emittedCounter.Add(1)
			r.seenCorrelation[ac.Pattern] = true
			newlyEmitted[ac.Pattern] = true
		}
	}

	// Debug log for ongoing correlations (active but already seen this run).
	hasOngoing := false
	for _, ac := range output.ActiveCorrelations {
		if !newlyEmitted[ac.Pattern] {
			r.logger.Debugf("[observer] ongoing anomaly correlation: pattern=%s members=%d",
				ac.Pattern, len(ac.Members))
			hasOngoing = true
		}
	}
	if hasOngoing {
		r.ongoingCounter.Add(1)
	}

	// Debug log for raw new anomalies detected this cycle.
	for _, a := range output.NewAnomalies {
		ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
		r.logger.Debugf("[observer] anomaly detected: source=%s detector=%s at=%s",
			a.Source.DisplayName(), a.DetectorName, ts)
	}

	// Recurrence cleanup: a pattern that was active before and is no longer active
	// is removed from seenCorrelation so it can fire at info level if it recurs.
	// Patterns that only ever appeared in CorrelationHistory (never active) are kept.
	for pattern := range r.activeBefore {
		if !currentlyActive[pattern] {
			delete(r.seenCorrelation, pattern)
			delete(r.activeBefore, pattern)
		}
	}
	for pattern := range currentlyActive {
		r.activeBefore[pattern] = true
	}

	return len(newlyEmitted) > 0 || hasOngoing
}
