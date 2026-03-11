// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
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

// =============================================================================
// H3: Changepoint mass uses prior predictive instead of standard BOCPD recurrence
// =============================================================================
//
// The standard BOCPD (Adams & MacKay 2007) recurrence computes:
//   newRunProbs[0] = hazard * SUM_r(runProbs[r] * pred(x | r))
// The implementation uses:
//   newRunProbs[0] = hazard * predPrior
// where predPrior = gaussianPDF(x, priorMean, obsVar + 1/priorPrecision).
//
// This may be intentional (anchoring changepoint probability to the warmup
// baseline) or a deviation from the standard algorithm. Needs author input.

func TestFindingH3_CPProbUsesOnlyPriorPredictiveNotSumOverRunLengths(t *testing.T) {
	t.Skip("H3: requires author confirmation -- prior-only cpProb may be intentional design choice vs standard BOCPD. See FINDINGS.md.")

	// Test strategy: snapshot posterior state, call updatePosterior, then
	// independently compute both standard and prior-only formulas through
	// normalization and compare against the implementation's actual output.

	warmup := 120
	d := &BOCPDDetector{
		WarmupPoints:       warmup,
		Hazard:             0.05,
		CPThreshold:        0.6,
		ShortRunLength:     5,
		CPMassThreshold:    0.7,
		MaxRunLength:       200,
		PriorVarianceScale: 100.0,
		MinVariance:        1.0,
		RecoveryPoints:     10,
		Aggregations:       []observerdef.Aggregate{observerdef.AggregateAverage},
		series:             make(map[string]*bocpdSeriesState),
	}

	storage := newTimeSeriesStorage()
	for i := 0; i < warmup; i++ {
		storage.Add("ns", "metric", 10.0, int64(i+1), nil)
	}
	for i := warmup; i < warmup+150; i++ {
		storage.Add("ns", "metric", 12.0, int64(i+1), nil)
	}
	d.Detect(storage, int64(warmup+150))

	var state *bocpdSeriesState
	for _, s := range d.series {
		state = s
		break
	}
	require.NotNil(t, state)
	require.True(t, state.initialized)

	x := 14.0
	hazard := d.Hazard

	// Snapshot state before updatePosterior mutates it.
	snapRunProbs := make([]float64, len(state.runProbs))
	copy(snapRunProbs, state.runProbs)
	snapMeans := make([]float64, len(state.means))
	copy(snapMeans, state.means)
	snapPrecisions := make([]float64, len(state.precisions))
	copy(snapPrecisions, state.precisions)

	// Call the implementation.
	_, implCpProb, _ := d.updatePosterior(state, x)

	// Independently compute the standard BOCPD formula from the snapshot.
	newLen := len(snapRunProbs) + 1
	standardProbs := make([]float64, newLen)
	var cpMass float64
	for r := range snapRunProbs {
		pred := gaussianPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
		standardProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
		cpMass += snapRunProbs[r] * pred
	}
	standardProbs[0] = hazard * cpMass
	normalizeProbs(standardProbs)
	expectedCpProb := standardProbs[0]

	// Independently compute the prior-only formula from the snapshot.
	priorProbs := make([]float64, newLen)
	predPrior := gaussianPDF(x, state.priorMean, state.obsVar+1.0/state.priorPrecision)
	for r := range snapRunProbs {
		pred := gaussianPDF(x, snapMeans[r], state.obsVar+1.0/snapPrecisions[r])
		priorProbs[r+1] = snapRunProbs[r] * (1.0 - hazard) * pred
	}
	priorProbs[0] = hazard * predPrior
	normalizeProbs(priorProbs)
	priorOnlyCpProb := priorProbs[0]

	t.Logf("standard cpProb:   %e", expectedCpProb)
	t.Logf("prior-only cpProb: %e", priorOnlyCpProb)
	t.Logf("impl cpProb:       %e", implCpProb)

	// The two formulas should differ for this scenario.
	require.Greater(t, math.Abs(expectedCpProb-priorOnlyCpProb), 1e-6,
		"test setup: standard and prior-only formulas should differ")

	// Assert the implementation matches the standard formula.
	assert.InDelta(t, expectedCpProb, implCpProb, 1e-10,
		"implementation cpProb should match standard BOCPD recurrence, not prior-only")
}

// =============================================================================
// M4: Unbounded growth of uniqueAnomalySources and accumulatedCorrelations
// =============================================================================

