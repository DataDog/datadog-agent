// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl: EnergyDist detector — streaming univariate energy-
// distance two-sample test between two abutting windows R (reference,
// [-2m,-m]) and T (test, [-m,0]) of size m=32.
//
// Statistic. The energy distance between two equal-size 1D samples
// (Székely & Rizzo 2004, "Testing for equal distributions in high
// dimension"; closed form in Rizzo & Székely 2016, "Energy distance",
// WIREs CompStat) is
//
//	E(R, T) = (1/m) · ((2/m)·A − (1/m)·B − (1/m)·C)
//
// with A = ΣΣ |R_i − T_j|, B = ΣΣ |R_i − R_j|, C = ΣΣ |T_i − T_j|.
// Implementation note: B and C are exactly twice the i<j sums of |R_i−R_j|
// and |T_i−T_j|, so we walk only j>i to halve the inner-loop work.
//
// Why this fills a gap. Wasserstein-1D (already shipped) is a sorted
// optimal-transport distance; MMD-RFF (already shipped) is a kernel-bandwidth
// embedding distance. The energy distance is a parameter-free distance metric
// whose B/C self-similarity terms penalize within-window dispersion that
// stays stable across a pure mean shift, so it is structurally less
// correlated with mean-shift detectors than Wasserstein. It is sensitive to
// both location and shape changes.
//
// Additivity gate (CRITICAL — keeps the detector additive to scanmw/bocpd).
// On a pure mean shift the energy statistic naturally rises along with W₁
// and the kernel-norm metrics, which would cause co-firing on the same
// incident and inflate per-incident FP counts. We mirror varshift's
// |meanT−meanR|/σR < 0.5 gate (varshift.go:142–146) so the detector fires
// only when the shift is NOT explained by a mean change scanmw/bocpd would
// already cover. This gate is the explicit fix for the recurring
// shopify/postgresql ΔF1 collapse seen across exp-0023, 0026, 0027.
//
// Threshold calibration. The fire threshold eCritical is calibrated ONCE
// per series at the moment both windows fill, by drawing B=200 random
// permutations of the 64-element pool (R||T) into two halves and computing
// the energy statistic on each. eCritical = factor · p95(bootstrap), with
// factor = 2.0 by default. This pays a one-time O(B·m²) cost per series
// (≈200k ops at m=32) and removes any per-tick permutation cost.
//
// Per-tick cost on a primed series. m² absolute differences for A
// (1024 ops) plus 2·(m·(m−1)/2) = m·(m−1) ops for B+C (≈992 ops), for
// roughly 2k fp ops per tick — comparable to denratio's per-tick budget.
// Per-(series, agg) memory: 2·m floats (rings) + K=3 floats (E history) +
// scalars ≈ 600 B.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Algorithm constants. Window size and persistence ring length are baked into
// the per-series state struct as fixed-size arrays, so they must be
// compile-time constants. Trigger constants are the defaults; tests may
// override them on the detector struct.
const (
	energyWindow        = 32
	energyEHistory      = 3
	energyRecovery      = 10
	energyMeanGate      = 0.5
	energyECritFactor   = 2.0
	energyBootstrapB    = 200
	energyBootstrapQ    = 0.95
	energyVarianceFloor = 1e-12
)

// energydistStateKey identifies per-series state by ref and aggregation.
type energydistStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// energydistSeriesState holds per-(series, aggregation) streaming state.
// Fixed-size; no hot-path allocations after the one-time bootstrap.
type energydistSeriesState struct {
	// Two abutting windows of size m. ringR holds positions [-2m, -m];
	// ringT holds positions [-m, 0]. Same FIFO/head semantics as VarShift:
	// when full, ringTHead is both "oldest in T" and "next slot to write".
	ringR     [energyWindow]float64
	ringT     [energyWindow]float64
	ringRHead int
	ringTHead int
	ringRN    int
	ringTN    int

	// Running sums for O(1) mean updates. Variance for σR is computed via
	// a single Welford pass over the ringR array (m=32 ops, ~400 ns) only
	// on ticks where E is actually evaluated.
	sumR float64
	sumT float64

	// Last K=energyEHistory energy statistics (ring); eN counts valid
	// entries until the ring is full.
	lastE [energyEHistory]float64
	eHead int
	eN    int

	// One-time per-series critical value. Set by bootstrapEnergyNull when
	// both rings first fill; subsequent ticks reuse it.
	eCritical float64

	// Alert lifecycle: inAlert suppresses re-emission while a regime shift
	// is ongoing; recoveryCnt counts consecutive non-triggering ticks toward
	// clearing the alert. After firing we mirror varshift: T → R, T zeroed,
	// E history cleared, so the detector is structurally near zero until T
	// refills (~m ticks).
	inAlert     bool
	recoveryCnt int

	// Cursor — same pattern as VarShift/DenRatio/BOCPD/ScanMW.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
}

