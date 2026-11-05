// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

type pipelineStubDetector struct {
	name       string
	ready      bool
	calls      int
	detected   []observer.MetricKind
	types      []metrics.MetricType
	aggregates []observer.Aggregate
	removed    []observer.SeriesRef
	resetCalls int
}

func (d *pipelineStubDetector) Name() string { return d.name }
func (d *pipelineStubDetector) Ready() bool  { return d.ready }

func (d *pipelineStubDetector) Detect(
	_ observer.StorageReader,
	_ int64,
	filter observer.DetectionFilter,
) observer.DetectionResult {
	d.calls++
	if filter.MetricKind != nil {
		d.detected = append(d.detected, *filter.MetricKind)
	}
	if filter.MetricType != nil {
		d.types = append(d.types, *filter.MetricType)
	}
	d.aggregates = append(d.aggregates, filter.Aggregations...)
	return observer.DetectionResult{Anomalies: []observer.Anomaly{{
		Timestamp: 1,
		SourceRef: &observer.QueryHandle{Ref: observer.SeriesRef(d.calls), Aggregate: observer.AggregateAverage},
	}}}
}

func (d *pipelineStubDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.removed = append(d.removed, refs...)
}
func (d *pipelineStubDetector) Reset() { d.resetCalls++ }

func newPipelineStub(kind observer.MetricKind, name string) (pipelineRegistration, *pipelineStubDetector) {
	detector := &pipelineStubDetector{name: name, ready: true}
	return pipelineRegistration{
		kind:         kind,
		metricType:   metrics.GaugeType,
		detector:     detector,
		aggregations: []observer.Aggregate{observer.AggregateAverage},
	}, detector
}

func newScriptedPipelineMatcher(
	minPoints, maxPoints, confirmations int,
	predictions ...observer.MetricKind,
) *metricKindMatcher {
	idx := 0
	return newMetricKindMatcherWithClassifier(
		observer.MetricKindMatcherConfig{
			MinPoints:               minPoints,
			MaxPoints:               maxPoints,
			TransitionConfirmations: confirmations,
		},
		func(_ []observer.Point, _ *matcherWorkspace) observer.MetricKind {
			if idx >= len(predictions) {
				return predictions[len(predictions)-1]
			}
			prediction := predictions[idx]
			idx++
			return prediction
		},
	)
}

func addPipelinePoints(s *timeSeriesStorage, key uint64, start, end int64, metricType metrics.MetricType) observer.SeriesRef {
	var ref observer.SeriesRef
	for ts := start; ts <= end; ts++ {
		ref = s.AddWithKeyAndHostComposite("work", fmt.Sprintf("metric_%d", key), "", 100, ts,
			tagset.CompositeTags{}, key, metricType).Ref
	}
	return ref
}

func addPipelineValues(s *timeSeriesStorage, key uint64, start, end int64, value float64) observer.SeriesRef {
	var ref observer.SeriesRef
	for ts := start; ts <= end; ts++ {
		ref = s.AddWithKeyAndHostComposite("work", fmt.Sprintf("metric_%d", key), "", value, ts,
			tagset.CompositeTags{}, key, metrics.GaugeType).Ref
	}
	return ref
}

func newScanMWPipelineWithMatcher(
	t *testing.T,
	cfg ScanMWConfig,
	minPoints, maxPoints, confirmations int,
	predictions ...observer.MetricKind,
) (*MetricPipeline, *ScanMWDetector) {
	scan := NewScanMWDetector(cfg)
	registration := pipelineRegistration{
		kind:         observer.STABLE,
		metricType:   metrics.GaugeType,
		detector:     scan,
		aggregations: []observer.Aggregate{observer.AggregateAverage},
	}
	p, err := newMetricPipeline(
		[]pipelineRegistration{registration}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()),
	)
	require.NoError(t, err)
	p.matcher = newScriptedPipelineMatcher(minPoints, maxPoints, confirmations, predictions...)
	return p, scan
}

func testScanMWPipelineConfig() ScanMWConfig {
	return ScanMWConfig{
		MinPoints: 6, MinSegment: 2, MaxPoints: 40,
		SignificanceThreshold: 1e-3, MinEffectSize: 0.85, MinDeviationMAD: 3,
	}
}

