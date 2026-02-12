// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package comparator provides telemetry comparison and anomaly detection logic.
package comparator

import (
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/collector"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/detector"
)

// TelemetryComparator handles comparison and anomaly detection for telemetry data
type TelemetryComparator struct {
	// Stateful detectors for CPU, Memory, and Errors
	cpuDetector    *CosineDistanceDetector
	memoryDetector *CosineDistanceDetector
	errorDetector  *CosineDistanceDetector

	// Stateful detector for Network
	networkDetector *NetworkDetector

	// Stateful detectors for Trace metrics (EWMA-based)
	traceP50Detector *TraceDetector
	traceP95Detector *TraceDetector
	traceP99Detector *TraceDetector

	// Stateful detector for custom Metrics (bidirectional EWMA)
	metricsDetector *MetricsDetector
}

// NewTelemetryComparator creates a new comparator with initialized detectors
func NewTelemetryComparator() *TelemetryComparator {
	// 288 data points = 24 hours at 5-minute intervals
	// alpha = 2/(N+1) where N=12 means roughly 12 profiles in the baseline (1 hour)
	windowSize := 288
	alpha := 2.0 / (12.0 + 1.0) // ~0.154

	// Trace detectors: EWMA-based with adaptive thresholding
	// alpha=0.2 provides good balance between responsiveness and smoothing
	// threshold=2.5 means ~2.5 "standard deviations" triggers anomaly
	traceAlpha := 0.2
	traceThreshold := 2.5

	// Metrics detector: bidirectional EWMA for all metric types
	// alpha=0.2 for balanced responsiveness, threshold=2.5 for moderate sensitivity
	// Detects both increases AND decreases (works for latencies, counts, sizes, etc.)
	metricsAlpha := 0.2
	metricsThreshold := 2.5

	return &TelemetryComparator{
		cpuDetector:      NewCosineDistanceDetector(windowSize, alpha),
		memoryDetector:   NewCosineDistanceDetector(windowSize, alpha),
		errorDetector:    NewCosineDistanceDetector(windowSize, alpha),
		networkDetector:  NewNetworkDetector(288, 288, 0.999), // historySize=288, scoreSize=288, percentile=0.999 (99.9%)
		traceP50Detector: NewTraceDetector(traceAlpha, traceThreshold),
		traceP95Detector: NewTraceDetector(traceAlpha, traceThreshold),
		traceP99Detector: NewTraceDetector(traceAlpha, traceThreshold),
		metricsDetector:  NewMetricsDetector(metricsAlpha, metricsThreshold),
	}
}

// Compare compares two telemetry snapshots and returns similarity scores
func (tc *TelemetryComparator) Compare(current collector.Telemetry, historical collector.Telemetry, mode collector.ComparisonMode) detector.TelemetryResult {
	result := detector.TelemetryResult{
		CPU:     tc.compareCPUMem(current.CPU, historical.CPU, mode.UseCPUMemV2),
		Mem:     tc.compareCPUMem(current.Memory, historical.Memory, mode.UseCPUMemV2),
		Err:     tc.compareErrorsWithMode(current.Error, historical.Error, mode.UseErrorV2),
		Metrics: tc.compareMetricsWithMode(current.Metrics, historical.Metrics, mode.UseMetricV2),
	}
	result.ClientSentByClient, result.ClientSentByServer = tc.compareNetworksWithMode(current.NetworkClient, historical.NetworkClient, mode.UseNetworkV2)
	result.ServerSentByClient, result.ServerSentByServer = tc.compareNetworksWithMode(current.NetworkServer, historical.NetworkServer, mode.UseNetworkV2)
	result.TraceP50, result.TraceP95, result.TraceP99 = tc.compareTracesWithMode(current.Trace, historical.Trace, mode.UseTraceV2)

	return result
}

