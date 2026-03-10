// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricMatches(t *testing.T) {
	tests := []struct {
		name   string
		source string
		key    string
		want   bool
	}{
		{"exact", "redis.cpu.sys", "redis:redis.cpu.sys", true},
		{"with aggregate suffix", "redis.cpu.sys:avg", "redis:redis.cpu.sys", true},
		{"no match", "redis.mem.used", "redis:redis.cpu.sys", false},
		{"different service same metric", "trace.http.request.hits", "driver-location-service:trace.http.request.hits", true},
		{"substring match", "trace.http.request.hits:avg", "driver-location-service:trace.http.request.hits", true},
		{"partial metric no match", "redis.cpu", "redis:redis.cpu.sys", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, metricMatches(tt.source, tt.key))
		})
	}
}

func TestScoreMetrics_Basic(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.cpu.sys", "redis.info.latency_ms"}},
			{Service: "driver-location-service", Metrics: []string{"trace.http.request.hits"}},
		},
		FalsePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.mem.used"}},
		},
	}

	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{Anomalies: []ObserverAnomaly{{Source: "redis.cpu.sys"}}},             // TP
			{Anomalies: []ObserverAnomaly{{Source: "redis.mem.used"}}},            // FP
			{Anomalies: []ObserverAnomaly{{Source: "some.other.metric"}}},         // unknown
			{Anomalies: []ObserverAnomaly{{Source: "trace.http.request.hits"}}},   // TP
		},
	}

	result := ScoreMetrics(output, gt, 0)

	assert.Equal(t, 2, result.TPCount)
	assert.Equal(t, 1, result.FPCount)
	assert.Equal(t, 1, result.UnknownCount)
	assert.Equal(t, 4, result.TotalCount)

	// Precision: 2 / (2 + 1) = 0.667
	assert.InDelta(t, 0.6667, result.MetricPrecision, 0.001)
	// Recall: 2 found / 3 total TP metrics = 0.667
	assert.InDelta(t, 0.6667, result.MetricRecall, 0.001)

	assert.Len(t, result.TPMetricsFound, 2)
	assert.Len(t, result.TPMetricsMissed, 1) // redis.info.latency_ms
}

func TestScoreMetrics_AllTPsFound(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.cpu.sys"}},
		},
		FalsePositives: []MetricGroundTruthEntry{},
	}

	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{Anomalies: []ObserverAnomaly{{Source: "redis.cpu.sys"}}},
		},
	}

	result := ScoreMetrics(output, gt, 0)
	assert.Equal(t, 1, result.TPCount)
	assert.Equal(t, 0, result.FPCount)
	assert.InDelta(t, 1.0, result.MetricPrecision, 0.001)
	assert.InDelta(t, 1.0, result.MetricRecall, 0.001)
	assert.InDelta(t, 1.0, result.MetricF1, 0.001)
}

func TestScoreMetrics_Empty(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives:  []MetricGroundTruthEntry{{Service: "redis", Metrics: []string{"redis.cpu.sys"}}},
		FalsePositives: []MetricGroundTruthEntry{},
	}

	output := &ObserverOutput{AnomalyPeriods: nil}

	result := ScoreMetrics(output, gt, 0)
	assert.Equal(t, 0, result.TPCount)
	assert.Equal(t, 0, result.TotalCount)
	assert.InDelta(t, 0.0, result.MetricRecall, 0.001)
	assert.Len(t, result.TPMetricsMissed, 1)
}

func TestScoreMetrics_AggSuffix(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.cpu.sys"}},
		},
	}

	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{Anomalies: []ObserverAnomaly{{Source: "redis.cpu.sys:avg"}}},
		},
	}

	result := ScoreMetrics(output, gt, 0)
	assert.Equal(t, 1, result.TPCount)
}

func TestScoreMetrics_TitleFallback(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.cpu.sys"}},
		},
	}

	// Non-verbose output: no Anomalies, but Title has metric name
	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{Title: "Passthrough[cusum]: redis.cpu.sys"},
		},
	}

	result := ScoreMetrics(output, gt, 0)
	assert.Equal(t, 1, result.TPCount)
}

