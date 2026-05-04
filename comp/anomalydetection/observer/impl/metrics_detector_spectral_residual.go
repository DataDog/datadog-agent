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

// Spectral-residual detector — Microsoft's frequency-domain saliency
// algorithm (Ren et al., "Time-Series Anomaly Detection Service at
// Microsoft", KDD 2019).
//
// Per (series, aggregation), we maintain a rolling WindowN-point ring of
// raw values plus two MAD windows (saliency-history, raw-value). On each
// ingested point — once the value ring has filled — we run the SR
// transform on the ring contents:
//
//  1. DFT(values) → log-magnitude A[k], phase φ[k]
//  2. moving-average smoother (half-width AvgFilterQ, reflective
//     boundaries) → V[k]
//  3. spectral residual R[k] = A[k] - V[k]
//  4. inverse DFT of exp(R)·exp(iφ) → saliency map S[n]
//  5. score for the latest point = |S[N-1]|
//
// We then fire when the latest saliency clears a robust MAD-scaled gate
// AND the raw value clears an effect-size MAD gate (mirroring
// holt_residual / kl_divergence so the precision frontier on existing
// labels stays intact). A short refractory suppresses repeat fires while
// the ring adapts.
//
// The detector is structurally orthogonal to the rest of the catalog —
// bocpd is Bayesian time-domain, scanmw / scanwelch / kl_divergence are
// window-pair drift tests, holt_residual is one-step forecast residual,
// matrix_profile is shape-distance — none look at frequency content. SR
// is well-suited to transient spikes, missing-seasonality, and structural
// breaks that time-domain detectors smooth over.
//
// Per-tick cost: one forward direct DFT on N=64 (≈ 4 K float
// multiplications) plus one O(N) inverse-DFT-at-N-1 (we only need the
// most-recent saliency sample) plus two O(W log W) MAD sorts on W=60
// windows. Per-(series, aggregation) memory: 64 raw + 60 saliency MAD
// + 60 value MAD float64 ≈ 1.5 KB + bookkeeping. A single shared
// 2*N*N pair of cos/sin tables (≈ 64 KB) is allocated lazily on first
// DFT, per detector instance.

// Spectral-residual tunable defaults. Higher z and effect-size gates
// than holt_residual because saliency on real metrics has heavier tails
// than smoothed forecast residuals — pulling the threshold up from 4.5
// to 5.0 keeps the false-positive frontier comparable.
const (
	srWindowN         = 64
	srSaliencyMADWin  = 60
	srValueMADWin     = 60
	srAvgFilterQ      = 3
	srZThreshold      = 5.0
	srMinDeviationMAD = 3.0
	srRefractory      = 24
	// srWarmupPoints is a coarse lower bound on points-seen before the
	// gate may evaluate. The MAD windows naturally fill at
	// WindowN + SaliencyMADWin = 124 points, which is the strict gate;
	// this constant exists so the gate is also configurable independently
	// of the window sizes.
	srWarmupPoints  = 64 + 30
	srTimestampRing = 16
)

// srStateKey identifies per-series state by ref and aggregation.
type srStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// srSeriesState holds the streaming state for one (series, aggregation).
//
// The lifecycle has two implicit stages: ring-fill (values has fewer than
// WindowN entries; we just accumulate raw values and feed valHist) and
// scoring (values is full; every ingested point produces a saliency that
// is gated against the MAD windows). The cursor (lastProcessedCount /
// lastWriteGen / lastProcessedTime) mirrors kl_divergence exactly.
type srSeriesState struct {
	// cursor (mirrors kl_divergence)
	lastProcessedCount int
	lastWriteGen       int64
	lastProcessedTime  int64

	// values is the raw-value FIFO ring; cap = WindowN. Once full, the
	// SR transform operates on it on every ingest.
	values []float64
	// saliencyHist is a FIFO ring of recent saliency scores; cap =
	// SaliencyMADWin. Used by the MAD-z gate.
	saliencyHist []float64
	// valHist is a FIFO ring of recent raw values; cap = ValueMADWin.
	// Used by the effect-size gate (devMAD).
	valHist []float64

	// pointsSeen counts every ingested point ever observed; the gate
	// requires it to reach WarmupPoints before evaluating.
	pointsSeen          int
	refractoryRemaining int

	recentTimestamps  []int64
	lastSeenTimestamp int64

	// captured series metadata (first non-empty observation suffices)
	seriesMetaCaptured bool
	seriesNamespace    string
	seriesName         string
	seriesTags         []string
}

