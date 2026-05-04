// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Two-sample Kolmogorov-Smirnov drift detector.
//
// Maintains two rolling windows per (series, aggregation): a reference window
// of the older WRef points and a test window of the more recent WTest points.
// On each Detect call, new points are streamed in via ForEachPoint and pushed
// through a FIFO that fills the test window first (warmup), then rolls oldest
// test entries into the reference window, and finally evicts oldest reference
// entries once both buffers are full.
//
// When both buffers are full the detector sorts each window and walks the
// merged ordering once, tracking the maximum gap between the two empirical
// CDFs — this is the two-sample KS statistic D = sup |F_ref - F_test|, which
// lies in [0, 1]. A second effect-size gate — |testMedian - refMedian| / MAD
// — prevents tiny shifts from firing, mirroring the gate used by the parametric
// (ScanWelch) and information-theoretic (KLDivergence) detectors.
//
// Why KS is complementary to KLDivergence: KL bins values into a fixed grid
// and is dominated by populated bin counts; KS operates on the empirical CDF
// directly, has no binning artefact, and tends to be more sensitive to changes
// in distribution shape and tail behaviour while being less sensitive to
// median shifts than KL. See Kolmogorov 1933 / Smirnov 1948 for the statistic
// and Hodges 1958 for the two-sample variant.
//
// On fire the reference is replaced with the post-shift distribution (so the
// detector locks onto the new regime) and a per-series refractory counts down
// the next WTest ingested points before another scan can fire.
//
// Memory per (series, aggregation) is bounded by (WRef + WTest) float64 +
// (WRef + WTest) int64 + transient sort scratch. Per-Detect cost is
// O((WRef + WTest) log(WRef + WTest)) for the sort plus O(WRef + WTest) for
// the merge-walk and MAD recompute. For the defaults (60 + 30 = 90) this is
// dominated by the per-detect MAD recompute already done by peer detectors.

// ksStateKey identifies per-series state by ref and aggregation.
type ksStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// ksSeriesState holds streaming state for one (series, aggregation) pair.
//
// Mirrors klSeriesState exactly except that no histogram scratch is held —
// the KS statistic is computed on sorted slices allocated transiently per
// scan, which is cheaper than the histogram cost it replaces.
type ksSeriesState struct {
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
	// may fire. Set to WTest on fire and decremented per ingested point.
	refractoryRemaining int

	// lastSeenTimestamp tracks the timestamp of the most recent ingested
	// point for use as the cursor advance and as the anomaly Timestamp.
	lastSeenTimestamp int64

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// timestamps of the points currently held in window. Used only to
	// estimate the median sampling interval for the anomaly. Strictly the
	// same length as window.
	timestamps []int64
}

// KSDriftDetector fires on the two-sample Kolmogorov-Smirnov statistic between
// a reference window of past points and a test window of recent points,
// gated by a MAD-based effect-size check.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields are
// exposed directly so callers (testbench, tests) can override defaults after
// construction; NewKSDriftDetector populates them.
type KSDriftDetector struct {
	// ReferenceWindow is the size of the reference (baseline) window in
	// points. Default 60.
	ReferenceWindow int
	// TestWindow is the size of the test (recent) window in points.
	// Default 30.
	TestWindow int
	// KSThreshold is the two-sample KS statistic value (max |F_ref - F_test|)
	// above which the divergence gate passes. Default 0.55. The asymptotic
	// critical value at WRef=60, WTest=30, alpha=1e-4 is roughly 0.34;
	// 0.55 is well above that to keep the false-positive rate low under
	// realistic noise — same conservative bias as KLDivergence's 1.5.
	KSThreshold float64
	// MinDeviationMAD is the minimum |testMedian - refMedian| / refMAD for
	// the deviation gate to pass. Default 3.0. Mirrors the gate used by
	// scanwelch / scanmw / kl_divergence and prevents pure-shape shifts
	// (where medians barely move) from firing as drift.
	MinDeviationMAD float64
	// Aggregations to run detection on. Default [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[ksStateKey]*ksSeriesState

	// cache the discovered series list across Detect calls (mirrors the
	// scanwelch / scanmw / bocpd / kl_divergence pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewKSDriftDetector creates a KSDrift detector with default settings.
// The catalog factory calls this with no arguments; tunables can be overridden
// post-construction by setting the exported fields.
func NewKSDriftDetector() *KSDriftDetector {
	return &KSDriftDetector{
		ReferenceWindow: 60,
		TestWindow:      30,
		KSThreshold:     0.55,
		MinDeviationMAD: 3.0,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[ksStateKey]*ksSeriesState),
	}
}

