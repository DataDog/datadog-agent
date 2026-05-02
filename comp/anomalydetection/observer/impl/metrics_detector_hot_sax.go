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

// Hot-SAX tunables. The window/word/alphabet geometry is fixed at compile time
// so the per-series state can live as fixed-size arrays without per-series
// allocation. Adjust with a recorded justification — these constants together
// control startup latency, false-positive discipline, and memory footprint per
// (series, aggregate) pair. References: Keogh, Lin & Fu 2005, "HOT SAX:
// Efficiently Finding the Most Unusual Time Series Subsequence" (ICDM); Lin,
// Keogh, Lonardi & Chiu 2003, "A Symbolic Representation of Time Series, with
// Implications for Streaming Algorithms" (DMKD).
const (
	hotSaxWindowSize     = 32 // subsequence length in points
	hotSaxWordLen        = 8  // PAA segments per window (must divide WindowSize)
	hotSaxPAAWidth       = hotSaxWindowSize / hotSaxWordLen
	hotSaxAlphabetSize   = 4    // SAX alphabet (Lin et al. recommend 3-7)
	hotSaxDictionarySize = 64   // recent words retained for NN search
	hotSaxWarmupPoints   = 96   // 3*WindowSize so the dictionary has room to grow
	hotSaxScoreThreshold = 1.6  // discord MINDIST/sigma threshold
	hotSaxCooldownPoints = 24   // suppress adjacent re-fires
	hotSaxScoreEveryNth  = 4    // amortize MINDIST scan
	hotSaxMinFill        = 16   // require >= this many dict words before scoring
	hotSaxScoreCap       = 50.0 // visual cap on emitted Score (matches mannkendall / hi_moments)
	hotSaxStddevFloor    = 1e-9 // flat-line guard: skip scoring on degenerate windows
)

// hotSAXStateKey identifies per-series state by ref and aggregation.
type hotSAXStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hotSAXSeriesState holds streaming state per (series, aggregate) pair.
//
// Memory footprint per key:
//
//	ring         (32 * 8)                       =  256 B
//	dict         (64 * 8 * 1)                   =  512 B
//	scalars (cursors + counters)                ~  100 B
//	                                            -------
//	                                            ~  870 B
//
// Comparable to ESN (~1.4 KB) and lighter than wasserstein (~2 KB).
type hotSAXSeriesState struct {
	// Cursor (mirrors ESN / mannkendall / wasserstein).
	lastProcessedCount int
	lastWriteGen       int64
	// lastProcessedTime is the highest point timestamp consumed so far. Used
	// as the exclusive lower bound for ForEachPoint so each point is appended
	// to the ring exactly once across replays/incremental advances.
	lastProcessedTime int64

	// ring is the chronological subsequence window in ring-buffer form. While
	// filling, ringHead == ringFill; once full, ringHead points to the OLDEST
	// entry (next write position) and cycles modulo WindowSize.
	ring     [hotSaxWindowSize]float64
	ringHead int
	ringFill int

	// Recent symbolic dictionary in round-robin form. Each cell is a SAX
	// symbol in [0, alphabet-1]. Eviction is deterministic (oldest insert
	// goes first), so two instances on identical input agree byte-for-byte.
	dict     [hotSaxDictionarySize][hotSaxWordLen]uint8
	dictHead int
	dictFill int

	// Tick counters & cooldown.
	seen         int
	cooldownLeft int
	lastFireTime int64
}