func TestFindingM4_UnboundedGrowthOfUniqueAnomalySources(t *testing.T) {
	// Run the engine with many unique anomaly source names. The
	// uniqueAnomalySources map should be bounded, but the finding says it grows
	// without eviction.

	storage := newTimeSeriesStorage()

	// We need a detector that emits anomalies with unique source names.
	// Use a custom detector that generates a unique source on each Detect call.
	det := &dynamicAnomalyDetector{prefix: "metric_"}

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{det},
	})

	// Generate 1000 unique anomaly sources across many advance cycles.
	for i := 0; i < 1000; i++ {
		det.currentIndex = i
		e.Advance(int64(i + 1))
	}

	sourceCount := e.UniqueAnomalySourceCount()
	t.Logf("uniqueAnomalySources size after 1000 unique anomalies: %d", sourceCount)

	// The bug: all 1000 unique sources are retained forever.
	// A bounded implementation would cap or evict old entries.
	// Assert that the map is bounded (e.g., under 500).
	// This WILL FAIL because the map grows unbounded.
	assert.LessOrEqual(t, sourceCount, 500,
		"uniqueAnomalySources has %d entries after 1000 anomalies; "+
			"expected bounded growth but map grows without eviction", sourceCount)
}

// dynamicAnomalyDetector produces one anomaly per Detect with a unique source name
// based on currentIndex.
type dynamicAnomalyDetector struct {
	prefix       string
	currentIndex int
}

func (d *dynamicAnomalyDetector) Name() string { return "dynamic_anomaly_detector" }
func (d *dynamicAnomalyDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: []observerdef.Anomaly{
			{
				Source:         observerdef.MetricName(fmt.Sprintf("%s%d", d.prefix, d.currentIndex)),
				SourceSeriesID: observerdef.SeriesID(fmt.Sprintf("ns|%s%d|", d.prefix, d.currentIndex)),
				DetectorName:   d.Name(),
				Title:          fmt.Sprintf("anomaly_%d", d.currentIndex),
				Timestamp:      dataTime,
			},
		},
	}
}

func TestFindingM4_UnboundedGrowthOfAccumulatedCorrelations(t *testing.T) {
	storage := newTimeSeriesStorage()

	// A correlator that produces unique patterns on each Advance.
	corr := &dynamicCorrelator{prefix: "pattern_"}

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{corr},
	})

	for i := 0; i < 1000; i++ {
		corr.currentIndex = i
		e.Advance(int64(i + 1))
	}

	corrCount := len(e.AccumulatedCorrelations())
	t.Logf("accumulatedCorrelations size after 1000 unique patterns: %d", corrCount)

	assert.LessOrEqual(t, corrCount, 500,
		"accumulatedCorrelations has %d entries after 1000 unique patterns; "+
			"expected bounded growth but map grows without eviction", corrCount)
}

// dynamicCorrelator produces a unique ActiveCorrelation pattern on each Advance call.
type dynamicCorrelator struct {
	prefix       string
	currentIndex int
}

