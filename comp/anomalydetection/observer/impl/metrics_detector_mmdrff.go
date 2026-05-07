// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: MMDRFF detector — streaming Maximum Mean Discrepancy
// computed in a Random Fourier Feature space (Gretton, Borgwardt, Rasch,
// Schölkopf, Smola, JMLR 2012, "A Kernel Two-Sample Test"; Rahimi & Recht,
// NIPS 2007, "Random Features for Large-Scale Kernel Machines"). Compares
// two abutting W=60 windows R (reference, [-2W,-W]) and T (test, [-W,0])
// in a fixed D=64-dim RFF embedding of a Gaussian kernel. The anomaly
// score is the squared L2 distance between the empirical mean embeddings
// of the two windows, which is the kernel-MMD² up to a constant.
//
// Why this fills a gap. ScanMW/BOCPD catch mean shifts; VarShift catches
// pure variance shifts; DenRatio targets full-distribution shifts via
// histogram divergence. None covers shifts in the higher moments that the
// Gaussian kernel captures via its characteristic property — e.g. unimodal
// → bimodal at fixed mean and variance — without leaning on histogram
// noise. MMD-RFF is the kernel-mean-embedding analogue: every moment of the
// distribution shows up in the embedding, but the score is computed in a
// fixed-dimensional D=64 feature space so it stays O(D) per tick.
//
// CRITICAL design decisions:
//   - The RFF embedding (omega, b) is drawn ONCE at construction with a
//     deterministic seed so the score is reproducible across runs and
//     comparable across series. h is fixed at 1.0; we z-score the input
//     using a Welford running mean/variance so the embedding's bandwidth
//     is calibration-correct on any metric scale.
//   - A STRONG mean-stationarity gate (|meanT-meanR|/sigmaR < 0.5) is the
//     additivity gate against ScanMW/BOCPD, copied verbatim from VarShift
//     because it's the only gate that has cleared review without leakage
//     flags. Without it, joint mean+distribution shifts would fire here
//     AND in ScanMW/BOCPD, double-counting a single incident.
//
// Algorithm (per series, per aggregation):
//  1. Update Welford running mean/variance with the raw value x INCLUDING
//     this tick, then z = (x - runMean) / max(sqrt(runM2/runN), 1e-6).
//  2. Maintain two abutting windows of size W=60 (mirroring VarShift's
//     pipeline) holding z-scored values. Maintain a per-window running
//     RFF feature mean: sumPhiR, sumPhiT in [D]float64. Updates are O(D)
//     per push (add new phi, subtract evicted phi).
//  3. After both rings are full, on each new tick compute
//        diff_j = (sumPhiT[j] - sumPhiR[j]) / W
//        mmd2   = sum_j diff_j²
//        meanGap = |sumT/W - sumR/W| / sqrt(varR), varR floored at 1e-6
//  4. Trigger gates (ALL must pass):
//     (a) mmd2 ≥ MMD2Threshold (default 0.30) for ALL last K=3 ticks.
//         Calibrated for D=64, h=1, z-scored input: under H0 the
//         asymptotic null variance of mmd2 is ≈ 1/(W·D) ≈ 2.6e-4
//         (Gretton 2012 Theorem 8), so 0.30 sits ~18 standard deviations
//         above the null and clears small-sample noise.
//     (b) meanGap < MeanStationaryGate (default 0.5σ) — additivity gate.
//  5. Alert lifecycle (mirrors VarShift): on fire emit ONE anomaly, set
//     inAlert=true, copy T into R (R becomes the new normal), zero T,
//     require RecoveryPoints=10 normal ticks before the next fire.
//
// Per-tick cost: O(D) for the new tick's phi + O(D) for each eviction
// recompute (≤ 2 evictions when both rings are full) = ≤ 3·D ≈ 192 cos
// calls. ~6 µs on modern hardware. No allocations on the hot path.
// Per-(series, agg) memory: 2·W float64 rings + 2·D phi sums + 3 Welford
// scalars + K MMD ring + cursor + alert ≈ 250 floats ≈ 2 KB; about 2× the
// VarShift footprint. If a hot-path profile ever shows the cos calls
// dominating, swap to a precomputed lookup table — but the additivity
// gate stays.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Algorithm constants. Window size, RFF dimension, and persistence-K are
// state-array-shape inputs and so must be compile-time constants. Trigger
// thresholds are exposed on the detector struct so tests can adjust them
// without resizing the per-series fixed-size buffers.
const (
	mmdrffWindow             = 60
	mmdrffRFFDim             = 64
	mmdrffPersistenceK       = 3
	mmdrffMMD2Threshold      = 0.30
	mmdrffMeanStationaryGate = 0.5
	mmdrffRecoveryPoints     = 10
	// mmdrffSigmaFloor avoids div-by-zero z-scoring when the running variance
	// is (nearly) zero — e.g. constant streams during cold start.
	mmdrffSigmaFloor = 1e-6
	// mmdrffSeed is a deterministic seed for the RFF (omega, b) draw so the
	// embedding is reproducible across runs. The numeric value is arbitrary;
	// it just needs to be fixed. Spelled to evoke "MMDFFEED" since the plan
	// canon for this seed is suggestive, not actual hex.
	mmdrffSeed int64 = 0x11DFFEED
)

