// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScanWelchDetector() *ScanWelchDetector {
	d := NewScanWelchDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

func TestScanWelch_NotEnoughPoints(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 10)
	assert.Empty(t, result.Anomalies, "should not fire with fewer than MinPoints")
}

func TestScanWelch_DetectsStepChange(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	assert.Contains(t, result.Anomalies[0].Title, "ScanWelch")
	assert.InDelta(t, 21, result.Anomalies[0].Timestamp, 3)
}

func TestScanWelch_IncrementalAdvance(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	r1 := d.Detect(storage, 20)
	assert.Empty(t, r1.Anomalies, "no anomaly in stable data")

	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 40)
	require.NotEmpty(t, r2.Anomalies, "should detect step change on second advance")

	r3 := d.Detect(storage, 40)
	assert.Empty(t, r3.Anomalies, "no new data should produce no anomalies")
}

func TestScanWelch_SegmentAdvancement(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	for i := 50; i < 90; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 90)
	assert.Empty(t, r2.Anomalies, "stable post-change data should not re-fire")
}

func TestScanWelch_TwoSequentialChanges(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	for i := 50; i < 80; i++ {
		storage.Add("ns", "metric", 500, int64(i+1), nil)
	}

	r2 := d.Detect(storage, 80)
	require.NotEmpty(t, r2.Anomalies, "should detect second changepoint after segment advancement")
}

func TestScanWelch_DeterministicReplay(t *testing.T) {
	makeDetector := func() *ScanWelchDetector { return testScanWelchDetector() }

	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	d1 := makeDetector()
	r1 := d1.Detect(storage, 40)

	d2 := makeDetector()
	r2 := d2.Detect(storage, 40)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies), "replay should produce same anomaly count")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp)
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source)
	}
}

func TestScanWelch_Reset(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 40; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	d.Detect(storage, 40)
	assert.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedRefs, "reset should clear cached refs")
}

// TestScanWelch_PreloadedReplayFiresAsDataTimeAdvances reproduces the testbench
// replay path: all points are written to storage up front (so WriteGeneration
// reaches its final value immediately), then Detect is called repeatedly with
// an advancing dataTime that gradually exposes the history. A WriteGeneration-
// only skip gate suppresses every scan after the first call here and detects
// nothing; gating on visible point count keeps the detector scanning as the
// data becomes visible. This is distinct from TestScanWelch_IncrementalAdvance,
// where each new point bumps WriteGeneration between Detect calls (the live
// path).
func TestScanWelch_PreloadedReplayFiresAsDataTimeAdvances(t *testing.T) {
	d := testScanWelchDetector()
	storage := newTimeSeriesStorage()

	// Preload the entire series before any Detect call.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	// Replay: advance dataTime one bucket at a time over preloaded storage.
	var fired bool
	for dataTime := int64(1); dataTime <= 40; dataTime++ {
		if len(d.Detect(storage, dataTime).Anomalies) > 0 {
			fired = true
		}
	}

	assert.True(t, fired, "preloaded replay should detect the step change as dataTime advances")
}
