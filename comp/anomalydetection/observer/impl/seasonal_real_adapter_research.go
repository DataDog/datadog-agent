// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Research-only ablation. The engine calls this adapter serially; posterior
// state remains private to each detector. Only transient likelihood work is shared.
// Research ablation only: retain per-series detectors/readers, bypass seasonal work.
var seasonalStructureOnly = os.Getenv("SEASONAL_ATTRIBUTION_STRUCTURE_ONLY") == "1"

var seasonalIntegrationMode = os.Getenv("OBSERVER_REAL_SEASONAL_INTEGRATION")

var realSeasonalTrain = realSeasonalPlan.cycles() * 240
var realSeasonalCalibration = realSeasonalPlan.CalibrationHours * 240

const realSeasonalStep int64 = 15

var realSeasonalGaugeNames = map[string]bool{
	"container.memory.usage": true, "container.memory.rss": true,
	"container.pid.thread_count": true, "container.pid.open_files": true,
	"system.load.1": true, "system.mem.used": true, "redis.mem.used": true,
	"redis.clients.connected": true, "postgresql.connections": true, "postgresql.locks": true,
}

type realSeasonalScale struct {
	Original      []float64 `json:"original"`
	Applied       []float64 `json:"applied"`
	V0            float64   `json:"v0"`
	TrainingStart int       `json:"training_start"`
}

func realSeasonalMedian(v []float64) float64 {
	x := append([]float64(nil), v...)
	sort.Float64s(x)
	n := len(x)
	if n%2 == 1 {
		return x[n/2]
	}
	return x[n/2-1]/2 + x[n/2]/2
}

// Exact preceding study estimator, reading only the frozen training ring.
func realSeasonalFitScale(m *hourlyAdaptiveSeasonalModel) (*realSeasonalScale, error) {
	if m.Profile == nil || m.Last != realSeasonalTrain-1 {
		return nil, fmt.Errorf("no frozen training profile")
	}
	p := m.Profile.Period
	n := realSeasonalPlan.cycles()
	if p <= 0 || n*p > realSeasonalTrain {
		return nil, fmt.Errorf("invalid period")
	}
	out := &realSeasonalScale{Original: make([]float64, p), Applied: make([]float64, p), TrainingStart: realSeasonalTrain - n*p}
	groups := make([][]float64, p)
	maxAbs := 0.0
	for i := out.TrainingStart; i < realSeasonalTrain; i++ {
		v, ok := m.sample(i)
		if !ok {
			return nil, fmt.Errorf("missing training slot%d", i)
		}
		q := hourlyAdaptiveMod(i-m.Offset, p)
		groups[q] = append(groups[q], v)
		maxAbs = math.Max(maxAbs, math.Abs(v))
	}
	pooled := make([]float64, 0, n*p)
	mse := make([]float64, p)
	for q, g := range groups {
		if len(g) != n {
			return nil, fmt.Errorf("incomplete phase")
		}
		bias := realSeasonalMedian(g) - m.Profile.Center[q]
		for i, v := range g {
			other := make([]float64, 0, n-1)
			for j, w := range g {
				if j != i {
					other = append(other, w)
				}
			}
			e := (v - realSeasonalMedian(other)) + bias
			if !hourlyAdaptiveFinite(e) {
				return nil, fmt.Errorf("nonfinite error")
			}
			pooled = append(pooled, e)
			mse[q] += e * e / float64(n)
		}
	}
	median := realSeasonalMedian(pooled)
	dev := make([]float64, len(pooled))
	for i, e := range pooled {
		dev[i] = math.Abs(e - median)
	}
	sigma := 1.4826 * realSeasonalMedian(dev)
	out.V0 = sigma * sigma
	floor := math.Max(math.SmallestNonzeroFloat64, 32*(math.Nextafter(maxAbs, math.Inf(1))-maxAbs))
	if !hourlyAdaptiveFinite(out.V0) || !hourlyAdaptiveFinite(floor) {
		return nil, fmt.Errorf("nonfinite pooled scale")
	}
	for q, v := range mse {
		out.Original[q] = math.Max(math.Sqrt((float64(n)*v+2*out.V0)/float64(n+2)), floor)
		out.Applied[q] = math.Max(out.Original[q], math.Sqrt(out.V0))
		if !hourlyAdaptiveFinite(out.Applied[q]) || out.Applied[q] <= 0 {
			return nil, fmt.Errorf("invalid divisor")
		}
	}
	return out, nil
}

