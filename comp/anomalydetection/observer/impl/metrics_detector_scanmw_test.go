// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScanMWDetector() *ScanMWDetector {
	d := NewScanMWDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

func TestScanMW_NotEnoughPoints(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 10)
	assert.Empty(t, result.Anomalies, "should not fire with fewer than MinPoints")
}

func TestScanMW_DetectsStepChange(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	result := d.Detect(storage, 40)

	require.NotEmpty(t, result.Anomalies, "should detect step change")
	title, description := observer.FormatAnomaly(result.Anomalies[0])
	assert.Equal(t, "ScanMW changepoint: metric:avg", title)
	assert.Contains(t, description, "increased")
	require.NotNil(t, result.Anomalies[0].DebugInfo)
	assert.Positive(t, result.Anomalies[0].DebugInfo.PValue)
	assert.NotZero(t, result.Anomalies[0].DebugInfo.EffectSize)
	// Changepoint should be near the transition at index 20.
	assert.InDelta(t, 21, result.Anomalies[0].Timestamp, 3)
}

func TestScanMW_DetectsDownwardStepChange(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	result := d.Detect(storage, 40)
	require.NotEmpty(t, result.Anomalies)
	title, description := observer.FormatAnomaly(result.Anomalies[0])
	assert.Equal(t, "ScanMW changepoint: metric:avg", title)
	assert.Contains(t, description, "decreased")
}

func TestScanMW_IncrementalAdvance(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// First advance: stable data, not enough to fire.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	r1 := d.Detect(storage, 20)
	assert.Empty(t, r1.Anomalies, "no anomaly in stable data")

	// Second advance: add shifted data — now has 40 points total.
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 40)
	require.NotEmpty(t, r2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	r3 := d.Detect(storage, 40)
	assert.Empty(t, r3.Anomalies, "no new data should produce no anomalies")
}

func TestScanMW_SegmentAdvancement(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: baseline at 50, shift to 200.
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// After fire, the segment start should have advanced.
	// Adding more data at 200 (same as post-change) should not re-fire
	// because the post-change segment is stable.
	for i := 50; i < 90; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 90)
	assert.Empty(t, r2.Anomalies, "stable post-change data should not re-fire")
}

func TestScanMW_TwoSequentialChanges(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// Phase 1: 50 → 200
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 50, int64(i+1), nil)
	}
	for i := 20; i < 50; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}

	r1 := d.Detect(storage, 50)
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// Phase 2: 200 → 500
	for i := 50; i < 80; i++ {
		storage.Add("ns", "metric", 500, int64(i+1), nil)
	}

	r2 := d.Detect(storage, 80)
	require.NotEmpty(t, r2.Anomalies, "should detect second changepoint after segment advancement")
}

func TestScanMW_DeterministicReplay(t *testing.T) {
	makeDetector := func() *ScanMWDetector { return testScanMWDetector() }

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
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp, "anomaly timestamps should match")
		assert.Equal(t, r1.Anomalies[i].Source, r2.Anomalies[i].Source, "anomaly sources should match")
	}
}

