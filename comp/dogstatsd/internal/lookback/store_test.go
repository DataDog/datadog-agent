// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookback

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

func TestMilestone6LookbackTopSeriesMatchesOfflineReference(t *testing.T) {
	store := NewStore(Options{ShardCount: 4, Window: 10 * time.Second, BucketWidth: time.Second, MaxContextsPerBucket: 10, MaxResults: 10})
	now := time.Unix(100, 0)
	points := []struct {
		at    time.Time
		point Point
	}{
		{now.Add(-9 * time.Second), testPoint(1, "alpha", "env:prod", "udp", "container-a")},
		{now.Add(-3 * time.Second), testPoint(1, "alpha", "env:prod", "udp", "container-a")},
		{now.Add(-2 * time.Second), testPoint(2, "beta", "env:dev", "uds", "container-b")},
		{now.Add(-1 * time.Second), testPoint(2, "beta", "env:dev", "uds", "container-b")},
		{now.Add(-1 * time.Second), testPoint(2, "beta", "env:dev", "udp", "container-b")},
		{now.Add(-11 * time.Second), testPoint(3, "expired", "env:old", "udp", "container-c")},
	}

	reference := offlineTopSeries(now, 10*time.Second, points)
	for _, point := range points {
		store.Observe(point.at, point.point)
	}

	actual := store.TopSeries(now, 10*time.Second, 10)
	require.Len(t, actual, len(reference))
	for i := range reference {
		assert.Equal(t, reference[i].Key, actual[i].Key)
		assert.Equal(t, reference[i].Name, actual[i].Name)
		assert.Equal(t, reference[i].DebugViewKey, actual[i].DebugViewKey)
		assert.Equal(t, reference[i].Count, actual[i].Count)
		assert.InDelta(t, reference[i].RatePerSecond, actual[i].RatePerSecond, 0.001)
	}
}

func TestMilestone6LookbackCountByFixedShapes(t *testing.T) {
	store := NewStore(Options{ShardCount: 2, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 10, MaxResults: 10})
	now := time.Unix(100, 0)

	store.Observe(now.Add(-5*time.Second), testPoint(1, "alpha", "alpha|env:prod", "udp", "container-a"))
	store.Observe(now.Add(-4*time.Second), testPoint(2, "alpha", "alpha|env:dev", "uds", "container-b"))
	store.Observe(now.Add(-3*time.Second), testPoint(2, "alpha", "alpha|env:dev", "uds", "container-b"))
	store.Observe(now.Add(-2*time.Second), testPoint(3, "beta", "beta|env:prod", "udp", "container-a"))

	byName, err := store.CountBy(now, 10*time.Second, GroupByMetricName, 10)
	require.NoError(t, err)
	assert.Equal(t, []GroupCount{
		{Group: "alpha", Count: 3, RatePerSecond: 0.3},
		{Group: "beta", Count: 1, RatePerSecond: 0.1},
	}, byName)

	byDebug, err := store.CountBy(now, 10*time.Second, GroupByDebugView, 10)
	require.NoError(t, err)
	assert.Equal(t, []GroupCount{
		{Group: "alpha|env:dev", Count: 2, RatePerSecond: 0.2},
		{Group: "alpha|env:prod", Count: 1, RatePerSecond: 0.1},
		{Group: "beta|env:prod", Count: 1, RatePerSecond: 0.1},
	}, byDebug)

	byListener, err := store.CountBy(now, 10*time.Second, GroupByListener, 10)
	require.NoError(t, err)
	assert.Equal(t, []GroupCount{
		{Group: "udp", Count: 2, RatePerSecond: 0.2},
		{Group: "uds", Count: 2, RatePerSecond: 0.2},
	}, byListener)

	byOrigin, err := store.CountBy(now, 10*time.Second, GroupByOrigin, 10)
	require.NoError(t, err)
	assert.Equal(t, []GroupCount{
		{Group: "container-a", Count: 2, RatePerSecond: 0.2},
		{Group: "container-b", Count: 2, RatePerSecond: 0.2},
	}, byOrigin)
}

