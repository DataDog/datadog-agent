// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: DirBF detector — streaming Dirichlet-multinomial
// Bayes-factor test on quantile-binned histograms across two abutting windows.
//
// Construction. Discretise observations into K=10 buckets whose edges are the
// inner deciles of a fixed warmup window. Maintain bin-count histograms over
// a long reference window (W_ref=400) and a short recent window (W_recent=40)
// using the same R/T-style hand-off as DenRatio/VarShift: each new point is
// binned, pushed into recent; when recent is full its oldest bin migrates into
// ref; when ref is also full its oldest bin drops. Counts are kept incrementally
// so the per-tick update is O(1) on the windows themselves.
//
// On every fully-warmed tick compute the closed-form log Bayes factor under a
// symmetric Dirichlet(α=1) prior:
//
//	log B₁₀ = log P(ref | M₁) + log P(recent | M₁) − log P(ref+recent | M₀)
//	       = [lgamma(K·α) − lgamma(K·α + n_ref)    + Σ_i lgamma(α + ref_i)]
//	       + [lgamma(K·α) − lgamma(K·α + n_recent) + Σ_i lgamma(α + recent_i)]
//	       − [lgamma(K·α) − lgamma(K·α + n_sum)    + Σ_i lgamma(α + sum_i)]
//
// where M₁ is "ref and recent come from independent Dirichlets" and M₀ is
// "they share a single Dirichlet". The closed form is the standard
// Dirichlet-multinomial marginal likelihood — see Berger & Pericchi (1996),
// JASA 91, "The Intrinsic Bayes Factor for Linear Models" §3, and Gelman et
// al. BDA3 §3.4 for the Beta-binomial / Dirichlet-multinomial conjugate form.
//
// Why this fills a gap. The other catalog detectors target the marginal mean
// (CUSUM/BOCPD/ScanMW/ScanWelch/PHT), variance (VarShift), autocorrelation
// (AcorrShift), or a kernelised distribution divergence (DenRatio, MMDRFF).
// None of them is a *Bayesian model-evidence ratio on bin counts*. The
// closed-form Dirichlet-multinomial BF is structurally distinct from the
// banned density-ratio (uLSIF / KDE), kernel mean embedding (MMD-RFF), and
// permutation-entropy (ordinal pattern) approaches: no kernel, no ordinal
// patterns, no asymptotic approximation. Trigger when log B₁₀ > λ (default
// 5.0; conservatively above the noise-floor 99.9th percentile of ≈3.0 for
// these window sizes).
//
// Per-tick cost. Binning is K-1=9 comparisons. Ring updates are O(1). The BF
// computation is 3 × K = 30 lgamma evaluations on fire-eligible ticks. Memory
// per (series, agg): two ring buffers (440 bytes) + two count arrays (80 B) +
// one parallel timestamp ring for the median sampling-interval computation
// (320 B) + 9 bin edges + scalars ≈ ~1 KB — significantly cheaper than the
// kernel-based detectors. No allocations on the hot path; the per-fire
// makeAnomaly path allocates a small interval slice for the median.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. Window sizes are state-array shapes and must therefore
// be compile-time constants. Trigger thresholds, persistence, recovery, and
// cooldown are exposed on the detector struct so tests can drive the state
// machine without rebuilding the per-series fixed-size buffers.
const (
	dirbfNumBins      = 10
	dirbfRefSize      = 400
	dirbfRecentSize   = 40
	dirbfWarmupPoints = 400  // need a full ref-window of samples to fix bin edges
	dirbfPersistenceK = 3    // consecutive over-threshold ticks before firing
	dirbfRecoveryPts  = 20   // consecutive sub-recovery ticks before clearing inAlert
	dirbfLambda       = 5.0  // log Bayes factor threshold
	dirbfCooldownSec  = 600  // minimum data-time gap between consecutive fires
	dirbfPriorAlpha   = 1.0  // symmetric Dirichlet prior
)