func (c *dynamicCorrelator) Name() string                         { return "dynamic_correlator" }
func (c *dynamicCorrelator) ProcessAnomaly(_ observerdef.Anomaly) {}
func (c *dynamicCorrelator) Advance(_ int64)                      {}
func (c *dynamicCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation {
	return []observerdef.ActiveCorrelation{
		{
			Pattern:     fmt.Sprintf("%s%d", c.prefix, c.currentIndex),
			Title:       fmt.Sprintf("Correlation %d", c.currentIndex),
			LastUpdated: int64(c.currentIndex),
		},
	}
}
func (c *dynamicCorrelator) Reset() { c.currentIndex = 0 }

// =============================================================================
// M6: BOCPD skips same-bucket value merges
// =============================================================================

func TestFindingM6_BOCPDSkipsSameBucketValueMerges(t *testing.T) {
	// When two values arrive at the same timestamp, storage merges them into
	// one bucket. But PointCountUpTo doesn't change (still 1 bucket), so
	// BOCPD's cache check skips the series on the second Detect call.
	//
	// Steps:
	// 1. Add first value at timestamp T, call Detect
	// 2. Add second value at same timestamp T (storage merges), call Detect again
	// 3. Assert the detector processed the updated merged value

	d := NewBOCPDDetector()
	d.WarmupPoints = 5
	d.Aggregations = []observerdef.Aggregate{observerdef.AggregateAverage}

	storage := newTimeSeriesStorage()

	// Build warmup data (timestamps 1-4)
	for i := 1; i <= 4; i++ {
		storage.Add("ns", "metric", 100.0, int64(i), nil)
	}

	// Add first value at timestamp 5 (this completes warmup)
	storage.Add("ns", "metric", 100.0, 5, nil)
	d.Detect(storage, 5)

	// Add a second value at timestamp 5 -- storage merges into the same bucket.
	// Average of {100, 200} = 150.
	storage.Add("ns", "metric", 200.0, 5, nil)

	// Verify storage actually merged: average should be 150, not 100.
	key := observerdef.SeriesKey{Namespace: "ns", Name: "metric"}
	series := storage.GetSeriesRange(key, 4, 5, observerdef.AggregateAverage)
	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 150.0, series.Points[0].Value,
		"storage should have merged the two values at timestamp 5")

	// Now Detect again. The detector should process the updated merged value.
	// But the bug is: PointCountUpTo still returns 5 (same as before), so the
	// detector's lastProcessedCount check causes it to skip this series.

	// To detect whether the detector re-processed, we check its internal state.
	// After processing x=100 at t=5, the posterior was updated with x=100.
	// After re-processing x=150 (merged average), it should update with x=150.
	// But if skipped, the posterior still reflects x=100.

	// Snapshot the writeGeneration the detector saw after first Detect.
	var stateBefore *bocpdSeriesState
	for _, s := range d.series {
		stateBefore = s
		break
	}
	require.NotNil(t, stateBefore)
	genBefore := stateBefore.lastWriteGen

	d.Detect(storage, 5)

	var stateAfter *bocpdSeriesState
	for _, s := range d.series {
		stateAfter = s
		break
	}
	genAfter := stateAfter.lastWriteGen

	pointCount := storage.PointCountUpTo(key, 5)
	t.Logf("genBefore=%d, genAfter=%d, PointCountUpTo=%d, writeGen=%d",
		genBefore, genAfter, pointCount, storage.WriteGeneration())

	// The detector should notice the merge via writeGeneration even though
	// PointCountUpTo didn't change. If it re-processed, genAfter > genBefore.
	assert.Greater(t, genAfter, genBefore,
		"detector should re-process when a same-bucket merge changes the value; "+
			"lastWriteGen should advance but didn't (%d == %d)", genBefore, genAfter)
}

// =============================================================================
// M8: shortRunMass includes cpProb (runProbs[0]) making triggers non-independent
// =============================================================================

func TestFindingM8_ShortRunMassExcludesCPProb(t *testing.T) {
	// shortRunLengthMass should sum runProbs[1] through runProbs[ShortRunLength],
	// excluding runProbs[0] (cpProb). The two trigger conditions (peak cpProb
	// vs short-run mass) should be independent.

	runProbs := make([]float64, 20)
	runProbs[0] = 0.55 // cpProb
	runProbs[1] = 0.05
	runProbs[2] = 0.04
	runProbs[3] = 0.04
	runProbs[4] = 0.04
	runProbs[5] = 0.04
	remaining := 1.0 - (0.55 + 0.05 + 0.04*4)
	for i := 6; i < len(runProbs); i++ {
		runProbs[i] = remaining / float64(len(runProbs)-6)
	}

	shortRunLength := 5

	mass := shortRunLengthMass(runProbs, shortRunLength)

	// Expected: sum of runProbs[1..5] = 0.05 + 0.04*4 = 0.21
	expectedMass := 0.05 + 0.04*4
	t.Logf("shortRunMass = %.4f, expected = %.4f (excluding cpProb=%.2f)", mass, expectedMass, runProbs[0])

	assert.InDelta(t, expectedMass, mass, 0.001,
		"shortRunLengthMass should exclude runProbs[0] (cpProb); "+
			"got %.4f but expected %.4f", mass, expectedMass)

	// Confirm this mass is below CPMassThreshold -- proving the short-run
	// trigger would NOT fire on its own without cpProb inflating it.
	assert.Less(t, mass, 0.7,
		"short-run mass (%.4f) without cpProb should be below CPMassThreshold (0.7)", mass)
}

// =============================================================================
// M9: SetDetectors/SetCorrelators have no synchronization
// =============================================================================

func TestFindingM9_SetDetectorsRace(t *testing.T) {
	// SetDetectors replaces engine slices without a lock.
	// Running concurrently with Advance should trigger the race detector.

	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{&mockDetector{name: "initial"}},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: Advance in a loop
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	// Goroutine B: SetDetectors in a loop
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetDetectors([]observerdef.Detector{
				&mockDetector{name: fmt.Sprintf("det_%d", i)},
			})
		}
	}()

	wg.Wait()
}

func TestFindingM9_SetCorrelatorsRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "initial"}},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	wg.Wait()
}