func realSeasonalVariance(values []float64) float64 {
	mean, m2 := 0.0, 0.0
	for i, x := range values {
		delta := x - mean
		mean += delta / float64(i+1)
		m2 += delta * (x - mean)
	}
	return m2 / float64(len(values)-1)
}

// The count detects preloaded future points becoming visible. The generation
// detects writes (including corrections) when the visible count is unchanged.
// This is only a read cache: time-based admission checks and detectors still run.
type seasonalReadStamp struct {
	initialized bool
	count       int
	generation  int64
}

func (c *seasonalReadStamp) changed(storage observer.StorageReader, ref observer.SeriesRef, now int64) bool {
	count, generation := storage.PointCountUpTo(ref, now), storage.WriteGeneration(ref)
	changed := !c.initialized || count != c.count || generation != c.generation
	c.initialized, c.count, c.generation = true, count, generation
	return changed
}

type realSeasonalGaugeState struct {
	readStamp               seasonalReadStamp
	model                   *hourlyAdaptiveSeasonalModel
	scale                   *realSeasonalScale
	detector, countDetector observer.Detector
	origin, last            int64
	started                 bool
	slot, closed            int
	pending                 map[int64]float64
	frozen, mature          bool
	finalized               bool
	reason                  string
	calibration             []observer.Point
	resets                  int
	calibrationVariance     float64
	slotCounts              [16]int
	maxWithinSlotSpread     float64
}

func (s *realSeasonalGaugeState) transformed(slot int, value float64, mode string) float64 {
	if mode == "raw_fullcycle" || mode == "raw_wrapped_nochange" {
		return value
	}
	profile := s.model.Profile
	if profile == nil {
		return value
	}
	value -= profile.Center[hourlyAdaptiveMod(slot-s.model.Offset, profile.Period)]
	if mode == "phase_pooled_floor" {
		value /= s.scale.Applied[hourlyAdaptiveMod(slot-s.model.Offset, s.model.Profile.Period)]
	}
	return value
}

// releaseDiagnosticScales runs after the maturity diagnostic is emitted.
// The frozen adapter needs only phase centers and applied divisors thereafter.
func (s *realSeasonalGaugeState) releaseDiagnosticScales() {
	if s.model.Profile != nil {
		s.model.Profile.Scale = nil
	}
	if s.scale != nil {
		s.scale.Original = nil
	}
}

