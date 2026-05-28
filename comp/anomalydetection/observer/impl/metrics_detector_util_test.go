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
