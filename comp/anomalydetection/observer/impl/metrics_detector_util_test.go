// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

func TestDetectorSeriesFilter(t *testing.T) {
	storage := newTimeSeriesStorage()
	add := func(key uint64, namespace string, metricType metrics.MetricType, kind observer.MetricKind) observer.SeriesRef {
		ref := storage.AddWithKeyAndHostComposite(namespace, "metric", "", 100, 1,
			tagset.CompositeTags{}, key, metricType).Ref
		storage.SetSeriesKind(ref, kind)
		return ref
	}
	stableGauge := add(1, "work", metrics.GaugeType, observer.STABLE)
	stableCount := add(2, "work", metrics.CountType, observer.STABLE)
	linearGauge := add(3, "work", metrics.GaugeType, observer.LINEAR)
	unset := add(4, "work", observer.UnknownType, observer.UNSET)
	unknownGauge := add(5, "work", metrics.GaugeType, observer.UNKNOWN)
	add(6, observer.TelemetryNamespace, metrics.GaugeType, observer.STABLE)

	stable, linear, unknownKind, unsetKind := observer.STABLE, observer.LINEAR, observer.UNKNOWN, observer.UNSET
	gauge, count, unknownType := metrics.GaugeType, metrics.CountType, observer.UnknownType
	for _, tc := range []struct {
		name   string
		filter observer.DetectionFilter
		want   []observer.SeriesRef
	}{
		{"zero", observer.DetectionFilter{}, []observer.SeriesRef{stableGauge, stableCount, linearGauge, unset, unknownGauge}},
		{"kind only", observer.DetectionFilter{MetricKind: &stable}, []observer.SeriesRef{stableGauge, stableCount}},
		{"type only", observer.DetectionFilter{MetricType: &gauge}, []observer.SeriesRef{stableGauge, linearGauge, unknownGauge}},
		{"combined", observer.DetectionFilter{MetricKind: &stable, MetricType: &count}, []observer.SeriesRef{stableCount}},
		{"explicit unset", observer.DetectionFilter{MetricKind: &unsetKind}, []observer.SeriesRef{unset}},
		{"explicit unknown type", observer.DetectionFilter{MetricType: &unknownType}, []observer.SeriesRef{unset}},
		{"unknown kind", observer.DetectionFilter{MetricKind: &unknownKind}, []observer.SeriesRef{unknownGauge}},
		{"no match", observer.DetectionFilter{MetricKind: &linear, MetricType: &count}, nil},
		{"aggregation does not select series", observer.DetectionFilter{
			MetricKind: &stable, Aggregations: []observer.Aggregate{observer.AggregateSum},
		}, []observer.SeriesRef{stableGauge, stableCount}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := newDetectorSelectionKey(tc.filter)
			selection := detectorSeriesFilter(tc.filter)
			require.Equal(t, observer.SeriesFilter{
				MetricKind: tc.filter.MetricKind, MetricType: tc.filter.MetricType,
				ExcludeNamespaces: []string{observer.TelemetryNamespace},
			}, selection)
			require.ElementsMatch(t, tc.want, storage.ListSeriesRefsInto(selection, nil))
			require.Equal(t, before, newDetectorSelectionKey(tc.filter), "selection must not modify caller values")
		})
	}
}

func TestDetectorSelectionKey(t *testing.T) {
	kind, metricType := observer.UNSET, observer.UnknownType
	filter := observer.DetectionFilter{MetricKind: &kind, MetricType: &metricType}
	original := newDetectorSelectionKey(filter)
	require.Equal(t, detectorSelectionKey{
		kindSet: true, kind: observer.UNSET, typeSet: true, typ: observer.UnknownType,
	}, original)
	require.NotEqual(t, original, newDetectorSelectionKey(observer.DetectionFilter{}))
	require.NotEqual(t, newDetectorSelectionKey(observer.DetectionFilter{}),
		newDetectorSelectionKey(observer.DetectionFilter{MetricKind: &kind}))
	require.NotEqual(t, newDetectorSelectionKey(observer.DetectionFilter{}),
		newDetectorSelectionKey(observer.DetectionFilter{MetricType: &metricType}))

	otherKind, otherType := kind, metricType
	require.Equal(t, original, newDetectorSelectionKey(observer.DetectionFilter{
		MetricKind: &otherKind, MetricType: &otherType,
	}), "pointer identity must not affect the key")
	filter.Aggregations = []observer.Aggregate{observer.AggregateCount, observer.AggregateAverage}
	require.Equal(t, original, newDetectorSelectionKey(filter), "aggregation restrictions do not affect series membership")

	kind = observer.STABLE
	require.NotEqual(t, original, newDetectorSelectionKey(filter))
	kind = observer.UNSET
	metricType = metrics.GaugeType
	require.NotEqual(t, original, newDetectorSelectionKey(filter))
	require.Equal(t, observer.UNSET, original.kind, "previous keys must retain their values")
	require.Equal(t, observer.UnknownType, original.typ)
}

