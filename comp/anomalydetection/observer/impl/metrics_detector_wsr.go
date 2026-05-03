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

// wsrGlitchZCap is the upper bound on |z| we are willing to surface. Anything
// beyond is almost certainly a sensor-glitch artefact (NaN-converted 1e308,
// counter reset) rather than a genuine level shift; we'd rather miss it than
// emit a runaway score. Mirrors hlGlitchShiftCap=50 / tbGlitchZCap=50 so all
// detectors share the same anti-runaway convention downstream.
const wsrGlitchZCap = 50.0

// wsrShiftMADGate is the secondary effect-size threshold. The Wilcoxon
// signed-rank test is sensitive to small but consistent symmetry-breaking
// patterns and can fire on tie-only / variance-only / micro-shift inputs at
// large enough N — exactly the over-fire failure mode the hl_shift
// ConfirmFraction gate was added to fix. Requiring shiftMAD >= 1.5 forces a
// non-trivial level move of the cur median relative to ref scale, in addition
// to the rank-based z-test.
const wsrShiftMADGate = 1.5

// wsrMinKept is the minimum non-zero |d_i| count after dropping zeros. Below
// this the normal approximation to the signed-rank null distribution is
// unreliable (Hollander & Wolfe 1973, §3.1 — recommend n >= 10 for the
// approximation; below that a permutation/exact test is required and we
// simply abstain rather than ship a noisy fire).
const wsrMinKept = 10

// wsrStateKey identifies per-series state by ref and aggregation.
type wsrStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// wsrSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key (rough):
//
//	refBuf       (40 * 16)                    =  640 B
//	curBuf       (40 * 16)                    =  640 B
//	scalars                                   ~  100 B
//	                                          -------
//	                                          ~1.4 KB
//
// Same shape and order as hlSeriesState; the two-buffer slide logic is
// identical.
type wsrSeriesState struct {
	// Cursor (mirrors mannkendall / wasserstein / hl_shift).
	lastProcessedCount int
	lastWriteGen       int64
	// lastProcessedTime is the highest point timestamp consumed so far. Used
	// as the exclusive lower bound for ForEachPoint so each point is appended
	// to the windows exactly once across replays/incremental advances.
	lastProcessedTime int64

	// refBuf is the older "reference" sliding window in ring-buffer form.
	// While filling, refHead stays at 0 and entries grow with append; once
	// full, refHead points to the oldest entry and cycles modulo WindowSize.
	refBuf   []observer.Point
	refHead  int
	refCount int

	// curBuf is the newer "current" sliding window. Same ring semantics as
	// refBuf. Points enter curBuf first; once it's full, every new point
	// causes the oldest curBuf entry to slide into refBuf.
	curBuf   []observer.Point
	curHead  int
	curCount int

	// ticksSinceScore counts new points ingested since the last scoring tick.
	// scoreSignedRank runs only when this clears ScoreEvery, amortizing the
	// O(N log N) sort over multiple ticks.
	ticksSinceScore int

	// cooldownLeft is decremented on every ingested point so it expires
	// regardless of whether scoring runs on this tick.
	cooldownLeft int
	lastFireTime int64
}

