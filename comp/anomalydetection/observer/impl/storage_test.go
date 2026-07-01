// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"unsafe"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeSeriesStorage_Add(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add first point
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	assert.Equal(t, "test", series.Namespace)
	assert.Equal(t, "my.metric", series.Name)
	assert.Equal(t, []string{"env:prod"}, series.Tags)
	require.Len(t, series.Points, 1)
	assert.Equal(t, int64(1000), series.Points[0].Timestamp)
	assert.Equal(t, 10.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_Average(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add multiple points to same bucket
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 5.0, 1000, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	// Average of 10, 20, 5 = 35/3 = 11.67
	assert.InDelta(t, 11.67, series.Points[0].Value, 0.01)
}

func TestTimeSeriesStorage_AddSameBucket_Sum(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)
	series := s.GetSeries("test", "my.metric", nil, AggregateSum)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 35.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_Count(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)
	series := s.GetSeries("test", "my.metric", nil, AggregateCount)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 3.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_MinMax(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)

	minSeries := s.GetSeries("test", "my.metric", nil, AggregateMin)
	maxSeries := s.GetSeries("test", "my.metric", nil, AggregateMax)

	require.NotNil(t, minSeries)
	require.NotNil(t, maxSeries)
	assert.Equal(t, 5.0, minSeries.Points[0].Value)
	assert.Equal(t, 20.0, maxSeries.Points[0].Value)
}

func TestTimeSeriesStorage_AddDifferentBuckets(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add points to different buckets
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1001, []string{"env:prod"})
	s.Add("test", "my.metric", 30.0, 1002, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	require.Len(t, series.Points, 3)
	// Points should be sorted by timestamp
	assert.Equal(t, int64(1000), series.Points[0].Timestamp)
	assert.Equal(t, int64(1001), series.Points[1].Timestamp)
	assert.Equal(t, int64(1002), series.Points[2].Timestamp)
	assert.Equal(t, 10.0, series.Points[0].Value)
	assert.Equal(t, 20.0, series.Points[1].Value)
	assert.Equal(t, 30.0, series.Points[2].Value)
}

func TestTimeSeriesStorage_PreservesOutOfOrderBuckets(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1002, nil)
	s.Add("test", "my.metric", 30.0, 1001, nil) // inserted in order
	s.Add("test", "my.metric", 40.0, 1002, nil) // same bucket: aggregated

	series := s.GetSeries("test", "my.metric", nil, AggregateAverage)
	require.NotNil(t, series)
	require.Len(t, series.Points, 3)
	assert.Equal(t, int64(1000), series.Points[0].Timestamp)
	assert.Equal(t, int64(1001), series.Points[1].Timestamp)
	assert.Equal(t, int64(1002), series.Points[2].Timestamp)
	assert.Equal(t, 30.0, series.Points[1].Value)
	assert.Equal(t, 30.0, series.Points[2].Value)
}

func TestTimeSeriesStorage_DifferentTags(t *testing.T) {
	s := newTimeSeriesStorage()

	// Different tags = different series
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:staging"})

	prodSeries := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)
	stagingSeries := s.GetSeries("test", "my.metric", []string{"env:staging"}, AggregateAverage)

	require.NotNil(t, prodSeries)
	require.NotNil(t, stagingSeries)
	assert.Equal(t, 10.0, prodSeries.Points[0].Value)
	assert.Equal(t, 20.0, stagingSeries.Points[0].Value)
}

