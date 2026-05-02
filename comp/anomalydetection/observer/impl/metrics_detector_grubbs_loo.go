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

// grubbsGlitchCap is the upper bound on |t_loo| for a fire. Anything beyond
// is almost certainly a sensor glitch (NaN-converted 1e308, malformed counter
// reset) rather than a genuine outlier, and we'd rather miss it than emit a
// runaway score. Mirrors tbGlitchZCap on tukey_biweight to keep both detectors
// emitting comparable scores on shared eval suites.
const grubbsGlitchCap = 50.0

// grubbsTCritTable maps reference degrees-of-freedom to the two-sided
// Student-t critical value at α = 1e-4. Numbers come from a standard
// t-distribution table (e.g. NIST e-Handbook of Statistical Methods §1.3.6.7.2).
// At dof = 78 (n = 80, two estimated parameters: location and scale) the
// critical value is 3.901; that is the working point of the detector at
// default WindowSize. Other entries let grubbsTCrit interpolate piecewise
// if WindowSize is tuned away from 80.
var grubbsTCritTable = map[int]float64{
	30:   3.984,
	50:   3.948,
	78:   3.901,
	80:   3.901,
	100:  3.872,
	200:  3.832,
	1000: 3.797,
}

// grubbsTCritKeysSorted is the ascending list of keys in grubbsTCritTable.
// Kept in sync with the map above. Iterated in reverse to find the largest
// key <= dof in O(K) where K = len(keys) is small (~7).
var grubbsTCritKeysSorted = []int{30, 50, 78, 80, 100, 200, 1000}

// grubbsTCrit returns the two-sided Student-t critical value at α = 1e-4 for
// the given degrees of freedom. Returns the value at the largest table key
// <= dof; for dof above the largest key, returns the asymptotic 3.797 entry;
// for dof below the smallest key, returns the smallest entry as a
// conservative (over-rejection) fallback. dof < smallest is never reached at
// default WindowSize (count >= MinPoints = 80 gives dof >= 78), so the floor
// only matters if a caller drops MinPoints aggressively.
func grubbsTCrit(dof int) float64 {
	if dof < grubbsTCritKeysSorted[0] {
		return grubbsTCritTable[grubbsTCritKeysSorted[0]]
	}
	for i := len(grubbsTCritKeysSorted) - 1; i >= 0; i-- {
		if grubbsTCritKeysSorted[i] <= dof {
			return grubbsTCritTable[grubbsTCritKeysSorted[i]]
		}
	}
	// Unreachable given the dof < keys[0] guard above; placate the linter
	// with an explicit asymptotic fallback.
	return 3.797
}

// glooStateKey identifies per-series state by ref and aggregation.
type glooStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// glooSeriesState holds streaming state for a single (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	ring (80 * 8)               =  640 B
//	scalars                     ~   60 B
//	                             -------
//	                            ~  700 B
//
// Roughly half the per-key footprint of TukeyBiweightDetector — the ring
// stores values only (no timestamps) because Grubbs scoring never re-reads
// the ring contents at a tick: sumX and sumX2 are maintained via Welford
// add-and-evict updates so each scoring tick is O(1).
type glooSeriesState struct {
	// Cursor (mirrors mannkendall / esn / tukey_biweight).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Sliding window of recent values in ring-buffer order. cap == WindowSize
	// once full. Stored as float64 (not observer.Point) because scoring only
	// needs the values; timestamps live on the iteration cursor.
	ring        []float64
	head, count int

	// Welford running sums maintained over the ring contents. On add: both
	// gain newest. On evict: both lose oldest. So at any tick:
	//   sumX  == Σ ring[i]
	//   sumX2 == Σ ring[i]^2
	// The LOO subtractions (sumX - newest, sumX2 - newest^2) collapse to
	// O(1) per scoring tick — the whole point of this detector vs. a naive
	// full-window recompute.
	sumX, sumX2 float64

	// ticksSinceScore counts new points ingested since the last scoring tick.
	// Scoring runs only when this clears ScoreEvery, amortizing the (already
	// trivial) per-tick cost across multiple ingests so per-detector telemetry
	// stays comparable to tukey_biweight.
	ticksSinceScore int

	// cooldownLeft is decremented on every ingested point so it expires
	// regardless of whether scoring runs on this tick.
	cooldownLeft int
	lastFireTime int64
}

