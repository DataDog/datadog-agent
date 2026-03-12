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

// testMWDetector returns a Mann-Whitney detector with a short warmup suitable for
// unit tests with small data sets.
func testMWDetector() *MannWhitneyDetector {
	d := NewMannWhitneyDetector()
	d.WindowSize = 20
	d.MinPoints = 10
	// WarmupPoints = WindowSize*2 + MinPoints = 50 (set by ensureDefaults)
	// Relax filters for test data sizes.
	d.SignificanceThreshold = 0.01
	d.MinEffectSize = 0.5
	d.MinDeviationSigma = 2.0
	d.MinRelativeChange = 0.10
	d.RecoveryPoints = 5
	return d
}

func TestMannWhitneyDetector_Name(t *testing.T) {
	d := NewMannWhitneyDetector()
	assert.Equal(t, "mannwhitney_detector", d.Name())
}

func TestMannWhitneyDetector_NotEnoughPoints(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()
	storage.Add("ns", "test.metric", 100, 1, nil)

	result := d.Detect(storage, 1)
	assert.Empty(t, result.Anomalies)
}

func TestMannWhitneyDetector_StableData(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()

	// 80 stable points (enough for warmup=50 + full recent window of 20).
	for i := 0; i < 80; i++ {
		storage.Add("ns", "test.metric", 100+float64(i%3-1), int64(i+1), nil)
	}

	result := d.Detect(storage, 80)
	assert.Empty(t, result.Anomalies, "stable data should not trigger Mann-Whitney")
}

func TestMannWhitneyDetector_DetectsStepChange(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()

	// 50 points at 100 (fills warmup: 20 baseline + 30 remaining).
	for i := 0; i < 50; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	// 30 points at 200 (fills recent window and triggers).
	for i := 50; i < 80; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 80)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	assert.Contains(t, result.Anomalies[0].Title, "Mann-Whitney")
}

func TestMannWhitneyDetector_IncrementalAdvance(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()

	// First advance: stable warmup data.
	for i := 0; i < 50; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	result1 := d.Detect(storage, 50)
	assert.Empty(t, result1.Anomalies, "no anomaly in stable warmup data")

	// Second advance: add shifted data to fill recent window.
	for i := 50; i < 80; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}
	result2 := d.Detect(storage, 80)
	assert.NotEmpty(t, result2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	result3 := d.Detect(storage, 80)
	assert.Empty(t, result3.Anomalies, "no new data should produce no anomalies")
}

func TestMannWhitneyDetector_SustainedIncidentEmitsOnce(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()

	// 50 stable warmup + 40 shifted points.
	for i := 0; i < 50; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	for i := 50; i < 90; i++ {
		storage.Add("ns", "test.metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 90)

	// Count anomalies for avg aggregation — should be at most 1.
	anomalyCount := 0
	for _, a := range result.Anomalies {
		if a.Source == "test.metric:avg" {
			anomalyCount++
		}
	}
	assert.Equal(t, 1, anomalyCount, "sustained incident should emit exactly one anomaly per series/agg")
}

func TestMannWhitneyDetector_Reset(t *testing.T) {
	d := testMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 60; i++ {
		storage.Add("ns", "test.metric", 100, int64(i+1), nil)
	}
	d.Detect(storage, 60)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
}

func TestMannWhitneyDetector_DeterministicReplay(t *testing.T) {
	makeDetector := func() *MannWhitneyDetector { return testMWDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 50; i++ {
		storage.Add("ns", "m", 100, int64(i+1), nil)
	}
	for i := 50; i < 80; i++ {
		storage.Add("ns", "m", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 80)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 80)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should produce same anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp, "anomaly timestamps should match")
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source, "anomaly sources should match")
	}
}

func TestMannWhitneyDetector_DefaultAggregations(t *testing.T) {
	d := NewMannWhitneyDetector()
	assert.Equal(t, []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}, d.Aggregations)
}

func TestMannWhitneyDetector_DefaultWarmup(t *testing.T) {
	d := NewMannWhitneyDetector()
	d.ensureDefaults()
	expected := d.WindowSize*2 + d.MinPoints // 60*2 + 50 = 170
	assert.Equal(t, expected, d.WarmupPoints, "default warmup should be WindowSize*2 + MinPoints")
}
