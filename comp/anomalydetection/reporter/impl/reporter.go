// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporterimpl provides the live reporter implementations:
// a stdout reporter (always active) and an optional Datadog event reporter
// (active when anomaly_detection.reporting.events.enabled=true).
package reporterimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
)

const (
	// telemetryReportsOngoing counts advances where at least one already-seen correlation was still active.
	telemetryReportsOngoing = "observer.reports.ongoing"
	// telemetryReportsEmitted counts report events produced locally, partitioned by kind.
	telemetryReportsEmitted = "observer.reports.emitted"

	reportKindCorrelation    = "correlation"
	reportKindEpisodeStarted = "episode_started"
	reportKindEpisodeEnded   = "episode_ended"
)

func newReportsEmittedCounter(telemetry telemetryComp.Component) telemetryComp.Counter {
	return telemetry.NewCounter(
		"observer",
		telemetryReportsEmitted,
		[]string{"kind"},
		"Number of report events produced locally, partitioned by kind",
	)
}

// Requires defines the dependencies for the live reporter component.
type Requires struct {
	Config        config.Component
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
// stdoutReporter and, when anomaly_detection.reporting.events.enabled=true and the
// event-platform forwarder is available, also provides an EventReporter that
// posts Datadog change events through the event-management intake pipeline.
func NewComponent(req Requires) (Provides, error) {
	ongoingCounter := req.Telemetry.NewCounter(
		"observer",
		telemetryReportsOngoing,
		nil,
		"Number of advances with at least one ongoing (already-seen) active anomaly correlation",
	)
	emittedCounter := newReportsEmittedCounter(req.Telemetry)

	reporters := []reporterdef.Reporter{&stdoutReporter{
		ongoingCounter: ongoingCounter,
		emittedCounter: emittedCounter,
		stdoutEnabled:  req.Config.GetBool("anomaly_detection.reporting.stdout.enabled"),
		stdoutVerbose:  req.Config.GetBool("anomaly_detection.reporting.stdout.verbose"),
	}}

	if req.Config.GetBool("anomaly_detection.reporting.events.enabled") {
		forwarder, ok := req.EventPlatform.Get()
		if !ok {
			logging.Warnf("reporter event_reporter disabled: event-platform forwarder is not running")
		} else {
			sender, err := newEventSender(forwarder, nil, req.Hostname)
			if err != nil {
				logging.Warnf("reporter event_reporter disabled: %v", err)
			} else {
				reporters = append(reporters, &EventReporter{sender: sender, maxRetries: defaultMaxRetryAttempts})
			}
		}
	}

	return Provides{Reporters: reporters}, nil
}

type stdoutReporter struct {
	storage        observerdef.StorageReader
	ongoingCounter telemetryComp.Counter
	emittedCounter telemetryComp.Counter
	// stdoutEnabled gates all anomaly-detection stdout log lines.
	// Controlled by anomaly_detection.reporting.stdout.enabled (default: true).
	stdoutEnabled bool
	// stdoutVerbose prints individual anomaly series lines after the title.
	// Controlled by anomaly_detection.reporting.stdout.verbose (default: false).
	stdoutVerbose bool
}

func (r *stdoutReporter) Name() string { return "stdout_reporter" }

// SetStorage lets scorer episode reports resolve their compact contributor
// handles only when they are rendered.
func (r *stdoutReporter) SetStorage(storage observerdef.StorageReader) {
	r.storage = storage
}

func (r *stdoutReporter) Report(output reporterdef.ReportOutput) bool {
	emitted := false

	// Build the set of newly-detected patterns from this cycle so they can be
	// excluded from the "ongoing" telemetry path below.
	newlyDetected := make(map[string]struct{}, len(output.CorrelatorEvents))

	// Log all correlator events at info level and drive the emitted counter.
	for _, ce := range output.CorrelatorEvents {
		switch ce.Kind {
		case observerdef.CorrelatorEventEpisodeStarted:
			r.emittedCounter.Add(1, reportKindEpisodeStarted)
			emitted = true
			if r.stdoutEnabled {
				message := formatScorerContributorMessage(ce.Contributors, r.storage)
				if message == "" {
					logging.Infof("reporter scorer episode started: scorer=%s pattern=%s t=%d",
						ce.CorrelatorName, ce.Correlation.Pattern, ce.Timestamp)
				} else {
					logging.Infof("reporter scorer episode started:\n%s", message)
				}
			}
		case observerdef.CorrelatorEventEpisodeEnded:
			r.emittedCounter.Add(1, reportKindEpisodeEnded)
			emitted = true
			if r.stdoutEnabled {
				logging.Infof("reporter scorer episode ended: scorer=%s pattern=%s t=%d duration=%ds",
					ce.CorrelatorName, ce.Correlation.Pattern, ce.Timestamp,
					ce.Correlation.LastUpdated-ce.Correlation.FirstSeen)
			}
		case observerdef.CorrelatorEventCorrelationDetected:
			newlyDetected[ce.Correlation.Pattern] = struct{}{}
			r.emittedCounter.Add(1, reportKindCorrelation)
			emitted = true
			if r.stdoutEnabled {
				logging.Infof("reporter anomaly detection report: pattern=%s title=%q members=%d",
					ce.Correlation.Pattern, ce.Correlation.Title, len(ce.Correlation.Members))
				if r.stdoutVerbose {
					for _, a := range ce.Correlation.Anomalies {
						ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
						logging.Infof("reporter anomaly: %s [%s] at %s",
							a.Source.DisplayName(), a.DetectorName, ts)
					}
				}
			}
		}
	}

	// Ongoing counter: fires when at least one active correlation was already
	// seen in a prior cycle (i.e. not newly detected this cycle). This mirrors
	// the pre-refactor semantics where ongoingCounter incremented once per
	// advance that had any pattern not in the freshly-emitted set.
	hasOngoing := false
	for _, ac := range output.ActiveCorrelations {
		if _, isNew := newlyDetected[ac.Pattern]; !isNew {
			if r.stdoutEnabled {
				logging.Debugf("reporter ongoing anomaly correlation: pattern=%s members=%d",
					ac.Pattern, len(ac.Members))
			}
			hasOngoing = true
		}
	}
	if hasOngoing {
		r.ongoingCounter.Add(1)
	}

	// Debug log for raw new anomalies detected this cycle.
	if r.stdoutEnabled {
		for _, a := range output.NewAnomalies {
			ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
			logging.Debugf("reporter anomaly detected: source=%s detector=%s at=%s",
				a.Source.DisplayName(), a.DetectorName, ts)
		}
	}

	return emitted || len(output.ActiveCorrelations) > 0
}
