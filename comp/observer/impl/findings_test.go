// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sync"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- H1: Storage methods missing RLock -- data race on s.series map ---
//
// Namespaces(), TimeBounds(), MaxTimestamp(), ListAllSeriesCompact(), and
// DroppedValueStats() iterate s.series without acquiring s.mu.
// Running with -race should catch this.

func TestFindingH1_StorageNamespacesRace(t *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine: continuously add data.
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	// Reader goroutine: call Namespaces() concurrently.
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.Namespaces()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageTimeBoundsRace(t *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_, _, _ = s.TimeBounds()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageMaxTimestampRace(t *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.MaxTimestamp()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageListAllSeriesCompactRace(t *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.ListAllSeriesCompact()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageDroppedValueStatsRace(t *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			// Add some NaN to trigger drop accounting writes
			s.Add("ns", "metric", math.NaN(), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_, _, _ = s.DroppedValueStats()
		}
	}()

	wg.Wait()
}

// --- H2: MinVariance=0 re-enables constant-series false positives ---
//
// ensureDefaults has no guard against MinVariance <= 0. Setting it to zero
// defeats the MinVariance floor added to fix constant-series false positives.

func TestFindingH2_MinVarianceZeroNotGuarded(t *testing.T) {
	// ensureDefaults has no guard against MinVariance <= 0.
	// After ensureDefaults runs, MinVariance should be >= some positive floor.
	// This test verifies the config guard is missing.
	d := &BOCPDDetector{
		MinVariance: 0,
	}
	d.ensureDefaults()
	assert.Greater(t, d.MinVariance, 0.0,
		"ensureDefaults should reject MinVariance=0 and set a positive floor, but it does not")

	dNeg := &BOCPDDetector{
		MinVariance: -1.0,
	}
	dNeg.ensureDefaults()
	assert.Greater(t, dNeg.MinVariance, 0.0,
		"ensureDefaults should reject MinVariance<0 and set a positive floor, but it does not")
}

// --- M1: Dedup key too coarse -- drops distinct anomalies on same series+detector+timestamp ---
//
// Two anomalies with the same SourceSeriesID+DetectorName+Timestamp but different
// Title collide. The second is silently dropped from rawAnomalies.

func TestFindingM1_DedupKeyTooCoarse(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike detected",
			Description:    "CPU spike",
			Timestamp:      100,
		},
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Trend change detected",
			Description:    "CPU trend shift",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test_detector", anomalies: anomalies},
		},
	})

	e.Advance(100)

	sv := e.StateView()
	raw := sv.Anomalies()
	assert.Len(t, raw, 2,
		"two anomalies with same seriesID+detector+timestamp but different titles should both survive dedup")
}

// --- M2: Log anomalies with empty SourceSeriesID collide on dedup key ---
//
// Log anomalies leave SourceSeriesID empty. Two log anomalies from the same
// detector in the same second share dedup key {"", detectorName, ts}.

func TestFindingM2_EmptySourceSeriesIDCollision(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{
			Type:           observerdef.AnomalyTypeLog,
			Source:         "logs",
			SourceSeriesID: "", // empty for log anomalies
			DetectorName:   "log_detector",
			Title:          "Error pattern A detected",
			Description:    "Pattern A",
			Timestamp:      100,
		},
		{
			Type:           observerdef.AnomalyTypeLog,
			Source:         "logs",
			SourceSeriesID: "", // empty for log anomalies
			DetectorName:   "log_detector",
			Title:          "Error pattern B detected",
			Description:    "Pattern B",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "log_detector", anomalies: anomalies},
		},
	})

	e.Advance(100)

	sv := e.StateView()
	raw := sv.Anomalies()
	assert.Len(t, raw, 2,
		"two log anomalies with empty SourceSeriesID but different content should both survive dedup")
}

// --- M3: Dedup asymmetry -- rawAnomalies deduped but events/correlator pipeline is not ---
//
// captureRawAnomaly deduplicates, but processAnomaly and allAnomalies run
// unconditionally. Events/reporters receive duplicates that the display store
// filtered out.

