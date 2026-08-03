// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// TestbenchLogCountViewConfig configures the detector-only, time-aware view of
// sparse log occurrence series. It is intentionally testbench-only while the
// representation and detector behavior are evaluated.
type TestbenchLogCountViewConfig struct {
	BucketSeconds  int64 `json:"bucket_seconds"`
	IdleTTLSeconds int64 `json:"idle_ttl_seconds"`
}

// TestbenchLogCountViewStats separates storage cost from detector work.
// LogicalDetectorObservations counts virtual points returned by range/iterator
// reads, so the same point may be counted more than once when detectors reread
// a window.
type TestbenchLogCountViewStats struct {
	Config                      TestbenchLogCountViewConfig `json:"config"`
	RawStoredPoints             int                         `json:"raw_stored_points"`
	RawStoredSeries             int                         `json:"raw_stored_series"`
	LogicalDetectorObservations int64                       `json:"logical_detector_observations"`
	LogicalZeroObservations     int64                       `json:"logical_zero_observations"`
	PeakActiveSeries            int                         `json:"peak_active_series"`
	RetainedViewSeriesState     int                         `json:"retained_view_series_state"`
}

// timeAwareLogCountStorage decorates detector reads without changing shared
// storage. Only .count series produced by the configured log extractors are
// virtualized. Ordinary metrics retain missing-is-unknown semantics.
//
// A virtual point is emitted only when its fixed-width bucket is complete.
// Buckets are anchored at the series' first observed occurrence, so the view
// never manufactures pre-discovery zeros. An occurrence keeps a series active
// for IdleTTLSeconds; a later occurrence can causally reactivate it.
type timeAwareLogCountStorage struct {
	inner      observerdef.StorageReader
	raw        *timeSeriesStorage
	config     TestbenchLogCountViewConfig
	namespaces map[string]struct{}

	statsMu sync.Mutex
	stats   TestbenchLogCountViewStats
}

var _ observerdef.StorageReader = (*timeAwareLogCountStorage)(nil)
var _ seriesRefLister = (*timeAwareLogCountStorage)(nil)
var _ bulkStatusReader = (*timeAwareLogCountStorage)(nil)
var _ seriesAggregateSupport = (*timeAwareLogCountStorage)(nil)

type countViewInterval struct {
	firstEnd int64
	lastEnd  int64
}

type countViewData struct {
	namespace string
	name      string
	tags      []string
	anchor    int64
	values    map[int64]float64
	intervals []countViewInterval
}

func newTimeAwareLogCountStorage(
	inner observerdef.StorageReader,
	raw *timeSeriesStorage,
	config TestbenchLogCountViewConfig,
	namespaces []string,
) *timeAwareLogCountStorage {
	namespaceSet := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		namespaceSet[namespace] = struct{}{}
	}
	return &timeAwareLogCountStorage{
		inner:      inner,
		raw:        raw,
		config:     config,
		namespaces: namespaceSet,
		stats: TestbenchLogCountViewStats{
			Config: config,
			// The view computes directly from sparse storage and retains no
			// per-series cache or materialized zero points.
			RetainedViewSeriesState: 0,
		},
	}
}

func (s *timeAwareLogCountStorage) isLogEventCount(ref observerdef.SeriesRef) bool {
	meta := s.raw.GetSeriesMeta(ref)
	if meta == nil || !strings.HasSuffix(meta.Name, ".count") || s.raw.GetContext(ref) == nil {
		return false
	}
	_, ok := s.namespaces[meta.Namespace]
	return ok
}

func (s *timeAwareLogCountStorage) ListSeries(filter observerdef.SeriesFilter) []observerdef.SeriesMeta {
	return s.inner.ListSeries(filter)
}

func (s *timeAwareLogCountStorage) ListSeriesRefsInto(filter observerdef.SeriesFilter, dst []observerdef.SeriesRef) []observerdef.SeriesRef {
	return listSeriesRefs(s.inner, filter, dst)
}

func (s *timeAwareLogCountStorage) SupportsAggregate(ref observerdef.SeriesRef, agg observerdef.Aggregate) bool {
	return !s.isLogEventCount(ref) || agg == observerdef.AggregateAverage
}