// closeSlot never reads the newly arriving point. Calibration is strictly past.
// Readiness means a model and calibration are available; the caller additionally
// validates the actual retained warmup before committing the single reset.
func (s *realSeasonalGaugeState) closeSlot() (bool, error) {
	if s.reason != "" || s.finalized {
		return false, nil
	}
	if s.slot != s.closed {
		return false, fmt.Errorf("missing_closed_slot")
	}
	if len(s.pending) == 0 || len(s.pending) > 15 {
		return false, fmt.Errorf("invalid_closed_slot_count")
	}
	timestamps := make([]int64, 0, len(s.pending))
	for ts := range s.pending {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	sum, low, high := 0.0, math.Inf(1), math.Inf(-1)
	for _, ts := range timestamps {
		v := s.pending[ts]
		if !hourlyAdaptiveFinite(v) {
			return false, fmt.Errorf("nonfinite_prereadiness")
		}
		sum += v
		low = math.Min(low, v)
		high = math.Max(high, v)
	}
	mean := sum / float64(len(timestamps))
	if !hourlyAdaptiveFinite(mean) {
		return false, fmt.Errorf("nonfinite_slot_mean")
	}
	if !hourlyAdaptiveFinite(high - low) {
		return false, fmt.Errorf("nonfinite_within_slot_spread")
	}
	s.slotCounts[len(timestamps)]++
	s.maxWithinSlotSpread = math.Max(s.maxWithinSlotSpread, high-low)
	if s.slot < realSeasonalTrain {
		s.model.Observe(s.slot, mean)
		if s.slot == realSeasonalTrain-1 {
			s.frozen = true
			if s.model.Profile == nil {
				return false, fmt.Errorf("no_admission_at_training_cutoff")
			}
			scale, err := realSeasonalFitScale(s.model)
			if err != nil {
				return false, err
			}
			s.scale = scale
			// This adapter freezes learning permanently at this boundary. Both
			// forecast and calibration now use the fitted profile/scale only.
			s.model.releaseHistory()
		}
	} else if s.slot < realSeasonalTrain+realSeasonalCalibration {
		for _, ts := range timestamps {
			s.calibration = append(s.calibration, observer.Point{Timestamp: ts, Value: s.pending[ts]})
		}
	}
	s.closed++
	s.finalized = s.closed == realSeasonalTrain+realSeasonalCalibration
	return s.finalized, nil
}

type realSeasonalDetector struct {
	sharedLikelihoodScratch []float64
	reusableReader          realSeasonalReader
	reusableExcludedReader  realSeasonalReader
	observer.Detector
	factory         func() observer.Detector
	states          map[observer.SeriesRef]*realSeasonalGaugeState
	mode            string
	ready           bool
	budget          *seasonalAdmissionBudget
	metas           []observer.SeriesMeta
	metaGeneration  uint64
	metaInitialized bool
}

func realSeasonalGaugeDetector(factory func() observer.Detector, agg observer.Aggregate, floor float64) observer.Detector {
	b, ok := factory().(*BOCPDDetector)
	if !ok {
		panic("real seasonal wrapper requires BOCPD")
	}
	b.config.Aggregations = []observer.Aggregate{agg}
	// Never change hazard/mass/recovery here: all series use the top-level config.
	if floor > 0 {
		b.config.MinVariance = floor
	}
	return b
}

// wrapSeasonalResearchDetector is opt-in and only compiled into the testbench.
func wrapSeasonalResearchDetector(d observer.Detector, factory func() observer.Detector) observer.Detector {
	mode := os.Getenv("OBSERVER_REAL_SEASONAL_MODE")
	if mode == "" || mode == "raw_unwrapped" {
		return d
	}
	if mode != "raw_fullcycle" && mode != "residual_fullcycle" && mode != "phase_pooled_floor" && mode != "raw_wrapped_nochange" {
		panic("invalid OBSERVER_REAL_SEASONAL_MODE")
	}
	if d.Name() != "bocpd" {
		return d
	}
	return &realSeasonalDetector{Detector: d, factory: factory, states: make(map[observer.SeriesRef]*realSeasonalGaugeState), mode: mode, budget: seasonalAdmissionFromEnvironment()}
}
func (d *realSeasonalDetector) Ready() bool { return d.ready || d.Detector.Ready() }
func (d *realSeasonalDetector) Reset() {
	d.sharedLikelihoodScratch = nil
	d.reusableReader = realSeasonalReader{}
	d.reusableExcludedReader = realSeasonalReader{}
	d.Detector = d.factory()
	d.states = make(map[observer.SeriesRef]*realSeasonalGaugeState)
	d.ready = false
	d.metas = nil
	d.metaInitialized = false
	if d.budget != nil {
		d.budget = newSeasonalAdmissionBudget(d.budget.limit)
	}
}
func (d *realSeasonalDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return d.Detector.(observer.DetectorPointWindowRequirement).DetectorPointWindow()
}
func (d *realSeasonalDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.Detector.(observer.SeriesRemover).RemoveSeries(refs)
	d.metas = nil
	d.metaInitialized = false
	for _, r := range refs {
		delete(d.states, r)
		if d.budget != nil {
			d.budget.remove(r)
		}
	}
}

// A reader always retains storage metadata, counts, aggregate policy and write
// generations. Only avg values in one admitted mature series are transformed.
// Bounds also constrain PointCountUpTo, so reset warmup cannot see future points.
type realSeasonalReader struct {
	observer.StorageReader
	ref              observer.SeriesRef
	state            *realSeasonalGaugeState
	mode             string
	exclude          bool
	minimum, maximum int64
}

func (r *realSeasonalReader) SupportsAggregate(ref observer.SeriesRef, agg observer.Aggregate) bool {
	return supportsSeriesAggregate(r.StorageReader, ref, agg)
}
func (r *realSeasonalReader) ListSeries(f observer.SeriesFilter) []observer.SeriesMeta {
	if !r.exclude {
		meta := r.StorageReader.GetSeriesMeta(r.ref)
		if meta == nil {
			return nil
		}
		stats := &seriesStats{Namespace: meta.Namespace, Name: meta.Name, Host: meta.Host, Tags: meta.Tags}
		if !matchesSeriesFilter(stats, f) {
			return nil
		}
		return []observer.SeriesMeta{*meta}
	}
	all := r.StorageReader.ListSeries(f)
	out := make([]observer.SeriesMeta, 0, len(all))
	for _, m := range all {
		if (r.exclude && !realSeasonalEligible(m)) || (!r.exclude && m.Ref == r.ref) {
			out = append(out, m)
		}
	}
	return out
}
func (r *realSeasonalReader) bounds(start, end int64) (int64, int64) {
	start = max(start, r.minimum)
	if r.maximum > 0 {
		end = min(end, r.maximum)
	}
	return start, end
}
func (r *realSeasonalReader) value(p observer.Point, agg observer.Aggregate) observer.Point {
	if agg == observer.AggregateAverage && r.state != nil && r.state.mature {
		p.Value = r.state.transformed(int((p.Timestamp-r.state.origin)/realSeasonalStep), p.Value, r.mode)
	}
	return p
}
func (r *realSeasonalReader) ForEachPoint(ref observer.SeriesRef, start, end int64, agg observer.Aggregate, fn func(*observer.Series, observer.Point)) bool {
	start, end = r.bounds(start, end)
	return r.StorageReader.ForEachPoint(ref, start, end, agg, func(s *observer.Series, p observer.Point) { fn(s, r.value(p, agg)) })
}
func (r *realSeasonalReader) GetSeriesRange(ref observer.SeriesRef, start, end int64, agg observer.Aggregate) *observer.Series {
	start, end = r.bounds(start, end)
	s := r.StorageReader.GetSeriesRange(ref, start, end, agg)
	if s != nil {
		for i, p := range s.Points {
			s.Points[i] = r.value(p, agg)
		}
	}
	return s
}
func (r *realSeasonalReader) SumRange(ref observer.SeriesRef, start, end int64, agg observer.Aggregate) float64 {
	sum := 0.0
	r.ForEachPoint(ref, start, end, agg, func(_ *observer.Series, p observer.Point) { sum += p.Value })
	return sum
}
func (r *realSeasonalReader) PointCountUpTo(ref observer.SeriesRef, end int64) int {
	if r.minimum == 0 && r.maximum == 0 {
		return r.StorageReader.PointCountUpTo(ref, end)
	}
	n := 0
	r.ForEachPoint(ref, 0, end, observer.AggregateAverage, func(_ *observer.Series, _ observer.Point) { n++ })
	return n
}

func (d *realSeasonalDetector) detectWithScratch(det observer.Detector, reader observer.StorageReader, now int64) observer.DetectionResult {
	if seasonalIntegrationMode == "" || seasonalIntegrationMode == "baseline" {
		return det.Detect(reader, now)
	}
	b := det.(*BOCPDDetector)
	b.likelihoodScratch = d.sharedLikelihoodScratch
	result := b.Detect(reader, now)
	d.sharedLikelihoodScratch = b.likelihoodScratch
	b.likelihoodScratch = nil
	return result
}

func (d *realSeasonalDetector) seriesReader(storage observer.StorageReader, ref observer.SeriesRef, state *realSeasonalGaugeState, mode string) *realSeasonalReader {
	if seasonalIntegrationMode != "scratch_readers" {
		return &realSeasonalReader{StorageReader: storage, ref: ref, state: state, mode: mode}
	}
	d.reusableReader = realSeasonalReader{StorageReader: storage, ref: ref, state: state, mode: mode}
	return &d.reusableReader
}

func realSeasonalAppend(to *observer.DetectionResult, from observer.DetectionResult) {
	to.Anomalies = append(to.Anomalies, from.Anomalies...)
}
func (d *realSeasonalDetector) log(meta observer.SeriesMeta, s *realSeasonalGaugeState, event string, extra map[string]any) {
	entry := map[string]any{"event": event, "mode": d.mode, "ref": meta.Ref, "metric": meta.Name, "namespace": meta.Namespace, "host": meta.Host, "tags": meta.Tags, "origin": s.origin, "last": s.last, "slot": s.slot, "closed_slots": s.closed, "reason": s.reason, "mature": s.mature, "resets": s.resets, "slot_count_histogram": s.slotCounts, "max_within_slot_spread": s.maxWithinSlotSpread}
	for k, v := range extra {
		entry[k] = v
	}
	b, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "SEASONAL_REAL %s\n", b)
}
func (d *realSeasonalDetector) disqualify(meta observer.SeriesMeta, s *realSeasonalGaugeState, reason string) {
	if s.reason != "" {
		return
	}
	s.reason = reason
	s.calibration = nil
	// Disqualification is permanent until this series is removed/reset.
	s.model.releaseHistory()
	d.log(meta, s, "ineligible", nil)
	if d.budget != nil {
		d.budget.finish(meta.Ref, false)
		s.model = &hourlyAdaptiveSeasonalModel{}
		s.scale = nil
	}
}

