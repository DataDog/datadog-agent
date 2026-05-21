// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookback provides bounded micro-bucket materialized views for recent
// DogStatsD activity.
package lookback

import (
	"errors"
	"sort"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

// GroupBy identifies one supported fixed-shape lookback aggregation.
type GroupBy string

const (
	// GroupByMetricName groups counts by metric name.
	GroupByMetricName GroupBy = "metric_name"
	// GroupBySeriesKey groups counts by the shared series key.
	GroupBySeriesKey GroupBy = "series_key"
	// GroupByListener groups counts by listener ID.
	GroupByListener GroupBy = "listener"
	// GroupByOrigin groups counts by origin/container ID.
	GroupByOrigin GroupBy = "origin"
)

var (
	// ErrUnsupportedGroupBy is returned for unknown fixed query shapes.
	ErrUnsupportedGroupBy = errors.New("unsupported lookback group-by")
)

// Options configures a Store.
type Options struct {
	ShardCount           int
	Window               time.Duration
	BucketWidth          time.Duration
	MaxContextsPerBucket int
	MaxResults           int
}

// Point is one DogStatsD metric observation for recent lookback views.
type Point struct {
	Key        ckey.ContextKey
	Name       string
	SeriesKey  string
	ListenerID string
	Origin     string
}

// Series identifies one retained series in recent lookback results.
type Series struct {
	Key       ckey.ContextKey
	Name      string
	SeriesKey string
}

// SeriesCount is a top-series query result.
type SeriesCount struct {
	Series
	Count         uint64
	RatePerSecond float64
}

// GroupCount is a grouped count/rate query result.
type GroupCount struct {
	Group         string
	Count         uint64
	RatePerSecond float64
}

// Stats describes boundedness counters for a Store.
type Stats struct {
	ShardCount           int
	BucketsPerShard      int
	BucketWidth          time.Duration
	Window               time.Duration
	MaxContextsPerBucket int
	MaxResults           int
	Dropped              uint64
}

// Store keeps recent DogStatsD counts in bounded, shard-local micro-bucket rings.
type Store struct {
	shards               []shard
	window               time.Duration
	bucketWidth          time.Duration
	maxContextsPerBucket int
	maxResults           int
	dropped              *atomic.Uint64
}

type shard struct {
	sync.Mutex
	buckets []bucket
}

type bucket struct {
	start       time.Time
	series      map[ckey.ContextKey]seriesBucketCount
	byName      map[string]uint64
	bySeriesKey map[string]uint64
	byListener  map[string]uint64
	byOrigin    map[string]uint64
}

type seriesBucketCount struct {
	series Series
	count  uint64
}

// NewStore returns a bounded recent lookback store.
func NewStore(opts Options) *Store {
	shardCount := opts.ShardCount
	if shardCount <= 0 {
		shardCount = 1
	}
	bucketWidth := opts.BucketWidth
	if bucketWidth <= 0 {
		bucketWidth = time.Second
	}
	window := opts.Window
	if window <= 0 {
		window = time.Minute
	}
	maxContextsPerBucket := opts.MaxContextsPerBucket
	if maxContextsPerBucket <= 0 {
		maxContextsPerBucket = 1024
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	bucketsPerShard := int(window/bucketWidth) + 1
	if window%bucketWidth != 0 {
		bucketsPerShard++
	}
	if bucketsPerShard < 1 {
		bucketsPerShard = 1
	}

	store := &Store{
		shards:               make([]shard, shardCount),
		window:               window,
		bucketWidth:          bucketWidth,
		maxContextsPerBucket: maxContextsPerBucket,
		maxResults:           maxResults,
		dropped:              atomic.NewUint64(0),
	}
	for i := range store.shards {
		store.shards[i].buckets = make([]bucket, bucketsPerShard)
		for j := range store.shards[i].buckets {
			store.shards[i].buckets[j].reset(time.Time{})
		}
	}
	return store
}

// Observe records one metric observation. It returns false when the current
// bucket is over budget and the point was dropped.
func (s *Store) Observe(now time.Time, point Point) bool {
	shard := s.shardForKey(point.Key)
	bucketStart := s.bucketStart(now)
	idx := s.bucketIndex(bucketStart, len(shard.buckets))

	shard.Lock()
	defer shard.Unlock()

	bucket := &shard.buckets[idx]
	if !bucket.start.Equal(bucketStart) {
		bucket.reset(bucketStart)
	}

	stat, exists := bucket.series[point.Key]
	if !exists && len(bucket.series) >= s.maxContextsPerBucket {
		s.dropped.Inc()
		return false
	}
	if !exists {
		stat.series = Series{
			Key:       point.Key,
			Name:      point.Name,
			SeriesKey: point.SeriesKey,
		}
	}
	stat.count++
	bucket.series[point.Key] = stat
	bucket.byName[point.Name]++
	bucket.bySeriesKey[point.SeriesKey]++
	bucket.byListener[point.ListenerID]++
	bucket.byOrigin[point.Origin]++
	return true
}

// TopSeries returns top series for a fixed recent window ending at now.
func (s *Store) TopSeries(now time.Time, window time.Duration, limit int) []SeriesCount {
	window = s.queryWindow(window)
	limit = s.queryLimit(limit)
	if limit <= 0 || window <= 0 {
		return nil
	}

	counts := make(map[ckey.ContextKey]seriesBucketCount)
	s.forEachRecentBucket(now, window, func(bucket *bucket) {
		for key, stat := range bucket.series {
			merged := counts[key]
			if merged.count == 0 {
				merged.series = stat.series
			}
			merged.count += stat.count
			counts[key] = merged
		}
	})

	results := make([]SeriesCount, 0, len(counts))
	for _, stat := range counts {
		results = append(results, SeriesCount{
			Series:        stat.series,
			Count:         stat.count,
			RatePerSecond: ratePerSecond(stat.count, window),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Count != results[j].Count {
			return results[i].Count > results[j].Count
		}
		if results[i].Name != results[j].Name {
			return results[i].Name < results[j].Name
		}
		if results[i].SeriesKey != results[j].SeriesKey {
			return results[i].SeriesKey < results[j].SeriesKey
		}
		return results[i].Key < results[j].Key
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// CountBy returns grouped counts for one supported fixed recent query shape.
func (s *Store) CountBy(now time.Time, window time.Duration, groupBy GroupBy, limit int) ([]GroupCount, error) {
	if !isSupportedGroupBy(groupBy) {
		return nil, ErrUnsupportedGroupBy
	}
	window = s.queryWindow(window)
	limit = s.queryLimit(limit)
	if limit <= 0 || window <= 0 {
		return nil, nil
	}

	counts := make(map[string]uint64)
	s.forEachRecentBucket(now, window, func(bucket *bucket) {
		var source map[string]uint64
		switch groupBy {
		case GroupByMetricName:
			source = bucket.byName
		case GroupBySeriesKey:
			source = bucket.bySeriesKey
		case GroupByListener:
			source = bucket.byListener
		case GroupByOrigin:
			source = bucket.byOrigin
		default:
			return
		}
		for group, count := range source {
			counts[group] += count
		}
	})

	results := make([]GroupCount, 0, len(counts))
	for group, count := range counts {
		results = append(results, GroupCount{
			Group:         group,
			Count:         count,
			RatePerSecond: ratePerSecond(count, window),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Count != results[j].Count {
			return results[i].Count > results[j].Count
		}
		return results[i].Group < results[j].Group
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Stats returns store boundedness counters.
func (s *Store) Stats() Stats {
	return Stats{
		ShardCount:           len(s.shards),
		BucketsPerShard:      len(s.shards[0].buckets),
		BucketWidth:          s.bucketWidth,
		Window:               s.window,
		MaxContextsPerBucket: s.maxContextsPerBucket,
		MaxResults:           s.maxResults,
		Dropped:              s.dropped.Load(),
	}
}

func (s *Store) forEachRecentBucket(now time.Time, window time.Duration, fn func(*bucket)) {
	oldest := s.bucketStart(now.Add(-window))
	newest := s.bucketStart(now)
	for i := range s.shards {
		shard := &s.shards[i]
		shard.Lock()
		for j := range shard.buckets {
			bucket := &shard.buckets[j]
			if bucket.start.IsZero() || bucket.start.Before(oldest) || bucket.start.After(newest) {
				continue
			}
			fn(bucket)
		}
		shard.Unlock()
	}
}

func (s *Store) queryWindow(window time.Duration) time.Duration {
	if window <= 0 || window > s.window {
		return s.window
	}
	return window
}

func (s *Store) queryLimit(limit int) int {
	if limit <= 0 || limit > s.maxResults {
		return s.maxResults
	}
	return limit
}

func (s *Store) shardForKey(key ckey.ContextKey) *shard {
	return &s.shards[uint64(key)%uint64(len(s.shards))]
}

func (s *Store) bucketStart(t time.Time) time.Time {
	return t.Truncate(s.bucketWidth)
}

func (s *Store) bucketIndex(bucketStart time.Time, bucketCount int) int {
	return int((bucketStart.UnixNano() / s.bucketWidth.Nanoseconds()) % int64(bucketCount))
}

func (b *bucket) reset(start time.Time) {
	b.start = start
	b.series = make(map[ckey.ContextKey]seriesBucketCount)
	b.byName = make(map[string]uint64)
	b.bySeriesKey = make(map[string]uint64)
	b.byListener = make(map[string]uint64)
	b.byOrigin = make(map[string]uint64)
}

func ratePerSecond(count uint64, window time.Duration) float64 {
	seconds := window.Seconds()
	if seconds <= 0 {
		return float64(count)
	}
	return float64(count) / seconds
}

func isSupportedGroupBy(groupBy GroupBy) bool {
	switch groupBy {
	case GroupByMetricName, GroupBySeriesKey, GroupByListener, GroupByOrigin:
		return true
	default:
		return false
	}
}