func TestAggregationAllowed(t *testing.T) {
	configured := []observer.Aggregate{observer.AggregateCount, observer.AggregateAverage}
	for _, tc := range []struct {
		name      string
		requested []observer.Aggregate
		want      []observer.Aggregate
	}{
		{"nil", nil, configured},
		{"empty", []observer.Aggregate{}, configured},
		{"intersection", []observer.Aggregate{observer.AggregateSum, observer.AggregateAverage}, []observer.Aggregate{observer.AggregateAverage}},
		{"no match", []observer.Aggregate{observer.AggregateSum}, nil},
		{"configured order", []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}, configured},
		{"duplicates", []observer.Aggregate{observer.AggregateAverage, observer.AggregateAverage}, []observer.Aggregate{observer.AggregateAverage}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]observer.Aggregate{}, tc.requested...)
			var selected []observer.Aggregate
			for _, agg := range configured {
				if aggregationAllowed(agg, tc.requested) {
					selected = append(selected, agg)
				}
			}
			require.Equal(t, tc.want, selected)
			require.Equal(t, before, append([]observer.Aggregate{}, tc.requested...), "caller aggregations must be unchanged")
		})
	}
}

func TestDetectorFilterHelpersDoNotAllocate(t *testing.T) {
	kind, metricType := observer.STABLE, metrics.GaugeType
	filter := observer.DetectionFilter{
		MetricKind: &kind, MetricType: &metricType,
		Aggregations: []observer.Aggregate{observer.AggregateCount, observer.AggregateAverage},
	}
	var key detectorSelectionKey
	var allowed bool
	allocations := testing.AllocsPerRun(100, func() {
		key = newDetectorSelectionKey(filter)
		allowed = aggregationAllowed(observer.AggregateAverage, filter.Aggregations)
	})
	require.Zero(t, allocations, "cache-key construction and membership checks are hot-path operations")
	require.Equal(t, newDetectorSelectionKey(filter), key)
	require.True(t, allowed)
}

func TestListSeriesRefsFallbackUsesListSeries(t *testing.T) {
	storage := &seriesListOnlyStorage{
		metas: []observer.SeriesMeta{
			{Ref: 12, Namespace: "work", Name: "cpu"},
			{Ref: 42, Namespace: "work", Name: "mem"},
		},
	}
	filter := observer.SeriesFilter{Namespace: "work", NamePattern: "c"}
	dst := []observer.SeriesRef{99, 100, 101}

	refs := listSeriesRefs(storage, filter, dst)

	require.Equal(t, []observer.SeriesRef{12, 42}, refs)
	require.Equal(t, 1, storage.listCalls)
	require.Equal(t, filter, storage.lastFilter)
	require.Same(t, &dst[0], &refs[0])
}

func TestDetectorPointWindows(t *testing.T) {
	bocpdConfig := DefaultBOCPDConfig()
	bocpdConfig.WarmupPoints = 40
	bocpd := NewBOCPDDetector(bocpdConfig)

	holt := NewHoltResidualDetector()
	holt.WarmupPoints = 24
	holt.ResidualWindow = 60

	tukey := NewTukeyBiweightDetector()
	tukey.MinPoints = 40
	tukey.WindowSize = 80

	for name, test := range map[string]struct {
		detector observer.DetectorPointWindowRequirement
		want     observer.DetectorPointWindow
	}{
		"bocpd":     {bocpd, observer.DetectorPointWindow{MinPoints: 40, MaxPoints: 120}},
		"holt":      {holt, observer.DetectorPointWindow{MinPoints: 24, MaxPoints: 60}},
		"tukey":     {tukey, observer.DetectorPointWindow{MinPoints: 40, MaxPoints: 80}},
		"scanmw":    {NewScanMWDetector(DefaultScanMWConfig()), observer.DetectorPointWindow{MinPoints: 30, MaxPoints: 120}},
		"scanwelch": {NewScanWelchDetector(), observer.DetectorPointWindow{MinPoints: 30, MaxPoints: 120}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, test.detector.DetectorPointWindow())
		})
	}
}

