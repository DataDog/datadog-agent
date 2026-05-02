// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// LODA tunables. The plan calls for these to be fixed; only adjust with
// recorded justification because the constants together control how quickly
// the detector warms up, how aggressive its scoring is, and how much memory
// it consumes per (series, agg) pair.
const (
	lodaProjections  = 10  // K — number of random projections in the ensemble
	lodaBins         = 20  // B — bins per projection histogram
	lodaFeatureDim   = 5   // D — synthetic feature vector size
	lodaRingSize     = 64  // raw-value ring buffer used for rolling stats
	lodaRecentSize   = 128 // history of LODA scores used for the p99 threshold
	lodaWarmup       = 120 // points consumed before scoring begins
	lodaHistDiscount = 0.05
	lodaEMAAlpha     = 0.1
	lodaP99          = 0.99
	// lodaLogFloor: -log(prob) below this never fires regardless of percentile.
	// The plan suggested 4.0 but with Laplace +1, B=20 and ALPHA=0.05 the
	// per-projection score saturates at -log(1/40) ≈ 3.69 and the mean
	// across K projections is bounded by the same value, so 4.0 would never
	// fire. 3.3 sits just below saturation: a fire requires nearly every
	// projection to hit a near-empty bin, which keeps stable-noise events
	// from triggering while still catching genuine outliers (where most
	// projections clamp to edge bins).
	lodaLogFloor         = 3.3
	lodaScorePercentMin  = 64 // recentScores fill required before percentile is meaningful
	lodaMinFireGapSec    = 60
	lodaProjectionSeed   = int64(0x10DA) // fixed projection-matrix seed (deterministic across runs)
	lodaSlopeWindow      = 10
	lodaProjectionRetry  = 100 // bound on retries when an Achlioptas row lands all-zero
	lodaConstantFallback = 1e-12

	// lodaOutOfRangeProbScale: when a projection value falls outside the
	// warmup-learned [binMin, binMax] range, the bin probability is scaled
	// down by 1/(histTotal + B * scale) instead of using the clamped
	// edge-bin's smoothed probability. This widens the score's dynamic range
	// so genuinely extreme spikes can clear LOG_FLOOR, which would otherwise
	// be impossible under the plan's hard-clamp behavior. See processPoint.
	lodaOutOfRangeProbScale = 100.0
)

// lodaStateKey identifies per-series state by ref and aggregation.
type lodaStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// lodaSeriesState is the streaming state kept per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	ringBuf      (64 * 8)                    =  512 B
//	hist         (10 * 20 * 8)               = 1600 B
//	binMin/Max   (2 * 10 * 8)                =  160 B
//	histTotal    (10 * 8)                    =   80 B
//	recentScores (128 * 8)                   = 1024 B
//	scalars                                  ~  100 B
//	                                          -------
//	                                          ~3.5 KB
type lodaSeriesState struct {
	// Cursor (mirrors scanmw / mannkendall).
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// Raw-value ring (used for rolling z, slope and baseline mean/std).
	ringBuf    [lodaRingSize]float64
	ringHead   int
	ringFilled int

	// EWMA of value (with alpha = lodaEMAAlpha).
	ema     float64
	emaInit bool

	// Previous raw value, for the lag-1 difference feature.
	prevValue float64
	hasPrev   bool

	// Warmup tracking. While warmupCount <= lodaWarmup we only learn the
	// projection ranges; histInit flips on the first post-warmup point so we
	// seed each histogram exactly once.
	warmupCount int
	histInit    bool

	// Per-projection histograms with discounted updates. binMin/binMax are
	// learned during warmup and frozen afterwards; histograms accumulate from
	// the first post-warmup point.
	hist      [lodaProjections][lodaBins]float64
	binMin    [lodaProjections]float64
	binMax    [lodaProjections]float64
	histTotal [lodaProjections]float64

	// Ring of past LODA scores used for the dynamic p99 threshold.
	recentScores [lodaRecentSize]float64
	recentHead   int
	recentFilled int

	// Last fire timestamp for the MIN_FIRE_GAP_SEC gate.
	lastFireTime int64
}