func TestFindingM3_DedupAsymmetry(t *testing.T) {
	// Two identical anomalies (same dedup key) -- one will be deduped from rawAnomalies.
	anomalies := []observerdef.Anomaly{
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike",
			Timestamp:      100,
		},
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test_detector", anomalies: anomalies},
		},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.Advance(100)

	sv := e.StateView()
	rawCount := len(sv.Anomalies())
	eventCount := len(sink.eventsOfKind(eventAnomalyCreated))

	// The bug: events will have 2 (no dedup) but rawAnomalies will have 1 (deduped).
	// If the system were consistent, these should match.
	assert.Equal(t, rawCount, eventCount,
		"anomalyCreated event count (%d) should match rawAnomalies count (%d); "+
			"mismatch means events/reporters see duplicates that rawAnomalies filtered out",
		eventCount, rawCount)
}

// --- M5: -math.MaxFloat64 not filtered in storage ---
//
// Positive math.MaxFloat64 is filtered, but negative is not. Two -MaxFloat64
// values in one bucket produce -Inf sum.

func TestFindingM5_NegativeMaxFloat64NotFiltered(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add two -MaxFloat64 values at the same timestamp.
	s.Add("ns", "metric", -math.MaxFloat64, 1000, nil)
	s.Add("ns", "metric", -math.MaxFloat64, 1000, nil)

	series := s.GetSeries("ns", "metric", nil, AggregateSum)
	if series == nil {
		// If both were filtered, the series would be nil, which is acceptable.
		// But if only one was stored...
		t.Skip("both values were filtered (series is nil), finding may be partially addressed")
		return
	}

	require.Len(t, series.Points, 1)
	sum := series.Points[0].Value
	assert.False(t, math.IsInf(sum, -1),
		"sum of two -MaxFloat64 values is -Inf (%v), storage should filter -MaxFloat64 like it filters +MaxFloat64", sum)
	assert.False(t, math.IsNaN(sum),
		"sum of two -MaxFloat64 values is NaN (%v), storage should filter -MaxFloat64", sum)
}

// --- M7: WarmupPoints=1 causes NaN variance via division by zero ---
//
// warmupM2 / (warmupCount - 1) with warmupCount=1 produces 0/0 = NaN.
// ensureDefaults guards <= 0 but not < 2.

func TestFindingM7_WarmupPointsOneCausesNaN(t *testing.T) {
	d := NewBOCPDDetector()
	d.WarmupPoints = 1
	d.Aggregations = []observerdef.Aggregate{observerdef.AggregateAverage}

	storage := newTimeSeriesStorage()

	// Feed a few points. With WarmupPoints=1, the first point triggers
	// initializeFromWarmup with warmupCount=1, causing 0/0 = NaN in
	// variance = warmupM2 / (warmupCount - 1).
	for i := 0; i < 10; i++ {
		storage.Add("ns", "metric", 100.0+float64(i), int64(i+1), nil)
	}

	result := d.Detect(storage, 10)

	// Check that no anomaly has NaN in its debug info.
	for _, a := range result.Anomalies {
		if a.DebugInfo != nil {
			assert.False(t, math.IsNaN(a.DebugInfo.BaselineMean),
				"NaN in BaselineMean due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.BaselineStddev),
				"NaN in BaselineStddev due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.CurrentValue),
				"NaN in CurrentValue due to WarmupPoints=1")
			assert.False(t, math.IsNaN(a.DebugInfo.DeviationSigma),
				"NaN in DeviationSigma due to WarmupPoints=1")
		}
	}

	// Also verify the detector's internal state is not corrupted with NaN.
	for key, state := range d.series {
		assert.False(t, math.IsNaN(state.baselineMean),
			"NaN baselineMean in series state %s", key)
		assert.False(t, math.IsNaN(state.baselineStddev),
			"NaN baselineStddev in series state %s", key)
		assert.False(t, math.IsNaN(state.obsVar),
			"NaN obsVar in series state %s", key)
		assert.False(t, math.IsNaN(state.priorMean),
			"NaN priorMean in series state %s", key)
		assert.False(t, math.IsNaN(state.priorPrecision),
			"NaN priorPrecision in series state %s", key)
		if state.initialized {
			for i, p := range state.runProbs {
				assert.False(t, math.IsNaN(p),
					"NaN in runProbs[%d] for series %s", i, key)
			}
		}
	}
}
