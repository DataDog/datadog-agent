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
	return NewScanMWDetector(DefaultScanMWConfig())
}

func TestScanMW_Config(t *testing.T) {
	defaults := ScanMWConfig{
		MinSegment: 12, MinPoints: 30, MaxPoints: 120,
		SignificanceThreshold: 1e-8, MinEffectSize: 0.85, MinDeviationMAD: 3,
	}
	require.Equal(t, defaults, DefaultScanMWConfig())
	explicit := ScanMWConfig{
		MinSegment: 6, MinPoints: 20, MaxPoints: 40,
		SignificanceThreshold: 1e-6, MinEffectSize: 0.7, MinDeviationMAD: 2,
	}
	raisedMax := defaults
	raisedMax.MinPoints, raisedMax.MaxPoints = 130, 130
	for _, tc := range []struct {
		name string
		cfg  ScanMWConfig
		want ScanMWConfig
	}{
		{"zero", ScanMWConfig{}, defaults},
		{"negative", ScanMWConfig{
			MinSegment: -1, MinPoints: -1, MaxPoints: -1,
			SignificanceThreshold: -1, MinEffectSize: -1, MinDeviationMAD: -1,
		}, defaults},
		{"explicit", explicit, explicit},
		{"max below min", ScanMWConfig{MinPoints: 130, MaxPoints: 60}, raisedMax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewScanMWDetector(tc.cfg)
			require.Equal(t, tc.want, d.config)
			require.Equal(t, observer.DetectorPointWindow{
				MinPoints: tc.want.MinPoints, MaxPoints: tc.want.MaxPoints,
			}, d.DetectorPointWindow())
		})
	}
}

func TestScanMW_ConfigOwnership(t *testing.T) {
	cfg := DefaultScanMWConfig()

	d := NewScanMWDetector(cfg)
	want := DefaultScanMWConfig()

	cfg.MinPoints, cfg.MaxPoints = 1, 2
	require.Equal(t, want, d.config)
	d.Reset()
	require.Equal(t, want, d.config, "reset must preserve constructor configuration")
}

func TestScanMW_DefaultAggregations(t *testing.T) {
	s := newTimeSeriesStorage()
	add := func(key uint64, namespace string, metricType metrics.MetricType, kind observer.MetricKind) observer.SeriesRef {
		var ref observer.SeriesRef
		for i := 1; i <= 40; i++ {
			value := 50.0
			if i > 20 {
				value = 200
			}
			ref = s.AddWithKeyAndHostComposite(namespace, "metric", "", value, int64(i),
				tagset.CompositeTags{}, key, metricType).Ref
		}
		s.SetSeriesKind(ref, kind)
		return ref
	}
	stable := add(1, "work", metrics.GaugeType, observer.STABLE)
	linear := add(2, "work", metrics.GaugeType, observer.LINEAR)
	count := add(3, "work", metrics.CountType, observer.STABLE)
	untyped := add(4, "work", observer.UnknownType, observer.STABLE)
	add(5, observer.TelemetryNamespace, metrics.GaugeType, observer.STABLE)
	add(6, "work", metrics.GaugeType, observer.UNKNOWN)
	add(7, "work", metrics.GaugeType, observer.UNSET)

	unrestricted := NewScanMWDetector(DefaultScanMWConfig())
	result := unrestricted.Detect(s, 40, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 6, "unscoped ScanMW retains all workload kinds and types")
	require.Len(t, unrestricted.series, 12, "each workload series has average and count state")
	for _, anomaly := range result.Anomalies {
		require.Equal(t, observer.AggregateAverage, anomaly.SourceRef.Aggregate,
			"counts are constant despite the average step")
	}
	for _, ref := range []observer.SeriesRef{stable, linear, count, untyped} {
		require.Contains(t, unrestricted.series, scanmwStateKey{ref: ref, agg: observer.AggregateAverage})
		require.Contains(t, unrestricted.series, scanmwStateKey{ref: ref, agg: observer.AggregateCount})
	}
}

func TestScanMW_NotEnoughPoints(t *testing.T) {
	d := testScanMWDetector()
	storage := newTimeSeriesStorage()

	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100, int64(i+1), nil)
	}

	result := d.Detect(storage, 10, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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

	result := d.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})

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
	result := d.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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
	r1 := d.Detect(storage, 20, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
	assert.Empty(t, r1.Anomalies, "no anomaly in stable data")

	// Second advance: add shifted data — now has 40 points total.
	for i := 20; i < 40; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
	require.NotEmpty(t, r2.Anomalies, "should detect step change on second advance")

	// Third advance: no new data — should emit nothing.
	r3 := d.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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

	r1 := d.Detect(storage, 50, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// After fire, the segment start should have advanced.
	// Adding more data at 200 (same as post-change) should not re-fire
	// because the post-change segment is stable.
	for i := 50; i < 90; i++ {
		storage.Add("ns", "metric", 200, int64(i+1), nil)
	}
	r2 := d.Detect(storage, 90, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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

	r1 := d.Detect(storage, 50, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
	require.NotEmpty(t, r1.Anomalies, "should detect first changepoint")

	// Phase 2: 200 → 500
	for i := 50; i < 80; i++ {
		storage.Add("ns", "metric", 500, int64(i+1), nil)
	}

	r2 := d.Detect(storage, 80, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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
	r1 := d1.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})

	d2 := makeDetector()
	r2 := d2.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})

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
	d.Detect(storage, 40, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}})
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
		if len(d.Detect(storage, dataTime, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateAverage}}).Anomalies) > 0 {
			fired = true
		}
	}

	assert.True(t, fired, "preloaded replay should detect the step change as dataTime advances")
}