// WSRDetector implements a streaming one-sample Wilcoxon Signed-Rank shift
// detector (Wilcoxon 1945, "Individual Comparisons by Ranking Methods"). On
// each scoring tick it computes
//
//	d_i := cur[i] - median(ref)
//	W   := Σ_{i: d_i > 0} rank(|d_i|)   (mid-rank ties; zeros dropped)
//	z   := (W - μ_W) / σ_W
//
// over two sliding windows — μ_W and σ_W are the H0 mean and (tie-corrected)
// standard deviation of the signed-rank statistic. Under H0 (no shift in the
// median of cur relative to median(ref)), z is asymptotically N(0, 1); a
// genuine level shift drives almost all d_i to one sign and pushes |z| past
// ZThreshold.
//
// The detector emits when |z| clears ZThreshold AND |median(cur) - median(ref)|
// scaled by MAD(ref) clears wsrShiftMADGate. The second gate is a structural
// effect-size guard mirroring hl_shift's ConfirmFraction — without it the
// rank-based test can fire on near-zero-shift inputs that happen to have
// asymmetric noise around medRef.
//
// Implements observer.Detector with explicit cursoring and observer.SeriesRemover
// (mirrors HLShiftDetector). The two-buffer slide pattern, the scoring cadence,
// and the chronological-snapshot helper are intentionally identical to hl_shift
// — only scoreSignedRank differs.
type WSRDetector struct {
	// WindowSize is the number of points per sliding window (ref and cur).
	// Default: 40 — matches hl_shift; with N=40 the kept count after dropping
	// zeros is comfortably above wsrMinKept=10 in practice.
	WindowSize int
	// MinPoints is the minimum total fill before scoring runs. Default:
	// WindowSize. Both windows must be full (each at MinPoints) before any
	// shift score is computed.
	MinPoints int
	// ZThreshold is the minimum |z| for a fire. Default: 4.0 — matches the
	// shiftMAD threshold used by hl_shift / wasserstein, so the family of
	// distributional-shift detectors shares a comparable null-rejection bar.
	ZThreshold float64
	// ScoreEvery amortizes the O(N log N) scoring cost across multiple
	// ingested points. Default: 4 — same value as hl_shift / tukey_biweight.
	ScoreEvery int
	// CooldownPoints is the per-series suppression window after a fire.
	// Default: 30 — matches mannkendall / wasserstein / hl_shift to bound
	// emission frequency on the same drift segment.
	CooldownPoints int
	// GlitchCap is the upper bound on |z| we will surface. Default: 50.
	GlitchCap float64
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[wsrStateKey]*wsrSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewWSRDetector returns a Wilcoxon signed-rank shift detector with default
// settings. Parameterless to match the wasserstein/tukey_biweight/hl_shift
// factory pattern.
func NewWSRDetector() *WSRDetector {
	return &WSRDetector{
		WindowSize:     40,
		MinPoints:      40,
		ZThreshold:     4.0,
		ScoreEvery:     4,
		CooldownPoints: 30,
		GlitchCap:      wsrGlitchZCap,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[wsrStateKey]*wsrSeriesState),
	}
}

// Name returns the detector name as registered in the catalog.
func (d *WSRDetector) Name() string { return "wsr" }

