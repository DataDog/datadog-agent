// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"errors"
	"fmt"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// pipelineDetector is a detector owned by one committed-kind registration.
type pipelineDetector interface {
	observer.Detector
	observer.SeriesRemover
	Reset()
}

// pipelineRegistration binds one detector instance to a fixed committed kind
// and input type. Aggregation restrictions may be intersected per call.
type pipelineRegistration struct {
	kind         observer.MetricKind
	metricType   metrics.MetricType
	detector     pipelineDetector
	aggregations []observer.Aggregate
	timing       detectorTiming
}

// metricPipelineSeriesState tracks matching progress without owning storage.
type metricPipelineSeriesState struct {
	lastMatchedTime int64
	reactivated     bool
	outputAfter     int64
	kindMatchFrom   int64 // Zero means no pending request.
}

// MetricPipeline routes gauges to detectors by their committed kind.
type MetricPipeline struct {
	name          string
	registrations []pipelineRegistration
	byKind        map[observer.MetricKind]int
	matcher       observer.MetricKindMatcher
	window        observer.DetectorPointWindow
	series        map[observer.SeriesRef]*metricPipelineSeriesState
	cachedRefs    []observer.SeriesRef
	cachedGen     uint64
	scratch       []observer.Point
	anomalyCutoff bool

	kindTransitions metricKindTransitionCounts
}

func newMetricPipeline(
	registrations []pipelineRegistration,
	matcher observer.MetricKindMatcher,
) (*MetricPipeline, error) {
	if matcher == nil {
		return nil, errors.New("metric pipeline requires a matcher")
	}
	p := &MetricPipeline{
		name:          "metric_pipeline",
		registrations: make([]pipelineRegistration, 0, len(registrations)),
		byKind:        make(map[observer.MetricKind]int, len(registrations)),
		matcher:       matcher,
		window:        matcher.Window(),
		series:        make(map[observer.SeriesRef]*metricPipelineSeriesState),
		anomalyCutoff: true,
	}
	for _, registration := range registrations {
		if registration.detector == nil {
			return nil, fmt.Errorf("metric pipeline kind %s has no detector", registration.kind)
		}
		if _, exists := p.byKind[registration.kind]; exists {
			return nil, fmt.Errorf("metric pipeline has duplicate kind %s", registration.kind)
		}
		if registration.aggregations != nil {
			registration.aggregations = append([]observer.Aggregate(nil), registration.aggregations...)
		}
		owned := registration
		p.registrations = append(p.registrations, owned)
		p.byKind[registration.kind] = len(p.registrations) - 1
		if p.name == "metric_pipeline" {
			p.name = registration.detector.Name()
		}
		pipelineExpandWindow(&p.window, registration.detector)
	}
	return p, nil
}