// SpectralResidualDetector implements the SR algorithm over a streaming
// per-series window.
//
// Implements observer.Detector and observer.SeriesRemover. Tunable fields
// are exported so tests / testbench may override defaults after
// construction; NewSpectralResidualDetector populates them.
type SpectralResidualDetector struct {
	// WindowN is the FFT window size. Must be ≥ 4. Powers of two are
	// preferred for cache-friendly DFT scratch sizing but not required.
	WindowN int
	// SaliencyMADWin is the rolling FIFO size for the saliency MAD gate.
	SaliencyMADWin int
	// ValueMADWin is the rolling FIFO size for the raw-value MAD gate.
	ValueMADWin int
	// AvgFilterQ is the half-width of the magnitude-smoothing moving
	// average. Total window = 2*AvgFilterQ + 1.
	AvgFilterQ int
	// ZThreshold is the saliency MAD-z threshold (one-sided positive).
	ZThreshold float64
	// MinDeviationMAD is the minimum |x_t - median(values)| / sigmaValue
	// the candidate point must clear. Suppresses noise-only false fires.
	MinDeviationMAD float64
	// Refractory is the number of ingested points to suppress fires for
	// after firing.
	Refractory int
	// WarmupPoints is a minimum points-seen gate before scoring. The
	// strict gate is the MAD-window fill, which naturally requires
	// WindowN + SaliencyMADWin points.
	WarmupPoints int

	// Aggregations to run detection on. Default: [Average, Count].
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[srStateKey]*srSeriesState

	// cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64

	// Pre-allocated DFT scratch and trig tables. Allocated lazily on
	// the first call that has a full ring. A detector that never sees
	// a full window therefore pays no memory cost. Reuse is single-
	// threaded — Detect is invoked serially per engine.run() tick.
	fftReal  []float64
	fftImag  []float64
	cosTable []float64 // cosTable[k*N+n] = cos(2π·k·n/N)
	sinTable []float64 // sinTable[k*N+n] = sin(2π·k·n/N)
	mag      []float64
	phase    []float64
	smoothed []float64
}

// NewSpectralResidualDetector creates an SR detector with default settings.
// The catalog factory calls this with no arguments; tunables can be
// overridden post-construction.
func NewSpectralResidualDetector() *SpectralResidualDetector {
	return &SpectralResidualDetector{
		WindowN:         srWindowN,
		SaliencyMADWin:  srSaliencyMADWin,
		ValueMADWin:     srValueMADWin,
		AvgFilterQ:      srAvgFilterQ,
		ZThreshold:      srZThreshold,
		MinDeviationMAD: srMinDeviationMAD,
		Refractory:      srRefractory,
		WarmupPoints:    srWarmupPoints,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[srStateKey]*srSeriesState),
	}
}

// Name implements observer.Detector.
func (d *SpectralResidualDetector) Name() string { return "spectral_residual" }