func TestTimeSeriesStorage_TagOrderDoesNotMatter(t *testing.T) {
	s := newTimeSeriesStorage()

	// Tags in different order should be same series
	s.Add("test", "my.metric", 10.0, 1000, []string{"a:1", "b:2"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"b:2", "a:1"})

	series := s.GetSeries("test", "my.metric", []string{"a:1", "b:2"}, AggregateAverage)
	require.NotNil(t, series)
	// Average of 10 and 20 = 15
	assert.Equal(t, 15.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_GetSeries_NotFound(t *testing.T) {
	s := newTimeSeriesStorage()

	series := s.GetSeries("test", "nonexistent", nil, AggregateAverage)
	assert.Nil(t, series)
}

func TestTimeSeriesStorage_AllSeries(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add series to different namespaces
	s.Add("ns1", "metric1", 10.0, 1000, nil)
	s.Add("ns1", "metric2", 20.0, 1000, nil)
	s.Add("ns2", "metric3", 30.0, 1000, nil)

	ns1Series := s.AllSeries("ns1", AggregateAverage)
	ns2Series := s.AllSeries("ns2", AggregateAverage)

	assert.Len(t, ns1Series, 2)
	assert.Len(t, ns2Series, 1)
}

func TestSeriesStats_AggregateAt(t *testing.T) {
	// Build a seriesStats with known columnar data to test aggregation.
	ss := &seriesStats{
		timestamps: []int64{1000},
		sums:       []float64{100.0},
		counts:     []int64{4},
		mins:       []float64{10.0},
		maxes:      []float64{40.0},
	}

	assert.Equal(t, 25.0, ss.aggregateAt(0, AggregateAverage))
	assert.Equal(t, 100.0, ss.aggregateAt(0, AggregateSum))
	assert.Equal(t, 4.0, ss.aggregateAt(0, AggregateCount))
	assert.Equal(t, 10.0, ss.aggregateAt(0, AggregateMin))
	assert.Equal(t, 40.0, ss.aggregateAt(0, AggregateMax))

	// Zero count returns 0 for average
	ss2 := &seriesStats{
		timestamps: []int64{1000},
		sums:       []float64{10.0},
		counts:     []int64{0},
		mins:       []float64{0},
		maxes:      []float64{0},
	}
	assert.Equal(t, 0.0, ss2.aggregateAt(0, AggregateAverage))
}

func TestAggSuffix(t *testing.T) {
	// Test all aggregation types return correct suffixes
	assert.Equal(t, "avg", aggSuffix(AggregateAverage))
	assert.Equal(t, "sum", aggSuffix(AggregateSum))
	assert.Equal(t, "count", aggSuffix(AggregateCount))
	assert.Equal(t, "min", aggSuffix(AggregateMin))
	assert.Equal(t, "max", aggSuffix(AggregateMax))

	// Unknown aggregation type
	assert.Equal(t, "unknown", aggSuffix(Aggregate(999)))
}

func TestTimeSeriesStorage_DropsNonFiniteValuesWithStats(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", math.Inf(1), 1000, nil)
	s.Add("test", "my.metric", math.NaN(), 1001, nil)

	series := s.GetSeries("test", "my.metric", nil, AggregateAverage)
	assert.Nil(t, series)

	nonFinite, extreme, byMetric := s.DroppedValueStats()
	assert.Equal(t, int64(2), nonFinite)
	assert.Equal(t, int64(0), extreme)
	assert.Equal(t, int64(2), byMetric["test|my.metric"])
}

func TestTimeSeriesStorage_DropsExtremeFiniteValuesWithStats(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", math.MaxFloat64, 1000, nil)
	s.Add("test", "my.metric", math.MaxFloat64/4, 1001, nil)

	series := s.GetSeries("test", "my.metric", nil, AggregateAverage)
	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, math.MaxFloat64/4, series.Points[0].Value)

	nonFinite, extreme, byMetric := s.DroppedValueStats()
	assert.Equal(t, int64(0), nonFinite)
	assert.Equal(t, int64(1), extreme)
	assert.Equal(t, int64(1), byMetric["test|my.metric"])
}

// --- Binary-search-based range query tests ---

func makeRangeStorage() *timeSeriesStorage {
	s := newTimeSeriesStorage()
	// Insert points at timestamps 10, 20, 30, 40, 50
	for _, ts := range []int64{10, 20, 30, 40, 50} {
		s.Add("ns", "m", float64(ts), ts, nil)
	}
	return s
}

// rangeID is the numeric ID for the first (and only) series added by makeRangeStorage.
const rangeID = observer.SeriesRef(0)

func TestGetSeriesRange_EmptySeries(t *testing.T) {
	s := newTimeSeriesStorage()
	result := s.GetSeriesRange(observer.SeriesRef(-1), 0, 100, AggregateSum)
	assert.Nil(t, result)
}

func TestGetSeriesRange_SinglePoint(t *testing.T) {
	s := newTimeSeriesStorage()
	s.Add("ns", "m", 42.0, 100, nil)

	// First series added gets ID 0
	id := observer.SeriesRef(0)

	// Range that includes the point: (0, 100]
	result := s.GetSeriesRange(id, 0, 100, AggregateSum)
	require.NotNil(t, result)
	require.Len(t, result.Points, 1)
	assert.Equal(t, int64(100), result.Points[0].Timestamp)
	assert.Equal(t, 42.0, result.Points[0].Value)

	// Range that excludes the point: start == point timestamp (exclusive)
	result = s.GetSeriesRange(id, 100, 200, AggregateSum)
	require.NotNil(t, result)
	assert.Empty(t, result.Points)

	// Range before the point
	result = s.GetSeriesRange(id, 0, 99, AggregateSum)
	require.NotNil(t, result)
	assert.Empty(t, result.Points)
}

func TestGetSeriesRange_StartExclusiveEndInclusive(t *testing.T) {
	s := makeRangeStorage()

	// (20, 40] should include 30, 40 but not 20
	result := s.GetSeriesRange(rangeID, 20, 40, AggregateSum)
	require.NotNil(t, result)
	require.Len(t, result.Points, 2)
	assert.Equal(t, int64(30), result.Points[0].Timestamp)
	assert.Equal(t, int64(40), result.Points[1].Timestamp)
}

func TestGetSeriesRange_ExactBoundaryHits(t *testing.T) {
	s := makeRangeStorage()

	// (10, 50] should include 20, 30, 40, 50 but not 10
	result := s.GetSeriesRange(rangeID, 10, 50, AggregateSum)
	require.NotNil(t, result)
	require.Len(t, result.Points, 4)
	assert.Equal(t, int64(20), result.Points[0].Timestamp)
	assert.Equal(t, int64(50), result.Points[3].Timestamp)

	// (0, 10] should include only 10
	result = s.GetSeriesRange(rangeID, 0, 10, AggregateSum)
	require.NotNil(t, result)
	require.Len(t, result.Points, 1)
	assert.Equal(t, int64(10), result.Points[0].Timestamp)
}

func TestGetSeriesRange_StartZeroReadsAll(t *testing.T) {
	s := makeRangeStorage()

	// (0, 999] with start=0 should include all 5 points
	result := s.GetSeriesRange(rangeID, 0, 999, AggregateSum)
	require.NotNil(t, result)
	assert.Len(t, result.Points, 5)
}

func TestGetSeriesRange_NoOverlap(t *testing.T) {
	s := makeRangeStorage()

	// Range entirely before data
	result := s.GetSeriesRange(rangeID, 0, 5, AggregateSum)
	require.NotNil(t, result)
	assert.Empty(t, result.Points)

	// Range entirely after data
	result = s.GetSeriesRange(rangeID, 50, 100, AggregateSum)
	require.NotNil(t, result)
	assert.Empty(t, result.Points)
}

func TestGetSeriesRange_AllAggregates(t *testing.T) {
	s := newTimeSeriesStorage()
	// Two values in the same bucket: sum=30, count=2, min=10, max=20, avg=15
	s.Add("ns", "m", 10.0, 100, nil)
	s.Add("ns", "m", 20.0, 100, nil)

	id := observer.SeriesRef(0)

	for _, tc := range []struct {
		agg      Aggregate
		expected float64
	}{
		{AggregateSum, 30.0},
		{AggregateCount, 2.0},
		{AggregateMin, 10.0},
		{AggregateMax, 20.0},
		{AggregateAverage, 15.0},
	} {
		result := s.GetSeriesRange(id, 0, 200, tc.agg)
		require.NotNil(t, result)
		require.Len(t, result.Points, 1)
		assert.Equal(t, tc.expected, result.Points[0].Value, "agg=%d", tc.agg)
	}
}

func TestPointCountUpTo_BinarySearch(t *testing.T) {
	s := makeRangeStorage()

	// All points <= 50
	assert.Equal(t, 5, s.PointCountUpTo(rangeID, 50))
	// Points <= 30: timestamps 10, 20, 30
	assert.Equal(t, 3, s.PointCountUpTo(rangeID, 30))
	// Points <= 25: timestamps 10, 20
	assert.Equal(t, 2, s.PointCountUpTo(rangeID, 25))
	// Points <= 9: none
	assert.Equal(t, 0, s.PointCountUpTo(rangeID, 9))
	// Points <= 10: just one
	assert.Equal(t, 1, s.PointCountUpTo(rangeID, 10))
	// Non-existent series
	assert.Equal(t, 0, s.PointCountUpTo(observer.SeriesRef(999), 100)) // non-existent ID
}

func TestPointCount_ColumnarLayout(t *testing.T) {
	s := makeRangeStorage()
	assert.Equal(t, 5, s.PointCount(rangeID))
	assert.Equal(t, 0, s.PointCount(observer.SeriesRef(999))) // non-existent ID
}

func TestGetSeriesRange_OutOfOrderInsert(t *testing.T) {
	s := newTimeSeriesStorage()
	// Insert out of order — storage keeps buckets sorted.
	s.Add("ns", "m", 30.0, 30, nil)
	s.Add("ns", "m", 10.0, 10, nil)
	s.Add("ns", "m", 20.0, 20, nil)

	result := s.GetSeriesRange(observer.SeriesRef(0), 0, 100, AggregateSum)
	require.NotNil(t, result)
	require.Len(t, result.Points, 3)
	assert.Equal(t, int64(10), result.Points[0].Timestamp)
	assert.Equal(t, int64(20), result.Points[1].Timestamp)
	assert.Equal(t, int64(30), result.Points[2].Timestamp)
}

func TestFindingH1_StorageNamespacesRace(_ *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine: continuously add data.
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	// Reader goroutine: call Namespaces() concurrently.
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.Namespaces()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageTimeBoundsRace(_ *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_, _, _ = s.TimeBounds()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageMaxTimestampRace(_ *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.MaxTimestamp()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageListAllSeriesCompactRace(_ *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Add("ns", "metric", float64(i), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.ListAllSeriesCompact()
		}
	}()

	wg.Wait()
}

func TestFindingH1_StorageDroppedValueStatsRace(_ *testing.T) {
	s := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			// Add some NaN to trigger drop accounting writes
			s.Add("ns", "metric", math.NaN(), int64(i), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_, _, _ = s.DroppedValueStats()
		}
	}()

	wg.Wait()
}

func TestFindingM5_NegativeMaxFloat64NotFiltered(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add two -MaxFloat64 values at the same timestamp.
	s.Add("ns", "metric", -math.MaxFloat64, 1000, nil)
	s.Add("ns", "metric", -math.MaxFloat64, 1000, nil)

	series := s.GetSeries("ns", "metric", nil, AggregateSum)
	if series == nil {
		// If both were filtered, the series would be nil, which is acceptable.
		// But if only one was stored...
		t.Skip("both values were filtered (series is nil), finding may be partially addressed")
		return
	}

	require.Len(t, series.Points, 1)
	sum := series.Points[0].Value
	assert.False(t, math.IsInf(sum, -1),
		"sum of two -MaxFloat64 values is -Inf (%v), storage should filter -MaxFloat64 like it filters +MaxFloat64", sum)
	assert.False(t, math.IsNaN(sum),
		"sum of two -MaxFloat64 values is NaN (%v), storage should filter -MaxFloat64", sum)
}

func TestTimeBoundsSkipsNonPositivePrefixOnly(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "metric", 1, 0, nil)
	s.Add("test", "metric", 2, 10, nil)
	s.Add("test", "metric", 3, 20, nil)

	minTs, maxTs, ok := s.TimeBounds()
	assert.True(t, ok)
	assert.Equal(t, int64(10), minTs)
	assert.Equal(t, int64(20), maxTs)
}

func TestTimeSeriesStorage_ListSeries_ExcludeNamespaces(t *testing.T) {
	s := newTimeSeriesStorage()
	s.Add(observer.TelemetryNamespace, "internal.gauge", 1, 1000, nil)
	s.Add("work", "cpu", 2, 1000, nil)

	all := s.ListSeries(observer.SeriesFilter{})
	require.Len(t, all, 2)

	workload := s.ListSeries(observer.WorkloadSeriesFilter())
	require.Len(t, workload, 1)
	assert.Equal(t, "work", workload[0].Namespace)

	workloadRefs := s.ListSeriesRefsInto(observer.WorkloadSeriesFilter(), nil)
	require.Equal(t, []observer.SeriesRef{workload[0].Ref}, workloadRefs)

	onlyTel := s.ListSeries(observer.SeriesFilter{Namespace: observer.TelemetryNamespace})
	require.Len(t, onlyTel, 1)
	assert.Equal(t, observer.TelemetryNamespace, onlyTel[0].Namespace)

	telRefs := s.ListSeriesRefsInto(observer.SeriesFilter{Namespace: observer.TelemetryNamespace}, workloadRefs)
	require.Equal(t, []observer.SeriesRef{onlyTel[0].Ref}, telRefs)
}

func TestTimeSeriesStorage_ListSeriesRefsInto_MatchesListSeriesFilters(t *testing.T) {
	s := newTimeSeriesStorage()
	s.Add(observer.TelemetryNamespace, "internal.gauge", 1, 1000, []string{"env:prod"})
	s.Add("work", "cpu.usage", 2, 1000, []string{"env:prod", "service:web"})
	s.Add("work", "cpu.load", 3, 1000, []string{"env:prod", "service:api"})
	s.Add("work", "mem.rss", 4, 1000, []string{"env:stage", "service:web"})

	for name, filter := range map[string]observer.SeriesFilter{
		"all":                {},
		"workload":           observer.WorkloadSeriesFilter(),
		"namespace":          {Namespace: observer.TelemetryNamespace},
		"name_prefix":        {NamePattern: "cpu."},
		"tag_matcher":        {TagMatchers: map[string]string{"service": "web"}},
		"namespace_and_tags": {Namespace: "work", TagMatchers: map[string]string{"env": "prod"}},
	} {
		t.Run(name, func(t *testing.T) {
			metas := s.ListSeries(filter)
			want := make([]observer.SeriesRef, 0, len(metas))
			for _, meta := range metas {
				want = append(want, meta.Ref)
			}

			got := s.ListSeriesRefsInto(filter, []observer.SeriesRef{999})
			require.Equal(t, want, got)
		})
	}
}

func TestTimeSeriesStorage_RemoveSeriesByRefs(t *testing.T) {
	s := newTimeSeriesStorage()

	resA := s.Add("ns", "a", 1.0, 1000, []string{"k:1"})
	resB := s.Add("ns", "b", 2.0, 1000, []string{"k:2"})
	resC := s.Add("ns", "c", 3.0, 1000, []string{"k:3"})
	require.Equal(t, 3, s.TotalSeriesCount(""))
	genBefore := s.SeriesGeneration()

	refA, refB, refC := resA.Ref, resB.Ref, resC.Ref

	// Remove b and c; pass a bogus ref (-1) that should be silently ignored.
	removed := s.RemoveSeriesByRefs([]observer.SeriesRef{refB, refC, -1})
	require.Len(t, removed, 2, "out-of-range refs are silently ignored")
	require.ElementsMatch(t, []observer.SeriesRef{refB, refC}, removed, "freed refs are returned for fan-out to detectors")
	require.Equal(t, 1, s.TotalSeriesCount(""), "only series 'a' should remain")
	require.Greater(t, s.SeriesGeneration(), genBefore, "seriesGen bumps on removal")

	require.Nil(t, s.GetSeriesMeta(refB), "removed ref resolves to nil")
	require.Nil(t, s.GetSeriesMeta(refC), "removed ref resolves to nil")
	require.NotNil(t, s.GetSeriesMeta(refA), "surviving series still resolvable")

	// A subsequent Add for the same series creates a fresh series with a new ref.
	res2B := s.Add("ns", "b", 99.0, 1100, []string{"k:2"})
	require.Equal(t, 2, s.TotalSeriesCount(""), "re-add re-creates the series")
	require.NotEqual(t, refB, res2B.Ref, "new ref minted; old ref is retired")
	require.Nil(t, s.GetSeriesMeta(refB), "old ref still resolves to nil after re-add")
}

func TestTimeSeriesStorage_FindRefsByHashes(t *testing.T) {
	s := newTimeSeriesStorage()

	resA := s.Add("ns", "a", 1.0, 1000, []string{"k:1"})
	resB := s.Add("ns", "b", 2.0, 1000, []string{"k:2"})
	s.Add("ns", "c", 3.0, 1000, []string{"k:3"})

	hA := seriesKeyHash("ns", "a", []string{"k:1"})
	hB := seriesKeyHash("ns", "b", []string{"k:2"})
	hMissing := seriesKeyHash("ns", "ghost", nil)

	refs := s.FindRefsByHashes(map[uint64]struct{}{hA: {}, hB: {}, hMissing: {}})

	require.Len(t, refs, 2)
	require.ElementsMatch(t, []observer.SeriesRef{resA.Ref, resB.Ref}, refs)
}

func TestTimeSeriesStorage_RemoveSeriesByRefsEmptyOrUnknown(t *testing.T) {
	s := newTimeSeriesStorage()
	s.Add("ns", "a", 1.0, 1000, nil)
	genBefore := s.SeriesGeneration()

	require.Empty(t, s.RemoveSeriesByRefs(nil))
	require.Empty(t, s.RemoveSeriesByRefs([]observer.SeriesRef{}))
	// Out-of-range refs (-1, 999) are silently skipped.
	require.Empty(t, s.RemoveSeriesByRefs([]observer.SeriesRef{-1, 999}))
	require.Equal(t, genBefore, s.SeriesGeneration(), "no removal → no gen bump")
}

func TestTimeSeriesStorage_AddReturnsRef(t *testing.T) {
	// Add returns a valid Ref (>= 0) for accepted points, and the same Ref
	// on subsequent writes. Each distinct series gets a unique Ref.
	s := newTimeSeriesStorage()

	cases := []struct {
		name      string
		namespace string
		metric    string
		tags      []string
	}{
		{"no_tags", "ns", "m1", nil},
		{"single_tag", "ns", "m2", []string{"env:prod"}},
		{"sorted_tags", "ns", "m3", []string{"a:1", "b:2", "c:3"}},
		{"unsorted_tags", "ns", "m4", []string{"c:3", "a:1", "b:2"}},
		{"empty_namespace", "", "m5", []string{"env:prod"}},
	}

	seen := make(map[observer.SeriesRef]bool)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := s.Add(tc.namespace, tc.metric, 1.0, 1000, tc.tags)
			assert.True(t, res.IsNew, "first write should report IsNew=true")
			assert.GreaterOrEqual(t, int(res.Ref), 0, "valid Ref must be >= 0")
			assert.False(t, seen[res.Ref], "each series must get a unique Ref")
			seen[res.Ref] = true

			// Second write returns same Ref and IsNew=false.
			res2 := s.Add(tc.namespace, tc.metric, 2.0, 1001, tc.tags)
			assert.False(t, res2.IsNew, "second write should report IsNew=false")
			assert.Equal(t, res.Ref, res2.Ref, "subsequent writes must return the same Ref")
		})
	}
}

