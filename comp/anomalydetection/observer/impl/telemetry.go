// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"runtime"
	"sync/atomic"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

const (
	telemetryObsChannelDropped               = "observer.channel.dropped"                            // Observations dropped when the observer channel is full.
	telemetryRRCFScore                       = "observer.rrcf.score"                                 // Latest RRCF score per detector.
	telemetryRRCFThreshold                   = "observer.rrcf.threshold"                             // Current RRCF anomaly threshold per detector.
	telemetryRRCFAllScoresCount              = "observer.rrcf.all_scores_count"                      // Number of entries in the unbounded allScores history slice.
	telemetryLogPatternExtractorPatternCount = "observer.log_pattern_extractor.pattern_count"        // Delta of active log-pattern count.
	telemetryLogPatternExtractorPatternGauge = "observer.log_pattern_extractor.pattern_count_live"   // Live gauge of active log-pattern cluster count.
	telemetryLogsIngested                    = "observer.logs.ingested"                              // Number of logs ingested by anomaly detection.
	telemetryProcessedLogSize                = "observer.logs.processed_bytes"                       // Total bytes processed from ingested logs.
	telemetryDroppedLogs                     = "observer.logs.dropped"                               // Number of logs dropped before processing.
	telemetrySeriesCount                     = "observer.series.count"                               // Number of active non-telemetry observer series.
	telemetryStorageTotalPoints              = "observer.storage.total_points"                       // Total data points stored per namespace.
	telemetryStorageIDStatsSlots             = "observer.storage.id_stats_slots"                     // Length of seriesIDStats slice including nil holes from evictions.
	telemetryStorageObservationTimestamps    = "observer.storage.observation_timestamps"             // Number of entries in the observation-timestamps map (log mode only).
	telemetryLogsInFlightCount               = "observer.logs.in_flight"                             // Number of logs currently queued/in flight.
	telemetryStorageSeriesEvicted            = "observer.storage.series_evicted"                     // Number of storage series evicted to enforce bounds.
	telemetryStorageCapacityHit              = "observer.storage.capacity_hit"                       // Number of times storage capacity eviction was triggered.
	telemetryAdvanceSkipped                  = "observer.scheduler.advance_skipped"                  // Number of advance requests skipped as already analyzed.
	telemetryAdvanceCount                    = "observer.advance.count"                              // Number of successful advance cycles completed.
	telemetryLogsSamplerDropped              = "observer.logs.sampler_dropped"                       // Logs dropped by the source sampler before reaching the observer, by source and priority.
	telemetryDetectorProcessingTimeNs        = "observer.detector.processing_time_ns"                // Per-detector processing time in nanoseconds.
	telemetryScorerEWMA                      = "observer.scorer.ewma"                                // Anomaly scorer smoothed EWMA signal, updated every second.
	telemetryScorerState                     = "observer.scorer.state"                               // Anomaly scorer severity level on transition (0=Low,1=Medium,2=High).
	telemetryCorrelatorClusterCount          = "observer.correlator.cluster_count"                   // Number of active time clusters per correlator.
	telemetryGoHeapAllocBytes                = "observer.go.heap_alloc_bytes"                        // Go runtime: bytes of allocated heap objects.
	telemetryGoHeapSysBytes                  = "observer.go.heap_sys_bytes"                          // Go runtime: bytes of heap memory obtained from the OS.
	telemetryGoNumGC                         = "observer.go.num_gc"                                  // Go runtime: number of completed GC cycles.
)

type observerTelemetry struct {
	channelDropped  telemetry.Counter
	rrcfScore       telemetry.Gauge
	rrcfThreshold   telemetry.Gauge
	rrcfAllScores   telemetry.Gauge
	logPatternCount telemetry.Counter
	logPatternGauge telemetry.Gauge

	logsIngested     telemetry.Counter
	processedLogSize telemetry.Counter
	droppedLogs      telemetry.Counter
	seriesCount      telemetry.Gauge
	storageTotalPts  telemetry.Gauge
	storageIDSlots   telemetry.Gauge
	storageObsTS     telemetry.Gauge
	logsInFlight     telemetry.Gauge
	storageEvicted   telemetry.Counter
	storageCapHit    telemetry.Counter
	advanceSkipped   telemetry.Counter
	advanceCount     telemetry.Counter
	samplerDropped   telemetry.Counter
	processingTime   telemetry.Gauge
	scorerEwma       telemetry.Gauge
	scorerState      telemetry.Gauge
	correlatorClusters telemetry.Gauge
	goHeapAlloc      telemetry.Gauge
	goHeapSys        telemetry.Gauge
	goNumGC          telemetry.Gauge

	inFlightInternal   atomic.Int64
	inFlightKubelet    atomic.Int64
	inFlightContainers atomic.Int64
}