func (tc *TelemetryComparator) compareCPUMem(current collector.TelemetrySignal, historical collector.TelemetrySignal, useV2 bool) float64 {
	if useV2 {
		// Use Cosine Distance algorithm
		// Select the appropriate detector based on signal type
		var detector *CosineDistanceDetector
		if current.Type == "cpu" {
			detector = tc.cpuDetector
		} else if current.Type == "mem" {
			detector = tc.memoryDetector
		} else {
			panic("Unknown signal type for V2 comparison: " + current.Type)
		}

		// Compute anomaly score
		anomalyScore, _, _ := detector.ComputeAnomalyScore(current.Values)

		// Convert anomaly score to similarity score
		// anomalyScore: 0 = similar, 1 = very different
		// similarity: 1 = similar, 0 = very different
		return 1.0 - anomalyScore
	}
	// Use the original IsSimilarTo method
	return current.IsSimilarTo(historical)
}

func (tc *TelemetryComparator) compareErrorsWithMode(current collector.TelemetrySignal, historical collector.TelemetrySignal, useV2 bool) float64 {
	if useV2 {
		// Use Cosine Distance algorithm (same as CPU/Memory)
		// Compute anomaly score using error detector
		anomalyScore, _, _ := tc.errorDetector.ComputeAnomalyScore(current.Values)

		// Convert anomaly score to similarity score
		// anomalyScore: 0 = similar, 1 = very different
		// similarity: 1 = similar, 0 = very different
		return 1.0 - anomalyScore
	}
	// Use the original compareErrors function
	return tc.compareErrors(current, historical)
}

func (tc *TelemetryComparator) compareNetworksWithMode(n1 collector.NetworkMetrics, n2 collector.NetworkMetrics, useV2 bool) (float64, float64) {
	if useV2 {
		// Use Mahalanobis distance algorithm (multivariate approach)
		// Compute Mahalanobis distance considering all 4 dimensions
		// Note: We use a single networkDetector for all network metrics
		// (both NetworkClient and NetworkServer flows are tracked together)
		anomalyScore, _ := tc.networkDetector.ComputeAnomalyScore(
			n1.BytesSentByClient,
			n1.BytesSentByServer,
			n1.PacketsSentByClient,
			n1.PacketsSentByServer,
		)

		// Convert anomaly score to similarity score
		// The detector returns squared Mahalanobis distance (unbounded, ≥ 0)
		// We normalize using: similarity = 1 / (1 + anomalyScore)
		// This gives: d²=0 → similarity=1.0, d²→∞ → similarity→0
		similarity := 1.0 / (1.0 + anomalyScore)

		// Return the same similarity score for both client and server dimensions
		// (Mahalanobis distance is a single multivariate metric)
		return similarity, similarity
	}
	// Use the original compareNetworks function
	return tc.compareNetworks(n1, n2)
}

func (tc *TelemetryComparator) compareTracesWithMode(t1 collector.TraceMetrics, t2 collector.TraceMetrics, useV2 bool) (float64, float64, float64) {
	if useV2 {
		// Use EWMA-based algorithm for V2
		// Compute similarity scores for each trace metric (P50, P95, P99)
		// Each detector maintains EWMA baseline and detects deviations
		// Returns scores where 1.0 = normal, 0.0 = anomalous
		similarityP50 := tc.traceP50Detector.ComputeScore(t1.P50Duration)
		similarityP95 := tc.traceP95Detector.ComputeScore(t1.P95Duration)
		similarityP99 := tc.traceP99Detector.ComputeScore(t1.P99Duration)

		return similarityP50, similarityP95, similarityP99
	}
	// Use the original compareTraces function
	return tc.compareTraces(t1, t2)
}

func (tc *TelemetryComparator) compareMetricsWithMode(current []collector.MetricTimeseries, historical []collector.MetricTimeseries, useV2 bool) float64 {
	if useV2 {
		// Use bidirectional EWMA-based detector for V2
		// Convert current metrics to map[name]value for detector
		metricsMap := make(map[string]float64)
		for _, metric := range current {
			metricsMap[metric.MetricName] = metric.Average
		}

		// Compute similarity score using bidirectional detector
		// Returns 1.0 = normal, 0.0 = anomalous
		return tc.metricsDetector.ComputeScore(metricsMap)
	}
	// Use the original compareMetrics function
	return tc.compareMetrics(current, historical)
}