// Reset clears all per-series state for replay/reanalysis.
func (d *SpectralResidualDetector) Reset() {
	d.series = make(map[srStateKey]*srSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops per-series state for refs that storage has freed.
// Without this teardown the per-series map would grow unbounded with the
// cumulative series count even after storage shrinks. Mirrors
// HoltResidualDetector.RemoveSeries.
func (d *SpectralResidualDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, srStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Streams new points into per-series
// SR state and emits anomalies when both gates pass and refractory is clear.
func (d *SpectralResidualDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
			sk := srStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newState()
				d.series[sk] = state
			}

			// Replay-gate: skip when nothing has been written and the
			// cursor matches.
			if status.pointCount == state.lastProcessedCount && status.writeGeneration == state.lastWriteGen {
				continue
			}

			anomalies := d.ingestNewPoints(storage, meta.Ref, agg, state, dataTime)
			for j := range anomalies {
				anomalies[j].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}
			allAnomalies = append(allAnomalies, anomalies...)

			state.lastProcessedCount = status.pointCount
			state.lastProcessedTime = dataTime
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// newState allocates a per-series state with appropriately sized scratch
// buffers.
func (d *SpectralResidualDetector) newState() *srSeriesState {
	return &srSeriesState{
		values:           make([]float64, 0, d.WindowN),
		saliencyHist:     make([]float64, 0, d.SaliencyMADWin),
		valHist:          make([]float64, 0, d.ValueMADWin),
		recentTimestamps: make([]int64, 0, srTimestampRing),
	}
}

// ingestNewPoints streams points in (state.lastProcessedTime, dataTime]
// into the per-series SR state. Returns any anomalies fired by the gate.
func (d *SpectralResidualDetector) ingestNewPoints(
	storage observer.StorageReader,
	ref observer.SeriesRef,
	agg observer.Aggregate,
	state *srSeriesState,
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
		srPushTimestamp(state, p.Timestamp)

		anomaly, hasFire := d.processPoint(state, p, agg)

		// Refractory countdown — decrement on every ingest, regardless
		// of whether the point would have fired. Mirrors holt_residual.
		if state.refractoryRemaining > 0 {
			state.refractoryRemaining--
		}

		if hasFire {
			fired = append(fired, anomaly)
		}
	})

	return fired
}

// processPoint runs one SR step: ring-update → (when ring is full) DFT →
// log-magnitude → moving-average smoother → spectral residual → IFFT-at-
// latest → gate. Returns the populated Anomaly when the gate fires (and
// refractory is clear); otherwise returns false.
//
// Mirrors HoltResidualDetector.processPoint's structure: gates evaluated
// against historical baselines BEFORE the new sample is pushed, so the
// candidate point doesn't bias its own threshold.
func (d *SpectralResidualDetector) processPoint(
	state *srSeriesState,
	p observer.Point,
	agg observer.Aggregate,
) (observer.Anomaly, bool) {
	state.pointsSeen++

	// 1. Push raw value into the SR ring.
	pushFIFO(&state.values, d.WindowN, p.Value)

	// 1b. While the ring is filling, just keep valHist in sync with the
	// raw-point stream and bail. We can't compute saliency without a full
	// ring.
	if len(state.values) < d.WindowN {
		pushFIFO(&state.valHist, d.ValueMADWin, p.Value)
		return observer.Anomaly{}, false
	}

	// 2. Compute the saliency at the latest sample (index N-1).
	saliency := d.computeSaliencyAtLatest(state.values)

	// 3. Gate (only meaningful once both MAD windows are full; the
	// candidate's saliency / value haven't been pushed yet so the
	// thresholds use the historical baseline).
	windowsReady := len(state.saliencyHist) >= d.SaliencyMADWin && len(state.valHist) >= d.ValueMADWin
	warmupOK := state.pointsSeen >= d.WarmupPoints
	var (
		fire     bool
		z        float64
		devMAD   float64
		medSal   float64
		sigmaSal float64
		medVal   float64
		sigmaVal float64
	)
	if windowsReady && warmupOK {
		medSal = detectorMedian(state.saliencyHist)
		sigmaSal = detectorMAD(state.saliencyHist, medSal, true)
		sigmaSal = floorSigma(sigmaSal, state.saliencyHist)
		z = (saliency - medSal) / sigmaSal

		medVal = detectorMedian(state.valHist)
		sigmaVal = detectorMAD(state.valHist, medVal, true)
		sigmaVal = floorSigma(sigmaVal, state.valHist)
		devMAD = math.Abs(p.Value-medVal) / sigmaVal

		// Saliency is non-negative by construction; the gate is
		// one-sided (only positive z fires) — a saliency below baseline
		// is the absence of a peak, not a peak.
		fire = z >= d.ZThreshold && devMAD >= d.MinDeviationMAD && state.refractoryRemaining == 0
	}

	// 4. Push new saliency and raw value into their MAD windows. Done
	// AFTER the gate so the standardisation uses the historical baseline.
	pushFIFO(&state.saliencyHist, d.SaliencyMADWin, saliency)
	pushFIFO(&state.valHist, d.ValueMADWin, p.Value)

	if !fire {
		return observer.Anomaly{}, false
	}

	// 5. Build the anomaly.
	score := z
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
		Title:        "Spectral residual: " + seriesName,
		Description: fmt.Sprintf(
			"%s saliency exceeded baseline (saliency=%.4f, medianSaliency=%.4f, |z|=%.2f, value=%.4f, %.1f valueMADs)",
			seriesName, saliency, medSal, z, p.Value, devMAD,
		),
		Timestamp:           state.lastSeenTimestamp,
		Score:               &score,
		SamplingIntervalSec: medianTimestampInterval(state.recentTimestamps),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: medSal,
			BaselineMAD:    sigmaSal,
			CurrentValue:   p.Value,
			DeviationSigma: z,
			Threshold:      d.ZThreshold,
		},
	}

	// 6. Arm refractory.
	state.refractoryRemaining = d.Refractory

	return anomaly, true
}

