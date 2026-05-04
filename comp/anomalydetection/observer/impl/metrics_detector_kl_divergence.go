// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// KL-divergence drift detector.
//
// Maintains two rolling windows per (series, aggregation): a reference window
// of the older WRef points and a test window of the more recent WTest points.
// On each Detect call, new points are streamed in via ForEachPoint and pushed
// through a FIFO that fills the test window first (warmup), then rolls oldest
// test entries into the reference window, and finally evicts oldest reference
// entries once both buffers are full.
//
// When both buffers are full, the values are discretised into B equal-width
// bins over the combined min/max range. Symmetric KL divergence is computed
// between the two empirical PMFs (with Laplace smoothing). A second
// effect-size gate — |testMedian - refMedian| / MAD — prevents tiny shifts
// from firing.
//
// On fire the reference is replaced with the post-shift distribution (so the
// detector locks onto the new regime) and a per-series refractory counts down
// the next WTest ingested points before another scan can fire.
//
// This detector is information-theoretic, complementing the parametric
// (ScanWelch), rank-based (ScanMW), and Bayesian (BOCPD) detectors already
// present in the catalog. Memory per (series, aggregation) is bounded by
// (WRef + WTest) float64 + 2*B int + scratch, and per-Detect cost is
// O(newPoints + WRef + WTest + B) per series-aggregation pair.

// klStateKey identifies per-series state by ref and aggregation.
type klStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// klSeriesState holds streaming state for one (series, aggregation) pair.
//
// The rolling window is a single buffer where the oldest refLen entries are
// the reference window and the next testLen entries are the test window.
// New points always land at the tail of the test window; once the test
// window is full the oldest test entry is rolled into the reference window;
// once both are full the oldest reference entry is dropped FIFO-style.
type klSeriesState struct {
	// cursor (mirrors scanwelch_pattern)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// rolling window: window[0:refLen] is refBuf, window[refLen:refLen+testLen]
	// is testBuf. cap(window) == WRef + WTest.
	window  []float64
	refLen  int
	testLen int

	// refractoryRemaining counts ingested points to skip before another scan
	// may fire. Set to WTest on fire and decremented per ingested point. Acts
	// as belt-and-suspenders alongside the natural buffer-warmup that already
	// blocks scans for at least WTest+WRef ingests post-fire.
	refractoryRemaining int

	// lastSeenTimestamp tracks the timestamp of the most recent ingested
	// point for use as the cursor advance and as the anomaly Timestamp.
	lastSeenTimestamp int64

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// scratch histogram buffers — alloc-once, len = NumBins.
	refHist  []int
	testHist []int

	// timestamps of the points currently held in window. Used only to
	// estimate the median sampling interval for the anomaly. cap matches
	// window. Strictly the same length as window.
	timestamps []int64
}

// KLDivergenceDetector fires on symmetric KL divergence between a reference
// window of past points and a test window of recent points.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields
// are exposed directly so callers (testbench, tests) can override defaults
// after construction; NewKLDivergenceDetector populates them.
type KLDivergenceDetector struct {
	// ReferenceWindow is the size of the reference (baseline) window in
	// points. Default 60.
	ReferenceWindow int
	// TestWindow is the size of the test (recent) window in points.
	// Default 30.
	TestWindow int
	// NumBins is the number of equal-width histogram bins. Default 16.
	NumBins int
	// DivergenceThreshold is the symmetric KL value above which the
	// divergence gate passes. Default 1.5.
	DivergenceThreshold float64
	// MinDeviationMAD is the minimum |testMedian - refMedian| / refMAD
	// for the deviation gate to pass. Default 3.0. Mirrors the gate used
	// by scanmw / scanwelch and prevents pure-variance shifts (where
	// medians barely move) from firing as drift.
	MinDeviationMAD float64
	// Aggregations to run detection on. Default [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[klStateKey]*klSeriesState

	// cache the discovered series list across Detect calls (mirrors the
	// scanwelch / scanmw / bocpd pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewKLDivergenceDetector creates a KLDivergence detector with default
// settings. The catalog factory calls this with no arguments; tunables can
// be overridden post-construction by setting the exported fields.
func NewKLDivergenceDetector() *KLDivergenceDetector {
	return &KLDivergenceDetector{
		ReferenceWindow:     60,
		TestWindow:          30,
		NumBins:             16,
		DivergenceThreshold: 1.5,
		MinDeviationMAD:     3.0,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[klStateKey]*klSeriesState),
	}
}

// Name implements observer.Detector.
func (d *KLDivergenceDetector) Name() string { return "kl_divergence" }