// =============================================================================
// M10: Reset() has no lock
// =============================================================================

func TestFindingM10_ResetRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	det := &resettableDetector{name: "det"}
	corr := &resettableCorrelator{name: "corr"}

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observerdef.Detector{det},
		correlators: []observerdef.Correlator{corr},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Reset()
		}
	}()

	wg.Wait()
}

// =============================================================================
// M11: StateView reads unprotected engine slices
// =============================================================================

func TestFindingM11_StateViewListDetectorsRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{&mockDetector{name: "det1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetDetectors([]observerdef.Detector{
				&mockDetector{name: fmt.Sprintf("det_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ListDetectors()
		}
	}()

	wg.Wait()
}

func TestFindingM11_StateViewListCorrelatorsRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "corr1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ListCorrelators()
		}
	}()

	wg.Wait()
}

func TestFindingM11_StateViewActiveCorrelationsRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "corr1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ActiveCorrelations()
		}
	}()

	wg.Wait()
}

// =============================================================================
// M12: Log-only timestamps skipped in replay advance sequence
// =============================================================================

// noopLogExtractor is a LogMetricsExtractor that returns no metrics.
// This simulates a log at a timestamp that produces no virtual metrics.
type noopLogExtractor struct{}

func (e *noopLogExtractor) Name() string { return "noop_extractor" }
func (e *noopLogExtractor) ProcessLog(_ observerdef.LogView) []observerdef.MetricOutput {
	return nil
}

func TestFindingM12_LogOnlyTimestampsSkippedInReplay(t *testing.T) {
	// DataTimestamps() only returns metric timestamps. A log at timestamp 103
	// that produces no virtual metrics won't appear, so replay skips it.
	//
	// In live-style ingestion, every IngestLog call triggers onObservation,
	// generating advance requests for that timestamp. In replay, only
	// DataTimestamps() are iterated.

	storage := newTimeSeriesStorage()

	extractor := &noopLogExtractor{}

	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	// --- Live-style ingestion ---
	liveSink := &collectingSink{}
	e.Subscribe(liveSink)

	// Ingest metrics at 100, 101, 102, 105
	for _, ts := range []int64{100, 101, 102, 105} {
		requests := e.IngestMetric("ns", &metricObs{
			name:      "cpu",
			value:     1.0,
			timestamp: ts,
		})
		for _, req := range requests {
			e.advanceWithReason(req.upToSec, req.reason)
		}
	}

	// Ingest log at 103 (no virtual metrics produced)
	logRequests := e.IngestLog("ns", &logObs{
		content:     []byte("error happened"),
		status:      "error",
		timestampMs: 103000, // 103 seconds in millis
	})
	for _, req := range logRequests {
		e.advanceWithReason(req.upToSec, req.reason)
	}

	// Flush remaining
	endRequests := e.scheduler.onReplayEnd(e.schedulerState())
	for _, req := range endRequests {
		e.advanceWithReason(req.upToSec, req.reason)
	}

	liveAdvances := liveSink.eventsOfKind(eventAdvanceCompleted)
	var liveTimestamps []int64
	for _, evt := range liveAdvances {
		liveTimestamps = append(liveTimestamps, evt.advanceCompleted.advancedToSec)
	}

	// --- Now reset and do replay ---
	unsub := e.Subscribe(&collectingSink{}) // dummy to capture unsub
	unsub()

	e.resetFull()

	replaySink := &collectingSink{}
	e.Subscribe(replaySink)

	e.ReplayStoredData()

	replayAdvances := replaySink.eventsOfKind(eventAdvanceCompleted)
	var replayTimestamps []int64
	for _, evt := range replayAdvances {
		replayTimestamps = append(replayTimestamps, evt.advanceCompleted.advancedToSec)
	}

	t.Logf("live advance timestamps:   %v", liveTimestamps)
	t.Logf("replay advance timestamps: %v", replayTimestamps)

	// The bug: replay's DataTimestamps() only has metric timestamps [100,101,102,105],
	// missing the log's timestamp 103. So replay doesn't advance through 103.
	// In live mode, the log at 103 DID trigger onObservation and potentially an advance.
	//
	// Check that DataTimestamps doesn't include 103.
	dataTS := storage.DataTimestamps()
	has103 := false
	for _, ts := range dataTS {
		if ts == 103 {
			has103 = true
			break
		}
	}
	assert.True(t, has103,
		"DataTimestamps() should include timestamp 103 from the log observation, "+
			"but it only returns metric timestamps: %v", dataTS)
}
