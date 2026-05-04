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

// Matrix-profile shape-anomaly detector.
//
// Per (series, aggregation) we maintain:
//
//   - a small staging buffer of the most recent raw values; once it reaches
//     SubseqLen it is z-normalized into a fixed-length subsequence and pushed
//     into the cache, then drained.
//   - a ring of up to HistorySubs z-normalized subsequences (non-overlapping
//     at stride SubseqLen). The cache stores the L float64 values of each
//     subseq packed into a single contiguous slab, allocated once at construction.
//   - a parallel ring of matrix-profile values: for each cached subseq, the
//     z-normalized Euclidean distance to its nearest non-trivial neighbour at
//     the time the subseq was added.
//
// Each new subseq fires a discord when its minDist clears both a robust
// data-driven gate (median(MP) + ThresholdK * MAD(MP)) and an absolute
// "0-correlation" floor. Within a single Detect batch the worst candidate is
// emitted; firing arms a refractory of HistorySubs/4 subsequences during which
// no further fires are emitted (the cache and the matrix profile keep
// updating).
//
// The detector is information-theoretically distinct from the level-shift
// (scanwelch / scanmw / kl_divergence) and Bayesian (bocpd) detectors already
// in the catalog: it captures *shape* anomalies — sudden glitches, missing
// seasonality, structural breaks — that none of those can see by construction
// because they all summarise a window into a small statistic.
//
// Reference:
//   - Yeh et al. 2016 "Matrix Profile I" (ICDM'16) — definition of P[i] as the
//     z-normalized Euclidean distance from subsequence i to its nearest
//     non-trivial neighbour.
//   - Zhu et al. 2016 "Matrix Profile XI / SCRIMP++" — incremental MASS update.
//
// This phase-1 implementation uses the naive O(H*L) update per new subseq;
// phase-2 may swap in the FFT-MASS trick to amortise to O(H + log H) per
// raw point ingested.

// Default tunables. SubseqLen and HistorySubs together set the cache memory:
// HistorySubs * SubseqLen float64s for the z-normalized slab plus HistorySubs
// float64s for the matrix profile, ~38KB per (series, aggregation) at
// defaults.
const (
	mpDefaultSubseqLen       = 20
	mpDefaultHistorySubs     = 240
	mpDefaultThresholdK      = 3.0
	mpDefaultAbsoluteFloor   = 2.0
	mpDefaultMinPointsToScan = 80 // = SubseqLen * 4
	// mpTimestampRing keeps the last N point timestamps for sampling-cadence
	// estimation (used for the anomaly's SamplingIntervalSec). Mirrors the
	// holt_residual ring for cheap reuse via medianTimestampInterval.
	mpTimestampRing = 16
)

// mpStateKey identifies per-series state by ref and aggregation.
type mpStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// mpSeriesState holds streaming state for one (series, aggregation) pair.
//
// Subsequences are non-overlapping (stride SubseqLen): the staging buffer
// fills with raw values, and once it reaches SubseqLen we z-normalize and
// commit it to the ring. The cache is logically a FIFO ring of the last
// HistorySubs subsequences; we evict the oldest when full.
type mpSeriesState struct {
	// streaming cursor (mirrors kl_divergence / holt_residual)
	lastProcessedCount int
	lastProcessedTime  int64
	lastWriteGen       int64

	// staging buffer for the in-progress subseq. cap = SubseqLen. Drained to
	// length 0 once a subseq is committed.
	pendingBuf []float64

	// total subsequences ever formed for this series. Used as the logical
	// position of each subseq in the ring (for the trivial-match guard) and
	// for the warmup gate.
	subsFormed int

	// z-normalized subsequence cache. Flat slab of size HistorySubs *
	// SubseqLen, allocated once at state construction. The ring stores up to
	// HistorySubs subseqs; subHead is the slot index of the oldest. The slot
	// for the kth oldest is (subHead + k) % HistorySubs; ring slots have
	// length SubseqLen each.
	zSubs    []float64
	subCount int
	subHead  int

	// matrix-profile ring (parallel to the subseq ring). Each entry is the
	// minDist computed at the time the corresponding subseq was committed.
	// Entries are only pushed when the subseq has at least one non-trivial
	// neighbour to compare against — see commitSubseq.
	mp      []float64
	mpCount int
	mpHead  int

	// refractory countdown in subsequences. Set to HistorySubs/4 on fire and
	// decremented per committed subseq. While >0, new subseqs are committed
	// to the cache and matrix profile but no anomaly is emitted.
	refractory int

	// captured series metadata (first non-empty observation suffices)
	seriesNamespace string
	seriesName      string
	seriesTags      []string
	metaCaptured    bool

	// recent timestamps for sampling-interval estimation. Small ring
	// (cap mpTimestampRing) — we only need a robust median for downstream
	// correlators, not the full window's worth.
	recentTimestamps  []int64
	lastSeenTimestamp int64
}

