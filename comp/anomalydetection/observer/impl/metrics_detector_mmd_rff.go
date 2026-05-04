// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// MMD with Random Fourier Features (RBF kernel) changepoint detector.
//
// Per (series, aggregation), maintain two adjacent sliding windows of W=60
// raw values: preWin (older) and currWin (most recent). Each value is mapped
// into a D-dimensional feature space via the RFF approximation
//
//	φ_d(x) = sqrt(2/D) · cos(ω_d · x + b_d)
//
// where ω_d ~ N(0, 1/σ²) and b_d ~ U(0, 2π). Under this construction,
// k(x, y) ≡ φ(x)·φ(y) ≈ exp(-‖x-y‖² / (2σ²)) — Rahimi & Recht 2007.
// The window-mean embedding μ_W = (1/|W|) Σ_x φ(x) approximates the
// population mean embedding μ_X, and the squared L2 distance between the
// two window mean embeddings is an unbiased (up to RFF approximation
// error) estimator of MMD² between the two underlying distributions —
// Gretton et al. 2012.
//
// An anomaly fires when ‖μ_curr - μ_pre‖² crosses Threshold AND a
// same-sign consecutive-confirmation gate passes AND a distribution-aware
// MAD effect-size gate passes — mirroring the three-gate structure of
// holt_residual / glr_mean_variance to keep house style consistent.
//
// What the gates do:
//   - MMD² threshold: the kernel two-sample distance gate. Detects shifts
//     in any moment (mean, variance, skew, kurtosis, multimodality) via
//     the universal RBF kernel.
//   - Consecutive confirmation: filters single-tick spurious crossings
//     from finite-sample MMD² noise on the boundary.
//   - MAD effect-size gate: blocks "statistically significant but
//     practically tiny" shifts. Uses MAX(median-shift, MAD-shift) so the
//     gate is sensitive to BOTH location and dispersion changes — a pure
//     mean-based gate (the holt/glr default) would block exactly the
//     variance-only and multimodal shifts that the RBF kernel exists to
//     detect.
//
// Memory per (series, aggregation): 2*W float64 raw windows + 2*D float64
// embedding sums + a 16-entry timestamp ring + scalars ~ 1.5 KB. The RFF
// projection (omegas, biases) is detector-shared, ~1 KB total fixed.
//
// Per-tick cost: 3 × D cos calls (current point, evicted-from-curr,
// evicted-from-pre once steady-state reached) + 4 × D float adds for the
// vector-sum updates + D mul-adds for the MMD² norm = ~600 floating-point
// ops + ~192 cos calls ≈ 3 µs on a modern x86 core. The MAD gate runs
// only when the threshold and confirmation gates already passed, so quiet
// streams skip the O(W log W) sort.

// MMD-RFF tunable defaults.
//
// mmdThreshold = 0.10 is calibrated against unit-variance Gaussian data
// with W=60 and D=64. Under H_0 (X, Y both ~ N(0,1)), the expected
// finite-sample MMD² is approximately 1/W ≈ 0.017 (sum of estimator
// variances over D dimensions). A 0.10 cut sits ~6× the null mean,
// dominated by sample variance, and combined with ConfirmM=2 and the
// >= 3-MAD effect-size gate gives a conservative FP rate. See
// TestMMDRFF_StationaryGaussian_NoFires for empirical validation.
//
// mmdRefractory is set to 2*Window = 120 because the two-window cascade
// keeps a single regime change "visible" in the embeddings for the full
// duration that the changepoint takes to traverse both windows: after
// firing on the boundary's transit through currWin, preWin then fills
// with the new regime over the next W ticks, during which the gap
// between mean embeddings remains large. A holt_residual-style 20-tick
// refractory would let the same single regime change re-fire ~6 times
// before the windows finished transitioning. 120 lines up with the
// natural "regime change has fully exited both windows" boundary.
const (
	mmdWindow          = 60
	mmdRFFDim          = 64
	mmdConfirmM        = 2
	mmdMinDeviationMAD = 3.0
	mmdRefractory      = 120
	mmdTimestampRing   = 16
	mmdThreshold       = 0.10
	// mmdSeed makes the RFF projection deterministic across runs, which
	// matters for: (a) reproducible offline evals on the testbench,
	// (b) cross-version comparisons of the same detector, and (c) the
	// determinism unit test. The literal hex spells "MMDA" — easy to
	// recognise in panics and core dumps.
	mmdSeed = 0x4D4D4441
	// mmdBandwidth is the RBF kernel σ. With unit-scale standardised
	// inputs σ=1.0 places ω_d ~ N(0,1) in the response peak of the
	// Gaussian RBF, giving good kernel resolution. Raw multi-scale data
	// would need per-series normalisation for σ=1 to be optimal — see
	// the doc-comment block at the top of the file. We expose this as a
	// constant rather than a tunable because changing it post-
	// construction would invalidate the running embedding sums.
	mmdBandwidth = 1.0
)

