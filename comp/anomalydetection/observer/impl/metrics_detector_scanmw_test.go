// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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

func TestScanMW_QuantityGaugeRoutingAndUnknownToGaugeTransition(t *testing.T) {
	cfg := DefaultScanMWConfig()
	cfg.Kind = observer.QuantityGauge
	d := NewScanMWDetectorWithConfig(cfg)

	require.Equal(t, "scanmw/QUANTITY_GAUGE", d.Name())
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage}, d.Aggregations)

	storage := newTimeSeriesStorage()
	for i := 0; i < 40; i++ {
		val := 50.0
		if i >= 20 {
			val = 200.0
		}
		storage.AddWithKeyAndHostComposite("ns", "gauge.metric", "host", val, int64(i+1), tagset.CompositeTags{}, 1, metrics.GaugeType)
		storage.AddWithKeyAndHostComposite("ns", "count.metric", "host", val, int64(i+1), tagset.CompositeTags{}, 2, metrics.CountType)
		storage.AddWithKeyAndHostComposite("ns", "late.gauge", "host", val, int64(i+1), tagset.CompositeTags{}, 3, observer.UnknownType)
	}

	res := d.Detect(storage, 40)
	require.Len(t, res.Anomalies, 1)
	assert.Equal(t, "gauge.metric", res.Anomalies[0].Source.Name)
	assert.Equal(t, observer.AggregateAverage, res.Anomalies[0].Source.Aggregate)
	assert.Equal(t, "scanmw", res.Anomalies[0].DetectorName)
	assert.Equal(t, observer.QuantityGauge, res.Anomalies[0].MetricKind)
	assert.Len(t, d.series, 1, "only the gauge series should allocate ScanMW state")

	// Transition late.gauge from UnknownType to GaugeType without changing SeriesGeneration;
	// cachedRefs must still pick it up on the next advance.
	storage.AddWithKeyAndHostComposite("ns", "late.gauge", "host", 200.0, 41, tagset.CompositeTags{}, 3, metrics.GaugeType)
	res2 := d.Detect(storage, 41)
	require.Len(t, res2.Anomalies, 1)
	assert.Equal(t, "late.gauge", res2.Anomalies[0].Source.Name)
	assert.Len(t, d.series, 2)
}

func TestScanMW_MinRelativeChangeGate(t *testing.T) {
	tests := []struct {
		name       string
		preVal     float64
		postVal    float64
		wantDetect bool
	}{
		{
			name:       "upward shift below 5% threshold is rejected",
			preVal:     1000,
			postVal:    1040, // +4% (statistically significant and >3 MADs, but <5%)
			wantDetect: false,
		},
		{
			name:       "downward shift below 5% threshold is rejected",
			preVal:     1000,
			postVal:    960, // -4%
			wantDetect: false,
		},
		{
			name:       "upward shift equal to 5% threshold passes",
			preVal:     1000,
			postVal:    1050, // +5%
			wantDetect: true,
		},
		{
			name:       "downward shift equal to 5% threshold passes",
			preVal:     1000,
			postVal:    950, // -5%
			wantDetect: true,
		},
		{
			name:       "upward shift above 5% threshold passes",
			preVal:     1000,
			postVal:    1100, // +10%
			wantDetect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultScanMWConfig()
			cfg.Kind = observer.QuantityGauge
			cfg.MinRelativeChange = 0.05
			d := NewScanMWDetectorWithConfig(cfg)

			storage := newTimeSeriesStorage()
			for i := 0; i < 20; i++ {
				storage.AddWithKeyAndHostComposite("ns", "memory.working_set", "host", tc.preVal, int64(i+1), tagset.CompositeTags{}, 1, metrics.GaugeType)
			}
			for i := 20; i < 40; i++ {
				storage.AddWithKeyAndHostComposite("ns", "memory.working_set", "host", tc.postVal, int64(i+1), tagset.CompositeTags{}, 1, metrics.GaugeType)
			}

			res := d.Detect(storage, 40)
			if tc.wantDetect {
				require.Len(t, res.Anomalies, 1)
				assert.Equal(t, tc.preVal, res.Anomalies[0].DebugInfo.BaselineMedian)
				assert.Equal(t, tc.postVal, res.Anomalies[0].DebugInfo.CurrentValue)
			} else {
				require.Empty(t, res.Anomalies)
				state := d.series[scanmwStateKey{ref: 0, agg: observer.AggregateAverage}]
				require.NotNil(t, state)
				assert.Zero(t, state.segmentStartTime, "rejected relative shift must not advance segmentStartTime")
			}
		})
	}
}