// Name implements observer.Detector.
func (d *KSDriftDetector) Name() string { return "ks_drift" }

// Reset clears all per-series state for replay/reanalysis.
func (d *KSDriftDetector) Reset() {
	d.series = make(map[ksStateKey]*ksSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Each per-series entry holds a rolling buffer and timestamp slice, so without
// this teardown the map keeps growing with the cumulative series count even
// after storage shrinks. Called by the engine right after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *KSDriftDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, ksStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. It iterates the series catalogue,
// streams new points into per-series rolling windows, then runs the KS scan
// when both windows are full and the replay/refractory gates are clear.
func (d *KSDriftDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := ksStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip both ingest and scan when nothing
			// observable has changed and we have not accumulated at
			// least one test-window's worth of new points. Mirrors
			// scanwelch / kl_divergence.
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

			anomaly, fired := d.scanKS(state, agg)
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

// newState allocates a per-series state with appropriately sized buffers.
// Splitting allocation here keeps Detect's hot path branch-free.
func (d *KSDriftDetector) newState() *ksSeriesState {
	totalCap := d.ReferenceWindow + d.TestWindow
	return &ksSeriesState{
		window:     make([]float64, 0, totalCap),
		timestamps: make([]int64, 0, totalCap),
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
func (d *KSDriftDetector) ingestNewPoints(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate, state *ksSeriesState, dataTime int64) int {
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
func (d *KSDriftDetector) rebaseAfterFire(state *ksSeriesState) {
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

// scanKS runs the KS + deviation scan when both windows are full. Returns
// the populated Anomaly when both gates pass.
func (d *KSDriftDetector) scanKS(state *ksSeriesState, agg observer.Aggregate) (observer.Anomaly, bool) {
	wRef := d.ReferenceWindow
	wTest := d.TestWindow

	refBuf := state.window[0:wRef]
	testBuf := state.window[wRef : wRef+wTest]

	// Sort transient copies of each window. The originals stay in arrival
	// order so the rolling FIFO and timestamp parallel slice remain coherent.
	sortedRef := make([]float64, wRef)
	copy(sortedRef, refBuf)
	sort.Float64s(sortedRef)

	sortedTest := make([]float64, wTest)
	copy(sortedTest, testBuf)
	sort.Float64s(sortedTest)

	// Two-sample KS statistic via merged-walk: D = sup |F_ref - F_test|.
	// Advance whichever side has the smaller (or tied-smaller) value next;
	// after each advance, recompute the empirical-CDF gap. The largest gap
	// over the walk is D ∈ [0, 1].
	var d2 float64
	{
		i, j := 0, 0
		nR, nT := wRef, wTest
		fnR := float64(nR)
		fnT := float64(nT)
		for i < nR && j < nT {
			if sortedRef[i] <= sortedTest[j] {
				i++
			} else {
				j++
			}
			cdfR := float64(i) / fnR
			cdfT := float64(j) / fnT
			gap := math.Abs(cdfR - cdfT)
			if gap > d2 {
				d2 = gap
			}
		}
		// Tail: once one side is exhausted its CDF is pinned at 1; the
		// remaining gap is monotonically shrinking, so the supremum is
		// already captured by the final iteration above. No extra work.
	}

	if d2 < d.KSThreshold {
		return observer.Anomaly{}, false
	}

	// Effect-size gate — mirror scanwelch / kl_divergence MAD-scaled deviation.
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

	// Build the anomaly. Shape mirrors kl_divergence.scanKL for consistency.
	// Score is the KS statistic itself — already a natural confidence in [0,1].
	score := d2
	currentValue := testBuf[len(testBuf)-1]
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
		Title:        "KS drift: " + seriesName,
		Description: fmt.Sprintf("%s drifted (refMedian=%.4f, testMedian=%.4f, KS=%.3f, %.1f MADs)",
			seriesName, refMedian, testMedian, d2, devMAD),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.timestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: refMedian,
			BaselineMAD:    refMAD,
			CurrentValue:   currentValue,
			DeviationSigma: devMAD,
			Threshold:      d.KSThreshold,
		},
	}

	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
// Mirrors scanmw / scanwelch / kl_divergence ensureDefaults so the detector
// behaves sanely even when constructed via reflective paths that bypass
// NewKSDriftDetector.
func (d *KSDriftDetector) ensureDefaults() {
	if d.ReferenceWindow <= 0 {
		d.ReferenceWindow = 60
	}
	if d.TestWindow <= 0 {
		d.TestWindow = 30
	}
	if d.KSThreshold <= 0 {
		d.KSThreshold = 0.55
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = 3.0
	}
	if d.series == nil {
		d.series = make(map[ksStateKey]*ksSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