func TestTimeSeriesStorage_AddDroppedReturnsNegativeRef(t *testing.T) {
	// Drops (non-finite, sentinel values) return Ref=-1.
	// Callers check res.Ref >= 0 before using Ref for downstream state.
	s := newTimeSeriesStorage()

	res := s.Add("ns", "m", math.NaN(), 1000, nil)
	assert.False(t, res.IsNew)
	assert.Equal(t, observer.SeriesRef(-1), res.Ref, "NaN drop must return Ref=-1")

	res = s.Add("ns", "m", math.Inf(1), 1000, nil)
	assert.False(t, res.IsNew)
	assert.Equal(t, observer.SeriesRef(-1), res.Ref, "+Inf drop must return Ref=-1")

	res = s.Add("ns", "m", math.MaxFloat64, 1000, nil)
	assert.False(t, res.IsNew)
	assert.Equal(t, observer.SeriesRef(-1), res.Ref, "MaxFloat64 sentinel drop must return Ref=-1")
}

func TestTimeSeriesStorage_TagIntern_PoolGrows(t *testing.T) {
	s := newTimeSeriesStorage()
	assert.Equal(t, 0, s.TagInternedCount())

	s.Add("ns", "m1", 1.0, 1000, []string{"env:prod", "host:a"})
	assert.Equal(t, 1, s.TagInternedCount(), "one combination interned")

	s.Add("ns", "m2", 1.0, 1000, []string{"env:prod", "host:b"})
	assert.Equal(t, 2, s.TagInternedCount(), "second distinct combination interned")

	// Same combination as m1 — pool must not grow.
	s.Add("ns2", "m1", 1.0, 1000, []string{"env:prod", "host:a"})
	assert.Equal(t, 2, s.TagInternedCount(), "repeated combination must not grow pool")
}

