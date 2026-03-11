// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBOCPDDetector returns a BOCPD detector with a short warmup suitable for
// unit tests with small data sets.
func testBOCPDDetector() *BOCPDDetector {
	d := NewBOCPDDetector()
	d.WarmupPoints = 20
	return d
}

func TestBOCPDDetector_Name(t *testing.T) {
	d := NewBOCPDDetector()
	assert.Equal(t, "bocpd_detector", d.Name())
}

func TestBOCPDDetector_NotEnoughPoints(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()
	storage.Add("ns", "test.metric", 100, 1, nil)

	result := d.Detect(storage, 1)
	assert.Empty(t, result.Anomalies)
}

func TestBOCPDDetector_StableData(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 40; i++ {
		storage.Add("ns", "test.metric", 100+float64(i%3-1), int64(i+1), nil)
	}

	result := d.Detect(storage, 40)
	assert.Empty(t, result.Anomalies, "stable data should not trigger BOCPD")
}

func TestBOCPDDetector_DetectsStepChange(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "test.metric", 140, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	assert.Contains(t, result.Anomalies[0].Title, "BOCPD")
	assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, int64(21))
}

func TestBOCPDDetector_DetectsDownwardStepChange(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 25; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 25; i < 50; i++ {
		storage.Add("ns", "test.metric", 70, int64(i+1), nil)
	}

	result := d.Detect(storage, 50)

	require.NotEmpty(t, result.Anomalies, "should detect downward step change")
	assert.Contains(t, result.Anomalies[0].Title, "BOCPD")
	assert.Contains(t, result.Anomalies[0].Description, "exceeded threshold")
}

func TestBOCPDDetector_DetectsSustainedShiftViaShortRunMass(t *testing.T) {
	d := testBOCPDDetector()
	d.CPThreshold = 0.99     // discourage pure r_t=0 triggers
	d.CPMassThreshold = 0.55 // allow short-run posterior mass trigger
	d.ShortRunLength = 6

	storage := newTimeSeriesStorage()

	// Use jittered warmup so the detector learns real variance (~10 stddev)
	// rather than relying on the MinVariance floor. This keeps the 100→115
	// shift small enough to trigger via short-run mass rather than cpProb.
	for i := 0; i < 30; i++ {
		storage.Add("ns", "test.metric", 100+float64(i%3-1)*5, int64(i+1), nil)
	}
	for i := 30; i < 60; i++ {
		storage.Add("ns", "test.metric", 115, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)

	require.NotEmpty(t, result.Anomalies, "should detect sustained shift")
	assert.Contains(t, result.Anomalies[0].Description, "short-run posterior mass")
}

func TestBOCPDDetector_SustainedIncidentEmitsOnce(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	// 20 stable points, then 40 shifted points.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 60; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 60)

	// Should emit at most one anomaly per series/agg despite sustained shift.
	anomalyCount := 0
	for _, a := range result.Anomalies {
		if a.Source == "test.metric:avg" {
			anomalyCount++
		}
	}
	assert.Equal(t, 1, anomalyCount, "sustained incident should emit exactly one anomaly per series/agg")
}

func TestBOCPDDetector_IncrementalAdvance(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	// First advance: 20 stable points.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	result1 := d.Detect(storage, 20)
	assert.Empty(t, result1.Anomalies, "no anomaly in stable data")

	// Second advance: add a big jump.
	for i := 20; i < 30; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}
	result2 := d.Detect(storage, 30)
	assert.NotEmpty(t, result2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	result3 := d.Detect(storage, 30)
	assert.Empty(t, result3.Anomalies, "no new data should produce no anomalies")
}

func TestBOCPDDetector_RecoveryAndReAlert(t *testing.T) {
	d := testBOCPDDetector()
	d.RecoveryPoints = 5
	storage := newTimeSeriesStorage()
	ts := int64(0)

	addN := func(n int, val float64) {
		for i := 0; i < n; i++ {
			ts++
			storage.Add("ns", "m", val, ts, nil)
		}
	}

	// Warmup + stable baseline.
	addN(25, 100)
	r1 := d.Detect(storage, ts)
	assert.Empty(t, r1.Anomalies)

	// First incident.
	addN(10, 300)
	r2 := d.Detect(storage, ts)
	assert.NotEmpty(t, r2.Anomalies, "should detect first incident")

	// Recovery: stable for enough points.
	addN(20, 100)
	r3 := d.Detect(storage, ts)

	// Second incident.
	addN(10, 300)
	r4 := d.Detect(storage, ts)

	// Count total anomalies for m:avg across r3 and r4.
	avgAnomalies := 0
	for _, a := range append(r3.Anomalies, r4.Anomalies...) {
		if a.Source == "m:avg" {
			avgAnomalies++
		}
	}
	assert.GreaterOrEqual(t, avgAnomalies, 1, "should detect second incident after recovery")
}

func TestBOCPDDetector_Reset(t *testing.T) {
	d := testBOCPDDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 30; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 30)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
}

func TestBOCPDDetector_DeterministicReplay(t *testing.T) {
	makeDetector := func() *BOCPDDetector { return testBOCPDDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "m", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 40)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 40)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should produce same anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp, "anomaly timestamps should match")
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source, "anomaly sources should match")
	}
}

func TestBOCPDDetector_DefaultAggregations(t *testing.T) {
	d := NewBOCPDDetector()
	assert.Equal(t, []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}, d.Aggregations)
}

func TestBOCPDDetector_DefaultWarmup120(t *testing.T) {
	d := NewBOCPDDetector()
	assert.Equal(t, 120, d.WarmupPoints, "default warmup should be 120 points")
}