func TestMetricPipelineRoutedDetection(t *testing.T) {
	stableReg, stable := newPipelineStub(observer.STABLE, "stable")
	linearReg, linear := newPipelineStub(observer.LINEAR, "linear")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg, linearReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)

	result := p.Detect(nil, 1, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 2)
	require.Equal(t, []observer.MetricKind{observer.STABLE}, stable.detected)
	require.Equal(t, []observer.MetricKind{observer.LINEAR}, linear.detected)
	require.Equal(t, []metrics.MetricType{metrics.GaugeType}, stable.types)
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage}, stable.aggregates)

	filter := observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateCount}}
	p.Detect(nil, 1, filter)
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage}, stable.aggregates)
	require.Equal(t, 1, stable.calls)
	require.Equal(t, 1, linear.calls)

	mismatch := observer.UNSET
	result = p.Detect(nil, 1, observer.DetectionFilter{MetricKind: &mismatch})
	require.Empty(t, result.Anomalies)
	require.Equal(t, 1, stable.calls)
	require.Equal(t, 1, linear.calls)
}

func TestMetricPipelineValidationAndLifecycle(t *testing.T) {
	stableReg, stable := newPipelineStub(observer.STABLE, "scanmw")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	require.Equal(t, "scanmw", p.Name())
	require.True(t, p.Ready())
	require.Equal(t, observer.DetectorPointWindow{MinPoints: 30, MaxPoints: 30}, p.DetectorPointWindow())

	stable.ready = false
	require.False(t, p.Ready())
	refs := []observer.SeriesRef{1, 2}
	p.RemoveSeries(refs)
	require.Equal(t, refs, stable.removed)
	p.Reset()
	require.Equal(t, 1, stable.resetCalls)
	require.Len(t, p.matcher.(*metricKindMatcher).series, 0)

	duplicate := stableReg
	_, err = newMetricPipeline([]pipelineRegistration{stableReg, duplicate}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.Error(t, err)
}

func TestScanMWMetricPipelineRouting(t *testing.T) {
	s := newTimeSeriesStorage()
	add := func(key uint64, kind observer.MetricKind, metricType metrics.MetricType) observer.SeriesRef {
		var ref observer.SeriesRef
		for ts := int64(1); ts <= 40; ts++ {
			value := 10.0
			if ts > 20 {
				value = 100
			}
			ref = s.AddWithKeyAndHostComposite("work", fmt.Sprintf("metric_%d", key), "", value, ts,
				tagset.CompositeTags{}, key, metricType).Ref
		}
		s.SetSeriesKind(ref, kind)
		return ref
	}
	stable := add(1, observer.STABLE, metrics.GaugeType)
	linear := add(2, observer.LINEAR, metrics.GaugeType)
	count := add(3, observer.STABLE, metrics.CountType)
	addConstant := func(key uint64, kind observer.MetricKind) observer.SeriesRef {
		var ref observer.SeriesRef
		for ts := int64(1); ts <= 40; ts++ {
			ref = s.AddWithKeyAndHostComposite("work", fmt.Sprintf("metric_%d", key), "", 100, ts,
				tagset.CompositeTags{}, key, metrics.GaugeType).Ref
		}
		s.SetSeriesKind(ref, kind)
		return ref
	}
	constant := addConstant(4, observer.STABLE)
	for ts := int64(1); ts <= 40; ts++ {
		s.AddWithKeyAndHostComposite(observer.TelemetryNamespace, "metric_5", "", 100, ts,
			tagset.CompositeTags{}, 5, metrics.GaugeType)
	}

	p := newScanMWMetricPipeline(testScanMWPipelineConfig())
	filter := observer.WorkloadSeriesFilter()
	stableKind, gaugeType := observer.STABLE, metrics.GaugeType
	filter.MetricKind, filter.MetricType = &stableKind, &gaugeType
	require.ElementsMatch(t, []observer.SeriesRef{stable, constant}, s.ListSeriesRefsInto(filter, nil))
	result := p.Detect(s, 40, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 1)
	require.Equal(t, stable, result.Anomalies[0].SourceRef.Ref)
	require.Equal(t, observer.AggregateAverage, result.Anomalies[0].SourceRef.Aggregate)
	require.Len(t, p.registrations[0].detector.(*ScanMWDetector).series, 2)
	require.Contains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: stable, agg: observer.AggregateAverage})
	require.Contains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: constant, agg: observer.AggregateAverage})
	require.NotContains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: linear, agg: observer.AggregateAverage})
	require.NotContains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: count, agg: observer.AggregateAverage})
}