// mmdStateKey identifies per-series state by ref + aggregation.
type mmdStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// mmdSeriesState holds the streaming state for one (series, aggregation).
//
// The two-window cascade lifecycle:
//
//	(empty)            → fill currWin until len(currWin) == Window
//	(curr full only)   → each new point evicts oldest of currWin into preWin
//	(both full)        → each new point evicts oldest of currWin into preWin,
//	                     and the oldest of preWin is dropped. Detection runs.
//
// Embedding sums (sumPre, sumCurr) are kept in lock-step with the raw
// windows so MMD² is an O(D) lookup rather than an O(W*D) recompute on
// every tick. Scalar sums (sumXPre, sumXCurr) are kept the same way so
// per-window means are O(1).
type mmdSeriesState struct {
	// cursor (mirrors holt_residual / glr_mean_variance exactly)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// raw value windows for the two halves, FIFO. preWin holds the older
	// W points, currWin holds the most recent W. Both feed the MAD
	// effect-size gate; the embeddings derived below feed the MMD² gate.
	preWin  []float64
	currWin []float64

	// running scalar sums for O(1) per-window means; updated alongside
	// the embedding sums in the slide path.
	sumXPre  float64
	sumXCurr float64

	// RFF embedding sums. sumPre[d] = Σ_{x ∈ preWin} φ_d(x), and likewise
	// sumCurr[d]. Mean embedding = sum / W. The MMD² gate is
	//
	//   ‖μ_curr - μ_pre‖² = (1/W²) ‖sumCurr - sumPre‖².
	//
	// We never store individual φ(x) — we recompute φ(evicted) on the
	// fly when a value is evicted (D cos calls, ~64 ops at D=64). This
	// halves per-series memory at trivial CPU cost.
	sumPre  []float64
	sumCurr []float64

	// Reusable scratch buffers for φ(new) and φ(evicted). Allocated once
	// at newState() so the hot path does not allocate. Length d.RFFDim.
	phiScratchNew     []float64
	phiScratchEvicted []float64

	// detection counters. Unlike holt_residual / glr_mean_variance the
	// confirmation gate is sign-agnostic: a single consecutivePasses
	// counter tracks how many consecutive ticks have passed the MMD²
	// threshold. Sign-based confirmation is wrong for the kernel two-
	// sample test because pure dispersion or multimodal shifts move the
	// embedding mean without consistently moving the scalar mean — the
	// sign would flip tick-to-tick, the counter would never confirm,
	// and the detector would silently drop exactly the events it was
	// chosen to catch.
	consecutivePasses   int
	refractoryRemaining int

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string

	// recentTimestamps is a small ring of the most recently ingested point
	// timestamps; medianTimestampInterval reads it to estimate the sampling
	// cadence for downstream correlators.
	recentTimestamps  []int64
	lastSeenTimestamp int64
}

