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

// hellingerCPStateKey identifies per-series state by ref and aggregation.
type hellingerCPStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hellingerCPSeriesState holds per-series streaming state for the HellingerCP
// detector. The long ring captures the stable baseline distribution; the
// short ring captures the recent distribution. Both histograms are computed
// over the same equal-width bin edges derived from the long ring's [Q5, Q95]
// inter-quantile range, recomputed periodically (every RebinEveryTicks ticks).
//
// Memory: LongWindow*8 + LongWindow*8 (bins) + ShortWindow*8 + ShortWindow*8
// (bins) + ShortWindow*8 (timestamps) + Bins*4*2 (hists) + Bins*8+8 (edges)
// ≈ 1.4 KB per (series, aggregation) at defaults — ~7x lighter than BOCPD's
// per-series posterior arrays.
type hellingerCPSeriesState struct {
	// Long-window ring buffer (size LongWindow).
	ringBuf  []float64
	ringBins []int
	ringHead int
	n        int

	// Short-window ring buffer (size ShortWindow).
	shortBuf        []float64
	shortBins       []int
	shortTimestamps []int64
	shortHead       int
	shortN          int

	// Tick counter since last rebin; triggers re-derivation of bin edges
	// every RebinEveryTicks ticks to track slow shifts in the baseline.
	ticksSinceRebin int

	// Equal-width bin edges over the long ring's [Q5, Q95] range.
	// Bin i covers [edges[i], edges[i+1]); out-of-range values clip to bin
	// 0 or bin Bins-1. binEdges is nil during warmup.
	binEdges  []float64
	longHist  []int
	shortHist []int

	// segmentStartTime advances on fire so subsequent storage reads only
	// see post-changepoint data. lastProcessedTime caps the (exclusive)
	// lower bound used by ForEachPoint so we only ingest new points.
	segmentStartTime  int64
	lastProcessedTime int64

	// Cooldown countdown after a fire to suppress repeat fires while the
	// post-changepoint baseline is still being established.
	recoveryCount int

	// Cursor tracking storage state — same pattern as ScanMW/BOCPD.
	lastProcessedCount int
	lastWriteGen       int64
}

// HellingerCPConfig configures the HellingerCP changepoint detector.
type HellingerCPConfig struct {
	// LongWindow is the size of the stable-baseline ring buffer.
	LongWindow int `json:"long_window"`
	// ShortWindow is the size of the recent-data ring buffer.
	ShortWindow int `json:"short_window"`
	// Bins is the number of equal-width histogram bins over [Q5, Q95].
	Bins int `json:"bins"`
	// HellingerThreshold is the minimum H(P, Q) to trigger anomaly review.
	HellingerThreshold float64 `json:"hellinger_threshold"`
	// MinDeviationMAD gates fires on |median(short) - median(long)| / MAD(long)
	// to suppress same-shape, same-level FPs.
	MinDeviationMAD float64 `json:"min_deviation_mad"`
	// RebinEveryTicks bounds how often bin edges are re-derived from the long
	// ring; smaller values track shifts faster but cost more O(N log N) sorts.
	RebinEveryTicks int `json:"rebin_every_ticks"`
	// RecoveryPoints suppresses repeat fires for this many post-fire points.
	RecoveryPoints int `json:"recovery_points"`
	// MinVariance is the minimum [Q5, Q95] spread; below this, bin edges fall
	// back to mean ± 3σ to avoid pathological zero-width bins on flat series.
	MinVariance float64 `json:"min_variance"`
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate `json:"-"`
}

// DefaultHellingerCPConfig returns the catalog-default HellingerCP config.
func DefaultHellingerCPConfig() HellingerCPConfig {
	return HellingerCPConfig{
		LongWindow:         120,
		ShortWindow:        20,
		Bins:               16,
		HellingerThreshold: 0.55,
		MinDeviationMAD:    2.0,
		RebinEveryTicks:    20,
		RecoveryPoints:     10,
		MinVariance:        1.0,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
	}
}