// EnergyDistDetector flags distribution-shape shifts via the streaming
// 1D energy statistic between two abutting equal-size windows. Implements
// observer.Detector + observer.SeriesRemover.
type EnergyDistDetector struct {
	// MeanStationaryGate is the additivity gate: |meanT−meanR|/σR at the
	// trigger tick must be BELOW this. Default: 0.5. Without this gate, any
	// joint mean+shape shift would fire here AND in scanmw/bocpd,
	// multiplying false positives on the same incident.
	MeanStationaryGate float64

	// PersistenceK is the number of consecutive E values that must clear
	// eCritical before a fire. Default: 3. Fixed by the per-series ring
	// size; setting >3 is silently capped at 3 in ensureDefaults.
	PersistenceK int

	// RecoveryPoints is the number of consecutive non-triggering ticks that
	// resets an active alert. Default: 10.
	RecoveryPoints int

	// ECritFactor multiplies the bootstrap p95 of E under the null to
	// produce the per-series fire threshold. Default: 2.0. Tunable via
	// fallback note in the design plan: drop to 1.5 for more recall, raise
	// to 2.5 for fewer FPs.
	ECritFactor float64

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-(series, aggregation) state.
	series map[energydistStateKey]*energydistSeriesState

	// Cache the discovered series list across Detect calls. Refreshed when
	// storage reports a generation change.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewEnergyDistDetector constructs an EnergyDistDetector with default
// settings.
func NewEnergyDistDetector() *EnergyDistDetector {
	return &EnergyDistDetector{
		MeanStationaryGate: energyMeanGate,
		PersistenceK:       energyEHistory,
		RecoveryPoints:     energyRecovery,
		ECritFactor:        energyECritFactor,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[energydistStateKey]*energydistSeriesState),
	}
}

// Name returns the detector name used by the catalog and reporters.
func (*EnergyDistDetector) Name() string { return "energydist" }

