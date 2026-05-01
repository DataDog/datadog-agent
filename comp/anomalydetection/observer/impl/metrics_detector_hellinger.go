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

// hellingerStateKey identifies per-series Hellinger state by ref and aggregation.
type hellingerStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// hellingerSeriesState holds per-series streaming state for the Hellinger
// drift detector.
//
// The cursor (lastProcessedTime/Count/WriteGen) and alert lifecycle
// (inAlert/recoveryCount) follow BOCPD's pattern (metrics_detector_bocpd.go).
// The ring buffer is small (capacity = 2*WindowPoints, default 60 floats), so
// FIFO eviction is O(1) and recomputing histograms over the full buffer per
// new point is cheap (~60 ops × 32 bins).
type hellingerSeriesState struct {
	// Cursor: same fields as bocpdSeriesState (metrics_detector_bocpd.go:24-26).
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64

	// Sliding window of the most recent values, capacity = 2*WindowPoints.
	// Implemented as a fixed-capacity ring buffer:
	//   - while filling: buf grows via append, head stays at 0.
	//   - once full:     new values overwrite buf[head] and head advances.
	// Chronological order at evaluation time is buf[(head+i) % cap] for i ∈ [0, size).
	buf  []float64
	head int
	size int

	// Alert lifecycle (mirrors BOCPD lines 271-287). Without the recovery gate
	// the detector re-fires every tick of a sustained drift, which pollutes
	// the correlator (this is the failure mode in exp-0030, exp-0036).
	inAlert       bool
	recoveryCount int
}

// HellingerConfig holds tunable parameters for the Hellinger detector.
//
// Defaults are taken from the proposer's plan (Bifet & Gavaldà 2007 /
// Ditzler & Polikar 2011 family). The MAD gate is the single most important
// false-positive guard: Hellinger alone fires on any tail-mass relocation, so
// we additionally require a real central-tendency shift before reporting.
type HellingerConfig struct {
	// WindowPoints is the size of each side of the two-window comparison.
	// Total buffer size is WindowPoints*2. Default: 30.
	WindowPoints int `json:"window_points"`

	// Bins is the number of histogram bins used to build the empirical PDFs.
	// Default: 32. With WindowPoints=30 and Bins=32 the histogram cost is
	// negligible (~60 element scan per fire).
	Bins int `json:"bins"`

	// HellingerThreshold is the minimum H(p, q) (range [0, 1]) required to
	// consider firing. Default: 0.55.
	HellingerThreshold float64 `json:"hellinger_threshold"`

	// MinDeviationMAD is the minimum |post_median - pre_median| / pre_MAD.
	// Default: 2.5. See package note above on why this gate is mandatory.
	MinDeviationMAD float64 `json:"min_deviation_mad"`

	// RecoveryPoints is how many consecutive non-triggering points are needed
	// to exit alert state. Default: 10 (same default as BOCPD).
	RecoveryPoints int `json:"recovery_points"`

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate `json:"-"`
}

// DefaultHellingerConfig returns a HellingerConfig with default values.
func DefaultHellingerConfig() HellingerConfig {
	return HellingerConfig{
		WindowPoints:       30,
		Bins:               32,
		HellingerThreshold: 0.55,
		MinDeviationMAD:    2.5,
		RecoveryPoints:     10,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
	}
}

// HellingerDetector detects distribution drift using the Hellinger distance
// between two adjacent equal-length sliding windows of histogrammed values
// (Bifet & Gavaldà 2007, Ditzler & Polikar 2011).
//
// Compared to the moment-based detectors in this catalog (BOCPD models a
// single Gaussian likelihood; ScanMW/ScanWelch are mean/median-shift tests),
// Hellinger compares full empirical PDFs and so picks up shape changes —
// variance bursts, bimodal onsets, distribution skewing — that purely
// moment-based detectors miss. The MAD gate trims the false-positive tail by
// requiring a real central-tendency shift alongside the shape change.
//
// Implements observer.Detector (streaming).
type HellingerDetector struct {
	config HellingerConfig

	// per-(series, aggregation) state.
	series map[hellingerStateKey]*hellingerSeriesState

	// Cache the discovered series list across Detect calls. Refresh when
	// storage reports the set of known series has grown.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64

	// Reusable scratch buffers used during evaluate() to avoid per-call
	// allocation. Reused across (series, agg) iterations and across calls.
	preBuf, postBuf   []float64
	preHist, postHist []float64
}

// NewHellingerDetector constructs the detector with default configuration.
// This is the catalog factory entry point (see component_catalog.go).
func NewHellingerDetector() *HellingerDetector {
	return NewHellingerDetectorWithConfig(DefaultHellingerConfig())
}

