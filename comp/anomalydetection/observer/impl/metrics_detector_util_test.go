// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/require"
)

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
		"scanmw":    {NewScanMWDetector(), observer.DetectorPointWindow{MinPoints: 30, MaxPoints: 120}},
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
		"scanmw": &ScanMWDetector{
			MinPoints:    4,
			MinSegment:   2,
			MaxPoints:    8,
			Aggregations: []observer.Aggregate{observer.AggregateAverage},
		},
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

func (*seriesListOnlyStorage) GetContext(observer.SeriesRef) *observer.MetricContext { return nil }

func (s *seriesListOnlyStorage) GetSeriesRange(observer.SeriesRef, int64, int64, observer.Aggregate) *observer.Series {
	return nil
}

func (s *seriesListOnlyStorage) ForEachPoint(observer.SeriesRef, int64, int64, observer.Aggregate, func(*observer.Series, observer.Point)) bool {
	return false
}

func (s *seriesListOnlyStorage) PointCount(observer.SeriesRef) int {
	return 0
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