// HotSAXDetector implements a streaming variant of the Hot-SAX symbolic
// discord detector (Keogh, Lin & Fu 2005, ICDM). At each tick it appends a
// point to a fixed-size sliding window, and on every Nth tick it
// z-normalizes the window, reduces it to a SAX word via PAA + alphabet
// quantization (Lin et al. 2003), and scores the word against a dictionary
// of recent words by the MINDIST lower bound on Euclidean distance. Words
// whose nearest-neighbor MINDIST exceeds a sigma-scaled threshold are
// emitted as anomalies — these are the discords.
//
// Hot-SAX has two attractive properties for the observer pipeline:
//
//   - MINDIST is a provable lower bound on Euclidean distance, so a discord
//     by MINDIST is guaranteed to be a real subsequence outlier.
//   - The z-normalization inside each window collapses sustained level
//     shifts to the same symbolic word — i.e. a level shift is NOT a
//     discord. mannkendall/wasserstein already cover the level-shift case;
//     Hot-SAX isolates SHAPE-level anomalies (transient spikes, period
//     changes, single-bin distributional novelty).
//
// Implements observer.Detector with explicit cursoring (modeled after ESN /
// mannkendall) and observer.SeriesRemover for the catalog teardown contract.
type HotSAXDetector struct {
	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// SAX breakpoints under N(0, 1) for the configured alphabet size.
	// Computed once at construction and shared across all series. For
	// alphabetSize=4 the breakpoints are {Φ⁻¹(0.25), Φ⁻¹(0.5), Φ⁻¹(0.75)} =
	// {-0.6745, 0, +0.6745}.
	breakpoints [hotSaxAlphabetSize - 1]float64

	// Precomputed cell-distance table (Lin 2003 §4): for letters i, j with
	// |i-j| <= 1 the cell distance is 0; otherwise it is
	// breakpoints[max(i,j)-1] - breakpoints[min(i,j)]. Squared up-front so
	// the MINDIST inner loop is one fma per word position.
	cellDistSq [hotSaxAlphabetSize][hotSaxAlphabetSize]float64

	// Per-series state keyed by ref+agg.
	series map[hotSAXStateKey]*hotSAXSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewHotSAXDetector returns a Hot-SAX detector with default settings and
// precomputed N(0,1) breakpoints / cell-distance table.
func NewHotSAXDetector() *HotSAXDetector {
	d := &HotSAXDetector{
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[hotSAXStateKey]*hotSAXSeriesState),
	}
	d.buildTables()
	return d
}

// Name returns the detector name as registered in the catalog.
func (d *HotSAXDetector) Name() string { return "hot_sax" }