// computeSaliencyAtLatest runs the SR transform on the value ring and
// returns the magnitude of the inverse-DFT at index N-1 (the most-recent
// sample). We compute only the latest output sample because the gate
// only inspects S[N-1]: an O(N²) forward DFT plus an O(N) reverse trick
// at one sample, instead of a second O(N²) inverse for the full map.
func (d *SpectralResidualDetector) computeSaliencyAtLatest(values []float64) float64 {
	n := d.WindowN
	d.ensureFFTScratch()

	// Forward DFT: F[k] = Σ_n x[n] * exp(-2πi·k·n/N)
	//   = Σ_n x[n] * (cos - i·sin) at angle 2π·k·n/N.
	for k := 0; k < n; k++ {
		var re, im float64
		row := k * n
		for nn := 0; nn < n; nn++ {
			c := d.cosTable[row+nn]
			s := d.sinTable[row+nn]
			v := values[nn]
			re += v * c
			im -= v * s
		}
		d.fftReal[k] = re
		d.fftImag[k] = im
	}

	// Log-magnitude (with epsilon to keep log finite at empty bins)
	// and phase. The phase is needed to reconstruct the complex
	// frequency-domain vector before the inverse DFT.
	for k := 0; k < n; k++ {
		re := d.fftReal[k]
		im := d.fftImag[k]
		mag := math.Sqrt(re*re + im*im)
		d.mag[k] = math.Log(mag + 1e-9)
		d.phase[k] = math.Atan2(im, re)
	}

	// Moving-average smoother on the log-magnitude, with reflective
	// boundaries: V[k] = mean_{j=-q..q} A[reflect(k+j)].
	q := d.AvgFilterQ
	denom := float64(2*q + 1)
	for k := 0; k < n; k++ {
		var sum float64
		for j := -q; j <= q; j++ {
			sum += d.mag[srReflectIdx(k+j, n)]
		}
		d.smoothed[k] = sum / denom
	}

	// Spectral residual R[k] = A[k] - V[k]; reconstruct
	// G[k] = exp(R[k]) · (cos φ + i·sin φ). We overwrite
	// fftReal/fftImag in place to avoid a second N-length scratch.
	for k := 0; k < n; k++ {
		amp := math.Exp(d.mag[k] - d.smoothed[k])
		d.fftReal[k] = amp * math.Cos(d.phase[k])
		d.fftImag[k] = amp * math.Sin(d.phase[k])
	}

	// Inverse DFT at index N-1 only:
	//   s[N-1] = Σ_k G[k] · exp(2πi·k·(N-1)/N)
	//          = Σ_k (Gre + i·Gim) · (cos + i·sin) at angle 2π·k·(N-1)/N.
	// The saliency is |s[N-1]|. No 1/N normalisation — the saliency is
	// used relatively against its own MAD baseline. We index cosTable
	// at [k*N + (N-1)] which is the (k, N-1) entry; cos/sin are
	// symmetric in (k, n) so this equals the (N-1, k) entry needed by
	// the IFFT formula.
	var re, im float64
	nm1 := n - 1
	for k := 0; k < n; k++ {
		c := d.cosTable[k*n+nm1]
		s := d.sinTable[k*n+nm1]
		gr := d.fftReal[k]
		gi := d.fftImag[k]
		re += gr*c - gi*s
		im += gr*s + gi*c
	}
	return math.Sqrt(re*re + im*im)
}