// LODADetector is an online streaming anomaly detector built around the
// Lightweight On-line Detector of Anomalies (Pevný 2016, Machine Learning
// Journal). It synthesizes a small feature vector from each new point
// (level, lag-1 diff, rolling z, short-window slope, EWMA), projects that
// vector through a fixed sparse Achlioptas matrix to generate K cheap
// 1-D views, and scores each view via a discounted equi-width histogram.
//
// The anomaly score is the mean negative log-probability across projections.
// Firing is gated by a dynamic p99 threshold over recent scores, an absolute
// floor (LOG_FLOOR), and a per-series cooldown — together this avoids the
// alarm storms that single-detector parametric density approaches exhibit.
//
// Implements observer.Detector with explicit cursoring so scans only touch
// new points, mirroring scanmw / mannkendall.
type LODADetector struct {
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// Per-series state keyed by ref+agg.
	series map[lodaStateKey]*lodaSeriesState

	// Projection matrix — built once at construction with a fixed seed so the
	// detector is deterministic across processes and tests.
	projection [lodaProjections][lodaFeatureDim]float64

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewLODADetector returns a LODA detector with default settings and a
// deterministic projection matrix (seeded from lodaProjectionSeed).
func NewLODADetector() *LODADetector {
	d := &LODADetector{
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[lodaStateKey]*lodaSeriesState),
	}
	d.buildProjection()
	return d
}

// Name returns the detector name as registered in the catalog.
func (d *LODADetector) Name() string { return "loda" }