// MatrixProfileDetector fires on subsequences whose z-normalized shape has no
// near-match in recent history.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields are
// exposed directly so callers (testbench, tests) can override defaults after
// construction; NewMatrixProfileDetector populates them.
type MatrixProfileDetector struct {
	// SubseqLen is the length L of each subsequence (z-normalized into a
	// shape signature). Default 20. Smaller L makes the detector more
	// reactive but noisier; larger L smooths.
	SubseqLen int
	// HistorySubs is the maximum number of cached subsequences (the H in
	// Yeh's matrix profile). Default 240; together with SubseqLen this
	// determines the lookback horizon (H * L raw points at default = 4800).
	HistorySubs int
	// ThresholdK scales the MAD term in the data-driven threshold. Default 3.0.
	ThresholdK float64
	// AbsoluteFloor is the minimum z-normalized Euclidean distance below
	// which we never fire, regardless of the data-driven threshold. Default
	// 2.0 corresponds to ~0.95 Pearson correlation at L=20 — a sane "still
	// looks like the cache" floor that suppresses fires when MP is uniformly
	// small (typical for highly periodic data).
	AbsoluteFloor float64
	// MinPointsToScan is the minimum number of raw points (across all
	// subseqs ever formed) before a fire is allowed. Default 80 = 4
	// subseqs at L=20. Acts as the warmup gate.
	MinPointsToScan int
	// Aggregations to run detection on. Default [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[mpStateKey]*mpSeriesState

	// cache the discovered series list across Detect calls (mirrors the
	// scanwelch / scanmw / bocpd / kl / holt pattern).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewMatrixProfileDetector creates a MatrixProfile detector with default
// settings. The catalog factory calls this with no arguments; tunables can
// be overridden post-construction by setting the exported fields.
func NewMatrixProfileDetector() *MatrixProfileDetector {
	return &MatrixProfileDetector{
		SubseqLen:       mpDefaultSubseqLen,
		HistorySubs:     mpDefaultHistorySubs,
		ThresholdK:      mpDefaultThresholdK,
		AbsoluteFloor:   mpDefaultAbsoluteFloor,
		MinPointsToScan: mpDefaultMinPointsToScan,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[mpStateKey]*mpSeriesState),
	}
}

// Name implements observer.Detector.
func (d *MatrixProfileDetector) Name() string { return "matrix_profile" }