func TestScoreMetrics_Detections_FirstSeenAndCount(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.cpu.sys", "redis.info.latency_ms"}},
		},
		FalsePositives: []MetricGroundTruthEntry{
			{Service: "redis", Metrics: []string{"redis.mem.used"}},
		},
	}

	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{PeriodStart: 1000, Anomalies: []ObserverAnomaly{{Source: "redis.cpu.sys"}}},
			{PeriodStart: 1100, Anomalies: []ObserverAnomaly{{Source: "redis.cpu.sys"}}},    // second hit
			{PeriodStart: 1050, Anomalies: []ObserverAnomaly{{Source: "redis.mem.used"}}},    // FP fires
			{PeriodStart: 1200, Anomalies: []ObserverAnomaly{{Source: "some.other.metric"}}}, // unknown
		},
	}

	result := ScoreMetrics(output, gt, 900)

	// 3 ground truth entries: 2 TP metrics + 1 FP metric
	assert.Len(t, result.Detections, 3)

	// Build lookup by classification+metric for deterministic assertions
	byKey := map[string]MetricDetection{}
	for _, d := range result.Detections {
		byKey[d.Classification+":"+d.Service+":"+d.Metric] = d
	}

	// TP: redis.cpu.sys — detected, first at 1000, count=2, delta=100s
	cpuDet := byKey["tp:redis:redis.cpu.sys"]
	assert.True(t, cpuDet.Detected)
	assert.Equal(t, 2, cpuDet.Count)
	assert.Equal(t, int64(1000), cpuDet.FirstSeenUnix)
	assert.InDelta(t, 100.0, cpuDet.DeltaFromDisruption, 0.001)

	// TP: redis.info.latency_ms — NOT detected
	latDet := byKey["tp:redis:redis.info.latency_ms"]
	assert.False(t, latDet.Detected)
	assert.Equal(t, 0, latDet.Count)
	assert.Equal(t, int64(0), latDet.FirstSeenUnix)

	// FP: redis.mem.used — fired, first at 1050, count=1
	memDet := byKey["fp:redis:redis.mem.used"]
	assert.True(t, memDet.Detected)
	assert.Equal(t, 1, memDet.Count)
	assert.Equal(t, int64(1050), memDet.FirstSeenUnix)
	assert.InDelta(t, 150.0, memDet.DeltaFromDisruption, 0.001)

	// Unknown counts
	assert.Equal(t, 1, result.UnknownMetricCount)
	assert.Equal(t, 1, result.UnknownDetectionCount)
}

func TestScoreMetrics_Detections_NoDisruptionStart(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "svc", Metrics: []string{"m1"}},
		},
	}

	output := &ObserverOutput{
		AnomalyPeriods: []ObserverCorrelation{
			{PeriodStart: 500, Anomalies: []ObserverAnomaly{{Source: "m1"}}},
		},
	}

	// disruptionStart=0 → delta should be 0
	result := ScoreMetrics(output, gt, 0)
	assert.Len(t, result.Detections, 1)
	assert.Equal(t, 0.0, result.Detections[0].DeltaFromDisruption)
	assert.True(t, result.Detections[0].Detected)
}

func TestScoreMetrics_Detections_SortOrder(t *testing.T) {
	gt := &MetricGroundTruth{
		TruePositives: []MetricGroundTruthEntry{
			{Service: "b-svc", Metrics: []string{"z-metric"}},
			{Service: "a-svc", Metrics: []string{"a-metric"}},
		},
		FalsePositives: []MetricGroundTruthEntry{
			{Service: "a-svc", Metrics: []string{"fp-metric"}},
		},
	}

	output := &ObserverOutput{AnomalyPeriods: nil}
	result := ScoreMetrics(output, gt, 0)

	// Should be sorted: fp first, then tp; within each, by service then metric
	assert.Len(t, result.Detections, 3)
	assert.Equal(t, "fp", result.Detections[0].Classification)
	assert.Equal(t, "tp", result.Detections[1].Classification)
	assert.Equal(t, "a-svc", result.Detections[1].Service)
	assert.Equal(t, "tp", result.Detections[2].Classification)
	assert.Equal(t, "b-svc", result.Detections[2].Service)
}