func newObserverTelemetry(telemetryComp telemetry.Component) *observerTelemetry {
	return &observerTelemetry{
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
		rrcfAllScores: telemetryComp.NewGauge(
			"observer",
			telemetryRRCFAllScoresCount,
			[]string{"detector"},
			"Number of entries in the RRCF allScores history slice; grows monotonically until Reset",
		),
		logPatternCount: telemetryComp.NewCounter(
			"observer",
			telemetryLogPatternExtractorPatternCount,
			[]string{"detector"},
			"Delta of active log-pattern cluster count (positive on insert, negative on eviction/GC)",
		),
		logPatternGauge: telemetryComp.NewGauge(
			"observer",
			telemetryLogPatternExtractorPatternGauge,
			[]string{"detector"},
			"Live count of active log-pattern clusters per extractor, updated on each advance",
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
		storageTotalPts: telemetryComp.NewGauge(
			"observer",
			telemetryStorageTotalPoints,
			[]string{"namespace"},
			"Total data points stored per namespace; proxy for per-namespace storage memory",
		),
		storageIDSlots: telemetryComp.NewGauge(
			"observer",
			telemetryStorageIDStatsSlots,
			nil,
			"Length of the seriesIDStats slice including nil holes; diverges from series.count after evictions",
		),
		storageObsTS: telemetryComp.NewGauge(
			"observer",
			telemetryStorageObservationTimestamps,
			nil,
			"Number of entries in the observation-timestamps map; grows one entry per second of log activity",
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
		advanceCount: telemetryComp.NewCounter(
			"observer",
			telemetryAdvanceCount,
			nil,
			"Number of successful advance cycles completed by the observer engine",
		),
		samplerDropped: telemetryComp.NewCounter(
			"observer",
			telemetryLogsSamplerDropped,
			[]string{"source", "priority"},
			"Logs dropped by the source sampler (rate limit or min_severity) before reaching the observer",
		),
		processingTime: telemetryComp.NewGauge(
			"observer",
			telemetryDetectorProcessingTimeNs,
			[]string{"detector"},
			"Per-detector processing time in nanoseconds",
		),
		scorerEwma: telemetryComp.NewGauge(
			"observer",
			telemetryScorerEWMA,
			[]string{"scorer"},
			"Anomaly scorer EWMA signal, updated every second",
		),
		scorerState: telemetryComp.NewGauge(
			"observer",
			telemetryScorerState,
			[]string{"scorer", "direction"},
			"Anomaly scorer severity level on transition (0=Low, 1=Medium, 2=High)",
		),
		correlatorClusters: telemetryComp.NewGauge(
			"observer",
			telemetryCorrelatorClusterCount,
			[]string{"correlator"},
			"Number of active time clusters per correlator",
		),
		goHeapAlloc: telemetryComp.NewGauge(
			"observer",
			telemetryGoHeapAllocBytes,
			nil,
			"Go runtime: bytes of live allocated heap objects (from runtime.MemStats.HeapAlloc)",
		),
		goHeapSys: telemetryComp.NewGauge(
			"observer",
			telemetryGoHeapSysBytes,
			nil,
			"Go runtime: bytes of heap memory obtained from the OS (from runtime.MemStats.HeapSys)",
		),
		goNumGC: telemetryComp.NewGauge(
			"observer",
			telemetryGoNumGC,
			nil,
			"Go runtime: cumulative number of completed GC cycles (from runtime.MemStats.NumGC)",
		),
	}
}

// updateGoMemStats reads Go runtime memory statistics and emits heap gauges.
// Called once per advance cycle; ReadMemStats acquires a lock but does not
// stop the world since Go 1.15.
func (t *observerTelemetry) updateGoMemStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.goHeapAlloc.Set(float64(ms.HeapAlloc))
	t.goHeapSys.Set(float64(ms.HeapSys))
	t.goNumGC.Set(float64(ms.NumGC))
}

func (t *observerTelemetry) recordChannelDropped(source string) {
	t.channelDropped.Add(1, source)
}

func (t *observerTelemetry) recordRRCFScore(detectorName string, score float64) {
	t.rrcfScore.Set(score, detectorName)
}

func (t *observerTelemetry) recordRRCFThreshold(detectorName string, threshold float64) {
	t.rrcfThreshold.Set(threshold, detectorName)
}

func (t *observerTelemetry) recordLogPatternCountDelta(detectorName string, delta float64) {
	t.logPatternCount.Add(delta, detectorName)
}

func (t *observerTelemetry) recordLogIngested(logSource string, sizeBytes int) {
	t.logsIngested.Add(1, logSource)
	t.processedLogSize.Add(float64(sizeBytes), logSource)
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

func (t *observerTelemetry) recordSamplerDropped(source, priority string) {
	t.samplerDropped.Add(1, source, priority)
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

func (t *observerTelemetry) recordProcessingTime(detectorTag string, durationNs float64) {
	t.processingTime.Set(durationNs, detectorTag)
}

func (t *observerTelemetry) recordRRCFAllScoresCount(detectorName string, count int) {
	t.rrcfAllScores.Set(float64(count), detectorName)
}

func (t *observerTelemetry) recordLogPatternCountGauge(detectorName string, count int) {
	t.logPatternGauge.Set(float64(count), detectorName)
}

func (t *observerTelemetry) recordStorageTotalPoints(namespace string, count int64) {
	t.storageTotalPts.Set(float64(count), namespace)
}

func (t *observerTelemetry) recordStorageIDStatsSlots(count int) {
	t.storageIDSlots.Set(float64(count))
}

func (t *observerTelemetry) recordStorageObservationTimestamps(count int) {
	t.storageObsTS.Set(float64(count))
}

func (t *observerTelemetry) recordAdvance() {
	t.advanceCount.Add(1)
}

func (t *observerTelemetry) recordCorrelatorClusterCount(correlatorName string, count int) {
	t.correlatorClusters.Set(float64(count), correlatorName)
}