// mmdrffStateKey identifies per-series state by ref and aggregation.
type mmdrffStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// mmdrffSeriesState holds per-series streaming state. Fixed-size; no hot-
// path allocations.
type mmdrffSeriesState struct {
	// Two abutting windows of size W. Stored values are z-scored (x has
	// already been passed through the Welford running mean/variance).
	// Same FIFO/head semantics as VarShift/DenRatio: when full, ringTHead
	// is both "oldest in T" and "next slot to write".
	ringR     [mmdrffWindow]float64
	ringT     [mmdrffWindow]float64
	ringRHead int
	ringTHead int
	ringRN    int
	ringTN    int

	// Running raw-value sums for meanGap (operate on z, not x). Matches
	// VarShift's sum/sumSq layout.
	sumR   float64
	sumSqR float64
	sumT   float64
	sumSqT float64

	// Per-window accumulated RFF feature mean (sum of phi(z) across the
	// window). Updated O(D) per push: +phi(new) and −phi(evicted). The
	// per-tick phi is NOT cached per slot — we recompute from the stored
	// z value on eviction. This trades 2× cos calls per eviction for D×W
	// memory savings and matches the plan.
	sumPhiR [mmdrffRFFDim]float64
	sumPhiT [mmdrffRFFDim]float64

	// Welford running mean and sum-of-squared-deviations for input
	// z-scoring. Population variance ≈ runM2/runN. Updated BEFORE z-
	// scoring this tick so z is computed against the running estimate
	// INCLUDING the current sample.
	runMean float64
	runM2   float64
	runN    int

	// Last K=3 mmd² values (ring); lastMMDN counts valid entries until full.
	lastMMD     [mmdrffPersistenceK]float64
	lastMMDHead int
	lastMMDN    int

	// Alert lifecycle (same shape as VarShift).
	inAlert     bool
	recoveryCnt int

	// Cursor — same pattern as VarShift/DenRatio/BOCPD/ScanMW.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// MMDRFFTwoSampleDetector flags distribution shifts via the streaming kernel-MMD²
// between two abutting windows in a Random Fourier Feature embedding.
// Implements observer.Detector + observer.SeriesRemover.
type MMDRFFTwoSampleDetector struct {
	// MMD2Threshold is the squared-MMD value every entry of the K-tick
	// MMD² history must clear to count toward a fire. Default: 0.30 — see
	// the package-doc calibration argument.
	MMD2Threshold float64

	// MeanStationaryGate is the additivity gate: |meanT-meanR|/sqrt(varR)
	// at the trigger tick must be BELOW this. Default: 0.5. Without this
	// gate, any joint mean+distribution shift would fire here AND in
	// ScanMW/BOCPD, multiplying false positives on the same incident.
	MeanStationaryGate float64

	// PersistenceK is the number of consecutive MMD² values that must
	// clear MMD2Threshold before a fire. Default: 3 (capped by the
	// per-series ring size; setting >3 is silently capped in
	// ensureDefaults).
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks
	// that resets an active alert. Default: 10.
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Random Fourier Feature parameters. Drawn once at construction with
	// a deterministic seed (mmdrffSeed) so the embedding is reproducible
	// and shared across all series. omega_j ~ N(0, 1) (h=1), b_j ~
	// Uniform[0, 2π].
	omega [mmdrffRFFDim]float64
	b     [mmdrffRFFDim]float64

	// Per-(series, aggregation) state keyed by ref+agg.
	series map[mmdrffStateKey]*mmdrffSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports that a new series was added.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewMMDRFFTwoSampleDetector constructs an MMDRFFTwoSampleDetector with default settings and
// a deterministic RFF embedding.
func NewMMDRFFTwoSampleDetector() *MMDRFFTwoSampleDetector {
	d := &MMDRFFTwoSampleDetector{
		MMD2Threshold:      mmdrffMMD2Threshold,
		MeanStationaryGate: mmdrffMeanStationaryGate,
		PersistenceK:       mmdrffPersistenceK,
		RecoveryPoints:     mmdrffRecoveryPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[mmdrffStateKey]*mmdrffSeriesState),
	}
	d.populateRFF()
	return d
}

// populateRFF draws the RFF (omega, b) parameters from a deterministic
// PRNG. h is fixed at 1; the input is z-scored before phi() so unit-
// variance bandwidth holds across metrics.
func (d *MMDRFFTwoSampleDetector) populateRFF() {
	rng := rand.New(rand.NewSource(mmdrffSeed))
	for j := 0; j < mmdrffRFFDim; j++ {
		// omega_j ~ N(0, 1/h²) = N(0, 1) for h=1.
		d.omega[j] = rng.NormFloat64()
		// b_j ~ Uniform[0, 2π].
		d.b[j] = rng.Float64() * 2 * math.Pi
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*MMDRFFTwoSampleDetector) Name() string { return "mmdrff" }

// Reset clears all per-series state for replay/reanalysis. The (omega, b)
// embedding is preserved — it is fixed at construction, not learned.
func (d *MMDRFFTwoSampleDetector) Reset() {
	d.series = make(map[mmdrffStateKey]*mmdrffSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry
// holds ~2 KB of fixed-size streaming state, so without this teardown the
// map would grow with the cumulative count of series ever observed even
// after their storage payload is gone. Called by the engine immediately
// after timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *MMDRFFTwoSampleDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, mmdrffStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors VarShift: gen-
// cached ListSeries → bulkSeriesStatus → ForEachPoint with a count+gen
// cursor → callback applies processPoint to each new visible point.
func (d *MMDRFFTwoSampleDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := mmdrffStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &mmdrffSeriesState{}
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

// processPoint applies the streaming algorithm to a single new point.
// Returns a non-nil anomaly only on alert onset (not while still in alert).
func (d *MMDRFFTwoSampleDetector) processPoint(state *mmdrffSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// Welford update INCLUDING this sample, then z-score against the
	// post-update mean/variance. This matches the plan's "update before
	// z-scoring" rule and keeps the very first sample yielding z=0
	// rather than a runaway divide.
	state.runN++
	delta := x - state.runMean
	state.runMean += delta / float64(state.runN)
	delta2 := x - state.runMean
	state.runM2 += delta * delta2

	sigma := math.Sqrt(state.runM2 / float64(state.runN))
	if sigma < mmdrffSigmaFloor {
		sigma = mmdrffSigmaFloor
	}
	z := (x - state.runMean) / sigma

	// Pipeline push: z → T; if T was full, oldest T rolls into R; if R
	// was also full, R's oldest drops. Running raw-z sums and per-window
	// phi sums updated incrementally.
	d.pushPoint(state, z)

	// Both rings must be full before mmd² has meaning.
	if state.ringRN < mmdrffWindow || state.ringTN < mmdrffWindow {
		// Treat as a non-trigger tick for recovery accounting so any
		// outstanding alert clears even during the structural refill that
		// follows a fire.
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	w := float64(mmdrffWindow)
	var mmd2 float64
	for j := 0; j < mmdrffRFFDim; j++ {
		diff := (state.sumPhiT[j] - state.sumPhiR[j]) / w
		mmd2 += diff * diff
	}
	meanR := state.sumR / w
	meanT := state.sumT / w
	varR := state.sumSqR/w - meanR*meanR
	if varR < mmdrffSigmaFloor*mmdrffSigmaFloor {
		varR = mmdrffSigmaFloor * mmdrffSigmaFloor
	}
	meanGap := math.Abs(meanT-meanR) / math.Sqrt(varR)

	// Push mmd² into the K-slot ring.
	state.lastMMD[state.lastMMDHead] = mmd2
	state.lastMMDHead = (state.lastMMDHead + 1) % mmdrffPersistenceK
	if state.lastMMDN < mmdrffPersistenceK {
		state.lastMMDN++
	}

	// Trigger condition (all three must hold):
	//   (a) mmd² ≥ MMD2Threshold for ALL last K entries
	//   (b) meanGap < MeanStationaryGate at the trigger tick
	// (a) is the persistence check; (b) is the additivity gate against
	// ScanMW/BOCPD. Both are O(K) or O(1) so we always run them — no
	// short-circuit savings worth the branch complexity.
	triggered := false
	if state.lastMMDN >= d.PersistenceK &&
		allAboveMMDThreshold(state.lastMMD[:], d.MMD2Threshold) &&
		meanGap < d.MeanStationaryGate {
		triggered = true
	}

	if triggered {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this incident; do not re-emit until recovery.
			return nil
		}
		state.inAlert = true
		// Reset post-fire structure: copy T into R wholesale (R becomes the
		// new normal), zero T (refills over W ticks), drop the mmd ring so
		// we need K fresh values before any re-fire is even possible.
		// Running raw sums AND phi sums must be migrated in lock-step with
		// the ring contents so subsequent mmd² values stay consistent.
		copy(state.ringR[:], state.ringT[:])
		state.ringRHead = state.ringTHead
		state.ringRN = state.ringTN
		state.sumR = state.sumT
		state.sumSqR = state.sumSqT
		state.sumPhiR = state.sumPhiT // value-copy of the [D]float64 array
		for i := range state.ringT {
			state.ringT[i] = 0
		}
		state.ringTHead = 0
		state.ringTN = 0
		state.sumT = 0
		state.sumSqT = 0
		for j := range state.sumPhiT {
			state.sumPhiT[j] = 0
		}
		state.lastMMDHead = 0
		state.lastMMDN = 0
		return d.makeAnomaly(p, series, agg, mmd2, meanGap)
	}

	if state.inAlert {
		state.recoveryCnt++
		if state.recoveryCnt >= d.RecoveryPoints {
			state.inAlert = false
			state.recoveryCnt = 0
		}
	}
	return nil
}

// pushPoint advances the R/T pipeline by one observation: z enters T; if
// T was full, T's oldest is read out and pushed onto R; if R was also
// full, R's oldest drops. Running raw-z sums (sumR/sumSqR/sumT/sumSqT)
// AND per-window phi sums (sumPhiR/sumPhiT) are updated incrementally so
// both meanGap and mmd² are available in O(D) per tick.
func (d *MMDRFFTwoSampleDetector) pushPoint(state *mmdrffSeriesState, z float64) {
	if state.ringTN < mmdrffWindow {
		// T not yet full: append, no spill.
		state.ringT[state.ringTHead] = z
		state.ringTHead = (state.ringTHead + 1) % mmdrffWindow
		state.ringTN++
		state.sumT += z
		state.sumSqT += z * z
		d.addPhi(&state.sumPhiT, z)
		return
	}
	// T is full: read its oldest (at ringTHead, the next-write slot),
	// migrate to R, then place z in the vacated slot.
	oldestT := state.ringT[state.ringTHead]
	state.sumT -= oldestT
	state.sumSqT -= oldestT * oldestT
	d.subPhi(&state.sumPhiT, oldestT)
	if state.ringRN < mmdrffWindow {
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % mmdrffWindow
		state.ringRN++
		state.sumR += oldestT
		state.sumSqR += oldestT * oldestT
		d.addPhi(&state.sumPhiR, oldestT)
	} else {
		// R also full: evict its oldest (FIFO replace).
		oldestR := state.ringR[state.ringRHead]
		state.sumR -= oldestR
		state.sumSqR -= oldestR * oldestR
		d.subPhi(&state.sumPhiR, oldestR)
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % mmdrffWindow
		state.sumR += oldestT
		state.sumSqR += oldestT * oldestT
		d.addPhi(&state.sumPhiR, oldestT)
	}
	state.ringT[state.ringTHead] = z
	state.ringTHead = (state.ringTHead + 1) % mmdrffWindow
	state.sumT += z
	state.sumSqT += z * z
	d.addPhi(&state.sumPhiT, z)
}

// addPhi adds the RFF embedding of z to sum, in-place. phi_j(z) =
// sqrt(2/D) * cos(omega_j * z + b_j). O(D) per call.
func (d *MMDRFFTwoSampleDetector) addPhi(sum *[mmdrffRFFDim]float64, z float64) {
	scale := math.Sqrt(2.0 / float64(mmdrffRFFDim))
	for j := 0; j < mmdrffRFFDim; j++ {
		sum[j] += scale * math.Cos(d.omega[j]*z+d.b[j])
	}
}

// subPhi subtracts the RFF embedding of z from sum, in-place. Used when
// a window evicts a previously-stored z (we recompute phi from the stored
// scalar rather than caching D-dim phi vectors per slot).
func (d *MMDRFFTwoSampleDetector) subPhi(sum *[mmdrffRFFDim]float64, z float64) {
	scale := math.Sqrt(2.0 / float64(mmdrffRFFDim))
	for j := 0; j < mmdrffRFFDim; j++ {
		sum[j] -= scale * math.Cos(d.omega[j]*z+d.b[j])
	}
}

// allAboveMMDThreshold reports whether every MMD² entry in history clears
// the threshold. Unlike VarShift's persistentLogRatio there is no sign
// component — mmd² is non-negative by construction.
func allAboveMMDThreshold(history []float64, threshold float64) bool {
	if len(history) == 0 {
		return false
	}
	for _, v := range history {
		if v < threshold {
			return false
		}
	}
	return true
}

// makeAnomaly constructs the alert-onset anomaly. Allocates only on the
// (rare) fire path.
func (d *MMDRFFTwoSampleDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, mmd2, meanGap float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "MMD-RFF distribution shift: " + displayName,
		Description: fmt.Sprintf("%s mmd² %.4f exceeded threshold %.2f for %d ticks (meanGap %.3fσ)",
			displayName, mmd2, d.MMD2Threshold, d.PersistenceK, meanGap),
		Timestamp: p.Timestamp,
		DebugInfo: &observer.AnomalyDebugInfo{
			Threshold:      d.MMD2Threshold,
			CurrentValue:   mmd2,
			DeviationSigma: mmd2,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults. Called from
// every public method that depends on configuration so the zero-valued
// struct works.
func (d *MMDRFFTwoSampleDetector) ensureDefaults() {
	if d.MMD2Threshold <= 0 {
		d.MMD2Threshold = mmdrffMMD2Threshold
	}
	if d.MeanStationaryGate <= 0 {
		d.MeanStationaryGate = mmdrffMeanStationaryGate
	}
	if d.PersistenceK <= 0 || d.PersistenceK > mmdrffPersistenceK {
		// Capped at the per-series ring size — larger values would index
		// past the fixed-size history array.
		d.PersistenceK = mmdrffPersistenceK
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = mmdrffRecoveryPoints
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[mmdrffStateKey]*mmdrffSeriesState)
	}
	// (omega, b) are populated in NewMMDRFFTwoSampleDetector. Cover the zero-value
	// detector case (uncommon, but possible if a caller skips the
	// constructor) by detecting an all-zero omega and re-drawing.
	allZero := true
	for j := 0; j < mmdrffRFFDim; j++ {
		if d.omega[j] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		d.populateRFF()
	}
}