// ensureFFTScratch allocates the DFT scratch buffers and pre-computes the
// cos/sin lookup tables on first use. Idempotent — subsequent calls are
// a single nil-check.
func (d *SpectralResidualDetector) ensureFFTScratch() {
	if d.fftReal != nil {
		return
	}
	n := d.WindowN
	d.fftReal = make([]float64, n)
	d.fftImag = make([]float64, n)
	d.mag = make([]float64, n)
	d.phase = make([]float64, n)
	d.smoothed = make([]float64, n)
	d.cosTable = make([]float64, n*n)
	d.sinTable = make([]float64, n*n)
	for k := 0; k < n; k++ {
		row := k * n
		for nn := 0; nn < n; nn++ {
			angle := 2 * math.Pi * float64(k*nn) / float64(n)
			d.cosTable[row+nn] = math.Cos(angle)
			d.sinTable[row+nn] = math.Sin(angle)
		}
	}
}

// srReflectIdx returns the reflective-boundary index for i in [0, n).
// For q << n one pass of reflection suffices: with q=3 and n=64 the
// extreme inputs are -3 and n+2, both within a single reflection step.
func srReflectIdx(i, n int) int {
	if i < 0 {
		return -i - 1
	}
	if i >= n {
		return 2*n - i - 1
	}
	return i
}

// srPushTimestamp appends ts to the recent-timestamp ring, dropping the
// oldest if full. Mirrors holt_residual.pushTimestamp but typed for
// srSeriesState. The ring is small (cap srTimestampRing) so the
// shift-left cost is negligible.
func srPushTimestamp(state *srSeriesState, ts int64) {
	if cap(state.recentTimestamps) == 0 {
		state.recentTimestamps = make([]int64, 0, srTimestampRing)
	}
	if len(state.recentTimestamps) < cap(state.recentTimestamps) {
		state.recentTimestamps = append(state.recentTimestamps, ts)
		return
	}
	copy(state.recentTimestamps, state.recentTimestamps[1:])
	state.recentTimestamps[len(state.recentTimestamps)-1] = ts
}

// ensureDefaults fills zero-valued config fields with sensible defaults.
// Mirrors holt_residual.ensureDefaults so the detector behaves sanely
// when constructed via reflective paths that bypass
// NewSpectralResidualDetector.
func (d *SpectralResidualDetector) ensureDefaults() {
	if d.WindowN <= 0 {
		d.WindowN = srWindowN
	}
	if d.SaliencyMADWin <= 0 {
		d.SaliencyMADWin = srSaliencyMADWin
	}
	if d.ValueMADWin <= 0 {
		d.ValueMADWin = srValueMADWin
	}
	if d.AvgFilterQ <= 0 {
		d.AvgFilterQ = srAvgFilterQ
	}
	if d.ZThreshold <= 0 {
		d.ZThreshold = srZThreshold
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = srMinDeviationMAD
	}
	if d.Refractory <= 0 {
		d.Refractory = srRefractory
	}
	if d.WarmupPoints <= 0 {
		d.WarmupPoints = srWarmupPoints
	}
	if d.series == nil {
		d.series = make(map[srStateKey]*srSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