func (s *timeAwareLogCountStorage) GetSeriesRange(
	ref observerdef.SeriesRef,
	start, end int64,
	agg observerdef.Aggregate,
) *observerdef.Series {
	if !s.isLogEventCount(ref) {
		return s.inner.GetSeriesRange(ref, start, end, agg)
	}
	// Each virtual point is already the occurrence count for one time window.
	// AggregateCount would only report "one virtual sample" and is meaningless.
	if agg != observerdef.AggregateAverage {
		return nil
	}

	data := s.buildData(ref, end)
	if data == nil {
		return nil
	}
	points := data.points(start, end, s.config.BucketSeconds)
	s.recordLogicalRead(points)
	return &observerdef.Series{
		Namespace: data.namespace,
		Name:      data.name,
		Tags:      data.tags,
		Points:    points,
	}
}

func (s *timeAwareLogCountStorage) ForEachPoint(
	ref observerdef.SeriesRef,
	start, end int64,
	agg observerdef.Aggregate,
	fn func(*observerdef.Series, observerdef.Point),
) bool {
	if !s.isLogEventCount(ref) {
		return s.inner.ForEachPoint(ref, start, end, agg, fn)
	}
	if agg != observerdef.AggregateAverage {
		return false
	}
	data := s.buildData(ref, end)
	if data == nil {
		return false
	}
	points := data.points(start, end, s.config.BucketSeconds)
	series := &observerdef.Series{
		Namespace: data.namespace,
		Name:      data.name,
		Tags:      data.tags,
	}
	for _, point := range points {
		fn(series, point)
	}
	s.recordLogicalRead(points)
	return true
}

func (s *timeAwareLogCountStorage) PointCount(ref observerdef.SeriesRef) int {
	if !s.isLogEventCount(ref) {
		return s.inner.PointCount(ref)
	}
	raw := s.inner.GetSeriesRange(ref, 0, int64(^uint64(0)>>1), observerdef.AggregateSum)
	if raw == nil || len(raw.Points) == 0 {
		return 0
	}
	end := raw.Points[len(raw.Points)-1].Timestamp + s.config.IdleTTLSeconds
	data := buildCountViewData(raw, end, s.config)
	return data.pointCount(0, end, s.config.BucketSeconds)
}

func (s *timeAwareLogCountStorage) PointCountUpTo(ref observerdef.SeriesRef, endTime int64) int {
	if !s.isLogEventCount(ref) {
		return s.inner.PointCountUpTo(ref, endTime)
	}
	data := s.buildData(ref, endTime)
	if data == nil {
		return 0
	}
	return data.pointCount(0, endTime, s.config.BucketSeconds)
}

func (s *timeAwareLogCountStorage) SumRange(
	ref observerdef.SeriesRef,
	start, end int64,
	agg observerdef.Aggregate,
) float64 {
	if !s.isLogEventCount(ref) {
		return s.inner.SumRange(ref, start, end, agg)
	}
	if agg != observerdef.AggregateAverage {
		return 0
	}
	data := s.buildData(ref, end)
	if data == nil {
		return 0
	}
	var total float64
	for _, point := range data.points(start, end, s.config.BucketSeconds) {
		total += point.Value
	}
	return total
}

func (s *timeAwareLogCountStorage) WriteGeneration(ref observerdef.SeriesRef) int64 {
	return s.inner.WriteGeneration(ref)
}

func (s *timeAwareLogCountStorage) SeriesGeneration() uint64 {
	return s.inner.SeriesGeneration()
}

func (s *timeAwareLogCountStorage) BulkSeriesStatus(refs []observerdef.SeriesRef, endTime int64) []seriesStatus {
	statuses := make([]seriesStatus, len(refs))
	for i, ref := range refs {
		if !s.isLogEventCount(ref) {
			statuses[i] = seriesStatus{
				pointCount:      s.inner.PointCountUpTo(ref, endTime),
				writeGeneration: s.inner.WriteGeneration(ref),
			}
			continue
		}
		count := s.PointCountUpTo(ref, endTime)
		// Closed virtual buckets never change once visible. The logical point
		// count is therefore a sufficient causal generation for detector gates,
		// even when the underlying sparse series received no new write.
		statuses[i] = seriesStatus{
			pointCount:      count,
			writeGeneration: int64(count),
		}
	}
	return statuses
}

func (s *timeAwareLogCountStorage) buildData(ref observerdef.SeriesRef, end int64) *countViewData {
	if s.config.BucketSeconds <= 0 || s.config.IdleTTLSeconds < 0 {
		return nil
	}
	raw := s.inner.GetSeriesRange(ref, 0, end, observerdef.AggregateSum)
	if raw == nil || len(raw.Points) == 0 {
		return nil
	}
	return buildCountViewData(raw, end, s.config)
}