// Reset clears all per-series state for replay/reanalysis.
func (d *KLDivergenceDetector) Reset() {
	d.series = make(map[klStateKey]*klSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Each per-series entry holds two rolling buffers and scratch histograms,
// so without this teardown the map keeps growing with the cumulative
// series count even after storage shrinks. Called by the engine right
// after timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *KLDivergenceDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, klStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. It iterates the series catalogue,
// streams new points into per-series rolling windows, then runs the KL
// divergence scan when both windows are full and the replay/refractory
// gates are clear.
func (d *KLDivergenceDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := klStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip both ingest and scan when nothing
			// observable has changed and we have not accumulated at
			// least one test-window's worth of new points. Mirrors
			// scanwelch's gate (metrics_detector_scanwelch.go).
			if status.pointCount < state.lastProcessedCount+d.TestWindow && status.writeGeneration == state.lastWriteGen {
				continue
			}

			// Ingest all new points since the last cursor.
			pointsIngested := d.ingestNewPoints(storage, meta.Ref, agg, state, dataTime)

			// Decrement refractory by points actually ingested.
			if state.refractoryRemaining > 0 {
				state.refractoryRemaining -= pointsIngested
				if state.refractoryRemaining < 0 {
					state.refractoryRemaining = 0
				}
			}

			// Update cursor unconditionally so a quiet series doesn't
			// keep replaying through the gate next call.
			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration

			// Buffers must be full before a scan is meaningful.
			if state.refLen < d.ReferenceWindow || state.testLen < d.TestWindow {
				continue
			}
			// Refractory still active — skip scan but keep accumulating.
			if state.refractoryRemaining > 0 {
				continue
			}

			anomaly, fired := d.scanKL(state, agg)
			if !fired {
				continue
			}
			anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			allAnomalies = append(allAnomalies, anomaly)

			// Lock onto the post-shift distribution: new reference
			// becomes the current test window, test window cleared.
			d.rebaseAfterFire(state)
			state.refractoryRemaining = d.TestWindow
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *KLDivergenceDetector) newState() *klSeriesState {
	totalCap := d.ReferenceWindow + d.TestWindow
	return &klSeriesState{
		window:     make([]float64, 0, totalCap),
		timestamps: make([]int64, 0, totalCap),
		refHist:    make([]int, d.NumBins),
		testHist:   make([]int, d.NumBins),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime] into
// the rolling window. Returns the number of points actually ingested.
//
// Buffer invariants:
//   - len(window) == refLen + testLen and equals len(timestamps).
//   - cap(window) == ReferenceWindow + TestWindow.
//   - During warmup the test window fills first; once full, oldest test
//     entries roll into the reference window; once both are full, the FIFO
//     evicts the oldest reference entry per push.
func (d *KLDivergenceDetector) ingestNewPoints(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate, state *klSeriesState, dataTime int64) int {
	if dataTime <= state.lastProcessedTime {
		return 0
	}
	wRef := d.ReferenceWindow
	wTest := d.TestWindow
	totalCap := wRef + wTest

	ingested := 0
	storage.ForEachPoint(ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		if !state.seriesMetaCaptured {
			state.seriesNamespace = s.Namespace
			state.seriesName = s.Name
			// Copy tags — *Series is reused by the storage callback.
			if len(s.Tags) > 0 {
				tagsCopy := make([]string, len(s.Tags))
				copy(tagsCopy, s.Tags)
				state.seriesTags = tagsCopy
			}
			state.seriesMetaCaptured = true
		}

		// Push the value into the rolling FIFO.
		switch {
		case state.testLen < wTest:
			// Test window not yet full — append to its tail.
			state.window = append(state.window, p.Value)
			state.timestamps = append(state.timestamps, p.Timestamp)
			state.testLen++

		case state.refLen < wRef:
			// Test window full, reference still warming up. Roll
			// oldest test entry into the reference window (boundary
			// slides forward by 1, no copy needed) and append the
			// new point at the test tail.
			state.refLen++
			state.window = append(state.window, p.Value)
			state.timestamps = append(state.timestamps, p.Timestamp)

		default:
			// Both windows full — FIFO evict: drop oldest reference,
			// shift everything left, place new point at the tail.
			// len(window) == totalCap; cost is O(totalCap) per push,
			// which is fine for the default 60+30 = 90.
			copy(state.window, state.window[1:])
			state.window[totalCap-1] = p.Value
			copy(state.timestamps, state.timestamps[1:])
			state.timestamps[totalCap-1] = p.Timestamp
		}

		state.lastSeenTimestamp = p.Timestamp
		ingested++
	})

	return ingested
}

// rebaseAfterFire collapses the test window into the reference window and
// clears the test window so the new reference reflects the post-shift
// distribution. Subsequent scans will only fire on a further shift.
func (d *KLDivergenceDetector) rebaseAfterFire(state *klSeriesState) {
	wTest := state.testLen
	// Old test window lives at window[refLen:refLen+testLen]; copy it to
	// the front so it becomes the new reference.
	copy(state.window[0:wTest], state.window[state.refLen:state.refLen+wTest])
	copy(state.timestamps[0:wTest], state.timestamps[state.refLen:state.refLen+wTest])
	state.window = state.window[:wTest]
	state.timestamps = state.timestamps[:wTest]
	state.refLen = wTest
	state.testLen = 0
}

// scanKL runs the KL-divergence + deviation scan when both windows are
// full. Returns the populated Anomaly when both gates pass.
func (d *KLDivergenceDetector) scanKL(state *klSeriesState, agg observer.Aggregate) (observer.Anomaly, bool) {
	wRef := d.ReferenceWindow
	wTest := d.TestWindow
	bins := d.NumBins

	refBuf := state.window[0:wRef]
	testBuf := state.window[wRef : wRef+wTest]

	// 4a. Combined min/max — constant series → no divergence to score.
	minV := refBuf[0]
	maxV := refBuf[0]
	for _, v := range refBuf {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	for _, v := range testBuf {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if maxV == minV {
		return observer.Anomaly{}, false
	}

	// 4b. Discretise into equal-width bins.
	for i := range state.refHist {
		state.refHist[i] = 0
		state.testHist[i] = 0
	}
	width := (maxV - minV) / float64(bins)
	binOf := func(v float64) int {
		idx := int((v - minV) / width)
		if idx >= bins {
			idx = bins - 1
		}
		if idx < 0 {
			idx = 0
		}
		return idx
	}
	for _, v := range refBuf {
		state.refHist[binOf(v)]++
	}
	for _, v := range testBuf {
		state.testHist[binOf(v)]++
	}

	// 4c-d. Symmetric KL with Laplace smoothing.
	denomRef := float64(wRef + bins)
	denomTest := float64(wTest + bins)
	var divergence float64
	for b := 0; b < bins; b++ {
		pRef := float64(state.refHist[b]+1) / denomRef
		pTest := float64(state.testHist[b]+1) / denomTest
		divergence += pTest * math.Log(pTest/pRef)
		divergence += pRef * math.Log(pRef/pTest)
	}

	if divergence < d.DivergenceThreshold {
		return observer.Anomaly{}, false
	}

	// 5. Effect-size gate — mirror scanwelch's MAD-scaled deviation check.
	refMedian := detectorMedian(refBuf)
	testMedian := detectorMedian(testBuf)
	refMAD := detectorMAD(refBuf, refMedian, false)

	denom := refMAD
	if denom < 1e-10 {
		denom = math.Max(math.Abs(refMedian)*0.01, 1e-6)
	}
	devMAD := math.Abs(testMedian-refMedian) / denom
	if devMAD < d.MinDeviationMAD {
		return observer.Anomaly{}, false
	}

	// Build the anomaly. Mirrors scanwelch's anomaly construction
	// (metrics_detector_scanwelch.go) for shape consistency.
	score := divergence
	seriesName := state.seriesName + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type: observer.AnomalyTypeMetric,
		Source: observer.SeriesDescriptor{
			Namespace: state.seriesNamespace,
			Name:      state.seriesName,
			Tags:      state.seriesTags,
			Aggregate: agg,
		},
		DetectorName: d.Name(),
		Title:        "KLDivergence drift: " + seriesName,
		Description: fmt.Sprintf("%s drifted (refMedian=%.4f, testMedian=%.4f, KL=%.3f, %.1f MADs)",
			seriesName, refMedian, testMedian, divergence, devMAD),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.timestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: refMedian,
			BaselineMAD:    refMAD,
			CurrentValue:   testMedian,
			DeviationSigma: devMAD,
			Threshold:      d.DivergenceThreshold,
		},
	}

	return anomaly, true
}

// medianTimestampInterval is the timestamp-array equivalent of
// medianPointInterval (metrics_detector_util.go). The KL detector keeps a
// parallel []int64 of point timestamps to avoid building a []Point just to
// compute the sampling cadence.
func medianTimestampInterval(timestamps []int64) int64 {
	if len(timestamps) < 2 {
		return 0
	}
	intervals := make([]int64, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		intervals[i-1] = timestamps[i] - timestamps[i-1]
	}
	// Insertion sort: timestamps slice is small (~90) and already nearly
	// sorted in monotonic ingest order, so this is cheap and avoids the
	// sort.Slice allocation.
	for i := 1; i < len(intervals); i++ {
		v := intervals[i]
		j := i - 1
		for j >= 0 && intervals[j] > v {
			intervals[j+1] = intervals[j]
			j--
		}
		intervals[j+1] = v
	}
	return intervals[len(intervals)/2]
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
// Mirrors scanmw / scanwelch ensureDefaults so the detector behaves sanely
// even when constructed via reflective paths that bypass NewKLDivergenceDetector.
func (d *KLDivergenceDetector) ensureDefaults() {
	if d.ReferenceWindow <= 0 {
		d.ReferenceWindow = 60
	}
	if d.TestWindow <= 0 {
		d.TestWindow = 30
	}
	if d.NumBins <= 0 {
		d.NumBins = 16
	}
	if d.DivergenceThreshold <= 0 {
		d.DivergenceThreshold = 1.5
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = 3.0
	}
	if d.series == nil {
		d.series = make(map[klStateKey]*klSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