func TestScanMW_Reset(t *testing.T) {
	d := testScanMWDetector()
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

// TestScanMW_PreloadedReplayFiresAsDataTimeAdvances reproduces the testbench
// replay path: all points are written to storage up front (so WriteGeneration
// reaches its final value immediately), then Detect is called repeatedly with
// an advancing dataTime that gradually exposes the history. A WriteGeneration-
// only skip gate suppresses every scan after the first call here and detects
// nothing; gating on visible point count keeps the detector scanning as the
// data becomes visible. This is distinct from TestScanMW_IncrementalAdvance,
// where each new point bumps WriteGeneration between Detect calls (the live
// path).
func TestScanMW_PreloadedReplayFiresAsDataTimeAdvances(t *testing.T) {
	d := testScanMWDetector()
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

func TestScanMW_MinRelativeChangeThresholdsAndBaselines(t *testing.T) {
	const oneGiB = float64(1 << 30)

	cases := []struct {
		name              string
		minRelativeChange *float64
		preVal            float64
		postVal           float64
		wantFire          bool
	}{
		{
			name:     "default 10pct suppresses 5pct increase",
			preVal:   100,
			postVal:  105,
			wantFire: false,
		},
		{
			name:     "default 10pct allows exact 10pct increase",
			preVal:   100,
			postVal:  110,
			wantFire: true,
		},
		{
			name:     "default 10pct allows 15pct increase",
			preVal:   100,
			postVal:  115,
			wantFire: true,
		},
		{
			name:     "default 10pct suppresses 5pct decrease",
			preVal:   100,
			postVal:  95,
			wantFire: false,
		},
		{
			name:     "default 10pct allows exact 10pct decrease",
			preVal:   100,
			postVal:  90,
			wantFire: true,
		},
		{
			name:     "default 10pct allows 15pct decrease",
			preVal:   100,
			postVal:  85,
			wantFire: true,
		},
		{
			name:     "GiB baseline suppresses 5pct increase",
			preVal:   oneGiB,
			postVal:  oneGiB * 1.05,
			wantFire: false,
		},
		{
			name:     "GiB baseline allows 10pct increase",
			preVal:   oneGiB,
			postVal:  oneGiB * 1.10,
			wantFire: true,
		},
		{
			name:     "negative baseline suppresses 5pct change",
			preVal:   -100,
			postVal:  -105,
			wantFire: false,
		},
		{
			name:     "negative baseline allows 10pct change",
			preVal:   -100,
			postVal:  -110,
			wantFire: true,
		},
		{
			name:     "zero baseline allows nonzero post median",
			preVal:   0,
			postVal:  5,
			wantFire: true,
		},
		{
			name:              "explicit zero disables relative change gate",
			minRelativeChange: func() *float64 { v := 0.0; return &v }(),
			preVal:            100,
			postVal:           105,
			wantFire:          true,
		},
		{
			name:              "custom 25pct threshold suppresses 20pct increase",
			minRelativeChange: func() *float64 { v := 0.25; return &v }(),
			preVal:            100,
			postVal:           120,
			wantFire:          false,
		},
		{
			name:              "custom 25pct threshold allows 25pct increase",
			minRelativeChange: func() *float64 { v := 0.25; return &v }(),
			preVal:            100,
			postVal:           125,
			wantFire:          true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := testScanMWDetector()
			if tc.minRelativeChange != nil {
				d.MinRelativeChange = *tc.minRelativeChange
			}
			storage := newTimeSeriesStorage()
			for i := 0; i < 20; i++ {
				storage.Add("ns", "metric", tc.preVal, int64(i+1), nil)
			}
			for i := 20; i < 40; i++ {
				storage.Add("ns", "metric", tc.postVal, int64(i+1), nil)
			}
			result := d.Detect(storage, 40)
			if tc.wantFire {
				require.NotEmpty(t, result.Anomalies)
			} else {
				assert.Empty(t, result.Anomalies)
			}
		})
	}
}

func TestScanMW_SuppressedSmallStepsRestartSegment(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	// Step 1: 100 -> 106 (+6%, statistically significant, below 10%).
	for i := 0; i < 20; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 106, int64(i+1), nil)
	}
	r1 := d.Detect(storage, 40)
	assert.Empty(t, r1.Anomalies, "6% change should be suppressed")
	require.Len(t, d.series, 1)
	for _, st := range d.series {
		assert.Equal(t, int64(20), st.segmentStartTime, "suppressed change should advance segmentStartTime")
	}

	// Step 2: 106 -> 112 (+5.66% vs 106, even though +12% vs initial 100).
	for i := 40; i < 60; i++ {
		storage.Add("ns", "metric", 112, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 60)
	assert.Empty(t, r2.Anomalies, "second small step should compare against 106 and stay suppressed")
	for _, st := range d.series {
		assert.Equal(t, int64(40), st.segmentStartTime, "second suppressed change should advance segmentStartTime again")
	}

	// Step 3: 112 -> 130 (+16.07% vs 112) should trigger.
	for i := 60; i < 80; i++ {
		storage.Add("ns", "metric", 130, int64(i+1), nil)
	}
	r3 := d.Detect(storage, 80)
	require.Len(t, r3.Anomalies, 1, "subsequent >=10% step from latest segment should fire")
	require.NotNil(t, r3.Anomalies[0].DebugInfo)
	assert.InDelta(t, 112.0, r3.Anomalies[0].DebugInfo.BaselineMedian, 1e-9)
	assert.InDelta(t, 130.0, r3.Anomalies[0].DebugInfo.CurrentValue, 1e-9)
}

func TestScanMW_CountAggregationMinRelativeChange(t *testing.T) {
	d := NewScanMWDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateCount}

	// 20 samples/bucket -> 21 samples/bucket (+5%) is suppressed.
	sSmall := newTimeSeriesStorage()
	for ts := int64(1); ts <= 20; ts++ {
		for j := 0; j < 20; j++ {
			sSmall.Add("ns", "metric", 1, ts, nil)
		}
	}
	for ts := int64(21); ts <= 40; ts++ {
		for j := 0; j < 21; j++ {
			sSmall.Add("ns", "metric", 1, ts, nil)
		}
	}
	assert.Empty(t, d.Detect(sSmall, 40).Anomalies, "5% count change should be suppressed")

	// 20 samples/bucket -> 22 samples/bucket (+10%) fires.
	d.Reset()
	sLarge := newTimeSeriesStorage()
	for ts := int64(1); ts <= 20; ts++ {
		for j := 0; j < 20; j++ {
			sLarge.Add("ns", "metric", 1, ts, nil)
		}
	}
	for ts := int64(21); ts <= 40; ts++ {
		for j := 0; j < 22; j++ {
			sLarge.Add("ns", "metric", 1, ts, nil)
		}
	}
	res := d.Detect(sLarge, 40)
	require.Len(t, res.Anomalies, 1, "10% count change should fire")
	assert.Equal(t, observer.AggregateCount, res.Anomalies[0].Source.Aggregate)
}

func TestScanMW_EnsureDefaultsNormalizesInvalidMinRelativeChange(t *testing.T) {
	for _, invalid := range []float64{-0.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		d := NewScanMWDetector()
		d.MinRelativeChange = invalid
		d.ensureDefaults()
		assert.InDelta(t, defaultScanMinRelativeChange, d.MinRelativeChange, 1e-12)
	}

	dZero := NewScanMWDetector()
	dZero.MinRelativeChange = 0
	dZero.ensureDefaults()
	assert.Zero(t, dZero.MinRelativeChange, "explicit 0 must remain 0 to disable the gate")
}
