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

func TestMetricPipelineRoutedDetection(t *testing.T) {
	stableReg, stable := newPipelineStub(observer.STABLE, "stable")
	linearReg, linear := newPipelineStub(observer.LINEAR, "linear")
	p, err := newMetricPipeline([]pipelineRegistration{stableReg, linearReg}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)

	result := p.Detect(nil, 1, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 2)
	require.Equal(t, []observer.MetricKind{observer.STABLE, observer.LINEAR}, stable.detected)
	require.Equal(t, []observer.MetricKind{observer.STABLE, observer.LINEAR}, linear.detected)
	require.Equal(t, []metrics.MetricType{metrics.GaugeType, metrics.GaugeType}, stable.types)
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage, observer.AggregateAverage}, stable.aggregates)

	filter := observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateCount}}
	p.Detect(nil, 1, filter)
	require.Equal(t, []observer.Aggregate{
		observer.AggregateAverage, observer.AggregateAverage, observer.AggregateCount,
	}, stable.aggregates)
	require.Equal(t, 3, stable.calls)
	require.Equal(t, 3, linear.calls)

	mismatch := observer.UNSET
	result = p.Detect(nil, 1, observer.DetectionFilter{MetricKind: &mismatch})
	require.Empty(t, result.Anomalies)
	require.Equal(t, 3, stable.calls)
	require.Equal(t, 3, linear.calls)
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
		for ts := int64(1); ts <= 20; ts++ {
			value := 10.0
			if ts > 10 {
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
	add(4, observer.STABLE, metrics.GaugeType)
	add(5, observer.TelemetryNamespace, metrics.GaugeType, observer.STABLE)

	p := newScanMWMetricPipeline(ScanMWConfig{MinPoints: 6, MinSegment: 2, MaxPoints: 20})
	result := p.Detect(s, 20, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 1)
	require.Equal(t, stable, result.Anomalies[0].SourceRef.Ref)
	require.Equal(t, observer.AggregateAverage, result.Anomalies[0].SourceRef.Aggregate)
	require.Len(t, p.registrations[0].detector.(*ScanMWDetector).series, 1)
	require.Contains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: stable, agg: observer.AggregateAverage})
	require.NotContains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: linear, agg: observer.AggregateAverage})
	require.NotContains(t, p.registrations[0].detector.(*ScanMWDetector).series,
		scanmwStateKey{ref: count, agg: observer.AggregateAverage})
}