func TestTimeSeriesStorage_TagIntern_SharedSlice(t *testing.T) {
	s := newTimeSeriesStorage()

	// Build tags at runtime to defeat Go's compile-time string interning.
	tags := []string{"env:" + string([]byte{'p', 'r', 'o', 'd'}), "host:a"}

	res1 := s.Add("ns", "m1", 1.0, 1000, tags)
	res2 := s.Add("ns", "m2", 1.0, 1000, tags)

	s.mu.RLock()
	stats1 := s.resolveByID(res1.Ref)
	stats2 := s.resolveByID(res2.Ref)
	s.mu.RUnlock()

	require.NotNil(t, stats1)
	require.NotNil(t, stats2)

	ptr1 := uintptr(unsafe.Pointer(unsafe.SliceData(stats1.Tags)))
	ptr2 := uintptr(unsafe.Pointer(unsafe.SliceData(stats2.Tags)))
	assert.Equal(t, ptr1, ptr2, "series with identical tag sets must share the same []string backing array")
	assert.Equal(t, 1, s.TagInternedCount())
}

func TestTimeSeriesStorage_TagIntern_Eviction(t *testing.T) {
	s := newTimeSeriesStorage()

	tags := []string{"env:prod", "host:a"}
	res1 := s.Add("ns", "m1", 1.0, 1000, tags)
	res2 := s.Add("ns", "m2", 1.0, 1000, tags)
	assert.Equal(t, 1, s.TagInternedCount(), "one pool entry for the shared combination")

	s.RemoveSeriesByRefs([]observer.SeriesRef{res1.Ref})
	assert.Equal(t, 1, s.TagInternedCount(), "pool entry must survive while m2 still references it")

	s.RemoveSeriesByRefs([]observer.SeriesRef{res2.Ref})
	assert.Equal(t, 0, s.TagInternedCount(), "pool entry must be freed when last referencing series is evicted")
}