// Reset clears all per-series state for replay/reanalysis.
func (d *WSRDetector) Reset() {
	d.series = make(map[wsrStateKey]*wsrSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series window state for refs that storage has freed.
// Mirrors hl_shift / wasserstein / mannkendall — without this hook the
// per-series map keeps growing with cumulative series cardinality even after
// storage shrinks.
func (d *WSRDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, wsrStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration shape is a structural
// copy of HLShiftDetector.Detect — cache series, bulk-fetch status,
// replay-skip when nothing has changed, then walk only the strictly-new
// points via ForEachPoint.
func (d *WSRDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := wsrStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &wsrSeriesState{}
				d.series[sk] = state
			}

			// Replay-skip: no new data and no in-place writes.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.lastProcessedTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					sCopy := *s
					seriesMeta = &sCopy
				}
				d.appendPoint(state, p)

				// Decrement cooldown per ingested point so it expires
				// regardless of whether we score this tick.
				if state.cooldownLeft > 0 {
					state.cooldownLeft--
				}
				state.ticksSinceScore++

				if state.refCount >= d.MinPoints && state.curCount >= d.MinPoints &&
					state.cooldownLeft == 0 && state.ticksSinceScore >= d.ScoreEvery {
					state.ticksSinceScore = 0
					if anomaly, fired := d.scoreSignedRank(state, seriesMeta, agg, p.Timestamp); fired {
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

// appendPoint pushes p into the per-series two-buffer state. Same two-buffer
// slide pattern as hl_shift.appendPoint / wasserstein:248-268: phase 1 fills
// curBuf; phase 2 slides the oldest curBuf entry into refBuf and writes p
// into curBuf at the cursor.
func (d *WSRDetector) appendPoint(state *wsrSeriesState, p observer.Point) {
	if state.curCount < d.WindowSize {
		state.curBuf = append(state.curBuf, p)
		state.curCount++
		return
	}
	// curBuf is full: slide its oldest entry into refBuf, then write p in
	// curBuf at curHead and advance.
	slid := state.curBuf[state.curHead]
	if state.refCount < d.WindowSize {
		state.refBuf = append(state.refBuf, slid)
		state.refCount++
	} else {
		state.refBuf[state.refHead] = slid
		state.refHead = (state.refHead + 1) % d.WindowSize
	}
	state.curBuf[state.curHead] = p
	state.curHead = (state.curHead + 1) % d.WindowSize
}

// wsrSnapshot returns the contents of a ring buffer in chronological order
// as a fresh []float64. Allocated only on scoring ticks (gated by ScoreEvery
// + cooldown + both-windows-full), so allocation pressure is amortized.
//
// Mirrors hlSnapshot / wassersteinSnapshot.
func wsrSnapshot(buf []observer.Point, head, count, capN int) []float64 {
	out := make([]float64, count)
	if count < capN {
		// Buffer hasn't wrapped yet — entries are already in order at [0..count).
		for i := 0; i < count; i++ {
			out[i] = buf[i].Value
		}
		return out
	}
	for i := 0; i < capN; i++ {
		out[i] = buf[(head+i)%capN].Value
	}
	return out
}

// scoreSignedRank runs the Wilcoxon SR dual gate (rank-based z-test +
// MAD-scaled effect size) on the current ref/cur pair. Returns
// (anomaly, fired). Pure function over the ring snapshots — no mutation of
// state happens here.
func (d *WSRDetector) scoreSignedRank(state *wsrSeriesState, series *observer.Series, agg observer.Aggregate, dataTime int64) (observer.Anomaly, bool) {
	n := d.WindowSize
	if state.refCount < n || state.curCount < n {
		return observer.Anomaly{}, false
	}

	ref := wsrSnapshot(state.refBuf, state.refHead, state.refCount, d.WindowSize)
	cur := wsrSnapshot(state.curBuf, state.curHead, state.curCount, d.WindowSize)

	medRef := detectorMedian(ref)

	// Wilcoxon convention: drop d_i == 0 before ranking. Keep absolute values
	// and signs in parallel slices.
	type wsrPair struct {
		abs  float64
		sign int8
	}
	pairs := make([]wsrPair, 0, len(cur))
	for _, v := range cur {
		diff := v - medRef
		if diff == 0 {
			continue
		}
		s := int8(1)
		if diff < 0 {
			s = -1
		}
		pairs = append(pairs, wsrPair{abs: math.Abs(diff), sign: s})
	}
	nPrime := len(pairs)
	if nPrime < wsrMinKept {
		return observer.Anomaly{}, false
	}

	// Sort by |d| ascending, then assign mid-ranks within tied groups
	// (canonical Wilcoxon mid-rank). Walk the sorted slice once: every run of
	// equal-|d| entries shares the mean of its position range as its rank.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].abs < pairs[j].abs })

	ranks := make([]float64, nPrime)
	var tieCorrection float64
	for i := 0; i < nPrime; {
		j := i
		for j+1 < nPrime && pairs[j+1].abs == pairs[i].abs {
			j++
		}
		// Group [i..j] in 0-based; positions are (i+1)..(j+1) in 1-based.
		meanRank := float64(i+1+j+1) / 2.0
		for k := i; k <= j; k++ {
			ranks[k] = meanRank
		}
		if t := j - i + 1; t > 1 {
			ft := float64(t)
			// Hollander & Wolfe 1973 eq. 3.4 tie correction (one-sample form).
			tieCorrection += (ft*ft*ft - ft) / 48.0
		}
		i = j + 1
	}

	// W = Σ rank_i where sign_i = +1.
	var W float64
	for k := 0; k < nPrime; k++ {
		if pairs[k].sign > 0 {
			W += ranks[k]
		}
	}

	nP := float64(nPrime)
	muW := nP * (nP + 1) / 4.0
	sigma2 := nP*(nP+1)*(2*nP+1)/24.0 - tieCorrection
	if sigma2 < 1e-12 {
		// Pathological: all signs identical AND all ties at the maximum
		// correction — variance collapses to ~0. Abstain rather than divide by
		// near-zero.
		return observer.Anomaly{}, false
	}
	sigma := math.Sqrt(sigma2)
	z := (W - muW) / sigma
	absZ := math.Abs(z)

	// Effect-size gate: requires a real level move (not just a rank-symmetry
	// break). Built from the ref MAD scaled to a sigma estimate, with the
	// same scale-stable fallback as hl_shift / wasserstein so a constant ref
	// window does not produce divide-by-near-zero shiftMAD values.
	medCur := detectorMedian(cur)
	madRef := detectorMAD(ref, medRef, true)
	scaleDenom := madRef
	if scaleDenom < 1e-9 {
		scaleDenom = 1e-9
	}
	if pct := math.Abs(medRef) * 0.01; pct > scaleDenom {
		scaleDenom = pct
	}
	shiftMAD := math.Abs(medCur-medRef) / scaleDenom

	// Fire gates: |z| in [ZThreshold, GlitchCap] AND shiftMAD >= 1.5.
	if absZ < d.ZThreshold || absZ > d.GlitchCap || shiftMAD < wsrShiftMADGate {
		return observer.Anomaly{}, false
	}

	score := absZ
	if score > d.GlitchCap {
		score = d.GlitchCap
	}

	direction := "increasing"
	if medCur < medRef {
		direction = "decreasing"
	}

	// Concatenate ref+cur in chronological order for medianPointInterval.
	// Same shape as hl_shift:431-446.
	chronological := make([]observer.Point, 0, 2*n)
	if state.refCount < d.WindowSize {
		chronological = append(chronological, state.refBuf[:state.refCount]...)
	} else {
		for i := 0; i < d.WindowSize; i++ {
			chronological = append(chronological, state.refBuf[(state.refHead+i)%d.WindowSize])
		}
	}
	if state.curCount < d.WindowSize {
		chronological = append(chronological, state.curBuf[:state.curCount]...)
	} else {
		for i := 0; i < d.WindowSize; i++ {
			chronological = append(chronological, state.curBuf[(state.curHead+i)%d.WindowSize])
		}
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "WSR shift: " + seriesName,
		Description: fmt.Sprintf("%s level shift (W=%.0f, z=%.2f, shiftMAD=%.2f, n=%d)",
			direction, W, z, shiftMAD, nPrime),
		Timestamp:           dataTime,
		Score:               &score,
		SamplingIntervalSec: medianPointInterval(chronological),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medRef,
			BaselineMAD:    madRef,
			CurrentValue:   chronological[len(chronological)-1].Value,
			DeviationSigma: absZ,
		},
	}
	return anomaly, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *WSRDetector) ensureDefaults() {
	if d.WindowSize <= 0 {
		d.WindowSize = 40
	}
	if d.MinPoints <= 0 {
		d.MinPoints = d.WindowSize
	}
	if d.MinPoints > d.WindowSize {
		// Scoring requires both buffers to be filled, so cap MinPoints.
		d.MinPoints = d.WindowSize
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = 4.0
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
	if d.GlitchCap <= 0 {
		d.GlitchCap = wsrGlitchZCap
	}
	if d.series == nil {
		d.series = make(map[wsrStateKey]*wsrSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