// applyHellingerDefaults fills zero/negative fields with sensible defaults so
// callers can override only what they care about.
func applyHellingerDefaults(cfg HellingerCPConfig) HellingerCPConfig {
	d := DefaultHellingerCPConfig()
	if cfg.LongWindow <= 0 {
		cfg.LongWindow = d.LongWindow
	}
	if cfg.ShortWindow <= 0 {
		cfg.ShortWindow = d.ShortWindow
	}
	if cfg.ShortWindow > cfg.LongWindow {
		cfg.ShortWindow = cfg.LongWindow
	}
	if cfg.Bins <= 0 {
		cfg.Bins = d.Bins
	}
	if cfg.HellingerThreshold <= 0 {
		cfg.HellingerThreshold = d.HellingerThreshold
	}
	if cfg.MinDeviationMAD <= 0 {
		cfg.MinDeviationMAD = d.MinDeviationMAD
	}
	if cfg.RebinEveryTicks <= 0 {
		cfg.RebinEveryTicks = d.RebinEveryTicks
	}
	if cfg.RecoveryPoints <= 0 {
		cfg.RecoveryPoints = d.RecoveryPoints
	}
	if cfg.MinVariance <= 0 {
		cfg.MinVariance = d.MinVariance
	}
	if len(cfg.Aggregations) == 0 {
		cfg.Aggregations = d.Aggregations
	}
	return cfg
}

