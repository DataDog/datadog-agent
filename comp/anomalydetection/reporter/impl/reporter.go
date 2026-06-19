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

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
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
		logger:         req.Log,
		ongoingCounter: ongoingCounter,
		emittedCounter: emittedCounter,
		stdoutEnabled:  req.Config.GetBool("anomaly_detection.reporting.stdout.enabled"),
		stdoutVerbose:  req.Config.GetBool("anomaly_detection.reporting.stdout.verbose"),
	}}

	if req.Config.GetBool("anomaly_detection.reporting.events.enabled") {
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
	// stdoutEnabled gates all [observer] stdout log lines.
	// Controlled by anomaly_detection.reporting.stdout.enabled (default: true).
	stdoutEnabled bool
	// stdoutVerbose prints individual anomaly series lines after the title.
	// Controlled by anomaly_detection.reporting.stdout.verbose (default: false).
	stdoutVerbose bool
}

func (r *stdoutReporter) Name() string { return "stdout_reporter" }

func (r *stdoutReporter) Report(output reporterdef.ReportOutput) bool {
	emitted := false

	// Log all correlator events at info level and drive the emitted counter.
	if r.stdoutEnabled {
		for _, ce := range output.CorrelatorEvents {
			switch ce.Kind {
			case observerdef.CorrelatorEventEpisodeStarted:
				r.logger.Infof("[observer] scorer episode started: scorer=%s pattern=%s t=%d",
					ce.CorrelatorName, ce.Correlation.Pattern, ce.Timestamp)
			case observerdef.CorrelatorEventEpisodeEnded:
				r.logger.Infof("[observer] scorer episode ended: scorer=%s pattern=%s t=%d duration=%ds",
					ce.CorrelatorName, ce.Correlation.Pattern, ce.Timestamp,
					ce.Correlation.LastUpdated-ce.Correlation.FirstSeen)
			case observerdef.CorrelatorEventCorrelationDetected:
				r.logger.Infof("[observer] anomaly detection report: pattern=%s title=%q members=%d",
					ce.Correlation.Pattern, ce.Correlation.Title, len(ce.Correlation.Members))
				if r.stdoutVerbose {
					for _, a := range ce.Correlation.Anomalies {
						ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
						r.logger.Infof("[observer]   - %s [%s] at %s",
							a.Source.DisplayName(), a.DetectorName, ts)
					}
				}
			}
		}
	}
	for range output.CorrelatorEvents {
		r.emittedCounter.Add(1)
		emitted = true
	}

	// Ongoing counter: at least one correlation is currently active.
	if len(output.ActiveCorrelations) > 0 {
		r.ongoingCounter.Add(1)
	}

	// Debug log for ongoing correlations.
	if r.stdoutEnabled {
		for _, ac := range output.ActiveCorrelations {
			r.logger.Debugf("[observer] ongoing anomaly correlation: pattern=%s members=%d",
				ac.Pattern, len(ac.Members))
		}
	}

	// Debug log for raw new anomalies detected this cycle.
	if r.stdoutEnabled {
		for _, a := range output.NewAnomalies {
			ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
			r.logger.Debugf("[observer] anomaly detected: source=%s detector=%s at=%s",
				a.Source.DisplayName(), a.DetectorName, ts)
		}
	}

	return emitted || len(output.ActiveCorrelations) > 0
}