// dirbfStateKey identifies per-series state by ref and aggregation.
type dirbfStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// dirbfSeriesState holds per-series streaming state. Sized to be
// allocation-free on the hot path; warmupBuf is freed (set to nil) once the
// bin edges have been computed, after which only the fixed-size rings/counts
// remain.
type dirbfSeriesState struct {
	// Warmup phase: collect the first WarmupPoints raw values to compute
	// bin edges. Cleared once edgesReady=true.
	warmupBuf []float64

	// Fixed bin edges (inner deciles of the warmup distribution). Length
	// dirbfNumBins-1 = 9. binIdx for x is the smallest i with x < binEdges[i],
	// or dirbfNumBins-1 if x >= every edge.
	binEdges   [dirbfNumBins - 1]float64
	edgesReady bool

	// Per-bin counts for ref and recent windows, kept in lock-step with the
	// ring contents so any membership change is an O(1) two-counter update.
	refCounts    [dirbfNumBins]int
	recentCounts [dirbfNumBins]int

	// Reference window: FIFO ring of bin indices, capacity dirbfRefSize.
	// refHead is both the oldest slot (when full) and the next-write slot.
	refRing [dirbfRefSize]uint8
	refHead int
	refN    int

	// Recent window: FIFO ring of bin indices and a parallel ring of
	// timestamps used to compute the median sampling interval at fire time.
	// Same head/N semantics as refRing.
	recentRing  [dirbfRecentSize]uint8
	recentTimes [dirbfRecentSize]int64
	recentHead  int
	recentN     int

	// Persistence + alert lifecycle.
	persistCnt   int
	inAlert      bool
	recoveryCnt  int
	lastFireTime int64

	// Bookkeeping.
	pointsSeen         int
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// DIRBFDetector flags distribution shifts via a streaming
// Dirichlet-multinomial Bayes factor on quantile-binned histograms.
// Implements observer.Detector + observer.SeriesRemover.
type DIRBFDetector struct {
	// Lambda is the log-Bayes-factor magnitude that gates a fire. Default
	// 5.0; conservatively above the noise-floor 99.9th percentile (~3.0) for
	// K=10 / W_ref=400 / W_recent=40 under H0.
	Lambda float64

	// PersistenceK is the number of consecutive over-threshold ticks needed
	// before a fire — suppresses single-tick noise spikes. Default 3.
	PersistenceK int

	// RecoveryPoints is the number of consecutive sub-recovery (logBF <
	// 0.5·Lambda) ticks required to clear an active alert. Default 20.
	RecoveryPoints int

	// WarmupPoints gates emission until the first WarmupPoints raw values
	// have been accumulated and used to fix the deciles bin edges.
	// Default 400.
	WarmupPoints int

	// CooldownSec is the minimum data-time interval between consecutive
	// fires for the same series; the wall-clock gate that prevents two
	// distinct shifts within a short window from producing two anomalies on
	// the same incident. Default 600.
	CooldownSec int64

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[dirbfStateKey]*dirbfSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewDIRBFDetector constructs a DIRBFDetector with default settings.
func NewDIRBFDetector() *DIRBFDetector {
	return &DIRBFDetector{
		Lambda:         dirbfLambda,
		PersistenceK:   dirbfPersistenceK,
		RecoveryPoints: dirbfRecoveryPts,
		WarmupPoints:   dirbfWarmupPoints,
		CooldownSec:    dirbfCooldownSec,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[dirbfStateKey]*dirbfSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*DIRBFDetector) Name() string { return "dirbf" }

// Reset clears all per-series state for replay/reanalysis.
func (d *DIRBFDetector) Reset() {
	d.series = make(map[dirbfStateKey]*dirbfSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~1 KB of fixed-size streaming state plus a possibly-still-live warmupBuf, so
// without this teardown the map would grow with the cumulative count of series
// ever observed even after their storage payload is gone. Called by the engine
// immediately after timeSeriesStorage.RemoveSeriesByKeys returns the freed
// refs.
func (d *DIRBFDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, dirbfStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors PHT/VarShift: a
// gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *DIRBFDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := dirbfStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &dirbfSeriesState{}
				d.series[sk] = state
			}

			mergeOccurred := status.pointCount == state.lastProcessedCount && status.writeGeneration != state.lastWriteGen
			if status.pointCount <= state.lastProcessedCount && !mergeOccurred {
				continue
			}

			startTime := state.lastProcessedTime
			if mergeOccurred {
				startTime = state.lastProcessedTime - 1
				if startTime < 0 {
					startTime = 0
				}
			}

			pointsSeen := false
			prevLen := len(allAnomalies)
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				pointsSeen = true
				if anomaly := d.processPoint(state, p, s, agg); anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}

			if !pointsSeen && mergeOccurred {
				state.lastWriteGen = status.writeGeneration
				continue
			}
			if pointsSeen {
				state.lastProcessedCount = status.pointCount
				state.lastWriteGen = status.writeGeneration
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint applies the streaming algorithm to a single new point. Returns
// a non-nil anomaly only on alert onset (not while still in alert, not during
// warmup, not while either window is filling, and not within cooldown).
func (d *DIRBFDetector) processPoint(state *dirbfSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	state.pointsSeen++

	// Warmup phase: collect the first WarmupPoints raw values, then freeze
	// the bin edges as their inner deciles.
	if !state.edgesReady {
		state.warmupBuf = append(state.warmupBuf, p.Value)
		if state.pointsSeen < d.WarmupPoints {
			return nil
		}
		// pointsSeen == WarmupPoints — set edges now. We sort a copy so
		// warmupBuf retains nothing after this; setting it to nil below
		// frees the warmup slice.
		sorted := make([]float64, len(state.warmupBuf))
		copy(sorted, state.warmupBuf)
		sort.Float64s(sorted)
		for i := 0; i < dirbfNumBins-1; i++ {
			idx := (i + 1) * d.WarmupPoints / dirbfNumBins
			// Defensive clamp: if WarmupPoints is configured to a value
			// not divisible by dirbfNumBins, the last index could hit
			// len(sorted). Clamp into range.
			if idx >= len(sorted) {
				idx = len(sorted) - 1
			}
			state.binEdges[i] = sorted[idx]
		}
		state.warmupBuf = nil
		state.edgesReady = true
		return nil
	}

	// Bin x — linear scan over 9 edges.
	binIdx := dirbfBin(p.Value, state.binEdges[:])

	// Slide the recent → ref pipeline. If recent is full, evict its oldest
	// bin (decrement recentCounts) and migrate it into ref. If ref is also
	// full, drop ref's oldest. All ring slots are then overwritten in place,
	// so this is O(1).
	if state.recentN == dirbfRecentSize {
		evictedBin := state.recentRing[state.recentHead]
		state.recentCounts[evictedBin]--
		if state.refN == dirbfRefSize {
			refOldest := state.refRing[state.refHead]
			state.refCounts[refOldest]--
			state.refRing[state.refHead] = evictedBin
			state.refHead = (state.refHead + 1) % dirbfRefSize
		} else {
			state.refRing[state.refHead] = evictedBin
			state.refHead = (state.refHead + 1) % dirbfRefSize
			state.refN++
		}
		state.refCounts[evictedBin]++
	}
	state.recentRing[state.recentHead] = uint8(binIdx)
	state.recentTimes[state.recentHead] = p.Timestamp
	state.recentCounts[binIdx]++
	state.recentHead = (state.recentHead + 1) % dirbfRecentSize
	if state.recentN < dirbfRecentSize {
		state.recentN++
	}

	// Both windows must be full before the BF has meaning. Skipping the
	// computation here avoids the lgamma calls during the structural fill
	// stretch (~ref+recent ticks after warmup).
	if state.refN < dirbfRefSize || state.recentN < dirbfRecentSize {
		return nil
	}

	// Closed-form log Bayes factor under symmetric Dirichlet(α=1).
	var sumCounts [dirbfNumBins]int
	for i := 0; i < dirbfNumBins; i++ {
		sumCounts[i] = state.refCounts[i] + state.recentCounts[i]
	}
	logBF := dirbfLogMarginal(state.refCounts[:]) +
		dirbfLogMarginal(state.recentCounts[:]) -
		dirbfLogMarginal(sumCounts[:])

	// Persistence counter: count consecutive ticks above the BF threshold.
	if logBF > d.Lambda {
		state.persistCnt++
	} else {
		state.persistCnt = 0
	}

	// Recovery state machine. Clear inAlert when logBF stays below
	// 0.5·Lambda for RecoveryPoints consecutive ticks.
	if state.inAlert {
		if logBF < 0.5*d.Lambda {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.persistCnt = 0
				state.recoveryCnt = 0
			}
		} else {
			state.recoveryCnt = 0
		}
	}

	// Fire decision. inAlert suppresses re-fires; the cooldown gate gives
	// the same suppression a wall-clock floor for the period between
	// recovery and a genuine new shift.
	if state.persistCnt < d.PersistenceK || state.inAlert {
		return nil
	}
	if state.lastFireTime != 0 && p.Timestamp-state.lastFireTime < d.CooldownSec {
		return nil
	}

	state.inAlert = true
	state.lastFireTime = p.Timestamp
	state.persistCnt = 0
	state.recoveryCnt = 0
	return d.makeAnomaly(state, p, series, agg, logBF)
}

// dirbfBin returns the bin index in [0, len(edges)] for x given the inner-edge
// sequence `edges`. The leftmost bin (0) covers (-inf, edges[0]); each
// successive bin i covers [edges[i-1], edges[i]); the rightmost bin
// (len(edges)) covers [edges[last], +inf).
func dirbfBin(x float64, edges []float64) int {
	for i, edge := range edges {
		if x < edge {
			return i
		}
	}
	return len(edges)
}

// dirbfLogMarginal computes the log marginal likelihood of a count vector
// under a symmetric Dirichlet(α=dirbfPriorAlpha) prior:
//
//	log P(counts) = lgamma(K·α) − lgamma(K·α + n) + Σ_i [lgamma(α + counts[i]) − lgamma(α)]
//
// The lgamma(α) term simplifies to 0 for α=1 but is kept explicit so the
// function reads as the standard Dirichlet-multinomial / Beta-binomial
// closed form (Gelman BDA3 §3.4) without having to special-case α.
func dirbfLogMarginal(counts []int) float64 {
	K := float64(len(counts))
	alpha := dirbfPriorAlpha
	n := 0
	for _, c := range counts {
		n += c
	}
	lgK, _ := math.Lgamma(K * alpha)
	lgKn, _ := math.Lgamma(K*alpha + float64(n))
	lgAlpha, _ := math.Lgamma(alpha)
	out := lgK - lgKn
	for _, c := range counts {
		lg, _ := math.Lgamma(alpha + float64(c))
		out += lg - lgAlpha
	}
	return out
}

// makeAnomaly builds the alert-onset anomaly for the firing tick.
func (d *DIRBFDetector) makeAnomaly(state *dirbfSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, logBF float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()

	score := logBF
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "DirichletBF distribution shift: " + displayName,
		Description: fmt.Sprintf("logBF=%.2f, λ=%.2f, K=%d, persist=%d",
			logBF, d.Lambda, dirbfNumBins, d.PersistenceK),
		Timestamp:           p.Timestamp,
		Score:               &score,
		SamplingIntervalSec: dirbfRecentMedianInterval(state.recentTimes[:], state.recentHead, state.recentN),
		DebugInfo: &observer.AnomalyDebugInfo{
			Threshold:    d.Lambda,
			CurrentValue: logBF,
		},
	}
}

// dirbfRecentMedianInterval computes the median gap between consecutive
// timestamps in the recent ring. Returns 0 if fewer than 2 valid timestamps,
// matching medianPointInterval's contract for short series. Allocates a
// (n-1) int64 slice — only called from the rare fire path.
func dirbfRecentMedianInterval(times []int64, head, n int) int64 {
	if n < 2 {
		return 0
	}
	cap := len(times)
	oldest := (head - n + cap) % cap
	intervals := make([]int64, n-1)
	prev := times[oldest]
	for i := 1; i < n; i++ {
		idx := (oldest + i) % cap
		intervals[i-1] = times[idx] - prev
		prev = times[idx]
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	return intervals[len(intervals)/2]
}

// ensureDefaults populates zero-valued fields with defaults so a zero-valued
// struct (or one with selectively-cleared fields) still works.
func (d *DIRBFDetector) ensureDefaults() {
	if d.Lambda <= 0 {
		d.Lambda = dirbfLambda
	}
	if d.PersistenceK <= 0 {
		d.PersistenceK = dirbfPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = dirbfRecoveryPts
	}
	if d.WarmupPoints <= 0 {
		d.WarmupPoints = dirbfWarmupPoints
	}
	if d.CooldownSec <= 0 {
		d.CooldownSec = dirbfCooldownSec
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[dirbfStateKey]*dirbfSeriesState)
	}
}
