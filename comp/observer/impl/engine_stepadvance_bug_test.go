// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

// fixedDetector is a mock detector that returns a fixed set of anomalies once.
type fixedDetector struct {
	anomalies []observer.Anomaly
	fired     bool
}

func (d *fixedDetector) Name() string { return "fixed" }

func (d *fixedDetector) Detect(_ observer.StorageReader, _ int64) observer.DetectionResult {
	if d.fired {
		return observer.DetectionResult{}
	}
	d.fired = true
	return observer.DetectionResult{Anomalies: d.anomalies}
}

// TestStepAdvance_SingleTimestampGroupLost reproduces the live bug where scan
// detectors produce anomalies with a single historical timestamp that is >120s
// behind upTo. Uses the real engine Advance path.
func TestStepAdvance_SingleTimestampGroupLost(t *testing.T) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 300; sec++ {
		storage.Add("ns", "metric_a", 100.0, sec, nil)
		storage.Add("ns", "metric_b", 100.0, sec, nil)
	}

	changepointTime := int64(100)

	detector := &fixedDetector{
		anomalies: []observer.Anomaly{
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_a", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_a:avg",
				DetectorName:   "scanmw",
				Timestamp:      changepointTime,
				Description:    "metric_a changed",
			},
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_b", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_b:avg",
				DetectorName:   "scanmw",
				Timestamp:      changepointTime,
				Description:    "metric_b changed",
			},
		},
	}

	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observer.Detector{detector},
		correlators: []observer.Correlator{correlator},
	})

	// Advance to 310 — 210s after the changepoint (>120s window).
	// This goes through the real runDetectorsAndCorrelatorsSnapshot.
	e.Advance(310)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated correlations: %d", len(accumulated))
	for _, ac := range accumulated {
		t.Logf("  %s: %d anomalies", ac.Pattern, len(ac.Anomalies))
	}

	assert.NotEmpty(t, accumulated,
		"time_cluster should accumulate the cluster even when all anomalies "+
			"share one timestamp that is >120s behind upTo")
}

// TestStepAdvance_TwoTimestampGroupsSurvive shows that with two distinct
// timestamps, the cluster is accumulated (step-advance fires between groups).
func TestStepAdvance_TwoTimestampGroupsSurvive(t *testing.T) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 300; sec++ {
		storage.Add("ns", "metric_a", 100.0, sec, nil)
		storage.Add("ns", "metric_b", 100.0, sec, nil)
	}

	detector := &fixedDetector{
		anomalies: []observer.Anomaly{
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_a", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_a:avg",
				DetectorName:   "scanmw",
				Timestamp:      100,
				Description:    "metric_a changed",
			},
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_b", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_b:avg",
				DetectorName:   "scanmw",
				Timestamp:      105, // different timestamp — triggers step-advance
				Description:    "metric_b changed",
			},
		},
	}

	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observer.Detector{detector},
		correlators: []observer.Correlator{correlator},
	})

	e.Advance(310)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated correlations: %d", len(accumulated))
	for _, ac := range accumulated {
		t.Logf("  %s: %d anomalies", ac.Pattern, len(ac.Anomalies))
	}

	assert.NotEmpty(t, accumulated,
		"time_cluster should accumulate when anomalies have different timestamps")
}

// TestStepAdvance_SingleGroupCloseToUpTo shows that when the single timestamp
// is within 120s of upTo, the cluster survives the final advance naturally.
func TestStepAdvance_SingleGroupCloseToUpTo(t *testing.T) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 200; sec++ {
		storage.Add("ns", fmt.Sprintf("metric_%d", sec%2), 100.0, sec, nil)
	}

	changepointTime := int64(100)

	detector := &fixedDetector{
		anomalies: []observer.Anomaly{
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_a", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_a:avg",
				DetectorName:   "scanmw",
				Timestamp:      changepointTime,
				Description:    "metric_a changed",
			},
			{
				Source:         observer.AnomalySource{Namespace: "ns", Name: "metric_b", Aggregate: observer.AggregateAverage},
				SourceSeriesID: "ns|metric_b:avg",
				DetectorName:   "scanmw",
				Timestamp:      changepointTime,
				Description:    "metric_b changed",
			},
		},
	}

	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observer.Detector{detector},
		correlators: []observer.Correlator{correlator},
	})

	// upTo = 200, only 100s ahead — within 120s window
	e.Advance(200)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated correlations: %d", len(accumulated))

	assert.NotEmpty(t, accumulated,
		"cluster should survive when upTo is within 120s of anomaly timestamps")
}