func (d *realSeasonalDetector) mature(storage observer.StorageReader, meta observer.SeriesMeta, s *realSeasonalGaugeState, result *observer.DetectionResult, activation, engineNow int64) {
	if d.mode == "raw_wrapped_nochange" {
		d.log(meta, s, "ready_nochange_control", map[string]any{"activation_timestamp": activation, "engine_now": engineNow, "warmup_last_timestamp": s.last})
		s.calibration = nil
		s.releaseDiagnosticScales()
		if d.budget != nil {
			d.budget.finish(meta.Ref, true)
		}
		return
	}
	warmupCount := s.detector.(*BOCPDDetector).config.WarmupPoints
	past := storage.GetSeriesRange(meta.Ref, 0, s.last, observer.AggregateAverage)
	if past == nil || len(past.Points) < warmupCount || warmupCount > 120 {
		d.disqualify(meta, s, "insufficient_retained_warmup")
		return
	}
	points := past.Points
	if len(points) > 120 {
		points = points[len(points)-120:]
	}
	for _, p := range points {
		if !hourlyAdaptiveFinite(p.Value) {
			d.disqualify(meta, s, "nonfinite_retained_warmup")
			return
		}
	}
	cal := make([]float64, len(s.calibration))
	for j, p := range s.calibration {
		cal[j] = s.transformed(int((p.Timestamp-s.origin)/realSeasonalStep), p.Value, d.mode)
	}
	variance := realSeasonalVariance(cal)
	if !hourlyAdaptiveFinite(variance) || variance < 0 {
		d.disqualify(meta, s, "invalid_calibration_variance")
		return
	}
	// Finish all cold observations before changing the coordinate system. Neither
	// the arriving point nor later points in the storage snapshot are visible.
	cold := &realSeasonalReader{StorageReader: storage, ref: meta.Ref, maximum: s.last}
	realSeasonalAppend(result, d.detectWithScratch(s.detector, cold, s.last))
	floor := math.Max(1, variance)
	s.detector = realSeasonalGaugeDetector(d.factory, observer.AggregateAverage, floor)
	s.mature = true
	s.resets++
	s.calibrationVariance = variance
	warm := &realSeasonalReader{StorageReader: storage, ref: meta.Ref, state: s, mode: d.mode, minimum: points[0].Timestamp - 1, maximum: s.last}
	warmResult := d.detectWithScratch(s.detector, warm, s.last) // Suppress historical emissions only; preserve resulting lifecycle.
	warmValues := make([]float64, warmupCount)
	for i, p := range points[:warmupCount] {
		warmValues[i] = s.transformed(int((p.Timestamp-s.origin)/realSeasonalStep), p.Value, d.mode)
	}
	b := s.detector.(*BOCPDDetector)
	state := b.series[bocpdStateKey{ref: meta.Ref, agg: observer.AggregateAverage}]
	if state == nil || !state.initialized {
		panic("mature warmup failed after validated history")
	}
	d.log(meta, s, "mature_reset", map[string]any{"actual_config": b.config, "activation_timestamp": activation, "engine_now": engineNow, "period": s.model.Profile.Period, "offset": s.model.Offset, "revision": s.model.Revision, "model_last": s.model.Last, "scale": s.scale, "calibration_count": len(cal), "calibration_variance": variance, "variance_floor": floor, "warmup_points": len(points), "warmup_first_timestamp": points[0].Timestamp, "warmup_last_timestamp": s.last, "warmup_sample_variance": realSeasonalVariance(warmValues), "actual_observation_variance": state.obsVar, "actual_baseline_mean": state.baselineMean, "actual_baseline_stddev": state.baselineStddev, "actual_prior_precision": state.priorPrecision, "warmup_configured_points": warmupCount, "in_alert": state.inAlert, "recovery_count": state.recoveryCount, "suppressed_historical_onsets": len(warmResult.Anomalies)})
	s.calibration = nil
	s.releaseDiagnosticScales()
	if d.budget != nil {
		d.budget.finish(meta.Ref, true)
	}
}