// newScanMWMetricPipeline builds the initial kind-routed composition.
// stableCfg may be nil to select the default stable pipeline configuration.
func newScanMWMetricPipeline(cfg ScanMWConfig, anomalyCutoff bool, stableCfg *StablePipelineConfig) (*MetricPipeline, error) {
	stableConfig := DefaultStablePipelineConfig()
	if stableCfg != nil {
		stableConfig = *stableCfg
	}
	stable, err := newStablePipeline(NewScanMWDetector(cfg), stableConfig)
	if err != nil {
		return nil, fmt.Errorf("build stable pipeline: %w", err)
	}
	registration := pipelineRegistration{
		kind:         observer.STABLE,
		metricType:   metrics.GaugeType,
		detector:     stable,
		aggregations: []observer.Aggregate{observer.AggregateAverage},
	}
	p, err := newMetricPipeline([]pipelineRegistration{registration}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	if err != nil {
		return nil, err
	}
	p.anomalyCutoff = anomalyCutoff
	return p, nil
}

// newDefaultMetricPipeline builds the default metric workflow. stableCfg may
// be nil to select the default stable pipeline configuration.
func newDefaultMetricPipeline(stableCfg *StablePipelineConfig, anomalyCutoff bool) (*MetricPipeline, error) {
	return newScanMWMetricPipeline(DefaultScanMWConfig(), anomalyCutoff, stableCfg)
}

func pipelineExpandWindow(window *observer.DetectorPointWindow, detector observer.Detector) {
	requirement, ok := detector.(observer.DetectorPointWindowRequirement)
	if !ok {
		return
	}
	points := requirement.DetectorPointWindow()
	// MinPoints is the earliest meaningful warmup across kinds, so take the
	// minimum. MaxPoints is the largest history requirement, so take the max.
	if window.MinPoints == 0 || (points.MinPoints > 0 && points.MinPoints < window.MinPoints) {
		window.MinPoints = points.MinPoints
	}
	if points.MaxPoints > window.MaxPoints {
		window.MaxPoints = points.MaxPoints
	}
}

// Name preserves the routed algorithm identity for diagnostics and selection.
func (p *MetricPipeline) Name() string { return p.name }

// Ready reports whether any routed detector is ready.
func (p *MetricPipeline) Ready() bool {
	for i := range p.registrations {
		if p.registrations[i].detector.Ready() {
			return true
		}
	}
	return false
}

// DetectorPointWindow reports the broadest detector history requirement. The
// matcher keeps its own bounded classification window.
func (p *MetricPipeline) DetectorPointWindow() observer.DetectorPointWindow { return p.window }

// Detect runs each detector under its constructor-bound kind and type.
func (p *MetricPipeline) Detect(
	storage observer.StorageReader,
	dataTime int64,
	filter observer.DetectionFilter,
) observer.DetectionResult {
	var result observer.DetectionResult
	for i := range p.registrations {
		detectorFilter, ok := pipelineDetectionFilter(&p.registrations[i], filter)
		if !ok {
			continue
		}
		registration := &p.registrations[i]
		start := time.Now()
		detected := registration.detector.Detect(storage, dataTime, detectorFilter)
		registration.timing.record(time.Since(start))
		for _, anomaly := range detected.Anomalies {
			anomaly.DetectorName = registration.detector.Name()
			anomaly.MetricKind = registration.kind
			if anomaly.SourceRef == nil {
				result.Anomalies = append(result.Anomalies, anomaly)
				continue
			}
			state := p.series[anomaly.SourceRef.Ref]
			if p.anomalyCutoff && state != nil && state.reactivated && anomaly.Timestamp <= state.outputAfter {
				continue
			}
			if state == nil {
				state = &metricPipelineSeriesState{}
				p.series[anomaly.SourceRef.Ref] = state
			}
			state.kindMatchFrom = max(state.kindMatchFrom, anomaly.Timestamp)
			result.Anomalies = append(result.Anomalies, anomaly)
		}
	}
	return result
}

// advanceKinds evaluates matching after detection. It is the only pipeline
// phase allowed to update committed kinds.
func (p *MetricPipeline) advanceKinds(storage *timeSeriesStorage, dataTime int64) {
	generation := storage.SeriesGeneration()
	if p.cachedRefs == nil || generation != p.cachedGen {
		gauge := metrics.GaugeType
		filter := observer.WorkloadSeriesFilter()
		filter.MetricType = &gauge
		p.cachedRefs = listSeriesRefs(storage, filter, p.cachedRefs)
		p.cachedGen = generation
	}

	statuses := bulkSeriesStatus(storage, p.cachedRefs, dataTime)
	matcherWindow := p.matcher.Window()
	for i, ref := range p.cachedRefs {
		current, _ := storage.SeriesKind(ref)
		_, hasDetector := p.byKind[current]
		state := p.series[ref]
		var transition observer.KindTransition
		if current != observer.UNSET && hasDetector {
			if state == nil || state.kindMatchFrom == 0 {
				continue
			}
			var expired bool
			p.scratch, expired = storage.snapshotFirstPointsFrom(
				ref, state.kindMatchFrom, dataTime, matcherWindow.MinPoints,
				observer.AggregateAverage, p.scratch,
			)
			if expired {
				state.kindMatchFrom = 0
				continue
			}
			if len(p.scratch) < matcherWindow.MinPoints {
				continue
			}
			next := p.matcher.Classify(p.scratch)
			state.kindMatchFrom = 0
			if next == current {
				continue
			}
			transition = observer.KindTransition{Ref: ref, From: current, To: next}
		} else {
			if statuses[i].pointCount < matcherWindow.MinPoints {
				continue
			}
			_, p.scratch = collectLastPoints(
				storage, ref, dataTime, matcherWindow.MaxPoints,
				observer.AggregateAverage, p.scratch,
			)
			if len(p.scratch) < matcherWindow.MinPoints {
				continue
			}
			lastPointTime := p.scratch[len(p.scratch)-1].Timestamp
			if state != nil && lastPointTime <= state.lastMatchedTime {
				continue
			}
			if state == nil {
				state = &metricPipelineSeriesState{}
				p.series[ref] = state
			}
			state.lastMatchedTime = lastPointTime
			state.kindMatchFrom = 0
			var changed bool
			transition, changed = p.matcher.Observe(ref, current, p.scratch, dataTime)
			if !changed {
				continue
			}
		}
		if previous, exists := p.byKind[transition.From]; exists {
			p.registrations[previous].detector.RemoveSeries([]observer.SeriesRef{ref})
		}
		if p.anomalyCutoff {
			if _, supported := p.byKind[transition.To]; supported && transition.From != observer.UNSET {
				state.reactivated = true
				state.outputAfter = dataTime
			}
		}
		storage.SetSeriesKind(ref, transition.To)
		p.recordKindTransition(transition.From, transition.To)
	}
}

func pipelineDetectionFilter(
	registration *pipelineRegistration,
	filter observer.DetectionFilter,
) (observer.DetectionFilter, bool) {
	if filter.MetricKind != nil && *filter.MetricKind != registration.kind {
		return observer.DetectionFilter{}, false
	}
	if filter.MetricType != nil && *filter.MetricType != registration.metricType {
		return observer.DetectionFilter{}, false
	}
	aggregations := intersectAggregations(registration.aggregations, filter.Aggregations)
	if len(aggregations) == 0 && len(registration.aggregations) > 0 && len(filter.Aggregations) > 0 {
		return observer.DetectionFilter{}, false
	}
	kind, metricType := registration.kind, registration.metricType
	return observer.DetectionFilter{
		MetricKind:   &kind,
		MetricType:   &metricType,
		Aggregations: aggregations,
	}, true
}

func intersectAggregations(configured, requested []observer.Aggregate) []observer.Aggregate {
	if len(requested) == 0 {
		return append([]observer.Aggregate(nil), configured...)
	}
	if len(configured) == 0 {
		return append([]observer.Aggregate(nil), requested...)
	}
	result := make([]observer.Aggregate, 0, min(len(configured), len(requested)))
	for _, agg := range configured {
		for _, want := range requested {
			if agg == want {
				result = append(result, agg)
				break
			}
		}
	}
	return result
}

// RemoveSeries drops routed detector and matcher state for evicted series.
func (p *MetricPipeline) RemoveSeries(refs []observer.SeriesRef) {
	if len(refs) == 0 {
		return
	}
	p.matcher.RemoveSeries(refs)
	for i := range p.registrations {
		p.registrations[i].detector.RemoveSeries(refs)
	}
	for _, ref := range refs {
		delete(p.series, ref)
	}
	p.cachedRefs = nil
	p.cachedGen = 0
}

// Reset clears routed detector and matcher state. Committed kinds are owned by
// storage and reset by the caller.
func (p *MetricPipeline) Reset() {
	p.matcher.Reset()
	p.kindTransitions = metricKindTransitionCounts{}
	for i := range p.registrations {
		p.registrations[i].detector.Reset()
		p.registrations[i].timing = detectorTiming{}
	}
	clear(p.series)
	p.cachedRefs = nil
	p.cachedGen = 0
	p.scratch = p.scratch[:0]
}

const metricKindCount = 4

type metricKindTransitionCounts [metricKindCount][metricKindCount]uint64

func (p *MetricPipeline) recordKindTransition(from, to observer.MetricKind) {
	if from < metricKindCount && to < metricKindCount && from != to {
		p.kindTransitions[from][to]++
	}
}

func (p *MetricPipeline) drainKindTransitionCounts() metricKindTransitionCounts {
	counts := p.kindTransitions
	p.kindTransitions = metricKindTransitionCounts{}
	return counts
}