func TestMetricPipelineInitialAndFreshMatching(t *testing.T) {
	s := newDetectorTestStorage()
	gauge := addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
	addPipelinePoints(s, 2, 1, 3, metrics.CountType)
	stableReg, _ := newPipelineStub(observer.STABLE, "scanmw")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	p.matcher = newScriptedPipelineMatcher(3, 3, 1, observer.STABLE, observer.UNKNOWN)

	p.advanceKinds(s, 3)
	kind, _ := s.SeriesKind(gauge)
	require.Equal(t, observer.STABLE, kind)
	require.Len(t, p.series, 1)
	require.Equal(t, int64(3), p.series[gauge].lastMatchedTime)

	p.advanceKinds(s, 3)
	kind, _ = s.SeriesKind(gauge)
	require.Equal(t, observer.STABLE, kind, "unchanged data must not classify again")
	p.advanceKinds(s, 3)
	kind, _ = s.SeriesKind(gauge)
	require.Equal(t, observer.STABLE, kind, "future points do not create fresh evidence")

	addPipelinePoints(s, 1, 4, 4, metrics.GaugeType)
	p.advanceKinds(s, 4)
	kind, _ = s.SeriesKind(gauge)
	require.Equal(t, observer.UNKNOWN, kind)
	require.Len(t, p.series, 1, "count series must not enter matching state")
}

func TestMetricPipelineTransitionCleanupAndHistory(t *testing.T) {
	s := newDetectorTestStorage()
	ref := addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
	stableReg, stable := newPipelineStub(observer.STABLE, "scanmw")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	p.matcher = newScriptedPipelineMatcher(3, 3, 3,
		observer.STABLE, observer.LINEAR, observer.LINEAR, observer.LINEAR,
	)

	p.advanceKinds(s, 3)
	kind, _ := s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)

	// Two eligible predictions are pending; the third confirms the transition.
	addPipelinePoints(s, 1, 4, 4, metrics.GaugeType)
	p.advanceKinds(s, 4)
	kind, _ = s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)
	addPipelinePoints(s, 1, 5, 5, metrics.GaugeType)
	p.advanceKinds(s, 5)
	kind, _ = s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)
	addPipelinePoints(s, 1, 6, 6, metrics.GaugeType)
	p.advanceKinds(s, 6)
	kind, _ = s.SeriesKind(ref)
	require.Equal(t, observer.LINEAR, kind)
	require.Equal(t, []observer.SeriesRef{ref}, stable.removed)
	require.EqualValues(t, 6, s.PointCountUpTo(ref, 6), "transition must retain history")
}

func TestMetricPipelineEligibilityAndLifecycle(t *testing.T) {
	s := newDetectorTestStorage()
	ref := addPipelinePoints(s, 1, 1, 3, observer.UnknownType)
	stableReg, stable := newPipelineStub(observer.STABLE, "scanmw")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	p.matcher = newScriptedPipelineMatcher(3, 3, 1, observer.STABLE)

	p.advanceKinds(s, 3)
	require.Empty(t, p.series, "unknown input type is not gauge-eligible")

	addPipelinePoints(s, 1, 4, 4, metrics.GaugeType)
	p.advanceKinds(s, 4)
	kind, _ := s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind, "type promotion must refresh matching")
	require.Len(t, stable.removed, 0)

	p.RemoveSeries([]observer.SeriesRef{ref})
	require.Empty(t, p.series)
	require.Empty(t, p.matcher.(*metricKindMatcher).series)
	require.Equal(t, uint64(0), p.cachedGen)

	addPipelinePoints(s, 1, 5, 5, metrics.GaugeType)
	p.advanceKinds(s, 5)
	require.EqualValues(t, 5, s.PointCountUpTo(ref, 5), "pipeline reset must not delete storage")
	p.Reset()
	require.Empty(t, p.series)
	require.Empty(t, p.matcher.(*metricKindMatcher).series)
	require.EqualValues(t, 5, s.PointCountUpTo(ref, 5))
}

func TestMetricPipelineInitialHistoricalOutput(t *testing.T) {
	s := newDetectorTestStorage()
	ref := addPipelineValues(s, 1, 1, 20, 10)
	addPipelineValues(s, 1, 21, 40, 100)
	p, _ := newScanMWPipelineWithMatcher(t,
		testScanMWPipelineConfig(), 30, 30, 3, observer.STABLE,
	)
	p.advanceKinds(s, 40)
	kind, _ := s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)

	result := p.Detect(s, 40, observer.DetectionFilter{})
	require.NotEmpty(t, result.Anomalies, "initial assignment keeps normal cold-start detection")
	require.Less(t, result.Anomalies[0].Timestamp, int64(40))
	require.Empty(t, p.rawOutputs)
}