func TestTimeSeriesStorage_TagIntern_NilAndEmptyTags(t *testing.T) {
	s := newTimeSeriesStorage()

	res1 := s.Add("ns", "nil_tags", 1.0, 1000, nil)
	assert.Equal(t, 0, s.TagInternedCount(), "nil tags must not create an intern entry")

	res2 := s.Add("ns", "empty_tags", 1.0, 1000, []string{})
	assert.Equal(t, 0, s.TagInternedCount(), "empty tags must not create an intern entry")

	s.RemoveSeriesByRefs([]observer.SeriesRef{res1.Ref, res2.Ref})
	assert.Equal(t, 0, s.TagInternedCount(), "removal of uninterned series must leave pool empty")
}

func TestTimeSeriesStorage_TagIntern_UnsortedTagsShareEntry(t *testing.T) {
	s := newTimeSeriesStorage()

	tags1 := []string{"host:a", "env:prod"} // unsorted
	tags2 := []string{"env:prod", "host:a"} // sorted

	res1 := s.Add("ns", "m1", 1.0, 1000, tags1)
	res2 := s.Add("ns", "m2", 1.0, 1000, tags2)
	assert.Equal(t, 1, s.TagInternedCount(), "same tags in different order must share one pool entry")

	s.mu.RLock()
	stats1 := s.resolveByID(res1.Ref)
	stats2 := s.resolveByID(res2.Ref)
	s.mu.RUnlock()

	ptr1 := uintptr(unsafe.Pointer(unsafe.SliceData(stats1.Tags)))
	ptr2 := uintptr(unsafe.Pointer(unsafe.SliceData(stats2.Tags)))
	assert.Equal(t, ptr1, ptr2, "unsorted and sorted variants must share the same backing array")
}

func TestTimeSeriesStorage_TagIntern_Cap(t *testing.T) {
	s := newTimeSeriesStorage()

	for i := 0; i < tagInternMaxSize; i++ {
		s.Add("ns", fmt.Sprintf("m%d", i), 1.0, 1000, []string{fmt.Sprintf("unique:tag%d", i)})
	}
	assert.Equal(t, tagInternMaxSize, s.TagInternedCount(), "pool should be at cap")

	s.Add("ns", "overflow", 1.0, 1000, []string{"unique:overflow"})
	assert.Equal(t, tagInternMaxSize, s.TagInternedCount(), "pool must not exceed cap")

	s.Add("ns2", "m0", 1.0, 1000, []string{"unique:tag0"})
	assert.Equal(t, tagInternMaxSize, s.TagInternedCount(), "hit on existing entry must not grow pool")
}