func buildCountViewData(
	raw *observerdef.Series,
	end int64,
	config TestbenchLogCountViewConfig,
) *countViewData {
	if raw == nil || len(raw.Points) == 0 || config.BucketSeconds <= 0 {
		return nil
	}
	anchor := raw.Points[0].Timestamp
	data := &countViewData{
		namespace: raw.Namespace,
		name:      raw.Name,
		tags:      raw.Tags,
		anchor:    anchor,
		values:    make(map[int64]float64),
	}

	for _, point := range raw.Points {
		if point.Timestamp > end {
			break
		}
		bucketEnd := countViewBucketEnd(point.Timestamp, anchor, config.BucketSeconds)
		data.values[bucketEnd] += point.Value

		lastEnd := bucketEnd
		activeThrough := point.Timestamp + config.IdleTTLSeconds
		if activeThrough >= bucketEnd {
			lastEnd += ((activeThrough - bucketEnd) / config.BucketSeconds) * config.BucketSeconds
		}
		candidate := countViewInterval{firstEnd: bucketEnd, lastEnd: lastEnd}
		n := len(data.intervals)
		if n == 0 || candidate.firstEnd > data.intervals[n-1].lastEnd+config.BucketSeconds {
			data.intervals = append(data.intervals, candidate)
		} else if candidate.lastEnd > data.intervals[n-1].lastEnd {
			data.intervals[n-1].lastEnd = candidate.lastEnd
		}
	}
	return data
}

func countViewBucketEnd(timestamp, anchor, bucketSeconds int64) int64 {
	return anchor + ((timestamp-anchor)/bucketSeconds+1)*bucketSeconds - 1
}

func (d *countViewData) points(start, end, bucketSeconds int64) []observerdef.Point {
	if d == nil || bucketSeconds <= 0 || end <= start {
		return nil
	}
	var points []observerdef.Point
	for _, interval := range d.intervals {
		first := interval.firstEnd
		if first <= start {
			first += ((start-first)/bucketSeconds + 1) * bucketSeconds
		}
		last := min(interval.lastEnd, end)
		if first > last {
			continue
		}
		if points == nil {
			points = make([]observerdef.Point, 0, int((last-first)/bucketSeconds)+1)
		}
		for timestamp := first; timestamp <= last; timestamp += bucketSeconds {
			points = append(points, observerdef.Point{
				Timestamp: timestamp,
				Value:     d.values[timestamp],
			})
		}
	}
	return points
}

func (d *countViewData) pointCount(start, end, bucketSeconds int64) int {
	if d == nil || bucketSeconds <= 0 || end <= start {
		return 0
	}
	count := 0
	for _, interval := range d.intervals {
		first := interval.firstEnd
		if first <= start {
			first += ((start-first)/bucketSeconds + 1) * bucketSeconds
		}
		last := min(interval.lastEnd, end)
		if first <= last {
			count += int((last-first)/bucketSeconds) + 1
		}
	}
	return count
}

func (s *timeAwareLogCountStorage) recordLogicalRead(points []observerdef.Point) {
	var zeros int64
	for _, point := range points {
		if point.Value == 0 {
			zeros++
		}
	}
	s.statsMu.Lock()
	s.stats.LogicalDetectorObservations += int64(len(points))
	s.stats.LogicalZeroObservations += zeros
	s.statsMu.Unlock()
}

func (s *timeAwareLogCountStorage) beginDetect(end int64) {
	active := 0
	for _, meta := range s.inner.ListSeries(observerdef.WorkloadSeriesFilter()) {
		if !s.isLogEventCount(meta.Ref) {
			continue
		}
		raw := s.inner.GetSeriesRange(meta.Ref, 0, end, observerdef.AggregateSum)
		if raw == nil || len(raw.Points) == 0 {
			continue
		}
		last := raw.Points[len(raw.Points)-1].Timestamp
		if end <= last+s.config.IdleTTLSeconds {
			active++
		}
	}
	s.statsMu.Lock()
	if active > s.stats.PeakActiveSeries {
		s.stats.PeakActiveSeries = active
	}
	s.statsMu.Unlock()
}

func (s *timeAwareLogCountStorage) snapshotStats() TestbenchLogCountViewStats {
	s.statsMu.Lock()
	stats := s.stats
	s.statsMu.Unlock()

	metas := s.inner.ListSeries(observerdef.WorkloadSeriesFilter())
	for _, meta := range metas {
		if !s.isLogEventCount(meta.Ref) {
			continue
		}
		stats.RawStoredSeries++
		stats.RawStoredPoints += s.inner.PointCount(meta.Ref)
	}
	return stats
}