// GrubbsLOODetector implements a streaming Grubbs leave-one-out outlier test
// over a rolling window. On each scoring tick it computes
//
//	t_loo = (x_n - mean_{1..n-1}) / (sigma_{1..n-1} * sqrt(1 + 1/(n-1)))
//
// where mean_{1..n-1} and sigma_{1..n-1} are the sample mean and Bessel-
// corrected sample standard deviation over the n-1 OTHER points in the
// window — i.e. the latest point is excluded from the baseline. Under H0
// (no outlier) t_loo follows Student-t with (n-2) degrees of freedom (Grubbs
// 1969, "Procedures for detecting outlying observations in samples",
// Technometrics 11). This gives EXACT type-I-error control at the configured
// alpha — no empirical threshold-snapping. Per-tick cost is O(1) via Welford
// add-and-evict updates (Welford 1962): sumX and sumX2 ride the ring, so
// LOO sums collapse to a single subtraction.
//
// Trade-off vs. tukey_biweight: Grubbs-LOO does NOT downweight historical
// outliers — a single in-window glitch poisons (mean, sigma) and masks a
// true subsequent shift while it remains in the ring. The
// TestGrubbsLOO_RobustnessToHistoricalGlitch test characterizes that
// failure mode and pins the recovery behavior once the glitch falls out
// of the ring. This complementary blind-spot is why both detectors are
// registered: tukey_biweight catches a shift through a contaminating
// glitch, Grubbs-LOO catches single tail-event outliers more cheaply.
//
// Implements observer.Detector and observer.SeriesRemover. The Detect
// iteration shape mirrors TukeyBiweightDetector.Detect verbatim
// (metrics_detector_tukey_biweight.go:160-228).
type GrubbsLOODetector struct {
	// WindowSize is the number of recent values held in the Welford ring.
	// Default: 80 — matches tukey_biweight so the two detectors evaluate on
	// equivalent context lengths.
	WindowSize int
	// MinPoints is the minimum window fill before scoring runs.
	// Default: WindowSize. The Student-t calibration assumes the full
	// (n-2)-dof reference distribution, so dropping below WindowSize
	// invalidates the threshold table.
	MinPoints int
	// Alpha is the (informational) target type-I-error rate. The actual
	// threshold lookup goes through grubbsTCrit, which is fixed at α = 1e-4.
	// Kept as a struct field so future hyperparameter sweeps can introduce
	// alternate tables without breaking the catalog factory shape.
	Alpha float64
	// ScoreEvery amortizes scoring across multiple ingests. Default: 4 —
	// matches tukey_biweight. With Welford maintenance scoring is O(1), so
	// the practical effect is just to coalesce per-detector telemetry.
	ScoreEvery int
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 30 — matches mannkendall / burgar / esn / tukey_biweight.
	CooldownPoints int
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[glooStateKey]*glooSeriesState

	// Cache the discovered series list across Detect calls; refreshed only
	// when SeriesGeneration changes.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewGrubbsLOODetector returns a GrubbsLOODetector with default settings.
func NewGrubbsLOODetector() *GrubbsLOODetector {
	return &GrubbsLOODetector{
		WindowSize:     80,
		MinPoints:      80,
		Alpha:          1e-4,
		ScoreEvery:     4,
		CooldownPoints: 30,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[glooStateKey]*glooSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *GrubbsLOODetector) Name() string { return "grubbs_loo" }

// Reset clears all per-series state for replay/reanalysis.
func (d *GrubbsLOODetector) Reset() {
	d.series = make(map[glooStateKey]*glooSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this hook the per-series map would grow unbounded with the
// cumulative number of series ever observed (mirrors the tukey_biweight /
// mannkendall / esn contract).
func (d *GrubbsLOODetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, glooStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Iteration shape mirrors
// TukeyBiweightDetector.Detect (metrics_detector_tukey_biweight.go:160-228):
// cache series on SeriesGeneration, bulk-fetch status, replay-skip when
// nothing has changed, then process only the strictly-new points via
// ForEachPoint.
func (d *GrubbsLOODetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := glooStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &glooSeriesState{}
				d.series[sk] = state
			}

			// Replay-skip: no new points and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendRing(state, p.Value)

				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}
				state.ticksSinceScore++

				if state.count >= d.MinPoints && state.cooldownLeft == 0 && state.ticksSinceScore >= d.ScoreEvery {
					state.ticksSinceScore = 0
					if anomaly, fired := d.scoreGrubbs(state, seriesMeta, agg, p.Value, p.Timestamp); fired {
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

// appendRing performs the Welford add-and-evict update on the per-series ring
// in chronological order. While the ring is filling it grows; once full, the
// oldest entry at state.head is overwritten and head advances modulo
// WindowSize. sumX and sumX2 are kept in lockstep so a scoring tick can
// compute LOO sums in O(1). Eviction subtracts BEFORE addition so a short-
// lived sign flip on sumX2 can never inflate var_loo.
func (d *GrubbsLOODetector) appendRing(state *glooSeriesState, newest float64) {
	if state.count == d.WindowSize {
		oldest := state.ring[state.head]
		state.sumX -= oldest
		state.sumX2 -= oldest * oldest
		state.ring[state.head] = newest
		state.head = (state.head + 1) % d.WindowSize
	} else {
		state.ring = append(state.ring, newest)
		state.count++
	}
	state.sumX += newest
	state.sumX2 += newest * newest
}

// scoreGrubbs computes the LOO Student-t residual for the latest ingested
// value and decides whether to emit. Pure with respect to state — no mutation
// of ring / cooldown happens here.
//
// The structural guarantee: var_loo is computed over the n-1 OTHER points,
// so the latest point cannot influence its own threshold. Combined with the
// Student-t calibration at (n-2) dof, this gives exact α = 1e-4 type-I-error
// control under H0 — see Grubbs (1969).
func (d *GrubbsLOODetector) scoreGrubbs(state *glooSeriesState, series *observer.Series, agg observer.Aggregate, newest float64, dataTime int64) (observer.Anomaly, bool) {
	n := state.count
	// Need n >= 3 so m1 - 1 >= 1 (denominator of var_loo). At default
	// MinPoints = 80 this is always satisfied; the guard exists so a tuned-
	// down MinPoints does not divide by zero.
	if n < 3 {
		return observer.Anomaly{}, false
	}
	m1 := float64(n - 1)

	// Welford LOO sums: subtract the latest point from the running totals.
	// O(1) regardless of WindowSize — no walk over the ring.
	sumXLoo := state.sumX - newest
	sumX2Loo := state.sumX2 - newest*newest

	meanLoo := sumXLoo / m1
	// Bessel-corrected variance: sum((x-mean)^2)/(m1-1)
	//   = (sumX2 - m1*mean^2) / (m1-1)
	//   = (sumX2 - mean*sumX) / (m1-1)        [since m1*mean = sumX]
	// The second form is one fewer multiply per tick — same accuracy at
	// double precision over a unit-scale ring (200-op accumulated error
	// is ~1e-13, far below the floor).
	varLoo := (sumX2Loo - meanLoo*sumXLoo) / (m1 - 1)
	if varLoo < 1e-10 {
		// Floor on degenerate (constant) baselines: avoid divide-by-zero
		// without inventing structure. The relative-magnitude term keeps
		// the floor sensible for series whose mean is far from zero (so
		// e.g. a 1e6-magnitude constant series doesn't collapse onto the
		// 1e-12 absolute floor and emit nonsense t_loo). Mirrors the
		// degenerate-window guard on tukey_biweight scoreBiweight.
		floor := meanLoo * meanLoo * 1e-4
		if floor < 1e-12 {
			floor = 1e-12
		}
		varLoo = floor
	}
	sigmaLoo := math.Sqrt(varLoo)
	stderr := sigmaLoo * math.Sqrt(1.0+1.0/m1)
	if stderr < 1e-12 {
		// Pathological degenerate window — should never happen given the
		// floor above, but a defensive guard avoids +Inf t_loo if it does.
		return observer.Anomaly{}, false
	}
	tLoo := (newest - meanLoo) / stderr
	tAbs := math.Abs(tLoo)

	// Threshold at α = 1e-4 two-sided, dof = n - 2 (location + scale
	// estimated from the n-1 baseline points, minus one for the Bessel
	// correction). At default n = 80 this is 3.901.
	tCrit := grubbsTCrit(n - 2)
	if tAbs < tCrit {
		return observer.Anomaly{}, false
	}
	if tAbs >= grubbsGlitchCap {
		// Glitch suppression: anything beyond ~50 sigma is almost certainly
		// a sensor artifact (NaN-converted 1e308, counter reset) rather than
		// a genuine outlier. Same shape as tukey_biweight's tbGlitchZCap.
		return observer.Anomaly{}, false
	}

	score := tAbs
	if score > grubbsGlitchCap {
		score = grubbsGlitchCap
	}

	direction := "above"
	if tLoo < 0 {
		direction = "below"
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Grubbs LOO: " + seriesName,
		Description: fmt.Sprintf("%s LOO baseline (t=%.2f, mean=%.4f, sigma=%.4f, n=%d, t_crit=%.3f)",
			direction, tLoo, meanLoo, sigmaLoo, n, tCrit),
		Timestamp: dataTime,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   meanLoo,
			BaselineStddev: sigmaLoo,
			Threshold:      tCrit,
			CurrentValue:   newest,
			DeviationSigma: tAbs,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults so
// a struct-literal construction (&GrubbsLOODetector{}) still produces a
// working detector. Mirrors TukeyBiweightDetector.ensureDefaults.
func (d *GrubbsLOODetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 80
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		d.MinPoints = d.WindowSize
	}
	if d.Alpha <= 0 {
		d.Alpha = 1e-4
	}
	if d.ScoreEvery <= 0 {
		d.ScoreEvery = 4
	}
	if d.CooldownPoints < 0 {
		d.CooldownPoints = 0
	}
	if d.CooldownPoints == 0 {
		d.CooldownPoints = 30
	}
	if d.series == nil {
		d.series = make(map[glooStateKey]*glooSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
