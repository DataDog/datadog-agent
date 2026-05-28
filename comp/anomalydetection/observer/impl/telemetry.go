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
	telemetryDetectorProcessingTimeNs        = "observer.detector.processing_time_ns"         // Time spent processing one detector/correlator/extractor step.
	telemetryObsChannelDropped               = "observer.channel.dropped"                     // Observations dropped when the observer channel is full.
	telemetryRRCFScore                       = "observer.rrcf.score"                          // Latest RRCF score per detector.
	telemetryRRCFThreshold                   = "observer.rrcf.threshold"                      // Current RRCF anomaly threshold per detector.
	telemetryLogPatternExtractorPatternCount = "observer.log_pattern_extractor.pattern_count" // Delta of active log-pattern count.
	telemetryLogsIngested                    = "observer.logs.ingested"                       // Number of logs ingested by anomaly detection.
	telemetryProcessedLogSize                = "observer.logs.processed_bytes"                // Total bytes processed from ingested logs.
	telemetryDroppedLogs                     = "observer.logs.dropped"                        // Number of logs dropped before processing.
	telemetrySeriesCount                     = "observer.series.count"                        // Number of active non-telemetry observer series.
	telemetryReportsEmitted                  = "observer.reports.emitted"                     // Number of reports emitted by reporters.
	telemetryLogsInFlightCount               = "observer.logs.in_flight"                      // Number of logs currently queued/in flight.
)

type observerTelemetry struct {
	// Existing telemetry
	detectorProcessingTime telemetry.Gauge
	channelDropped         telemetry.Counter
	rrcfScore              telemetry.Gauge
	rrcfThreshold          telemetry.Gauge
	logPatternCount        telemetry.Counter

	// New telemetry
	logsIngested     telemetry.Counter
	processedLogSize telemetry.Counter
	droppedLogs      telemetry.Counter
	seriesCount      telemetry.Gauge
	reportsEmitted   telemetry.Counter
	logsInFlight     telemetry.Gauge

	inFlightInternal   atomic.Int64
	inFlightKubelet    atomic.Int64
	inFlightContainers atomic.Int64
}

func newObserverTelemetry(telemetryComp telemetry.Component) *observerTelemetry {
	return &observerTelemetry{
		detectorProcessingTime: telemetryComp.NewGauge(
			"observer",
			telemetryDetectorProcessingTimeNs,
			[]string{"detector"},
			"Per detector/correlator/extractor processing time in nanoseconds",
		),
		channelDropped: telemetryComp.NewCounter(
			"observer",
			telemetryObsChannelDropped,
			[]string{"source"},
			"Observations dropped because the internal channel was full, tagged by source handle",
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
		logPatternCount: telemetryComp.NewCounter(
			"observer",
			telemetryLogPatternExtractorPatternCount,
			[]string{"detector"},
			"Log pattern extractor number of active patterns",
		),
		logsIngested: telemetryComp.NewCounter(
			"observer",
			telemetryLogsIngested,
			[]string{"log_source"},
			"Number of logs ingested by anomaly detection",
		),
		processedLogSize: telemetryComp.NewCounter(
			"observer",
			telemetryProcessedLogSize,
			[]string{"log_source"},
			"Processed log size in bytes by anomaly detection",
		),
		droppedLogs: telemetryComp.NewCounter(
			"observer",
			telemetryDroppedLogs,
			[]string{"log_source"},
			"Logs dropped because observer queue was full",
		),
		seriesCount: telemetryComp.NewGauge(
			"observer",
			telemetrySeriesCount,
			nil,
			"Number of non-telemetry series currently stored in observer storage",
		),
		reportsEmitted: telemetryComp.NewCounter(
			"observer",
			telemetryReportsEmitted,
			[]string{"reporter"},
			"Number of reports emitted by observer reporters",
		),
		logsInFlight: telemetryComp.NewGauge(
			"observer",
			telemetryLogsInFlightCount,
			[]string{"log_source"},
			"Number of logs currently in flight in the observer queue",
		),
	}
}

func (t *observerTelemetry) recordDetectorProcessingTime(detectorTag string, nanos float64) {
	t.detectorProcessingTime.Set(nanos, detectorTag)
}

func (t *observerTelemetry) recordChannelDropped(source string) {
	t.channelDropped.Add(1, source)
}

func (t *observerTelemetry) recordRRCFScore(detectorName string, score float64) {
	t.rrcfScore.Set(score, "detector:"+detectorName)
}

func (t *observerTelemetry) recordRRCFThreshold(detectorName string, threshold float64) {
	t.rrcfThreshold.Set(threshold, "detector:"+detectorName)
}

func (t *observerTelemetry) recordLogPatternCountDelta(detectorName string, delta float64) {
	t.logPatternCount.Add(delta, "detector:"+detectorName)
}

func (t *observerTelemetry) recordLogIngested(source string, tags []string, sizeBytes int) string {
	logSource := classifyLogSource(source, tags)
	t.logsIngested.Add(1, logSource)
	t.processedLogSize.Add(float64(sizeBytes), logSource)
	return logSource
}

func (t *observerTelemetry) recordDroppedLog(source string, tags []string) {
	logSource := classifyLogSource(source, tags)
	t.droppedLogs.Add(1, logSource)
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

func (t *observerTelemetry) recordReportEmitted(reporter string) {
	t.reportsEmitted.Add(1, "reporter:"+reporter)
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
	if source == "agent-internal-logs" {
		return "internal"
	}
	for _, tag := range tags {
		if tag == "source:kubelet" {
			return "kubelet"
		}
	}
	return "containers"
}