func TestMilestone6LookbackEnforcesBucketBudget(t *testing.T) {
	store := NewStore(Options{ShardCount: 1, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 2, MaxResults: 10})
	now := time.Unix(100, 0)

	require.True(t, store.Observe(now, testPoint(1, "first", "first", "udp", "origin")))
	require.True(t, store.Observe(now, testPoint(2, "second", "second", "udp", "origin")))
	require.False(t, store.Observe(now, testPoint(3, "dropped", "dropped", "udp", "origin")))

	stats := store.Stats()
	assert.Equal(t, uint64(1), stats.Dropped)
	assert.Equal(t, 2, stats.MaxContextsPerBucket)

	top := store.TopSeries(now, time.Second, 10)
	require.Len(t, top, 2)
	assert.NotEqual(t, "dropped", top[0].Name)
	assert.NotEqual(t, "dropped", top[1].Name)
}

func TestMilestone6LookbackQueriesAreBoundedAndSafe(t *testing.T) {
	store := NewStore(Options{ShardCount: 1, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 10, MaxResults: 2})
	now := time.Unix(100, 0)
	for i := 0; i < 5; i++ {
		store.Observe(now, testPoint(ckey.ContextKey(i), fmt.Sprintf("metric-%d", i), fmt.Sprintf("debug-%d", i), "udp", "origin"))
	}

	top := store.TopSeries(now, time.Second, 0)
	require.Len(t, top, 2, "non-positive limits are capped by MaxResults")

	byName, err := store.CountBy(now, time.Second, GroupByMetricName, 100)
	require.NoError(t, err)
	require.Len(t, byName, 2, "large limits are capped by MaxResults")

	_, err = store.CountBy(now, time.Second, GroupBy("unsupported"), 10)
	assert.True(t, errors.Is(err, ErrUnsupportedGroupBy))
}

func TestMilestone6LookbackUsesShardLocalLocks(t *testing.T) {
	store := NewStore(Options{ShardCount: 2, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 10, MaxResults: 10})
	now := time.Unix(100, 0)
	blockedShard := store.shardForKey(ckey.ContextKey(0))
	blockedShard.Lock()
	defer blockedShard.Unlock()

	done := make(chan struct{})
	go func() {
		store.Observe(now, testPoint(1, "unblocked", "unblocked", "udp", "origin"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "storing into another shard should not wait on the blocked shard")
	}
}

func BenchmarkMilestone6LookbackObserve(b *testing.B) {
	store := NewStore(Options{ShardCount: 32, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 4096, MaxResults: 100})
	now := time.Unix(100, 0)
	points := make([]Point, 1024)
	for i := range points {
		points[i] = testPoint(ckey.ContextKey(i), fmt.Sprintf("metric-%d", i%32), fmt.Sprintf("metric-%d|env:%d", i%32, i%8), "udp", fmt.Sprintf("origin-%d", i%16))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Observe(now.Add(time.Duration(i%60)*time.Second), points[i%len(points)])
	}
}

func testPoint(key ckey.ContextKey, name string, debugViewKey string, listenerID string, origin string) Point {
	return Point{Key: key, Name: name, DebugViewKey: debugViewKey, ListenerID: listenerID, Origin: origin}
}

func offlineTopSeries(now time.Time, window time.Duration, points []struct {
	at    time.Time
	point Point
}) []SeriesCount {
	counts := make(map[ckey.ContextKey]seriesBucketCount)
	oldest := now.Add(-window)
	for _, point := range points {
		if point.at.Before(oldest) || point.at.After(now) {
			continue
		}
		stat := counts[point.point.Key]
		if stat.count == 0 {
			stat.series = Series{Key: point.point.Key, Name: point.point.Name, DebugViewKey: point.point.DebugViewKey}
		}
		stat.count++
		counts[point.point.Key] = stat
	}
	results := make([]SeriesCount, 0, len(counts))
	for _, stat := range counts {
		results = append(results, SeriesCount{Series: stat.series, Count: stat.count, RatePerSecond: float64(stat.count) / window.Seconds()})
	}
	sortSeriesCounts(results)
	return results
}

func sortSeriesCounts(results []SeriesCount) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if lessSeriesCount(results[j], results[i]) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func lessSeriesCount(left, right SeriesCount) bool {
	if left.Count != right.Count {
		return left.Count > right.Count
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.DebugViewKey != right.DebugViewKey {
		return left.DebugViewKey < right.DebugViewKey
	}
	return left.Key < right.Key
}