// Reset clears all per-series state for replay/reanalysis. The projection
// matrix is left intact — it is constant by design.
func (d *LODADetector) Reset() {
	d.series = make(map[lodaStateKey]*lodaSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Mirrors scanmw / mannkendall — without this hook the per-series map
// would grow unbounded with the cumulative number of series ever observed
// even after storage shrinks.
func (d *LODADetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, lodaStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration pattern matches
// mannkendall (metrics_detector_mannkendall.go:142-215): cache series on
// SeriesGeneration, bulk-fetch status, replay-skip when nothing changed,
// then process only the strictly-new points via ForEachPoint.
func (d *LODADetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := lodaStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &lodaSeriesState{}
				d.series[sk] = state
			}

			// Skip when nothing has changed. Equivalent to the early-skip
			// in scanmw/mannkendall: no new points and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				if anomaly, fired := d.processPoint(state, seriesMeta, agg, p); fired {
					anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
					allAnomalies = append(allAnomalies, anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint runs the full per-tick LODA pipeline for a single new point
// and returns (anomaly, fired). It is a method on the detector so it can
// access the projection matrix without copying it per call.
//
// Deviation from plan: we update ring / EMA *before* computing the slope
// and EMA features so a single-point spike actually shows up in those
// features on its own tick. Otherwise projections that only touch f[3]
// (slope) or f[4] (ema) are blind to the spike — empirically with the
// fixed projection seed two of the ten projections fall in that bucket,
// which is enough to drag the ensemble mean below the LOG_FLOOR and miss
// the event entirely. The lag-1 diff and rolling-z features still use the
// PRE-update prevValue / ring stats so they remain maximally sensitive.
func (d *LODADetector) processPoint(state *lodaSeriesState, series *observer.Series, agg observer.Aggregate, p observer.Point) (observer.Anomaly, bool) {
	var feat [lodaFeatureDim]float64

	// Features that need the PRE-update state (lag-1 diff against the
	// previous point; rolling-z against the previous ring stats). On the
	// first tick prevValue is 0 / hasPrev=false, giving f[1]=0 per plan.
	feat[0] = p.Value
	if state.hasPrev {
		feat[1] = p.Value - state.prevValue
	}
	feat[2] = d.rollingZ(state, p.Value)

	// Now bring the rolling buffers up to date with the current value.
	d.pushRing(state, p.Value)
	if !state.emaInit {
		state.ema = p.Value
		state.emaInit = true
	} else {
		state.ema = lodaEMAAlpha*p.Value + (1-lodaEMAAlpha)*state.ema
	}
	state.prevValue = p.Value
	state.hasPrev = true

	// Features that benefit from the POST-update state (slope and EMA see
	// the spike on its own tick, which keeps slope-only / ema-only
	// projections from being blind to single-point anomalies).
	feat[3] = d.recentSlope(state)
	feat[4] = state.ema

	// Step 2: project f through the K-row matrix.
	var projVals [lodaProjections]float64
	for k := 0; k < lodaProjections; k++ {
		var pk float64
		row := &d.projection[k]
		for di := 0; di < lodaFeatureDim; di++ {
			pk += row[di] * feat[di]
		}
		projVals[k] = pk
	}

	state.warmupCount++

	// Step 3: warmup — only learn the projection ranges, never fire.
	if state.warmupCount <= lodaWarmup {
		for k := 0; k < lodaProjections; k++ {
			pk := projVals[k]
			if state.warmupCount == 1 || pk < state.binMin[k] {
				state.binMin[k] = pk
			}
			if state.warmupCount == 1 || pk > state.binMax[k] {
				state.binMax[k] = pk
			}
		}
		return observer.Anomaly{}, false
	}

	// Step 4: first post-warmup point — seed each histogram with the current
	// projection so the discounted update has a sensible starting state.
	if !state.histInit {
		for k := 0; k < lodaProjections; k++ {
			bin := lodaBinIndex(projVals[k], state.binMin[k], state.binMax[k])
			state.hist[k][bin] = 1.0
			state.histTotal[k] = 1.0
		}
		state.histInit = true
	}

	// Step 5: score each projection via Laplace-smoothed bin probability and
	// average the negative log-probabilities. Track the best (most surprising)
	// projection for the description string.
	//
	// Deviation from plan: when a projection value lies outside the warmup
	// [binMin, binMax] range we don't fall back to the clamped edge-bin
	// probability — instead we use an out-of-range probability scaled down
	// by lodaOutOfRangeProbScale. The plan's hard clamp + Laplace +1 caps
	// the per-projection -log at ~3.69 even on a 1e6× spike, which makes
	// the LOG_FLOOR gate effectively unreachable. The histogram is still
	// updated against the clamped bin in step 6 so future similarly-extreme
	// points that have happened before fall within the trained range.
	var (
		score      float64
		bestK      int
		bestNegLog float64
	)
	var bins [lodaProjections]int
	for k := 0; k < lodaProjections; k++ {
		pk := projVals[k]
		bin := lodaBinIndex(pk, state.binMin[k], state.binMax[k])
		bins[k] = bin
		var prob float64
		if pk < state.binMin[k] || pk > state.binMax[k] {
			prob = 1.0 / (state.histTotal[k] + float64(lodaBins)*lodaOutOfRangeProbScale)
		} else {
			prob = (state.hist[k][bin] + 1.0) / (state.histTotal[k] + float64(lodaBins))
		}
		negLog := -math.Log(prob)
		if k == 0 || negLog > bestNegLog {
			bestNegLog = negLog
			bestK = k
		}
		score += negLog
	}
	score /= float64(lodaProjections)

	// Step 6: discounted update. Multiply existing mass by (1-ALPHA), then
	// add 1.0 to the bin and total this point falls into.
	for k := 0; k < lodaProjections; k++ {
		for b := 0; b < lodaBins; b++ {
			state.hist[k][b] *= (1 - lodaHistDiscount)
		}
		state.histTotal[k] *= (1 - lodaHistDiscount)
		state.hist[k][bins[k]] += 1.0
		state.histTotal[k] += 1.0
	}

	// Step 7: compute the percentile threshold over PRIOR scores only — if
	// we pushed the new score first, a fresh maximum would equal its own
	// p99 and the strict-greater fire gate would never trigger on the very
	// first extreme event. We push after the gate.
	if state.recentFilled < lodaScorePercentMin {
		d.pushRecent(state, score)
		return observer.Anomaly{}, false
	}

	p99 := lodaPercentile(state.recentScores[:], state.recentFilled, lodaP99)
	d.pushRecent(state, score)

	// Step 8: fire condition — score must STRICTLY exceed both the dynamic
	// p99 and the absolute floor, and must respect the per-series cooldown.
	if score <= p99 || score <= lodaLogFloor {
		return observer.Anomaly{}, false
	}
	if state.lastFireTime != 0 && p.Timestamp-state.lastFireTime <= lodaMinFireGapSec {
		return observer.Anomaly{}, false
	}

	// Build the anomaly. Top-contributing feature is the one whose absolute
	// contribution to the most-surprising projection is largest.
	bestFeat := 0
	bestFeatMag := 0.0
	for di := 0; di < lodaFeatureDim; di++ {
		c := math.Abs(d.projection[bestK][di] * feat[di])
		if di == 0 || c > bestFeatMag {
			bestFeatMag = c
			bestFeat = di
		}
	}

	mean := d.ringMean(state)
	std := d.ringStd(state, mean)

	seriesName := series.Name + ":" + aggSuffix(agg)
	scoreOut := score
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "LODA anomaly: " + seriesName,
		Description: fmt.Sprintf("%s LODA score=%.3f (p99=%.3f, top proj=%d/-log=%.2f, top feat=%s contrib=%.3f)",
			seriesName, score, p99, bestK, bestNegLog, lodaFeatureName(bestFeat), bestFeatMag),
		Timestamp:           p.Timestamp,
		Score:               &scoreOut,
		SamplingIntervalSec: d.medianRingInterval(state, p.Timestamp),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   mean,
			BaselineStddev: std,
			CurrentValue:   p.Value,
			DeviationSigma: score,
		},
	}

	state.lastFireTime = p.Timestamp
	return anomaly, true
}

// lodaBinIndex maps a projection value into a bin index, clamped to
// [0, lodaBins-1]. Uses the plan's lodaConstantFallback floor on bin width
// so a flat-projection warmup can't blow up via division by zero.
func lodaBinIndex(pk, binMin, binMax float64) int {
	width := math.Max((binMax-binMin)/float64(lodaBins), lodaConstantFallback)
	idx := int((pk - binMin) / width)
	if idx < 0 {
		return 0
	}
	if idx >= lodaBins {
		return lodaBins - 1
	}
	return idx
}

// lodaPercentile computes the q-percentile of the first n entries of buf
// (chronological order doesn't matter — only the sorted order does).
// Allocates a temporary slice of size n; called at most once per fire-eligible
// tick so allocation cost is acceptable.
func lodaPercentile(buf []float64, n int, q float64) float64 {
	if n <= 0 {
		return 0
	}
	tmp := make([]float64, n)
	copy(tmp, buf[:n])
	sort.Float64s(tmp)
	idx := int(math.Floor(q * float64(n)))
	if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	return tmp[idx]
}

// lodaFeatureName returns a short label for one of the synthesised features,
// used in the description string when reporting an anomaly.
func lodaFeatureName(i int) string {
	switch i {
	case 0:
		return "level"
	case 1:
		return "lag1_diff"
	case 2:
		return "rolling_z"
	case 3:
		return "slope"
	case 4:
		return "ema"
	default:
		return "unknown"
	}
}

// pushRing inserts a new value into the raw-value ring buffer.
func (d *LODADetector) pushRing(state *lodaSeriesState, value float64) {
	state.ringBuf[state.ringHead] = value
	state.ringHead = (state.ringHead + 1) % lodaRingSize
	if state.ringFilled < lodaRingSize {
		state.ringFilled++
	}
}

// pushRecent inserts a score into the recent-scores ring buffer used for
// the p99 threshold.
func (d *LODADetector) pushRecent(state *lodaSeriesState, score float64) {
	state.recentScores[state.recentHead] = score
	state.recentHead = (state.recentHead + 1) % lodaRecentSize
	if state.recentFilled < lodaRecentSize {
		state.recentFilled++
	}
}

// rollingZ returns (value - mean(ring)) / max(std(ring), 1e-9). Returns 0
// when the ring is too small to give a meaningful baseline (avoids the
// divide-by-zero blowup that would dominate the projections on early ticks).
func (d *LODADetector) rollingZ(state *lodaSeriesState, value float64) float64 {
	if state.ringFilled < 2 {
		return 0
	}
	mean := d.ringMean(state)
	std := d.ringStd(state, mean)
	if std < 1e-9 {
		std = 1e-9
	}
	return (value - mean) / std
}

// recentSlope returns the OLS slope of the last min(ringFilled, lodaSlopeWindow)
// values vs index 0..m-1. Returns 0 when fewer than two points are available.
func (d *LODADetector) recentSlope(state *lodaSeriesState) float64 {
	m := state.ringFilled
	if m > lodaSlopeWindow {
		m = lodaSlopeWindow
	}
	if m < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2 float64
	// Walk the ring tail backwards to gather the m most-recent values.
	for i := 0; i < m; i++ {
		idx := (state.ringHead - m + i + lodaRingSize) % lodaRingSize
		v := state.ringBuf[idx]
		fi := float64(i)
		sumX += fi
		sumY += v
		sumXY += fi * v
		sumX2 += fi * fi
	}
	fM := float64(m)
	denom := fM*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (fM*sumXY - sumX*sumY) / denom
}

// ringMean returns the mean of the values currently in the ring buffer.
func (d *LODADetector) ringMean(state *lodaSeriesState) float64 {
	n := state.ringFilled
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += state.ringBuf[i]
	}
	return sum / float64(n)
}

// ringStd returns the population standard deviation of the values currently
// in the ring buffer.
func (d *LODADetector) ringStd(state *lodaSeriesState, mean float64) float64 {
	n := state.ringFilled
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		dx := state.ringBuf[i] - mean
		sum += dx * dx
	}
	return math.Sqrt(sum / float64(n))
}

