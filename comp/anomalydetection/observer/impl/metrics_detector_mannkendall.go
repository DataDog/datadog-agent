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

// mkStateKey identifies per-series state by ref and aggregation.
type mkStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// mkSeriesState holds per-series streaming state for the Mann-Kendall detector.
//
// The detector keeps a sliding window of the last WindowSize points per series
// in a ring buffer so each ingestion adds O(1) work; scoring is only done
// when new points arrive AND the window is full.
type mkSeriesState struct {
	// Cursor (same pattern as ScanMW: metrics_detector_scanmw.go:27-29).
	lastProcessedCount int
	lastWriteGen       int64
	// lastProcessedTime is the highest point timestamp consumed so far. Used
	// as the exclusive lower bound for ForEachPoint so each point is appended
	// to the window exactly once across replays/incremental advances.
	lastProcessedTime int64

	// Sliding window of recent values + timestamps in chronological order
	// when read via windowSnapshot().
	window []observer.Point // ring buffer, cap = WindowSize when full
	head   int              // index of oldest entry once full
	count  int              // logical entries (capped at WindowSize)

	// cooldownLeft counts down on each new point after a fire so we don't
	// repeatedly emit on the same drift segment.
	cooldownLeft int
	lastFireTime int64
}

// MannKendallDetector implements the Mann-Kendall non-parametric trend test
// (Mann 1945, Kendall 1975 §1.4) as a streaming detector for slow-drift
// anomalies (memory leaks, queue buildups, slow disk fill, FD exhaustion).
//
// On each new point it slides a window of the last WindowSize values forward.
// Once full, it computes the tie-corrected Mann-Kendall S statistic
// (Hirsch & Slack 1984) and standardizes it. A high |Z| alone over-fires on
// long-but-tiny drifts, so the detector additionally requires the Theil-Sen
// median slope, scaled to MAD-units across the window, to clear MinSlopeMAD.
// This dual gate mirrors the p-value + effect-size + MAD triple gate used by
// ScanMW (metrics_detector_scanmw.go).
//
// Implements observer.Detector with explicit cursoring (modeled after ScanMW)
// and observer.SeriesRemover for compatibility with the catalog teardown
// contract validated by TestDefaultCatalog_DetectorTeardownContract.
type MannKendallDetector struct {
	// WindowSize is the number of recent points used for each MK score.
	// Default: 60.
	WindowSize int
	// MinPoints is the minimum window fill before scoring runs.
	// Default: WindowSize.
	MinPoints int
	// ZThreshold is the |Z| above which the MK trend is considered significant.
	// Default: 5.0 — very strict (Bonferroni-ish for many concurrent series).
	ZThreshold float64
	// MinSlopeMAD is the minimum |slope|·WindowSeconds / MAD ratio required.
	// Acts as an effect-size floor preventing fires on long-but-tiny drifts.
	// Default: 3.0.
	MinSlopeMAD float64
	// CooldownPoints is the number of points to skip after a fire before
	// re-evaluating the window. Default: 30.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[mkStateKey]*mkSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewMannKendallDetector returns a Mann-Kendall detector with default settings.
func NewMannKendallDetector() *MannKendallDetector {
	return &MannKendallDetector{
		WindowSize:     60,
		MinPoints:      60,
		ZThreshold:     5.0,
		MinSlopeMAD:    3.0,
		CooldownPoints: 30,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[mkStateKey]*mkSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *MannKendallDetector) Name() string {
	return "mannkendall"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *MannKendallDetector) Reset() {
	d.series = make(map[mkStateKey]*mkSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series window state for refs that storage has freed.
// Called by the engine right after timeSeriesStorage.RemoveSeriesByKeys returns
// the freed refs. Without this hook the per-series map keeps growing with the
// cumulative series count even after storage shrinks (see ScanMW for the same
// pattern).
func (d *MannKendallDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, mkStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration pattern mirrors ScanMW
// (metrics_detector_scanmw.go:135-205) — series cache, bulk status,
// replay-skip when nothing has changed, ForEachPoint to walk only the new
// points since the last call.
func (d *MannKendallDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	// Bulk-fetch point counts and write generations in a single lock acquisition.
	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := mkStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &mkSeriesState{}
				d.series[sk] = state
			}

			// No new data and no in-place writes: skip entirely. This is the
			// equivalent of ScanMW's replay-skip — for MK there is no MinSegment
			// gap to amortize against, but we still avoid the ForEachPoint walk
			// when nothing changed.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			// Walk only the points strictly after the highest one we've already
			// appended. ForEachPoint's start parameter is exclusive, so passing
			// state.lastProcessedTime gives us exactly the new points.
			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendWindow(state, p)

				// Decrement cooldown per ingested point so it expires regardless
				// of whether we score this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}

				if state.count >= d.MinPoints && state.cooldownLeft == 0 {
					if anomaly, fired := d.scoreMK(state, seriesMeta, agg, p.Timestamp); fired {
						anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
						allAnomalies = append(allAnomalies, anomaly)
						state.cooldownLeft = d.CooldownPoints
						state.lastFireTime = p.Timestamp
					}
				}

				state.lastProcessedTime = p.Timestamp
			})

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// appendWindow appends a point to the per-series ring buffer in chronological
// order. While the window is filling, it grows; once full, the oldest entry
// at state.head is overwritten and head advances modulo WindowSize.
func (d *MannKendallDetector) appendWindow(state *mkSeriesState, p observer.Point) {
	if state.count < d.WindowSize {
		state.window = append(state.window, p)
		state.count++
		// head stays at 0 until the buffer is full.
		return
	}
	state.window[state.head] = p
	state.head = (state.head + 1) % d.WindowSize
}

// windowSnapshot returns the current window contents in chronological order.
// Allocates a small []Point of size state.count; called only on scoring ticks
// and gated by ZThreshold/MinSlopeMAD so the allocation is rare in steady
// state for a healthy series.
func (d *MannKendallDetector) windowSnapshot(state *mkSeriesState) []observer.Point {
	if state.count < d.WindowSize {
		// Buffer hasn't wrapped yet — entries are already in order.
		out := make([]observer.Point, state.count)
		copy(out, state.window[:state.count])
		return out
	}
	out := make([]observer.Point, d.WindowSize)
	for i := 0; i < d.WindowSize; i++ {
		out[i] = state.window[(state.head+i)%d.WindowSize]
	}
	return out
}

// scoreMK runs the Mann-Kendall + Theil-Sen dual gate on the current window.
// Returns (anomaly, fired). Pure function over the snapshot.
func (d *MannKendallDetector) scoreMK(state *mkSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	points := d.windowSnapshot(state)
	n := len(points)
	if n < 2 {
		return observer.Anomaly{}, false
	}

	values := make([]float64, n)
	for i, p := range points {
		values[i] = p.Value
	}

	// Mann-Kendall S statistic via O(n^2) pair loop. At n=60 this is 1770
	// sign comparisons; correctness is easier to audit than Knight's
	// merge-sort variant.
	S := mannKendallS(values)

	// Tie-corrected variance (Hirsch & Slack 1984).
	varS := mannKendallVariance(values)
	if varS <= 0 {
		// All-tied window (e.g. constant series): no trend, nothing to report.
		return observer.Anomaly{}, false
	}

	// Z with continuity correction: (S - sign(S)) / sqrt(Var(S)).
	sgn := 0.0
	switch {
	case S > 0:
		sgn = 1
	case S < 0:
		sgn = -1
	}
	z := (float64(S) - sgn) / math.Sqrt(varS)
	zAbs := math.Abs(z)
	if zAbs < d.ZThreshold {
		return observer.Anomaly{}, false
	}

	// Robust slope (Theil-Sen median of pairwise (dy/dt)).
	slope := theilSenSlope(points)

	// MAD of values, scaled to a sigma-equivalent so the slope·window/MAD
	// gate behaves like a sigma effect-size threshold (matches the rest of
	// the catalog — ScanMW uses raw MAD for its 3-MAD gate, but MK's slope
	// is already in value/time units, so we want a scale-stable denominator).
	med := detectorMedian(values)
	mad := detectorMAD(values, med, false)
	denom := mad
	if denom < 1e-10 {
		denom = math.Max(math.Abs(med)*0.01, 1e-6)
	}

	windowSeconds := float64(points[n-1].Timestamp - points[0].Timestamp)
	if windowSeconds <= 0 {
		return observer.Anomaly{}, false
	}
	slopeMAD := math.Abs(slope) * windowSeconds / denom
	if slopeMAD < d.MinSlopeMAD {
		return observer.Anomaly{}, false
	}

	direction := "increasing"
	if slope < 0 {
		direction = "decreasing"
	}

	// Score is |Z|, clamped to a sane visual range. Many concurrent ties
	// or numerical edges can produce very large Z values; cap them so the
	// downstream UI/correlator scoring isn't dominated by a single series.
	score := zAbs
	if score > 50 {
		score = 50
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Mann-Kendall trend: " + seriesName,
		Description: fmt.Sprintf("%s drift (Z=%.2f, slope=%.4f, MAD=%.4f, n=%d)",
			direction, z, slope, mad, n),
		Timestamp:           dataTime,
		Score:               &score,
		SamplingIntervalSec: medianPointInterval(points),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: med,
			BaselineMAD:    mad,
			CurrentValue:   values[n-1],
			DeviationSigma: slopeMAD,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *MannKendallDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 60
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		// Scoring requires the window to be filled, so cap MinPoints.
		d.MinPoints = d.WindowSize
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = 5.0
	}
	if d.MinSlopeMAD <= 0 {
		d.MinSlopeMAD = 3.0
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = 30
	}
	if d.series == nil {
		d.series = make(map[mkStateKey]*mkSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}

// mannKendallS returns the Kendall S statistic: the sum over all pairs i<j of
// sign(values[j] - values[i]). O(n^2) — fine at n=60 (1770 ops).
func mannKendallS(values []float64) int {
	n := len(values)
	S := 0
	for i := 0; i < n; i++ {
		vi := values[i]
		for j := i + 1; j < n; j++ {
			d := values[j] - vi
			if d > 0 {
				S++
			} else if d < 0 {
				S--
			}
		}
	}
	return S
}

// mannKendallVariance returns Var(S) under the null hypothesis with the
// Hirsch & Slack 1984 tie correction:
//
//	Var(S) = [n(n-1)(2n+5) - sum_p t_p (t_p-1) (2 t_p+5)] / 18
//
// where t_p is the size of the p-th tie group. Returns <=0 only for
// pathological inputs (n<2 or every value tied), in which case the caller
// must skip scoring.
func mannKendallVariance(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	tieSum := 0.0
	i := 0
	for i < n {
		j := i
		for j < n && sorted[j] == sorted[i] {
			j++
		}
		t := float64(j - i)
		if t > 1 {
			tieSum += t * (t - 1) * (2*t + 5)
		}
		i = j
	}
	nF := float64(n)
	return (nF*(nF-1)*(2*nF+5) - tieSum) / 18.0
}

// theilSenSlope returns the Theil-Sen median pairwise slope of (value/timestamp).
// O(n^2) pairs with O(n^2 log n) sort via detectorMedian — at n=60 this is
// ~1770 entries and a tiny sort, well below the per-tick budget.
func theilSenSlope(points []observer.Point) float64 {
	n := len(points)
	if n < 2 {
		return 0
	}
	slopes := make([]float64, 0, n*(n-1)/2)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dt := points[j].Timestamp - points[i].Timestamp
			if dt == 0 {
				continue
			}
			slopes = append(slopes, (points[j].Value-points[i].Value)/float64(dt))
		}
	}
	if len(slopes) == 0 {
		return 0
	}
	return detectorMedian(slopes)
}