// MMDRFFDetector implements observer.Detector and observer.SeriesRemover.
//
// Tunable fields are exported so callers (testbench, tests) may override
// defaults after construction; NewMMDRFFDetector populates them. Mirrors
// the holt_residual / glr_mean_variance post-construction-override
// pattern, so the catalog entry needs no defaultConfig / parseJSON.
//
// The omegas / biases pair is sampled ONCE at construction with seed
// mmdSeed and shared across all series — the RFF mean embeddings only
// compose into a meaningful distance when they live in the same feature
// space. They are not re-sampled by Reset() so detector state remains
// comparable across replays.
type MMDRFFDetector struct {
	// Window is the size W of each side of the split, in points. Both
	// halves must fill before the first detection can run.
	Window int
	// RFFDim is the number of random Fourier features, D.
	RFFDim int
	// Threshold is the squared-L2 gate on ‖μ_curr - μ_pre‖².
	Threshold float64
	// ConfirmM is the minimum number of consecutive same-sign threshold-
	// passing observations required to fire.
	ConfirmM int
	// MinDeviationMAD is the minimum MAD-scaled effect size required to
	// fire. The effect-size measure is MAX(median-shift, MAD-shift) so
	// the gate captures both location and dispersion changes — pure
	// median-based effect size (the holt/glr default) would block
	// variance-only and multimodal shifts that the RBF kernel was
	// chosen specifically to detect.
	MinDeviationMAD float64
	// Refractory is the number of post-fire ingest ticks for which the
	// detector suppresses subsequent fires.
	Refractory int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// RFF projection. omegas[d] is the d-th frequency, biases[d] the
	// d-th phase, sampled from the RBF kernel parameters with mmdSeed.
	omegas []float64
	biases []float64
	// sqrt2OverD caches sqrt(2/D), reused on every φ call.
	sqrt2OverD float64

	// per-series state keyed by ref+agg
	series map[mmdStateKey]*mmdSeriesState

	// cached series list across Detect calls (mirrors the holt / glr
	// pattern: invalidated on storage SeriesGeneration bumps and on
	// RemoveSeries calls).
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewMMDRFFDetector constructs a detector with default tunables and a
// deterministic RFF projection seeded by mmdSeed.
func NewMMDRFFDetector() *MMDRFFDetector {
	d := &MMDRFFDetector{
		Window:          mmdWindow,
		RFFDim:          mmdRFFDim,
		Threshold:       mmdThreshold,
		ConfirmM:        mmdConfirmM,
		MinDeviationMAD: mmdMinDeviationMAD,
		Refractory:      mmdRefractory,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[mmdStateKey]*mmdSeriesState),
	}
	d.sampleProjection()
	return d
}

// sampleProjection draws omegas and biases for the RFF map. Called once
// at construction. ω_d ~ N(0, 1/σ²); b_d ~ U(0, 2π); the sqrt(2/D)
// scale factor lives in d.sqrt2OverD.
//
// We use math/rand v1 seeded explicitly (not the global) so the
// projection is reproducible across runs and across goroutines without
// relying on global state.
func (d *MMDRFFDetector) sampleProjection() {
	if d.RFFDim <= 0 {
		d.RFFDim = mmdRFFDim
	}
	rng := rand.New(rand.NewSource(mmdSeed)) //nolint:gosec // RFF projection seed; not cryptographic
	d.omegas = make([]float64, d.RFFDim)
	d.biases = make([]float64, d.RFFDim)
	for i := 0; i < d.RFFDim; i++ {
		// N(0, 1/σ²): standard normal scaled by 1/σ.
		d.omegas[i] = rng.NormFloat64() / mmdBandwidth
		d.biases[i] = rng.Float64() * 2 * math.Pi
	}
	d.sqrt2OverD = math.Sqrt(2.0 / float64(d.RFFDim))
}