func TestDetectorPointWindowActivation(t *testing.T) {
	bocpdConfig := DefaultBOCPDConfig()
	bocpdConfig.WarmupPoints = 4
	bocpdConfig.MaxRunLength = 8
	bocpdConfig.Aggregations = []observer.Aggregate{observer.AggregateAverage}

	for name, detector := range map[string]observer.Detector{
		"bocpd": NewBOCPDDetector(bocpdConfig),
		"holt": &HoltResidualDetector{
			WarmupPoints:   4,
			ResidualWindow: 1,
			Aggregations:   []observer.Aggregate{observer.AggregateAverage},
		},
		"tukey": &TukeyBiweightDetector{
			WindowSize:   8,
			MinPoints:    4,
			ScoreEvery:   1,
			Aggregations: []observer.Aggregate{observer.AggregateAverage},
		},
		"scanmw": NewScanMWDetector(ScanMWConfig{
			MinPoints:    4,
			MinSegment:   2,
			MaxPoints:    8,
			Aggregations: []observer.Aggregate{observer.AggregateAverage},
		}),
		"scanwelch": &ScanWelchDetector{
			MinPoints:    4,
			MinSegment:   2,
			MaxPoints:    8,
			Aggregations: []observer.Aggregate{observer.AggregateAverage},
		},
	} {
		t.Run(name, func(t *testing.T) {
			storage := newDetectorTestStorage()
			for timestamp := int64(1); timestamp <= 3; timestamp++ {
				storage.Add("ns", "metric", float64(timestamp), timestamp, nil)
				detector.Detect(storage, timestamp)
				require.False(t, detector.Ready())
				require.Zero(t, detectorStateCount(detector))
			}

			storage.Add("ns", "metric", 4, 4, nil)
			detector.Detect(storage, 4)
			require.Equal(t, 1, detectorStateCount(detector))

			storage.Add("ns", "metric", 5, 5, nil)
			detector.Detect(storage, 5)
			require.True(t, detector.Ready())
		})
	}
}

func detectorStateCount(detector observer.Detector) int {
	switch d := detector.(type) {
	case *BOCPDDetector:
		return len(d.series)
	case *HoltResidualDetector:
		return len(d.series)
	case *TukeyBiweightDetector:
		return len(d.series)
	case *ScanMWDetector:
		return len(d.series)
	case *ScanWelchDetector:
		return len(d.series)
	default:
		return 0
	}
}

func TestAppendPointWindow(t *testing.T) {
	points := make([]observer.Point, 0, 3)
	for timestamp := int64(1); timestamp <= 5; timestamp++ {
		points = appendPointWindow(points, 3, observer.Point{Timestamp: timestamp})
	}
	require.Equal(t, []observer.Point{{Timestamp: 3}, {Timestamp: 4}, {Timestamp: 5}}, points)
}

type seriesListOnlyStorage struct {
	metas      []observer.SeriesMeta
	listCalls  int
	lastFilter observer.SeriesFilter
}

func (s *seriesListOnlyStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesMeta {
	s.listCalls++
	s.lastFilter = filter
	return s.metas
}

func (s *seriesListOnlyStorage) GetSeriesMeta(ref observer.SeriesRef) *observer.SeriesMeta {
	for i := range s.metas {
		if s.metas[i].Ref == ref {
			return &s.metas[i]
		}
	}
	return nil
}

func (*seriesListOnlyStorage) GetContext(observer.SeriesRef) (observer.MetricContext, bool) {
	return observer.MetricContext{}, false
}

func (s *seriesListOnlyStorage) GetSeriesRange(observer.SeriesRef, int64, int64, observer.Aggregate) *observer.Series {
	return nil
}

func (s *seriesListOnlyStorage) ForEachPoint(observer.SeriesRef, int64, int64, observer.Aggregate, func(*observer.Series, observer.Point)) bool {
	return false
}

func (s *seriesListOnlyStorage) PointCountUpTo(observer.SeriesRef, int64) int {
	return 0
}

func (s *seriesListOnlyStorage) SumRange(observer.SeriesRef, int64, int64, observer.Aggregate) float64 {
	return 0
}

func (s *seriesListOnlyStorage) WriteGeneration(observer.SeriesRef) int64 {
	return 0
}

func (s *seriesListOnlyStorage) SeriesGeneration() uint64 {
	return 0
}