// medianRingInterval reports the median sampling interval inferred from the
// ring buffer. Falls back to the gap between the most recent stored point
// and the current timestamp when the ring carries fewer than two samples.
func (d *LODADetector) medianRingInterval(state *lodaSeriesState, current int64) int64 {
	n := state.ringFilled
	if n < 2 {
		return 0
	}
	// We do not store timestamps in the ring buffer; the engine drives this
	// from real-time data. Use lastProcessedTime as the freshest available
	// timestamp gap. Because medianRingInterval is only used for the
	// SamplingIntervalSec hint (correlator scaling), a coarse estimate is
	// acceptable.
	if state.lastProcessedTime == 0 || current <= state.lastProcessedTime {
		return 0
	}
	return current - state.lastProcessedTime
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *LODADetector) ensureDefaults() {
	if d.series == nil {
		d.series = make(map[lodaStateKey]*lodaSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
	if d.projection[0][0] == 0 && d.projection[0][1] == 0 && d.projection[0][2] == 0 {
		// Constructor-built detectors will have a populated row 0; only
		// detectors built via &LODADetector{} (e.g. some tests) need this.
		d.buildProjection()
	}
}

// buildProjection populates the K×D sparse Achlioptas projection matrix.
//
// Each entry is +1 with prob 1/6, -1 with prob 1/6 and 0 otherwise; rows are
// L1-normalised so projections stay on a comparable scale across the
// ensemble. The RNG is seeded from a fixed value (lodaProjectionSeed) so the
// matrix — and therefore every downstream score — is deterministic across
// runs and processes.
//
// With K=10 and D=5 the all-zero-row probability is (2/3)^5 ≈ 13%, so we
// retry a row until at least one non-zero entry shows up. The retry loop is
// bounded by lodaProjectionRetry to keep the constructor terminating in
// pathological RNG states; if the bound is hit we fall back to a single
// non-zero entry so the row is still usable.
func (d *LODADetector) buildProjection() {
	rng := rand.New(rand.NewSource(lodaProjectionSeed)) //nolint:gosec // deterministic seed for repeatability
	for k := 0; k < lodaProjections; k++ {
		ok := false
		for retry := 0; retry < lodaProjectionRetry && !ok; retry++ {
			var sum float64
			for di := 0; di < lodaFeatureDim; di++ {
				r := rng.Float64()
				switch {
				case r < 1.0/6.0:
					d.projection[k][di] = 1.0
					sum++
				case r < 2.0/6.0:
					d.projection[k][di] = -1.0
					sum++
				default:
					d.projection[k][di] = 0.0
				}
			}
			if sum > 0 {
				for di := 0; di < lodaFeatureDim; di++ {
					d.projection[k][di] /= sum
				}
				ok = true
			}
		}
		if !ok {
			// Pathological RNG run — fall back to a deterministic non-zero
			// projection so the row carries some signal.
			d.projection[k] = [lodaFeatureDim]float64{}
			d.projection[k][k%lodaFeatureDim] = 1.0
		}
	}
}
