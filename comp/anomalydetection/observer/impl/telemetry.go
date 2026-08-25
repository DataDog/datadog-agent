// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"sync/atomic"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

const (
	// observerTelemetryMetricPrefix is the Agent metric prefix used when observer
	// telemetry is emitted through the metric pipeline. Keep this separate from
	// the Prometheus subsystem/name used below so the ingestion loop guard stays
	// correct if the emitted name changes.
	observerTelemetryMetricPrefix            = "datadog.agent.observer."
	telemetryObservationsAccepted            = "observer.observations.accepted"               // Observations accepted by the observer admission boundary.
	telemetryObservationsDropped             = "observer.observations.dropped"                // Observations dropped when the observer channel is full.
	telemetryRRCFScore                       = "observer.rrcf.score"                          // Latest RRCF score per detector.
	telemetryRRCFThreshold                   = "observer.rrcf.threshold"                      // Current RRCF anomaly threshold per detector.
	telemetryLogPatternExtractorPatternCount = "observer.log_pattern_extractor.pattern_count" // Current number of active log patterns.
	telemetryLogsAcceptedBytes               = "observer.logs.accepted_bytes"                 // Total bytes accepted into observer log ingestion.
	telemetryFilteredMetrics                 = "observer.metrics.filtered"                    // Number of metrics filtered out before enqueue/ingest.
	telemetrySeriesCount                     = "observer.series.count"                        // Number of active non-telemetry observer series.
	telemetryLogsInFlightCount               = "observer.logs.in_flight"                      // Number of logs currently queued/in flight.
	telemetryStorageSeriesEvicted            = "observer.storage.series_evicted"              // Number of storage series evicted to enforce bounds.
	telemetryStorageCapacityHit              = "observer.storage.capacity_hit"                // Number of times storage capacity eviction was triggered.
	telemetryAdvanceSkipped                  = "observer.scheduler.advance_skipped"           // Number of advance requests skipped as already analyzed.
	telemetryLogsInputRateLimiterDropped     = "observer.logs.input_rate_limiter.dropped"     // Logs dropped by the observer ingress rate limiter.
	telemetryDetectorProcessingTimeNs        = "observer.detector.processing_time_ns"         // Per-detector processing time in nanoseconds.
	telemetryDetectorEmissions               = "observer.detections.detector_emissions"       // Deduplicated detector emissions by score severity before correlation.
	telemetryScorerEWMA                      = "observer.scorer.ewma"                         // Anomaly scorer smoothed EWMA signal, updated every second.
	telemetryScorerSeverity                  = "observer.scorer.severity"                     // Current anomaly scorer severity level (0=Low,1=Medium,2=High).
)

type observerTelemetry struct {
	observationsAccepted telemetry.Counter
	observationsDropped  telemetry.Counter
	rrcfScore            telemetry.Gauge
	rrcfThreshold        telemetry.Gauge
	logPatternCount      telemetry.Gauge

	logsAcceptedBytes    telemetry.Counter
	filteredMetrics      telemetry.Counter
	seriesCount          telemetry.Gauge
	logsInFlight         telemetry.Gauge
	storageEvicted       telemetry.Counter
	storageCapHit        telemetry.Counter
	advanceSkipped       telemetry.Counter
	inputRateLimiterDrop telemetry.Counter
	processingTime       telemetry.Gauge
	detectorEmissions    telemetry.Counter
	scorerEwma           telemetry.Gauge
	scorerSeverity       telemetry.Gauge

	inFlightInternal   atomic.Int64
	inFlightKubelet    atomic.Int64
	inFlightContainers atomic.Int64
}