// Reset clears all per-series state for replay/reanalysis.
func (d *EnergyDistDetector) Reset() {
	d.series = make(map[energydistStateKey]*energydistSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed. Each entry holds
// ~600 B of fixed-size streaming state, so without this teardown the map
// would grow with the cumulative count of series ever observed even after
// their storage payload is gone. Called by the engine immediately after
// timeSeriesStorage.RemoveSeriesByKeys returns the freed refs.
func (d *EnergyDistDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, energydistStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration mirrors VarShift/Wasserstein
// verbatim: gen-cached ListSeries → bulkSeriesStatus → ForEachPoint with a
// count+gen cursor → callback applies processPoint to each new visible point.
func (d *EnergyDistDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := energydistStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &energydistSeriesState{}
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
func (d *EnergyDistDetector) processPoint(state *energydistSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	x := p.Value

	// Pipeline push: x → T; if T was full, oldest T rolls into R; if R was
	// also full, R's oldest drops. Running sums maintained incrementally.
	d.pushPoint(state, x)

	// Both rings must be full before E has meaning.
	if state.ringRN < energyWindow || state.ringTN < energyWindow {
		// Treat as a non-trigger tick for recovery accounting so any
		// outstanding alert clears even during the structural refill that
		// follows a fire (T is zeroed and refills over m ticks).
		if state.inAlert {
			state.recoveryCnt++
			if state.recoveryCnt >= d.RecoveryPoints {
				state.inAlert = false
				state.recoveryCnt = 0
			}
		}
		return nil
	}

	// One-time per-series bootstrap of the null distribution. Set ONCE the
	// first time both rings are full; subsequent ticks reuse the cached
	// eCritical with no per-tick permutation cost.
	if state.eCritical == 0 {
		d.bootstrapEnergyNull(state)
	}

	e := energyStatistic(state.ringR[:], state.ringT[:])

	m := float64(energyWindow)
	meanR := state.sumR / m
	meanT := state.sumT / m
	varR := energydistRingVariance(state.ringR[:], meanR)
	if varR < energyVarianceFloor {
		varR = energyVarianceFloor
	}
	sigmaR := math.Sqrt(varR)
	meanGap := math.Abs(meanT-meanR) / sigmaR

	// Push E into the K-slot ring.
	state.lastE[state.eHead] = e
	state.eHead = (state.eHead + 1) % energyEHistory
	if state.eN < energyEHistory {
		state.eN++
	}

	// Trigger gates (ALL must hold):
	//  (i) eN ≥ K AND ALL last K E values ≥ eCritical (persistence)
	// (ii) meanGap < MeanStationaryGate (additivity gate vs scanmw/bocpd)
	triggered := false
	if state.eN >= d.PersistenceK &&
		allEnergyAboveCritical(state.lastE[:], state.eCritical) &&
		meanGap < d.MeanStationaryGate {
		triggered = true
	}

	if triggered {
		state.recoveryCnt = 0
		if state.inAlert {
			// Already alerting on this incident; do not re-emit until recovery.
			return nil
		}
		// Capture eCritical for the anomaly description before the cold-restart
		// zeroes it.
		eCritical := state.eCritical
		state.inAlert = true
		// Post-fire structural reset. Unlike varshift (which has a fixed
		// log-ratio threshold and can simply migrate T → R), energydist's
		// trigger threshold eCritical is calibrated by the per-series
		// bootstrap from the original pool. Migrating only T → R while
		// keeping eCritical from the pre-fire pool causes spurious second
		// fires once T refills with post-shift data: R retains a few
		// pre-shift samples (FIFO position-pinned for ~m ticks) which inflate
		// E(R, T) above an eCritical that no longer reflects the right null.
		//
		// The clean fix is a full cold-restart of the state machine: zero
		// both rings, both running sums, the E history, AND eCritical. This
		// forces 2m=64 ticks of post-fire warmup before any new fire is even
		// possible, during which the recovery handshake (recoveryCnt ≥ 10)
		// clears inAlert. After warmup the bootstrap re-runs on a fresh,
		// fully-post-shift pool so the next eCritical reflects the new
		// regime. This is the principled deviation from the plan's
		// "mirror varshift fire-recovery exactly" — varshift doesn't have a
		// data-driven threshold and so doesn't suffer the same issue.
		for i := range state.ringR {
			state.ringR[i] = 0
		}
		for i := range state.ringT {
			state.ringT[i] = 0
		}
		state.ringRHead = 0
		state.ringTHead = 0
		state.ringRN = 0
		state.ringTN = 0
		state.sumR = 0
		state.sumT = 0
		for i := range state.lastE {
			state.lastE[i] = 0
		}
		state.eHead = 0
		state.eN = 0
		state.eCritical = 0
		return d.makeAnomaly(p, series, agg, e, eCritical, meanGap)
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

// pushPoint advances the R/T pipeline by one observation: x enters T; if T
// was full, T's oldest is read out and pushed onto R; if R was also full,
// R's oldest drops. Running sums updated incrementally so meanR/meanT are
// always available in O(1). Mirrors varshift.pushPoint without the
// sum-of-squares maintenance (variance is computed lazily via Welford on
// the ringR array, only when E is evaluated).
func (d *EnergyDistDetector) pushPoint(state *energydistSeriesState, x float64) {
	if state.ringTN < energyWindow {
		// T not yet full: append, no spill.
		state.ringT[state.ringTHead] = x
		state.ringTHead = (state.ringTHead + 1) % energyWindow
		state.ringTN++
		state.sumT += x
		return
	}
	// T is full: read its oldest (at ringTHead, the next-write slot),
	// migrate to R, then place x in the vacated slot.
	oldestT := state.ringT[state.ringTHead]
	state.sumT -= oldestT
	if state.ringRN < energyWindow {
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % energyWindow
		state.ringRN++
		state.sumR += oldestT
	} else {
		// R also full: evict its oldest (FIFO replace).
		oldestR := state.ringR[state.ringRHead]
		state.sumR -= oldestR
		state.ringR[state.ringRHead] = oldestT
		state.ringRHead = (state.ringRHead + 1) % energyWindow
		state.sumR += oldestT
	}
	state.ringT[state.ringTHead] = x
	state.ringTHead = (state.ringTHead + 1) % energyWindow
	state.sumT += x
}

// energyStatistic computes E(R, T) = (1/m) · ((2/m)·A − (1/m)·B − (1/m)·C)
// for two equal-size 1D samples. Uses the i<j symmetry to halve the inner-
// loop work for B and C: |R_i−R_j| = |R_j−R_i| so we compute each unordered
// pair once and double-count.
//
// Total fp ops at m=32: m² for A (1024) + 2·m·(m−1)/2 for B+C (992) ≈ 2k.
func energyStatistic(R, T []float64) float64 {
	m := len(R)
	if m == 0 || len(T) != m {
		return 0
	}
	var a, b, c float64
	for i := 0; i < m; i++ {
		ri := R[i]
		ti := T[i]
		for j := 0; j < m; j++ {
			a += math.Abs(ri - T[j])
		}
		for j := i + 1; j < m; j++ {
			b += 2 * math.Abs(ri-R[j])
			c += 2 * math.Abs(ti-T[j])
		}
	}
	fm := float64(m)
	return (1.0 / fm) * (2*a - b - c) / fm
}

// energydistRingVariance returns the population variance of values about
// the given mean. A single linear pass — m=32 ops, ≈400 ns. Called only on
// ticks where E is actually evaluated, not on every push. Named to avoid a
// collision with the broader windowVariance helper used by scanmw/SST that
// computes its own mean internally.
func energydistRingVariance(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var s float64
	for _, v := range values {
		dx := v - mean
		s += dx * dx
	}
	return s / float64(len(values))
}

// allEnergyAboveCritical reports whether every entry of the K-slot ring is
// at least the critical threshold. Energy distance is non-negative by
// construction so we don't need a sign check (cf. varshift, where logRatio
// can flip sign across regimes).
func allEnergyAboveCritical(history []float64, threshold float64) bool {
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

// bootstrapEnergyNull computes the per-series fire threshold by drawing
// B=energyBootstrapB random permutations of the pool R||T into two
// equal halves, computing the energy statistic for each split, and taking
// factor · p95 as eCritical. Allocates ONCE per series at warmup completion.
//
// Determinism: the RNG is seeded from a fixed function of pool length so
// runs are reproducible for tests. The pool *contents* differ per series, so
// the resulting eCritical still reflects the per-series scale even though
// the shuffle order is shared across series.
func (d *EnergyDistDetector) bootstrapEnergyNull(state *energydistSeriesState) {
	const poolSize = 2 * energyWindow
	pool := make([]float64, poolSize)
	copy(pool, state.ringR[:])
	copy(pool[energyWindow:], state.ringT[:])

	// nolint:gosec — math/rand is deliberate: this is statistical resampling,
	// not crypto. Seeded deterministically so tests are reproducible.
	rng := rand.New(rand.NewSource(int64(len(pool))))
	es := make([]float64, energyBootstrapB)
	var half1 [energyWindow]float64
	var half2 [energyWindow]float64
	for b := 0; b < energyBootstrapB; b++ {
		rng.Shuffle(poolSize, func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		copy(half1[:], pool[:energyWindow])
		copy(half2[:], pool[energyWindow:])
		es[b] = energyStatistic(half1[:], half2[:])
	}
	sort.Float64s(es)
	qIdx := int(math.Floor(energyBootstrapQ * float64(energyBootstrapB)))
	if qIdx >= energyBootstrapB {
		qIdx = energyBootstrapB - 1
	}
	p95 := es[qIdx]
	state.eCritical = d.ECritFactor * p95
	if state.eCritical <= 0 {
		// Defensive: a perfectly constant pool yields zero bootstrap E. Set a
		// tiny positive sentinel so we don't perpetually re-bootstrap on
		// every tick; the algorithm will simply not fire on a constant
		// series, which is the desired behaviour.
		state.eCritical = energyVarianceFloor
	}
}

// makeAnomaly constructs the alert-onset anomaly. Allocates only on the
// (rare) fire path.
func (d *EnergyDistDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, e, eCritical, meanGap float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	score := e / eCritical
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "EnergyDist distribution shift: " + displayName,
		Description: fmt.Sprintf(
			"%s energy statistic %.4g exceeded critical %.4g for %d ticks (meanGap=%.2fσ, factor=%.2f)",
			displayName, e, eCritical, d.PersistenceK, meanGap, d.ECritFactor,
		),
		Timestamp: p.Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			Threshold:      eCritical,
			CurrentValue:   e,
			DeviationSigma: score,
		},
	}
}

// ensureDefaults populates zero-valued fields with defaults so a zero-valued
// struct is usable. Mirrors varshift.ensureDefaults.
func (d *EnergyDistDetector) ensureDefaults() {
	if d.MeanStationaryGate <= 0 {
		d.MeanStationaryGate = energyMeanGate
	}
	if d.PersistenceK <= 0 || d.PersistenceK > energyEHistory {
		// Capped at the per-series ring size — larger values would index
		// past the fixed-size history array.
		d.PersistenceK = energyEHistory
	}
	if d.RecoveryPoints <= 0 {
		d.RecoveryPoints = energyRecovery
	}
	if d.ECritFactor <= 0 {
		d.ECritFactor = energyECritFactor
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	}
	if d.series == nil {
		d.series = make(map[energydistStateKey]*energydistSeriesState)
	}
}