// NewHellingerDetectorWithConfig constructs the detector with the given config.
// Zero-valued / out-of-range fields are filled from DefaultHellingerConfig().
func NewHellingerDetectorWithConfig(config HellingerConfig) *HellingerDetector {
	defaults := DefaultHellingerConfig()
	if config.WindowPoints < 2 {
		config.WindowPoints = defaults.WindowPoints
	}
	if config.Bins < 2 {
		config.Bins = defaults.Bins
	}
	if config.HellingerThreshold <= 0 || config.HellingerThreshold > 1 {
		config.HellingerThreshold = defaults.HellingerThreshold
	}
	if config.MinDeviationMAD <= 0 {
		config.MinDeviationMAD = defaults.MinDeviationMAD
	}
	if config.RecoveryPoints <= 0 {
		config.RecoveryPoints = defaults.RecoveryPoints
	}
	if len(config.Aggregations) == 0 {
		config.Aggregations = defaults.Aggregations
	}
	return &HellingerDetector{
		config: config,
		series: make(map[hellingerStateKey]*hellingerSeriesState),
	}
}

// Name returns the detector name.
func (d *HellingerDetector) Name() string {
	return "hellinger"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *HellingerDetector) Reset() {
	d.series = make(map[hellingerStateKey]*hellingerSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. It discovers series, reads only newly
// visible points, and updates per-series ring buffer state incrementally.
//
// Iteration shape mirrors BOCPD (metrics_detector_bocpd.go:189-250) — the
// recovery counter requires point-by-point processing rather than ScanMW's
// segment-rescan model.
func (d *HellingerDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	var allAnomalies []observer.Anomaly

	for _, meta := range d.cachedSeries {
		for _, agg := range d.config.Aggregations {
			sk := hellingerStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &hellingerSeriesState{
					buf: make([]float64, 0, 2*d.config.WindowPoints),
				}
				d.series[sk] = state
			}

			visibleCount := storage.PointCountUpTo(meta.Ref, dataTime)
			currentGen := storage.WriteGeneration(meta.Ref)
			mergeOccurred := visibleCount == state.lastProcessedCount && currentGen != state.lastWriteGen
			if visibleCount <= state.lastProcessedCount && !mergeOccurred {
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
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(series *observer.Series, p observer.Point) {
				pointsSeen = true
				if anomaly := d.processPoint(state, p, series, agg); anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}

			if !pointsSeen && mergeOccurred {
				state.lastWriteGen = currentGen
				continue
			}
			if pointsSeen {
				state.lastProcessedCount = visibleCount
				state.lastWriteGen = currentGen
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint advances state with one new observation. Returns a non-nil
// anomaly only on the rising edge of an alert (mirrors BOCPD.processPoint).
func (d *HellingerDetector) processPoint(state *hellingerSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate) *observer.Anomaly {
	d.appendValue(state, p.Value)

	cap2W := 2 * d.config.WindowPoints
	if state.size < cap2W {
		// Warming up — need a full buffer to evaluate two equal-length windows.
		return nil
	}

	h, preMedian, postMedian, preMAD, triggered := d.evaluate(state)

	if !triggered {
		if state.inAlert {
			state.recoveryCount++
			if state.recoveryCount >= d.config.RecoveryPoints {
				state.inAlert = false
				state.recoveryCount = 0
			}
		}
		return nil
	}

	// Triggered: reset recovery counter regardless.
	state.recoveryCount = 0
	if state.inAlert {
		// Already alerting — wait for recovery before re-emitting.
		return nil
	}
	state.inAlert = true

	denom := preMAD
	if denom < 1e-10 {
		denom = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}
	deviationMAD := math.Abs(postMedian-preMedian) / denom

	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	displayName := series.Name + ":" + aggSuffix(agg)
	score := h
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "Hellinger drift: " + displayName,
		Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, H=%.3f, %.1f MADs)",
			displayName, direction, preMedian, postMedian, h, deviationMAD),
		Timestamp: p.Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: preMedian,
			BaselineMAD:    preMAD,
			Threshold:      d.config.HellingerThreshold,
			CurrentValue:   postMedian,
			DeviationSigma: deviationMAD,
		},
	}
}

// appendValue appends v to the ring buffer, evicting the oldest entry when full.
func (d *HellingerDetector) appendValue(state *hellingerSeriesState, v float64) {
	capW := cap(state.buf)
	if state.size < capW {
		state.buf = append(state.buf, v)
		state.size++
		return
	}
	// Buffer full: overwrite at head and advance head (FIFO eviction).
	state.buf[state.head] = v
	state.head++
	if state.head == capW {
		state.head = 0
	}
}

// evaluate computes the Hellinger distance and (when it passes the threshold)
// the median/MAD gate. Returns (H, preMedian, postMedian, preMAD, triggered).
//
// median/MAD are only computed when H >= HellingerThreshold; otherwise
// preMedian/postMedian/preMAD are 0 — they are not used by the caller in the
// not-triggered branch.
func (d *HellingerDetector) evaluate(state *hellingerSeriesState) (float64, float64, float64, float64, bool) {
	W := d.config.WindowPoints
	cap2W := 2 * W

	// Materialize windows in chronological order using the reusable scratch
	// slices. After warmup state.size == cap2W, so this always copies cap2W
	// values total.
	d.preBuf = d.preBuf[:0]
	d.postBuf = d.postBuf[:0]
	for i := 0; i < W; i++ {
		d.preBuf = append(d.preBuf, state.buf[(state.head+i)%cap2W])
	}
	for i := 0; i < W; i++ {
		d.postBuf = append(d.postBuf, state.buf[(state.head+W+i)%cap2W])
	}

	h := hellingerDistance(d.preBuf, d.postBuf, d.config.Bins, &d.preHist, &d.postHist)
	if h < d.config.HellingerThreshold {
		return h, 0, 0, 0, false
	}

	// Hellinger threshold met — verify the central-tendency shift gate.
	preMedian := detectorMedian(d.preBuf)
	postMedian := detectorMedian(d.postBuf)
	preMAD := detectorMAD(d.preBuf, preMedian, false)

	denom := preMAD
	if denom < 1e-10 {
		denom = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}
	deviationMAD := math.Abs(postMedian-preMedian) / denom
	if deviationMAD < d.config.MinDeviationMAD {
		return h, preMedian, postMedian, preMAD, false
	}

	return h, preMedian, postMedian, preMAD, true
}

// hellingerDistance computes the Hellinger distance H(p, q) where p and q are
// empirical PDFs from histogramming `pre` and `post` over `bins` equally
// spaced bins spanning [min(combined), max(combined)]. Result is in [0, 1].
//
// Returns 0 in the degenerate cases that admit no defined drift:
//   - either side empty
//   - bins < 1
//   - all combined values identical (zero span → distributions trivially equal)
//
// The histograms are written into *preHistOut and *postHistOut and reused
// across calls to avoid per-call allocation.
func hellingerDistance(pre, post []float64, bins int, preHistOut, postHistOut *[]float64) float64 {
	if len(pre) == 0 || len(post) == 0 || bins < 1 {
		return 0
	}

	// Determine combined [min, max] in a single pass per side.
	min, max := pre[0], pre[0]
	for _, v := range pre {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	for _, v := range post {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	if span == 0 {
		// All values identical → distributions equal → H = 0.
		return 0
	}

	// Reuse / grow histogram scratch slices and zero them.
	if cap(*preHistOut) < bins {
		*preHistOut = make([]float64, bins)
	} else {
		*preHistOut = (*preHistOut)[:bins]
		for i := range *preHistOut {
			(*preHistOut)[i] = 0
		}
	}
	if cap(*postHistOut) < bins {
		*postHistOut = make([]float64, bins)
	} else {
		*postHistOut = (*postHistOut)[:bins]
		for i := range *postHistOut {
			(*postHistOut)[i] = 0
		}
	}
	preHist := *preHistOut
	postHist := *postHistOut

	bf := float64(bins)
	bin := func(v float64) int {
		// Map v ∈ [min, max] → bin in [0, bins). Clamp the max-end inclusive
		// case to the last bin so we never index out of range.
		idx := int(((v - min) / span) * bf)
		if idx >= bins {
			idx = bins - 1
		}
		if idx < 0 {
			idx = 0
		}
		return idx
	}
	for _, v := range pre {
		preHist[bin(v)]++
	}
	for _, v := range post {
		postHist[bin(v)]++
	}

	preTotal := float64(len(pre))
	postTotal := float64(len(post))
	if preTotal == 0 || postTotal == 0 {
		return 0
	}

	// H(p, q) = (1/√2) * sqrt( Σ_i (sqrt(p_i) - sqrt(q_i))^2 )
	var sumSq float64
	for i := 0; i < bins; i++ {
		dp := math.Sqrt(preHist[i] / preTotal)
		dq := math.Sqrt(postHist[i] / postTotal)
		diff := dp - dq
		sumSq += diff * diff
	}
	h := math.Sqrt(sumSq) / math.Sqrt2
	if h > 1 {
		h = 1
	}
	if h < 0 {
		h = 0
	}
	return h
}