func (d *realSeasonalDetector) Detect(storage observer.StorageReader, now int64) observer.DetectionResult {
	excluded := &realSeasonalReader{StorageReader: storage, exclude: true}
	if seasonalIntegrationMode == "scratch_readers" {
		d.reusableExcludedReader = realSeasonalReader{StorageReader: storage, exclude: true}
		excluded = &d.reusableExcludedReader
	}
	result := d.Detector.Detect(excluded, now)
	generation := storage.SeriesGeneration()
	if !d.metaInitialized || generation != d.metaGeneration {
		d.metas = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.metaGeneration, d.metaInitialized = generation, true
	}
	metas := d.metas
	if !seasonalStructureOnly {
		d.prepareAdmission(storage, metas, now)
	}
	for _, meta := range metas {
		if !realSeasonalEligible(meta) {
			continue
		}
		s := d.states[meta.Ref]
		if s == nil && seasonalStructureOnly {
			s = &realSeasonalGaugeState{detector: realSeasonalGaugeDetector(d.factory, observer.AggregateAverage, 0), countDetector: realSeasonalGaugeDetector(d.factory, observer.AggregateCount, 0)}
			d.states[meta.Ref] = s
		}
		if s == nil {
			model, err := newHourlyAdaptiveSeasonalModel(15, []int{1800, 3600})
			if err != nil {
				panic(err)
			}
			s = &realSeasonalGaugeState{model: model, detector: realSeasonalGaugeDetector(d.factory, observer.AggregateAverage, 0), countDetector: realSeasonalGaugeDetector(d.factory, observer.AggregateCount, 0)}
			d.states[meta.Ref] = s
		}
		if s.model != nil && s.readStamp.changed(storage, meta.Ref, now) {
			// Same timestamp merges refresh only the still-pending slot. Closed slots
			// never get revised or observed again; replay must have ordered ingestion.
			if s.started && !s.finalized && s.reason == "" {
				storage.ForEachPoint(meta.Ref, s.origin+int64(s.slot)*realSeasonalStep-1, s.last, observer.AggregateAverage, func(_ *observer.Series, p observer.Point) { s.pending[p.Timestamp] = p.Value })
			}
			var points []observer.Point
			storage.ForEachPoint(meta.Ref, s.last, now, observer.AggregateAverage, func(_ *observer.Series, p observer.Point) { points = append(points, p) })
			for _, p := range points {
				if !s.started {
					s.started = true
					s.origin = p.Timestamp
					s.slot = 0
					s.last = p.Timestamp
					s.pending = map[int64]float64{p.Timestamp: p.Value}
					d.log(meta, s, "origin", nil)
					continue
				}
				slot := int((p.Timestamp - s.origin) / realSeasonalStep)
				if slot > s.slot {
					if slot != s.slot+1 {
						if !s.finalized {
							d.disqualify(meta, s, "missing_prereadiness_slot")
						} else {
							d.log(meta, s, "post_ready_gap", map[string]any{"next_slot": slot})
						}
					}
					if s.reason == "" && !s.finalized {
						wasFrozen := s.frozen
						ready, err := s.closeSlot()
						if err != nil {
							d.disqualify(meta, s, err.Error())
						} else {
							if !wasFrozen && s.frozen {
								d.log(meta, s, "training_frozen", map[string]any{"period": s.model.Profile.Period, "offset": s.model.Offset, "revision": s.model.Revision, "model_last": s.model.Last, "profile": s.model.Profile})
							}
							if ready {
								d.mature(storage, meta, s, &result, p.Timestamp, now)
							}
						}
					}
					s.slot = slot
					clear(s.pending)
				}
				s.last = p.Timestamp
				s.pending[p.Timestamp] = p.Value
			}
		}
		reader := d.seriesReader(storage, meta.Ref, s, d.mode)
		realSeasonalAppend(&result, d.detectWithScratch(s.detector, reader, now))
		countReader := d.seriesReader(storage, meta.Ref, nil, "")
		realSeasonalAppend(&result, d.detectWithScratch(s.countDetector, countReader, now))
		d.ready = d.ready || s.detector.Ready() || s.countDetector.Ready()
	}
	d.reusableReader = realSeasonalReader{}
	d.reusableExcludedReader = realSeasonalReader{}
	return result
}