func newObserverTelemetry(telemetryComp telemetry.Component) *observerTelemetry {
	return &observerTelemetry{
		observationsAccepted: telemetryComp.NewCounter(
			"observer",
			telemetryObservationsAccepted,
			[]string{"kind", "source"},
			"Observations accepted into the observer admission boundary, tagged by kind and source",
		),
		observationsDropped: telemetryComp.NewCounter(
			"observer",
			telemetryObservationsDropped,
			[]string{"kind", "source"},
			"Observations dropped because the internal channel was full, tagged by kind and source",
		),
		rrcfScore: telemetryComp.NewGauge(
			"observer",
			telemetryRRCFScore,
			[]string{"detector"},
			"RRCF CoDisp score per scored shingle",
		),
		rrcfThreshold: telemetryComp.NewGauge(
			"observer",
			telemetryRRCFThreshold,
			[]string{"detector"},
			"RRCF dynamic anomaly detection threshold (post-warmup)",
		),
		logPatternCount: telemetryComp.NewGauge(
			"observer",
			telemetryLogPatternExtractorPatternCount,
			nil,
			"Current number of patterns held by the log pattern extractor",
		),
		logsAcceptedBytes: telemetryComp.NewCounter(
			"observer",
			telemetryLogsAcceptedBytes,
			[]string{"source"},
			"Log content bytes accepted into the observer admission boundary",
		),
		filteredMetrics: telemetryComp.NewCounter(
			"observer",
			telemetryFilteredMetrics,
			[]string{"source"},
			"Metrics filtered out before observer ingest, tagged by normalized source",
		),
		seriesCount: telemetryComp.NewGauge(
			"observer",
			telemetrySeriesCount,
			nil,
			"Number of non-telemetry series currently stored in observer storage",
		),
		logsInFlight: telemetryComp.NewGauge(
			"observer",
			telemetryLogsInFlightCount,
			[]string{"log_source"},
			"Number of logs currently in flight in the observer queue",
		),
		storageEvicted: telemetryComp.NewCounter(
			"observer",
			telemetryStorageSeriesEvicted,
			[]string{"reason"},
			"Number of storage series evicted by reason",
		),
		storageCapHit: telemetryComp.NewCounter(
			"observer",
			telemetryStorageCapacityHit,
			nil,
			"Number of times storage capacity eviction was triggered",
		),
		advanceSkipped: telemetryComp.NewCounter(
			"observer",
			telemetryAdvanceSkipped,
			[]string{"reason"},
			"Number of skipped advance requests by trigger reason",
		),
		inputRateLimiterDrop: telemetryComp.NewCounter(
			"observer",
			telemetryLogsInputRateLimiterDropped,
			[]string{"source", "priority"},
			"Logs dropped by the observer ingress rate limiter before reaching the observer",
		),
		processingTime: telemetryComp.NewGauge(
			"observer",
			telemetryDetectorProcessingTimeNs,
			[]string{"detector"},
			"Per-detector processing time in nanoseconds",
		),
		detectorEmissions: telemetryComp.NewCounter(
			"observer",
			telemetryDetectorEmissions,
			[]string{"detector", "severity"},
			"Deduplicated detector emissions by score severity before correlation and reporting",
		),
		scorerEwma: telemetryComp.NewGauge(
			"observer",
			telemetryScorerEWMA,
			[]string{"scorer"},
			"Anomaly scorer EWMA signal, updated every second",
		),
		scorerSeverity: telemetryComp.NewGauge(
			"observer",
			telemetryScorerSeverity,
			[]string{"scorer"},
			"Current anomaly scorer severity level (0=Low, 1=Medium, 2=High)",
		),
	}
}

func (t *observerTelemetry) recordObservationAccepted(kind, source string) {
	t.observationsAccepted.Add(1, kind, source)
}

func (t *observerTelemetry) recordObservationDropped(kind, source string) {
	t.observationsDropped.Add(1, kind, source)
}

func (t *observerTelemetry) recordRRCFScore(detectorName string, score float64) {
	t.rrcfScore.Set(score, detectorName)
}

func (t *observerTelemetry) recordRRCFThreshold(detectorName string, threshold float64) {
	t.rrcfThreshold.Set(threshold, detectorName)
}

func (t *observerTelemetry) setLogPatternCount(count int) {
	t.logPatternCount.Set(float64(count))
}

func (t *observerTelemetry) recordLogAccepted(source string, sizeBytes int) {
	t.recordObservationAccepted("logs", source)
	t.logsAcceptedBytes.Add(float64(sizeBytes), source)
}

func (t *observerTelemetry) recordMetricAccepted(source string) {
	t.recordObservationAccepted("metrics", source)
}

func (t *observerTelemetry) recordFilteredMetric(source string) {
	t.filteredMetrics.Add(1, source)
}

func (t *observerTelemetry) incrementLogsInFlight(logSource string) {
	inFlight := t.inFlightCounter(logSource).Add(1)
	t.logsInFlight.Set(float64(inFlight), logSource)
}

func (t *observerTelemetry) decrementLogsInFlight(logSource string) {
	counter := t.inFlightCounter(logSource)
	inFlight := counter.Add(-1)
	if inFlight < 0 {
		counter.Store(0)
		inFlight = 0
	}
	t.logsInFlight.Set(float64(inFlight), logSource)
}

func (t *observerTelemetry) initLogsInFlight() {
	t.logsInFlight.Set(0, "internal")
	t.logsInFlight.Set(0, "kubelet")
	t.logsInFlight.Set(0, "containers")
}

func (t *observerTelemetry) setSeriesCount(count int) {
	t.seriesCount.Set(float64(count))
}

func (t *observerTelemetry) recordStorageSeriesEvicted(reason string, count int) {
	if count <= 0 {
		return
	}
	t.storageEvicted.Add(float64(count), reason)
}

func (t *observerTelemetry) recordStorageCapacityHit() {
	t.storageCapHit.Add(1)
}

func (t *observerTelemetry) recordAdvanceSkipped(reason string) {
	t.advanceSkipped.Add(1, reason)
}

func (t *observerTelemetry) recordInputRateLimiterDropped(source, priority string) {
	t.inputRateLimiterDrop.Add(1, source, priority)
}

func (t *observerTelemetry) recordDetectorEmission(detector, severity string) {
	t.detectorEmissions.Add(1, detector, severity)
}

func (t *observerTelemetry) inFlightCounter(logSource string) *atomic.Int64 {
	switch logSource {
	case "internal":
		return &t.inFlightInternal
	case "kubelet":
		return &t.inFlightKubelet
	default:
		return &t.inFlightContainers
	}
}

func classifyLogSource(source string, tags []string) string {
	if source == "agent_logs" {
		return "internal"
	}
	for _, tag := range tags {
		if tag == "source:kubelet" {
			return "kubelet"
		}
	}
	return "containers"
}

func (t *observerTelemetry) recordProcessingTime(detectorTag string, durationNs float64) {
	t.processingTime.Set(durationNs, detectorTag)
}