func (tc *TelemetryComparator) compareErrors(current collector.TelemetrySignal, historical collector.TelemetrySignal) float64 {
	// Only detect NEW error types as anomalies
	// - No current errors → 1.0 (always good)
	// - Errors exist → penalize only NEW error types not seen in historical

	// Calculate what fraction of current errors are NEW (not in historical)
	newErrorWeight := 0.0
	for errorType, weight := range current.Values {
		if _, existsInHistorical := historical.Values[errorType]; !existsInHistorical {
			newErrorWeight += weight
		}
	}

	// If all errors are new, score approaches 0
	// If no errors are new (all seen before), score = 1.0
	return 1.0 - newErrorWeight
}

func (tc *TelemetryComparator) compareNetworks(n1 collector.NetworkMetrics, n2 collector.NetworkMetrics) (float64, float64) {
	// For network volume metrics, we want:
	// - 1.0 when values are identical (no anomaly)
	// - Values approaching 0 when there's a large difference (anomaly)
	// Same approach as compareTraces: min(ratio, 1/ratio)

	client := float64(1)
	if n2.BytesSentByClient > 0 {
		ratio := float64(n1.BytesSentByClient) / float64(n2.BytesSentByClient)
		if ratio > 1 {
			client = 1.0 / ratio
		} else if ratio > 0 {
			client = ratio
		}
	}

	server := float64(1)
	if n2.BytesSentByServer > 0 {
		ratio := float64(n1.BytesSentByServer) / float64(n2.BytesSentByServer)
		if ratio > 1 {
			server = 1.0 / ratio
		} else if ratio > 0 {
			server = ratio
		}
	}

	return client, server
}

func (tc *TelemetryComparator) compareTraces(t1 collector.TraceMetrics, t2 collector.TraceMetrics) (float64, float64, float64) {
	// For latency metrics, we want:
	// - 1.0 when values are identical (no anomaly)
	// - Values approaching 0 when there's a large difference (anomaly)
	// Uses formula: 1 / (1 + |ratio - 1|)
	// Where ratio = current / historical

	p50 := float64(1)
	if t2.P50Duration > 0 {
		ratio := t1.P50Duration / t2.P50Duration
		// Convert ratio to similarity score [0,1]
		// ratio=1 → score=1, ratio=2 or 0.5 → score=0.5, ratio→∞ or 0 → score→0
		if ratio > 1 {
			p50 = 1.0 / ratio
		} else if ratio > 0 {
			p50 = ratio
		}
	}

	p95 := float64(1)
	if t2.P95Duration > 0 {
		ratio := t1.P95Duration / t2.P95Duration
		if ratio > 1 {
			p95 = 1.0 / ratio
		} else if ratio > 0 {
			p95 = ratio
		}
	}

	p99 := float64(1)
	if t2.P99Duration > 0 {
		ratio := t1.P99Duration / t2.P99Duration
		if ratio > 1 {
			p99 = 1.0 / ratio
		} else if ratio > 0 {
			p99 = ratio
		}
	}

	return p50, p95, p99
}

func (tc *TelemetryComparator) compareMetrics(current []collector.MetricTimeseries, historical []collector.MetricTimeseries) float64 {
	// Compare error metrics by their average values
	// Only penalize when current value is HIGHER (more errors)
	// Ignore when current value is lower (fewer errors = good)
	// Returns aggregated similarity score across all metrics [0,1]

	// Create a map of historical metrics by name for easy lookup
	historicalMap := make(map[string]float64)
	for _, m := range historical {
		historicalMap[m.MetricName] = m.Average
	}

	// Compare each current metric to its historical counterpart
	totalScore := 0.0
	count := 0

	for _, currentMetric := range current {
		historicalAvg, exists := historicalMap[currentMetric.MetricName]
		if !exists {
			continue // Skip metrics not in historical or with zero baseline
		}

		count++

		if historicalAvg == 0 {
			historicalAvg = 0.000000001
		}

		// Calculate ratio
		ratio := currentMetric.Average / historicalAvg

		// Only penalize when ratio > 1 (current value is higher = more errors)
		// When ratio <= 1 (current value is lower = fewer errors), ignore it
		if ratio > 1 {
			score := 1.0 / ratio
			totalScore += score
		} else {
			totalScore++
		}
		// If ratio <= 1, skip this metric (don't count it)
	}

	// Return average similarity score, default to 1.0 if no comparable metrics
	if count > 0 {
		return totalScore / float64(count)
	}
	return 1.0
}