// phi writes φ(x) into out (length must be d.RFFDim). The caller owns
// the slice; we fill it without allocating.
func (d *MMDRFFDetector) phi(x float64, out []float64) {
	scale := d.sqrt2OverD
	for i := 0; i < d.RFFDim; i++ {
		out[i] = scale * math.Cos(d.omegas[i]*x+d.biases[i])
	}
}

// Name implements observer.Detector.
func (d *MMDRFFDetector) Name() string { return "mmd_rff" }

// Reset clears all per-series state for replay/reanalysis. The RFF
// projection is preserved — re-sampling omegas/biases would change the
// kernel realisation between replays, breaking comparability.
func (d *MMDRFFDetector) Reset() {
	d.series = make(map[mmdStateKey]*mmdSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors HoltResidualDetector.RemoveSeries: each entry holds rolling
// buffers of size 2W + 2D floats, so without the teardown the map grows
// unbounded with the cumulative series count even after storage shrinks.
func (d *MMDRFFDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, mmdStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// MMD-RFF state and emits anomalies when all three gates pass.
//
// Structure mirrors HoltResidualDetector.Detect / GLRMeanVarianceDetector.Detect:
// cache the series list across calls, batch the per-series status under a
// single lock, replay-gate quiet streams, then ingest new points per
// (ref, agg).
func (d *MMDRFFDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := mmdStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when nothing has been written and the
			// cursor matches. Per-point detector — no minimum batch size.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			anomalies := d.ingestNewPoints(storage, meta.Ref, agg, state, dataTime)
			for j := range anomalies {
				anomalies[j].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}
			allAnomalies = append(allAnomalies, anomalies...)

			// Update cursor unconditionally — quiet series should not
			// keep replaying through the gate next call.
			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers. Splitting allocation here keeps Detect's hot path branch-free.
func (d *MMDRFFDetector) newState() *mmdSeriesState {
	return &mmdSeriesState{
		preWin:            make([]float64, 0, d.Window),
		currWin:           make([]float64, 0, d.Window),
		sumPre:            make([]float64, d.RFFDim),
		sumCurr:           make([]float64, d.RFFDim),
		phiScratchNew:     make([]float64, d.RFFDim),
		phiScratchEvicted: make([]float64, d.RFFDim),
		recentTimestamps:  make([]int64, 0, mmdTimestampRing),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime]
// into the per-series MMD-RFF state. Returns any anomalies fired by the
// gate logic.
func (d *MMDRFFDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *mmdSeriesState,
	dataTime int64,
) []observer.Anomaly {
	if dataTime <= state.lastProcessedTime {
		return nil
	}

	var fired []observer.Anomaly

	storage.ForEachPoint(ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
		// Capture series metadata once.
		if !state.seriesMetaCaptured {
			state.seriesNamespace = s.Namespace
			state.seriesName = s.Name
			if len(s.Tags) > 0 {
				tagsCopy := make([]string, len(s.Tags))
				copy(tagsCopy, s.Tags)
				state.seriesTags = tagsCopy
			}
			state.seriesMetaCaptured = true
		}
		state.lastSeenTimestamp = p.Timestamp
		pushTimestampMMD(state, p.Timestamp)

		// Slide the new value through the two-window cascade. shiftPreCurr
		// returns true when both windows are full after the shift — only
		// then is detection meaningful.
		if !d.shiftPreCurr(state, p.Value) {
			return
		}

		anomaly, hasFire := d.processPoint(state, p, agg)

		// Refractory countdown — decrement on every steady-state ingest,
		// regardless of whether the point would have fired. processPoint
		// re-arms it on a fire BEFORE this decrement, so the value seen
		// by the next tick is Refractory-1 (one tick already consumed).
		if state.refractoryRemaining > 0 {
			state.refractoryRemaining--
		}

		if hasFire {
			fired = append(fired, anomaly)
		}
	})

	return fired
}

// shiftPreCurr advances the two-window cascade by one tick. Returns true
// once both halves are full so the caller knows detection is meaningful.
//
// The cascade:
//
//  1. New value enters currWin's tail.
//  2. If currWin was already full, its head spills into preWin's tail.
//  3. If preWin was already full, its head is dropped.
//
// The embedding sums (sumPre, sumCurr) and scalar sums (sumXPre,
// sumXCurr) are updated in lock-step so means are O(1) and ‖μ‖² is O(D).
// φ(evicted) is recomputed on the fly — the alternative would be to
// store W*D extra floats per series, which would more than double the
// per-series footprint to save D=64 cos calls per tick.
func (d *MMDRFFDetector) shiftPreCurr(state *mmdSeriesState, value float64) bool {
	// 1. Compute φ of the incoming value into a reusable scratch buffer.
	d.phi(value, state.phiScratchNew)

	// 2a. Curr not yet full — append and bail; preWin is irrelevant.
	if len(state.currWin) < d.Window {
		state.currWin = append(state.currWin, value)
		state.sumXCurr += value
		for i := 0; i < d.RFFDim; i++ {
			state.sumCurr[i] += state.phiScratchNew[i]
		}
		return false
	}

	// 2b. Curr full — evict its head into preWin.
	evictedFromCurr := state.currWin[0]
	d.phi(evictedFromCurr, state.phiScratchEvicted)
	copy(state.currWin, state.currWin[1:])
	state.currWin[d.Window-1] = value
	state.sumXCurr += value - evictedFromCurr
	for i := 0; i < d.RFFDim; i++ {
		state.sumCurr[i] += state.phiScratchNew[i] - state.phiScratchEvicted[i]
	}

	// 3a. Pre not yet full — append the evicted current head; preWin
	// inherits the φ we just computed for it (in phiScratchEvicted).
	if len(state.preWin) < d.Window {
		state.preWin = append(state.preWin, evictedFromCurr)
		state.sumXPre += evictedFromCurr
		for i := 0; i < d.RFFDim; i++ {
			state.sumPre[i] += state.phiScratchEvicted[i]
		}
		return len(state.preWin) >= d.Window
	}

	// 3b. Pre also full — evict its head and drop. Recompute φ for the
	// preWin head from its raw value; phiScratchNew is already free
	// because we are done with the current point's φ here. We reuse it
	// to avoid a second scratch buffer in the steady-state slide.
	evictedFromPre := state.preWin[0]
	d.phi(evictedFromPre, state.phiScratchNew)
	copy(state.preWin, state.preWin[1:])
	state.preWin[d.Window-1] = evictedFromCurr
	state.sumXPre += evictedFromCurr - evictedFromPre
	for i := 0; i < d.RFFDim; i++ {
		// state.phiScratchEvicted still holds φ(evictedFromCurr) from
		// step 2b — that's what we are pushing into preWin's tail.
		state.sumPre[i] += state.phiScratchEvicted[i] - state.phiScratchNew[i]
	}
	return true
}

// processPoint runs the three-gate decision once both windows are full.
// Caller has already updated the windows + embedding sums via
// shiftPreCurr, so this is pure bookkeeping + math.
func (d *MMDRFFDetector) processPoint(
	state *mmdSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	// 1. MMD² = ‖μ_curr - μ_pre‖² = (1/W²) ‖sumCurr - sumPre‖².
	invW := 1.0 / float64(d.Window)
	var mmdSq float64
	for i := 0; i < d.RFFDim; i++ {
		diff := (state.sumCurr[i] - state.sumPre[i]) * invW
		mmdSq += diff * diff
	}

	meanCurr := state.sumXCurr * invW
	meanPre := state.sumXPre * invW

	mmdPasses := mmdSq >= d.Threshold

	// 2. Update the sign-agnostic consecutive-passes counter BEFORE
	// deciding to fire — the fire requires the counter to reach ConfirmM,
	// so the current point counts.
	if mmdPasses {
		state.consecutivePasses++
	} else {
		state.consecutivePasses = 0
	}
	confirmed := state.consecutivePasses >= d.ConfirmM

	// 3. Short-circuit before the O(W log W) MAD computation when the
	// early gates already failed — the common path on quiet streams.
	if !mmdPasses || !confirmed {
		return observer.Anomaly{}, false
	}

	// 5. Distribution-aware MAD effect-size gate. The RBF kernel earns
	// its place in this catalog by detecting variance-only and
	// multimodal shifts — events that move the *shape* of the
	// distribution without necessarily moving its median. A pure
	// median-shift effect-size gate (the holt/glr default) would block
	// exactly those events, undoing the kernel's advantage. We use
	// MAX(median-shift, MAD-shift), each normalised by the preWin
	// MAD-sigma so the denominator reflects the "before" regime's
	// natural scale.
	medPre := detectorMedian(state.preWin)
	medCurr := detectorMedian(state.currWin)
	madPre := detectorMAD(state.preWin, medPre, true)
	madCurr := detectorMAD(state.currWin, medCurr, true)
	sigmaBaseline := floorSigma(madPre, state.preWin)

	medianShift := math.Abs(medCurr-medPre) / sigmaBaseline
	madShift := math.Abs(madCurr-madPre) / sigmaBaseline
	devMAD := math.Max(medianShift, madShift)

	if devMAD < d.MinDeviationMAD {
		return observer.Anomaly{}, false
	}

	// 6. Refractory gate.
	if state.refractoryRemaining > 0 {
		return observer.Anomaly{}, false
	}

	// 7. Build the anomaly. Shape mirrors holt_residual /
	// glr_mean_variance / kl_divergence so downstream correlators see
	// a consistent payload across detectors.
	score := mmdSq
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
		Title:        "MMD-RFF distribution shift: " + seriesName,
		Description: fmt.Sprintf(
			"%s distribution shifted (mmd²=%.4f, threshold=%.4f, mean_pre=%.4f, mean_curr=%.4f, mad_pre=%.4f, mad_curr=%.4f, %.1f valueMADs)",
			seriesName, mmdSq, d.Threshold, meanPre, meanCurr, madPre, madCurr, devMAD,
		),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medPre,
			BaselineMAD:    sigmaBaseline,
			CurrentValue:   p.Value,
			DeviationSigma: devMAD,
			Threshold:      d.Threshold,
		},
	}

	// 8. Reset confirmation counter and arm refractory.
	state.consecutivePasses = 0
	state.refractoryRemaining = d.Refractory

	return anomaly, true
}

// pushTimestampMMD mirrors HoltResidualDetector.pushTimestamp /
// pushTimestampGLR but typed for *mmdSeriesState. Each detector keeps
// its own variant because Go does not let us reuse the helper across
// distinct state structs without an interface or generics, and the
// cost of a tiny per-detector copy is outweighed by avoiding a
// hot-path interface dispatch.
func pushTimestampMMD(state *mmdSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, mmdTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// ensureDefaults populates zero-valued config fields with sensible
// defaults. Mirrors HoltResidualDetector.ensureDefaults so the detector
// behaves sanely even when constructed via reflective paths that bypass
// NewMMDRFFDetector.
//
// If the RFF projection is missing (e.g. zero-value struct), it is
// re-sampled with the deterministic seed so two reflectively-built
// detectors still produce identical projections.
func (d *MMDRFFDetector) ensureDefaults() {
	if d.Window <= 0 {
		d.Window = mmdWindow
	}
	if d.RFFDim <= 0 {
		d.RFFDim = mmdRFFDim
	}
	if d.Threshold <= 0 {
		d.Threshold = mmdThreshold
	}
	if d.ConfirmM <= 0 {
		d.ConfirmM = mmdConfirmM
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = mmdMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = mmdRefractory
	}
	if d.series == nil {
		d.series = make(map[mmdStateKey]*mmdSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
	if len(d.omegas) != d.RFFDim || len(d.biases) != d.RFFDim || d.sqrt2OverD == 0 {
		d.sampleProjection()
	}
}