// Reset clears all per-series state for replay/reanalysis.
func (d *MatrixProfileDetector) Reset() {
	d.series = make(map[mpStateKey]*mpSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Each per-series entry holds a HistorySubs*SubseqLen z-norm slab plus a
// HistorySubs matrix-profile ring (~38KB at defaults), so without this
// teardown the map keeps growing unbounded with the cumulative series count
// even after storage shrinks. Called by the engine right after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *MatrixProfileDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, mpStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. It iterates the series catalogue,
// streams new points into per-series staging buffers, commits new
// subsequences once L points have accumulated, and emits a discord anomaly
// when the matrix profile gate fires.
func (d *MatrixProfileDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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

	L := d.SubseqLen

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := mpStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip both ingest and scan when nothing
			// observable has changed and we have not accumulated at
			// least one subseq's worth of new points. Mirrors
			// kl_divergence's gate (metrics_detector_kl_divergence.go)
			// which requires TestWindow new points before re-running.
			if status.pointCount < state.lastProcessedCount+L && status.writeGeneration == state.lastWriteGen {
				continue
			}

			anomaly, fired := d.ingestAndScan(storage, meta.Ref, agg, state, dataTime)

			// Update cursor unconditionally so a quiet series doesn't
			// keep replaying through the gate next call.
			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration

			if fired {
				anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
				allAnomalies = append(allAnomalies, anomaly)
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *MatrixProfileDetector) newState() *mpSeriesState {
	return &mpSeriesState{
		pendingBuf:       make([]float64, 0, d.SubseqLen),
		zSubs:            make([]float64, d.HistorySubs*d.SubseqLen),
		mp:               make([]float64, d.HistorySubs),
		recentTimestamps: make([]int64, 0, mpTimestampRing),
	}
}

// ingestAndScan streams new points into staging, commits subseqs as they
// fill, and tracks the worst fire-eligible candidate across the batch. At
// most one anomaly is emitted per Detect call (the worst); firing arms the
// refractory after the batch is complete so all subseqs in the batch get a
// fair shot at being the chosen worst.
func (d *MatrixProfileDetector) ingestAndScan(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *mpSeriesState,
	dataTime int64,
) (observer.Anomaly, bool) {
	if dataTime <= state.lastProcessedTime {
		return observer.Anomaly{}, false
	}

	// Snapshot whether we entered the batch already in refractory; this is
	// what gates fire emission for every subseq committed in this batch.
	// Cross-batch refractory is handled by decrementing on each commit.
	startedInRefractory := state.refractory > 0

	var bestAnomaly observer.Anomaly
	bestFired := false
	var bestScore float64

	storage.ForEachPoint(ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		if !state.metaCaptured {
			state.seriesNamespace = s.Namespace
			state.seriesName = s.Name
			if len(s.Tags) > 0 {
				tagsCopy := make([]string, len(s.Tags))
				copy(tagsCopy, s.Tags)
				state.seriesTags = tagsCopy
			}
			state.metaCaptured = true
		}
		state.lastSeenTimestamp = p.Timestamp
		pushTimestampMP(state, p.Timestamp)

		state.pendingBuf = append(state.pendingBuf, p.Value)
		if len(state.pendingBuf) < d.SubseqLen {
			return
		}

		// Commit a new subseq. commitSubseq returns the minDist (or +Inf
		// when no non-trivial neighbours exist yet) and resets pendingBuf.
		minDist := d.commitSubseq(state)

		// Decrement refractory per committed subseq. The decision to fire
		// uses the snapshot taken at the start of the batch — all subseqs
		// in this batch see the same refractory state for emission gating,
		// while the underlying counter advances toward expiry.
		if state.refractory > 0 {
			state.refractory--
		}

		if startedInRefractory {
			return
		}
		if math.IsInf(minDist, 1) {
			return
		}
		// Warmup: enough total raw points AND enough mp values for a
		// stable median+MAD threshold. Without the mpCount guard, the
		// first finite minDist would be compared against itself
		// (median == minDist, MAD == 0, threshold == minDist) and never
		// fire — but the guard also stops us evaluating very early
		// thresholds with too few samples.
		if state.subsFormed*d.SubseqLen < d.MinPointsToScan {
			return
		}
		if state.mpCount < 4 {
			return
		}

		median := detectorMedian(state.mp[:state.mpCount])
		mad := detectorMAD(state.mp[:state.mpCount], median, false)
		threshold := median + d.ThresholdK*mad

		if minDist <= threshold || minDist <= d.AbsoluteFloor {
			return
		}

		score := minDist
		if !bestFired || score > bestScore {
			seriesName := state.seriesName + ":" + aggSuffix(agg)
			devDenom := mad
			if devDenom < 1e-9 {
				devDenom = 1e-9
			}
			bestAnomaly = observer.Anomaly{
				Type: observer.AnomalyTypeMetric,
				Source: observer.SeriesDescriptor{
					Namespace: state.seriesNamespace,
					Name:      state.seriesName,
					Tags:      state.seriesTags,
					Aggregate: agg,
				},
				DetectorName: d.Name(),
				Title:        "Matrix profile discord: " + seriesName,
				Description: fmt.Sprintf(
					"%s subsequence is structurally unlike recent history (minDist=%.3f, threshold=%.3f, mpMedian=%.3f, mpMAD=%.3f)",
					seriesName, minDist, threshold, median, mad,
				),
				Timestamp:           state.lastSeenTimestamp,
				Score:               &score,
				SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineMedian: median,
					BaselineMAD:    mad,
					CurrentValue:   minDist,
					DeviationSigma: (minDist - median) / devDenom,
					Threshold:      threshold,
				},
			}
			bestFired = true
			bestScore = score
		}
	})

	if bestFired {
		state.refractory = d.HistorySubs / 4
		if state.refractory < 1 {
			state.refractory = 1
		}
	}

	return bestAnomaly, bestFired
}

// commitSubseq z-normalizes the staging buffer, writes it into the ring slab
// (evicting the oldest if full), computes the minimum z-normalized Euclidean
// distance to all non-trivial cached neighbours, and pushes that minDist
// onto the matrix-profile ring. Returns the minDist (or +Inf if no
// non-trivial neighbours exist yet). Drains pendingBuf and increments
// subsFormed.
func (d *MatrixProfileDetector) commitSubseq(state *mpSeriesState) float64 {
	L := d.SubseqLen
	H := d.HistorySubs

	// Z-normalize via Welford on the L pending values.
	mean, std := mpMeanStd(state.pendingBuf)

	// Choose the slot in the ring: append until full, then overwrite the
	// oldest (FIFO eviction).
	var slot int
	if state.subCount < H {
		slot = (state.subHead + state.subCount) % H
		state.subCount++
	} else {
		slot = state.subHead
		state.subHead = (state.subHead + 1) % H
	}
	slabBase := slot * L
	if std < 1e-10 {
		// Degenerate (constant) subseq — its z-normalization is undefined.
		// Storing zeros is the conventional fallback: distances against
		// non-flat subseqs evaluate to sqrt(2L) (zero correlation) and
		// distances against other flat subseqs evaluate to sqrt(2L) as
		// well. The AbsoluteFloor at default 2.0 keeps this from firing
		// for constant series.
		for j := 0; j < L; j++ {
			state.zSubs[slabBase+j] = 0
		}
	} else {
		for j := 0; j < L; j++ {
			state.zSubs[slabBase+j] = (state.pendingBuf[j] - mean) / std
		}
	}

	currentSubPos := state.subsFormed
	// Position of the oldest cached subseq, after the just-written one is
	// counted. With state.subCount now reflecting the just-committed subseq,
	// oldestPos = currentSubPos + 1 - subCount.
	oldestPos := currentSubPos + 1 - state.subCount

	// Trivial-match guard: skip cached subseqs whose position is within
	// L/2 of the current one. This includes self (distance 0) and prevents
	// a sustained shape change spanning two adjacent subseqs from matching
	// itself and going undetected. The plan calls this out as the standard
	// Yeh-2016 guard adapted to non-overlapping stride.
	guard := L / 2
	if guard < 1 {
		guard = 1
	}

	minDist := math.Inf(1)
	for k := 0; k < state.subCount; k++ {
		pos := oldestPos + k
		diff := currentSubPos - pos
		if diff < 0 {
			diff = -diff
		}
		if diff < guard {
			continue
		}
		otherSlot := (state.subHead + k) % H
		dist := mpZNormDist(state.zSubs, slabBase, otherSlot*L, L)
		if dist < minDist {
			minDist = dist
		}
	}

	// Push the new minDist onto the matrix-profile ring, but only if we
	// actually compared against at least one non-trivial neighbour. The
	// alternative (pushing +Inf) would corrupt detectorMedian and
	// detectorMAD downstream.
	if !math.IsInf(minDist, 1) {
		if state.mpCount < H {
			slot := (state.mpHead + state.mpCount) % H
			state.mp[slot] = minDist
			state.mpCount++
		} else {
			state.mp[state.mpHead] = minDist
			state.mpHead = (state.mpHead + 1) % H
		}
	}

	state.subsFormed++
	state.pendingBuf = state.pendingBuf[:0]
	return minDist
}

// mpMeanStd computes the mean and population standard deviation of vals via
// Welford's online algorithm. Returns std=0 for empty input. Welford avoids
// the cancellation that hurts the naive (sum-of-squares - n*mean^2) form on
// near-constant data — important here because the z-normalization is the
// detector's primary precision-sensitive step.
func mpMeanStd(vals []float64) (mean, std float64) {
	n := len(vals)
	if n == 0 {
		return 0, 0
	}
	var m, m2 float64
	for i, v := range vals {
		delta := v - m
		m += delta / float64(i+1)
		m2 += delta * (v - m)
	}
	variance := m2 / float64(n)
	if variance < 0 {
		variance = 0
	}
	return m, math.Sqrt(variance)
}

// mpZNormDist computes the z-normalized Euclidean distance between two
// subsequences stored in the same flat slab. Both subseqs are length L.
//
// Each z-normalized subseq has population mean 0 and variance 1, so
// sum_j z[j]^2 == L, and dist^2 == sum (a-b)^2 == 2L - 2*dot. We clamp
// negatives that arise only from float rounding when the two subseqs are
// nearly identical (dot ≈ L) before the sqrt.
func mpZNormDist(slab []float64, base1, base2, L int) float64 {
	var dot float64
	a := slab[base1 : base1+L]
	b := slab[base2 : base2+L]
	for j := 0; j < L; j++ {
		dot += a[j] * b[j]
	}
	val := 2.0 * (float64(L) - dot)
	if val < 0 {
		val = 0
	}
	return math.Sqrt(val)
}

// pushTimestampMP appends ts to the recent-timestamp ring, dropping the
// oldest if full. Mirrors holt_residual.pushTimestamp; the ring is small
// (cap mpTimestampRing) so the shift-left cost is negligible.
func pushTimestampMP(state *mpSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, mpTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// ensureDefaults fills zero-valued config fields with sensible defaults so
// the detector behaves sanely even when constructed via reflective paths
// that bypass NewMatrixProfileDetector. Mirrors the kl_divergence and
// holt_residual ensureDefaults idiom.
func (d *MatrixProfileDetector) ensureDefaults() {
	if d.SubseqLen <= 0 {
		d.SubseqLen = mpDefaultSubseqLen
	}
	if d.HistorySubs <= 0 {
		d.HistorySubs = mpDefaultHistorySubs
	}
	if d.ThresholdK <= 0 {
		d.ThresholdK = mpDefaultThresholdK
	}
	if d.AbsoluteFloor <= 0 {
		d.AbsoluteFloor = mpDefaultAbsoluteFloor
	}
	if d.MinPointsToScan <= 0 {
		d.MinPointsToScan = mpDefaultMinPointsToScan
	}
	if d.series == nil {
		d.series = make(map[mpStateKey]*mpSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