func TestMetricPipelineReactivationCutoff(t *testing.T) {
	s := newDetectorTestStorage()
	ref := addPipelineValues(s, 1, 1, 40, 10)
	p, scan := newScanMWPipelineWithMatcher(t,
		testScanMWPipelineConfig(), 30, 30, 3,
		observer.STABLE, observer.UNKNOWN, observer.UNKNOWN, observer.UNKNOWN, observer.STABLE,
	)
	p.advanceKinds(s, 40)
	kind, _ := s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)

	// Leave STABLE, then recover through three fresh STABLE predictions. The
	// detector rediscovers the change made while it was inactive.
	for ts := int64(41); ts <= 46; ts++ {
		addPipelineValues(s, 1, ts, ts, 100)
		p.Detect(s, ts, observer.DetectionFilter{})
		p.advanceKinds(s, ts)
	}
	kind, _ = s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)
	require.True(t, p.series[ref].reactivated)
	require.Equal(t, int64(46), p.series[ref].outputAfter)

	suppressed := p.Detect(s, 46, observer.DetectionFilter{})
	require.Empty(t, suppressed.Anomalies)
	require.NotEmpty(t, p.rawOutputs, "historical candidates stay available for diagnostics")
	require.NotEmpty(t, scan.series, "suppression must not prevent detector state updates")

	addPipelineValues(s, 1, 47, 56, 200)
	fresh := p.Detect(s, 56, observer.DetectionFilter{})
	require.NotEmpty(t, fresh.Anomalies)
	require.Greater(t, fresh.Anomalies[0].Timestamp, int64(46))

	p.Reset()
	require.Empty(t, p.rawOutputs)
}

func TestMetricPipelineReplayParity(t *testing.T) {
	streaming, preloaded := newDetectorTestStorage(), newDetectorTestStorage()
	for ts := int64(1); ts <= 100; ts++ {
		value := 10.0
		if ts > 60 {
			value = 100
		}
		preloaded.AddWithKeyAndHostComposite("work", "metric", "", value, ts,
			tagset.CompositeTags{}, 1, metrics.GaugeType)
	}
	live := newScanMWMetricPipeline(testScanMWPipelineConfig())
	replayed := newScanMWMetricPipeline(testScanMWPipelineConfig())
	found := 0
	for ts := int64(1); ts <= 100; ts++ {
		value := 10.0
		if ts > 60 {
			value = 100
		}
		ref := streaming.AddWithKeyAndHostComposite("work", "metric", "", value, ts,
			tagset.CompositeTags{}, 1, metrics.GaugeType).Ref
		got := live.Detect(streaming, ts, observer.DetectionFilter{}).Anomalies
		want := replayed.Detect(preloaded, ts, observer.DetectionFilter{}).Anomalies
		require.ElementsMatch(t, want, got, "at observation %d", ts)
		require.Empty(t, live.Detect(streaming, ts, observer.DetectionFilter{}).Anomalies)
		live.advanceKinds(streaming, ts)
		replayed.advanceKinds(preloaded, ts)
		liveKind, _ := streaming.SeriesKind(ref)
		replayKind, _ := preloaded.SeriesKind(ref)
		require.Equal(t, replayKind, liveKind, "kind parity at %d", ts)
		found += len(got)
	}
	require.Positive(t, found, "fixture must exercise anomaly output")
}

func BenchmarkMetricPipeline(b *testing.B) {
	for _, numSeries := range []int{5, 200, 2000} {
		for _, mode := range []string{"fresh_data", "unchanged_data"} {
			b.Run(fmt.Sprintf("series=%d/%s", numSeries, mode), func(b *testing.B) {
				s := newDetectorTestStorage()
				for series := 1; series <= numSeries; series++ {
					addPipelineValues(s, uint64(series), 1, 30, 100)
				}
				p := newScanMWMetricPipeline(DefaultScanMWConfig())
				p.advanceKinds(s, 30)
				now := int64(30)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if mode == "fresh_data" {
						b.StopTimer()
						now++
						for series := 1; series <= numSeries; series++ {
							addPipelineValues(s, uint64(series), now, now, 100)
						}
						b.StartTimer()
					}
					p.Detect(s, now, observer.DetectionFilter{})
					p.advanceKinds(s, now)
				}
				b.ReportMetric(float64(numSeries), "series/op")
			})
		}
	}
}