// HellingerCPDetector detects changepoints by comparing the empirical
// distributions of a long stable window and a short recent window using the
// Hellinger distance H(P, Q) ∈ [0, 1]. Bounded and symmetric, with stable
// behavior on empty bins (no KL asymptote, no chi-square sample-size
// dependency). Magnitude-invariant — unlike Wasserstein it does not scale
// with the absolute series level. A median-deviation gate suppresses the
// dominant FP mode (sampling-noise Hellinger ≈ 0.3 between two i.i.d. samples
// of these sizes, which can briefly approach the threshold).
//
// Implements observer.Detector and observer.SeriesRemover. After a fire, the
// segment cursor advances and per-series state resets so accumulation
// restarts past the changepoint — mirroring the ScanMW pattern.
type HellingerCPDetector struct {
	cfg    HellingerCPConfig
	series map[hellingerCPStateKey]*hellingerCPSeriesState

	// Cache the discovered series list across Detect calls (same pattern
	// as ScanMW / BOCPD).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewHellingerCPDetector constructs a HellingerCP detector with default settings.
// The catalog factory uses this no-arg constructor; tests that want to tune
// hyperparameters use NewHellingerCPDetectorWithConfig.
func NewHellingerCPDetector() *HellingerCPDetector {
	return NewHellingerCPDetectorWithConfig(DefaultHellingerCPConfig())
}

// NewHellingerCPDetectorWithConfig constructs a HellingerCP detector with a
// caller-supplied config. Zero-valued fields fall back to defaults via
// applyHellingerDefaults.
func NewHellingerCPDetectorWithConfig(cfg HellingerCPConfig) *HellingerCPDetector {
	cfg = applyHellingerDefaults(cfg)
	return &HellingerCPDetector{
		cfg:    cfg,
		series: make(map[hellingerCPStateKey]*hellingerCPSeriesState),
	}
}

// Name returns the detector identifier.
func (d *HellingerCPDetector) Name() string { return "hellingercp" }

// Reset clears all per-series state for replay/reanalysis.
func (d *HellingerCPDetector) Reset() {
	d.series = make(map[hellingerCPStateKey]*hellingerCPSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed. Without
// this hook the per-series map keeps growing with the cumulative number of
// series ever observed even after storage shrinks.
func (d *HellingerCPDetector) RemoveSeries(refs []observer.SeriesRef) {
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.cfg.Aggregations {
			delete(d.series, hellingerCPStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. It discovers series, streams new points
// through per-series ring buffers, and emits one anomaly per series per
// changepoint. Iteration pattern mirrors ScanMW.Detect (SeriesGeneration
// gating + bulkSeriesStatus + ForEachPoint).
func (d *HellingerCPDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	if d.series == nil {
		d.series = make(map[hellingerCPStateKey]*hellingerCPSeriesState)
	}

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
		for _, agg := range d.cfg.Aggregations {
			sk := hellingerCPStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Skip if no new writes since last call.
			if status.writeGeneration == state.lastWriteGen && status.pointCount == state.lastProcessedCount {
				continue
			}

			anomaly, fired := d.processSeries(storage, meta, agg, state, dataTime)
			if fired {
				allAnomalies = append(allAnomalies, anomaly)
			}

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processSeries drains the new points for a single (series, aggregation),
// updating the streaming state and emitting at most one anomaly per call.
// On fire the per-series state is reset and segmentStartTime advances, so the
// next call starts a fresh window past the changepoint.
func (d *HellingerCPDetector) processSeries(
	storage observer.StorageReader,
	meta observer.SeriesMeta,
	agg observer.Aggregate,
	state *hellingerCPSeriesState,
	dataTime int64,
) (observer.Anomaly, bool) {
	startTime := state.lastProcessedTime
	if state.segmentStartTime > startTime {
		startTime = state.segmentStartTime
	}

	var fired bool
	var firedAnomaly observer.Anomaly

	storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		if fired {
			// Stop processing in this call; the rest of the buffer will be
			// re-considered on the next Detect with the advanced cursor.
			return
		}
		if p.Timestamp <= state.lastProcessedTime {
			return
		}
		state.lastProcessedTime = p.Timestamp

		d.pushPoint(state, p.Value, p.Timestamp)

		// Detection gate: skip during recovery cooldown and warmup.
		if state.recoveryCount > 0 {
			state.recoveryCount--
			return
		}
		if state.n < d.cfg.LongWindow || state.shortN < d.cfg.ShortWindow || state.binEdges == nil {
			return
		}

		h := hellingerCPDistance(state.longHist, state.shortHist, state.n, state.shortN)
		if h < d.cfg.HellingerThreshold {
			return
		}

		// Median-deviation gate. Detector{Median,MAD} reused from
		// metrics_detector_util.go for consistency with ScanMW/ScanWelch.
		longVals := append([]float64(nil), state.ringBuf[:state.n]...)
		shortVals := append([]float64(nil), state.shortBuf[:state.shortN]...)
		longMedian := detectorMedian(longVals)
		shortMedian := detectorMedian(shortVals)
		longMAD := detectorMAD(longVals, longMedian, false)

		denom := longMAD
		if denom < 1e-10 {
			denom = math.Max(math.Abs(longMedian)*0.01, 1e-6)
		}
		deviation := math.Abs(shortMedian-longMedian) / denom
		if deviation < d.cfg.MinDeviationMAD {
			return
		}

		// Estimate the changepoint as the oldest timestamp currently in the
		// short window: the algorithm has just confirmed that this window's
		// distribution differs from the long baseline, so the change must
		// have entered somewhere within it. This makes the reported anomaly
		// timestamp track the actual shift rather than the (lagging)
		// detection time.
		oldestIdx := (state.shortHead - state.shortN + d.cfg.ShortWindow) % d.cfg.ShortWindow
		cpTime := state.shortTimestamps[oldestIdx]
		if cpTime <= 0 || cpTime > p.Timestamp {
			cpTime = p.Timestamp
		}

		direction := "increased"
		if shortMedian < longMedian {
			direction = "decreased"
		}
		seriesName := s.Name + ":" + aggSuffix(agg)
		score := h
		firedAnomaly = observer.Anomaly{
			Type: observer.AnomalyTypeMetric,
			Source: observer.SeriesDescriptor{
				Namespace: s.Namespace,
				Name:      s.Name,
				Tags:      s.Tags,
				Aggregate: agg,
			},
			SourceRef:    &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg},
			DetectorName: d.Name(),
			Title:        "HellingerCP changepoint: " + seriesName,
			Description: fmt.Sprintf(
				"%s %s (long_median=%.4f, short_median=%.4f, H=%.3f, %.1f MADs)",
				seriesName, direction, longMedian, shortMedian, h, deviation,
			),
			Timestamp: cpTime,
			Score:     &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineMedian: longMedian,
				BaselineMAD:    longMAD,
				CurrentValue:   shortMedian,
				DeviationSigma: deviation,
			},
		}
		fired = true

		// Reset streaming state and advance the cursor past the changepoint
		// so the next call starts accumulating a fresh post-CP baseline.
		// RecoveryPoints also gates re-fires while the new windows fill in.
		state.segmentStartTime = p.Timestamp
		state.lastProcessedTime = p.Timestamp
		state.ringHead = 0
		state.n = 0
		state.shortHead = 0
		state.shortN = 0
		state.ticksSinceRebin = 0
		state.binEdges = nil
		for i := range state.longHist {
			state.longHist[i] = 0
		}
		for i := range state.shortHist {
			state.shortHist[i] = 0
		}
		state.recoveryCount = d.cfg.RecoveryPoints
	})

	return firedAnomaly, fired
}

// newState allocates per-series state sized to the configured windows.
func (d *HellingerCPDetector) newState() *hellingerCPSeriesState {
	return &hellingerCPSeriesState{
		ringBuf:         make([]float64, d.cfg.LongWindow),
		ringBins:        make([]int, d.cfg.LongWindow),
		shortBuf:        make([]float64, d.cfg.ShortWindow),
		shortBins:       make([]int, d.cfg.ShortWindow),
		shortTimestamps: make([]int64, d.cfg.ShortWindow),
		longHist:        make([]int, d.cfg.Bins),
		shortHist:       make([]int, d.cfg.Bins),
	}
}

// pushPoint appends x (with timestamp t) to both rings and updates the
// histograms. When binEdges is uninitialized or the rebin tick interval is
// reached, the histograms and bin index parallel slices are recomputed from
// scratch (O(n_long log n_long) sort + O(n_long + n_short) re-binning); on
// other ticks the update is incremental: O(log B) for the binary search +
// O(1) for the bucket increment/decrement.
func (d *HellingerCPDetector) pushPoint(state *hellingerCPSeriesState, x float64, t int64) {
	state.ticksSinceRebin++

	longSlot := state.ringHead
	longEvictBin := state.ringBins[longSlot]
	longHadVal := state.n == d.cfg.LongWindow
	state.ringBuf[longSlot] = x

	shortSlot := state.shortHead
	shortEvictBin := state.shortBins[shortSlot]
	shortHadVal := state.shortN == d.cfg.ShortWindow
	state.shortBuf[shortSlot] = x
	state.shortTimestamps[shortSlot] = t

	state.ringHead = (state.ringHead + 1) % d.cfg.LongWindow
	if !longHadVal {
		state.n++
	}
	state.shortHead = (state.shortHead + 1) % d.cfg.ShortWindow
	if !shortHadVal {
		state.shortN++
	}

	needRebin := state.binEdges == nil || state.ticksSinceRebin >= d.cfg.RebinEveryTicks
	if needRebin {
		if state.n < d.cfg.LongWindow {
			// Wait for warmup before computing bin edges; until then the
			// rings just accumulate values without histogram tracking.
			return
		}
		d.rebin(state)
		return
	}

	// Incremental histogram update — same point goes into both rings, so
	// the bin index is computed once.
	newBin := binIndex(state.binEdges, x)
	state.ringBins[longSlot] = newBin
	if longHadVal {
		state.longHist[longEvictBin]--
	}
	state.longHist[newBin]++

	state.shortBins[shortSlot] = newBin
	if shortHadVal {
		state.shortHist[shortEvictBin]--
	}
	state.shortHist[newBin]++
}

// rebin recomputes bin edges from the long ring's [Q5, Q95] range and
// rebuilds both histograms. Called on the first warmup tick and every
// RebinEveryTicks ticks thereafter.
func (d *HellingerCPDetector) rebin(state *hellingerCPSeriesState) {
	if state.n == 0 {
		return
	}
	scratch := make([]float64, state.n)
	copy(scratch, state.ringBuf[:state.n])
	sort.Float64s(scratch)

	q05Idx := state.n * 5 / 100
	q95Idx := state.n * 95 / 100
	if q95Idx >= state.n {
		q95Idx = state.n - 1
	}
	if q05Idx < 0 {
		q05Idx = 0
	}
	q05 := scratch[q05Idx]
	q95 := scratch[q95Idx]

	if q95-q05 < d.cfg.MinVariance {
		// Fallback: derive a reasonable range from mean ± 3σ. This avoids
		// pathological zero-width bins on flat or near-flat series, where
		// any new point would be classified as an outlier.
		var sum, sqsum float64
		for _, v := range scratch {
			sum += v
			sqsum += v * v
		}
		fn := float64(state.n)
		mean := sum / fn
		variance := sqsum/fn - mean*mean
		if variance < 0 {
			variance = 0
		}
		stddev := math.Sqrt(variance)
		// Floor σ so the bin range is at least MinVariance/2 wide.
		minStddev := d.cfg.MinVariance / 6
		if stddev < minStddev {
			stddev = minStddev
		}
		q05 = mean - 3*stddev
		q95 = mean + 3*stddev
	}

	if state.binEdges == nil || len(state.binEdges) != d.cfg.Bins+1 {
		state.binEdges = make([]float64, d.cfg.Bins+1)
	}
	width := (q95 - q05) / float64(d.cfg.Bins)
	if width <= 0 {
		// Should be unreachable thanks to the MinVariance fallback, but
		// guard against degenerate floats anyway.
		width = 1.0
	}
	for i := 0; i <= d.cfg.Bins; i++ {
		state.binEdges[i] = q05 + width*float64(i)
	}

	for i := range state.longHist {
		state.longHist[i] = 0
	}
	for i := 0; i < state.n; i++ {
		b := binIndex(state.binEdges, state.ringBuf[i])
		state.ringBins[i] = b
		state.longHist[b]++
	}
	for i := range state.shortHist {
		state.shortHist[i] = 0
	}
	for i := 0; i < state.shortN; i++ {
		b := binIndex(state.binEdges, state.shortBuf[i])
		state.shortBins[i] = b
		state.shortHist[b]++
	}

	state.ticksSinceRebin = 0
}

// binIndex returns the bin index for x given monotonically non-decreasing
// edges. Bin i covers [edges[i], edges[i+1]); out-of-range values clip to bin
// 0 or bin (B-1). Returned index is always in [0, B-1] when len(edges) >= 2.
func binIndex(edges []float64, x float64) int {
	B := len(edges) - 1
	if B <= 0 {
		return 0
	}
	if x <= edges[0] {
		return 0
	}
	if x >= edges[B] {
		return B - 1
	}
	// Smallest j such that edges[j] > x; bin = j - 1.
	idx := sort.Search(len(edges), func(j int) bool { return edges[j] > x })
	return idx - 1
}

// hellingerCPDistance computes H(P, Q) = (1/√2) * sqrt(Σ (√P_i - √Q_i)²) where
// P[i] = longHist[i]/longN and Q[i] = shortHist[i]/shortN. Bounded in [0, 1]:
// 0 when the histograms are identical, 1 when they have disjoint support.
func hellingerCPDistance(longHist, shortHist []int, longN, shortN int) float64 {
	if longN <= 0 || shortN <= 0 {
		return 0
	}
	fLong := float64(longN)
	fShort := float64(shortN)
	sumSq := 0.0
	for i := range longHist {
		p := float64(longHist[i]) / fLong
		q := float64(shortHist[i]) / fShort
		diff := math.Sqrt(p) - math.Sqrt(q)
		sumSq += diff * diff
	}
	h := math.Sqrt(sumSq) / math.Sqrt2
	if h > 1 {
		h = 1
	}
	return h
}