// Reset clears all per-series state for replay/reanalysis. The breakpoints
// and cell-distance table are left intact — they are constant by design.
func (d *HotSAXDetector) Reset() {
	d.series = make(map[hotSAXStateKey]*hotSAXSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this hook the per-series map keeps growing with the cumulative
// number of series ever observed even after storage shrinks (mirrors ESN /
// mannkendall / wasserstein / hi_moments).
func (d *HotSAXDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, hotSAXStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. The iteration shape mirrors ESN
// (metrics_detector_esn.go:183-236): cache series on SeriesGeneration,
// bulk-fetch status, replay-skip when nothing has changed, then walk only
// the strictly-new points via ForEachPoint.
func (d *HotSAXDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := hotSAXStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &hotSAXSeriesState{}
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

// processPoint advances the streaming state by exactly one point and returns
// (anomaly, fired). Order of operations is:
//
//  1. Append to the ring; advance ringHead/ringFill.
//  2. Increment seen; decrement cooldown.
//  3. Warmup gate: skip scoring until the dictionary has had a chance to fill.
//  4. Score-stride gate: only score every hotSaxScoreEveryNth tick to amortize
//     the NN MINDIST scan (the dominant per-tick cost).
//  5. Compute the window's mean / stddev; flat-line guard skips degenerate
//     windows AND skips the dict insert (so a flat phase doesn't pollute the
//     dictionary with a single under-defined word).
//  6. Build the SAX word via PAA → quantile lookup.
//  7. NN MINDIST scan against the existing dict (the current word is NOT yet
//     in dict, so no self-match exclusion is needed); score = mindist/stddev.
//  8. Emit if the dictionary has enough entries, the score clears the
//     threshold, and cooldown has expired.
//  9. Insert the word into the dict (round-robin).
func (d *HotSAXDetector) processPoint(state *hotSAXSeriesState, series *observer.Series, agg observer.Aggregate, p observer.Point) (observer.Anomaly, bool) {
	// Step 1: append to ring.
	state.ring[state.ringHead] = p.Value
	state.ringHead = (state.ringHead + 1) % hotSaxWindowSize
	if state.ringFill < hotSaxWindowSize {
		state.ringFill++
	}

	// Step 2: counters / cooldown.
	state.seen++
	if state.cooldownLeft > 0 {
		state.cooldownLeft--
	}

	// Step 3: warmup gate.
	if state.seen <= hotSaxWarmupPoints {
		return observer.Anomaly{}, false
	}

	// Step 4: score-stride gate.
	if state.seen%hotSaxScoreEveryNth != 0 {
		return observer.Anomaly{}, false
	}

	// Step 5: compute window mean / stddev directly from the ring. The ring
	// is fixed at 32 entries, so a direct two-pass computation is ~64 fp ops
	// — already dominated by the NN scan that follows, and avoids the
	// stability concerns of a long-running Welford remove path.
	mean, stddev := ringMeanStd(&state.ring, state.ringFill)
	if stddev < hotSaxStddevFloor {
		return observer.Anomaly{}, false
	}

	// Step 6: build the SAX word. PAA reduces the 32-point window to 8
	// segment means; each segment mean is z-normalized and mapped to a SAX
	// symbol via a fixed-quantile breakpoint comparison (no sort).
	var word [hotSaxWordLen]uint8
	d.encodeWord(&state.ring, state.ringHead, mean, stddev, &word)

	// Step 7: NN MINDIST scan against the existing dictionary. The current
	// word is NOT yet in dict (insert happens in step 9), so trivial
	// self-matches cannot occur.
	var (
		fired   bool
		anomaly observer.Anomaly
	)
	if state.dictFill >= hotSaxMinFill {
		minDist := d.minDistanceToDict(state, &word)
		// Score = MINDIST (Lin 2003 eq. 5). MINDIST is already scale-free
		// because the SAX symbols are derived from z-normalized PAA values
		// using breakpoints in σ units — the cell-distance table inherits
		// those units, so MINDIST is in σ. DEVIATION FROM PLAN: the
		// proposer's plan specified `score = mindist / stddev`, but that
		// would divide by the WINDOW stddev, which is INFLATED by a
		// transient spike — defeating spike detection (see worked example
		// in the test file). The standard Hot-SAX score is raw MINDIST and
		// we follow that here. Threshold (1.6) is unchanged.
		score := minDist

		if score > hotSaxScoreThreshold && state.cooldownLeft <= 0 {
			// Visual cap mirrors the catalog idiom (mannkendall / hi_moments
			// / hl_shift): keep a single near-degenerate window from
			// dominating downstream UI/correlator scoring.
			visualScore := score
			if visualScore > hotSaxScoreCap {
				visualScore = hotSaxScoreCap
			}

			seriesName := series.Name + ":" + aggSuffix(agg)
			anomaly = observer.Anomaly{
				Type: observer.AnomalyTypeMetric,
				Source: observer.SeriesDescriptor{
					Namespace: series.Namespace,
					Name:      series.Name,
					Tags:      series.Tags,
					Aggregate: agg,
				},
				DetectorName: d.Name(),
				Title:        "Hot-SAX discord: " + seriesName,
				Description: fmt.Sprintf(
					"%s SAX discord (word=%v, mindist=%.4f, sigma=%.4f, score=%.2f, dict=%d/%d)",
					seriesName, word, minDist, stddev, score, state.dictFill, hotSaxDictionarySize,
				),
				Timestamp: p.Timestamp,
				Score:     &visualScore,
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineMean:   mean,
					BaselineStddev: stddev,
					CurrentValue:   p.Value,
					DeviationSigma: score,
				},
			}
			state.cooldownLeft = hotSaxCooldownPoints
			state.lastFireTime = p.Timestamp
			fired = true
		}
	}

	// Step 9: insert the word into the dictionary. Always — both pre- and
	// post-fire — so the dictionary keeps tracking the most recent symbolic
	// behavior of the series.
	state.dict[state.dictHead] = word
	state.dictHead = (state.dictHead + 1) % hotSaxDictionarySize
	if state.dictFill < hotSaxDictionarySize {
		state.dictFill++
	}

	return anomaly, fired
}

// ringMeanStd computes mean and population stddev of the first `fill` entries
// of the ring, treating it as an unordered bag. The ring's logical order is
// irrelevant for mean/std, so we don't pay for the modular indexing here.
func ringMeanStd(ring *[hotSaxWindowSize]float64, fill int) (mean, stddev float64) {
	if fill <= 0 {
		return 0, 0
	}
	var sum float64
	for i := 0; i < fill; i++ {
		sum += ring[i]
	}
	mean = sum / float64(fill)
	var sq float64
	for i := 0; i < fill; i++ {
		d := ring[i] - mean
		sq += d * d
	}
	stddev = math.Sqrt(sq / float64(fill))
	return mean, stddev
}

// encodeWord builds a SAX word from the ring. PAA reduces the 32-point window
// to 8 segments of 4 points each by averaging; each segment average is
// z-normalized and mapped to a symbol in [0, alphabet-1] by counting how many
// breakpoints it exceeds.
//
// The ring is read in chronological order: position k of the logical
// subsequence is ring[(ringHead + k) % WindowSize] (after the ring is full,
// ringHead points to the oldest entry, which is the next write position).
func (d *HotSAXDetector) encodeWord(ring *[hotSaxWindowSize]float64, ringHead int, mean, stddev float64, word *[hotSaxWordLen]uint8) {
	for i := 0; i < hotSaxWordLen; i++ {
		var s float64
		base := ringHead + i*hotSaxPAAWidth
		for j := 0; j < hotSaxPAAWidth; j++ {
			s += ring[(base+j)%hotSaxWindowSize]
		}
		paa := s / float64(hotSaxPAAWidth)
		z := (paa - mean) / stddev

		// Symbol = number of breakpoints strictly less than z. Three
		// compares for alphabet=4 — no sort, no log.
		var sym uint8
		for k := 0; k < hotSaxAlphabetSize-1; k++ {
			if z >= d.breakpoints[k] {
				sym++
			}
		}
		word[i] = sym
	}
}

// minDistanceToDict scans the active prefix of the dictionary and returns the
// MINDIST (Lin 2003 eq. 5) between `word` and its nearest neighbor in dict:
//
//	MINDIST(Q', S') = sqrt(n/w) * sqrt(Σ_i cellDist(Q'_i, S'_i)²)
//
// where n=WindowSize and w=WordLen, so n/w = paaWidth = 4. We have squared
// cell distances precomputed in cellDistSq, so the inner loop is one
// addition per word position.
func (d *HotSAXDetector) minDistanceToDict(state *hotSAXSeriesState, word *[hotSaxWordLen]uint8) float64 {
	min := math.MaxFloat64
	for i := 0; i < state.dictFill; i++ {
		stored := &state.dict[i]
		var sumSq float64
		for k := 0; k < hotSaxWordLen; k++ {
			sumSq += d.cellDistSq[word[k]][stored[k]]
		}
		if sumSq < min {
			min = sumSq
		}
		if min == 0 {
			// Trivial nearest neighbor (byte-identical word in dict). No
			// further scan can lower this; short-circuit.
			break
		}
	}
	if min == math.MaxFloat64 {
		return 0
	}
	return math.Sqrt(float64(hotSaxPAAWidth) * min)
}

// buildTables populates breakpoints and cellDistSq for the configured
// alphabet. Hot-SAX uses fixed N(0,1) quantile boundaries; for alphabet=4
// the breakpoints partition the standard normal into four equiprobable
// regions at {-0.6745, 0, +0.6745} (Lin 2003 Table 3).
func (d *HotSAXDetector) buildTables() {
	// alphabet=4: equiprobable quartile boundaries of N(0,1).
	d.breakpoints[0] = -0.6744897501960817
	d.breakpoints[1] = 0
	d.breakpoints[2] = 0.6744897501960817

	for i := 0; i < hotSaxAlphabetSize; i++ {
		for j := 0; j < hotSaxAlphabetSize; j++ {
			if absInt(i-j) <= 1 {
				d.cellDistSq[i][j] = 0
				continue
			}
			lo, hi := i, j
			if lo > hi {
				lo, hi = hi, lo
			}
			c := d.breakpoints[hi-1] - d.breakpoints[lo]
			d.cellDistSq[i][j] = c * c
		}
	}
}

// absInt is a tiny helper so the cell-distance loop reads naturally.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ensureDefaults fills in zero-valued config fields and rebuilds the lookup
// tables if a detector was constructed via &HotSAXDetector{} without using
// the constructor. Mirrors loda / ESN's lazy initialization pattern.
func (d *HotSAXDetector) ensureDefaults() {
	if d.series == nil {
		d.series = make(map[hotSAXStateKey]*hotSAXSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
	// Detect a zero-valued breakpoints array (struct-literal construction)
	// and rebuild. breakpoints[2] is positive in any valid table, so a zero
	// there is the unambiguous signal.
	if d.breakpoints[2] == 0 {
		d.buildTables()
	}
}
