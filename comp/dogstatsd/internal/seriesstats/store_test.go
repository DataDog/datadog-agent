// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package seriesstats

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

func TestMilestone4SeriesStatsStoreEvictsOldestWhenBudgetExceeded(t *testing.T) {
	store := NewStore(Options{ShardCount: 1, MaxContexts: 2, TTL: time.Hour})
	now := time.Unix(100, 0)

	store.Observe(now, testPoint(1, "oldest"))
	store.Observe(now.Add(time.Second), testPoint(2, "middle"))
	store.Observe(now.Add(2*time.Second), testPoint(3, "newest"))

	snapshot := store.Snapshot(now.Add(2 * time.Second))
	require.Len(t, snapshot, 2)
	assert.NotContains(t, snapshot, ckey.ContextKey(1))
	assert.Contains(t, snapshot, ckey.ContextKey(2))
	assert.Contains(t, snapshot, ckey.ContextKey(3))
}

func TestMilestone4SeriesStatsStoreExpiresStaleContexts(t *testing.T) {
	store := NewStore(Options{ShardCount: 1, MaxContexts: 10, TTL: time.Second})
	now := time.Unix(100, 0)

	store.Observe(now, testPoint(1, "stale"))
	store.Observe(now, testPoint(2, "fresh"))

	snapshot := store.Snapshot(now.Add(time.Second))
	require.Len(t, snapshot, 2, "entries at the TTL boundary are still retained")

	snapshot = store.Snapshot(now.Add(time.Second + time.Nanosecond))
	require.Empty(t, snapshot, "snapshot prunes entries older than the TTL")
	assert.Zero(t, store.Len())
}

func TestMilestone4SeriesStatsStoreResetsExpiredContextCount(t *testing.T) {
	store := NewStore(Options{ShardCount: 1, MaxContexts: 10, TTL: time.Second})
	now := time.Unix(100, 0)
	point := testPoint(1, "reset")

	store.Observe(now, point)
	store.Observe(now.Add(500*time.Millisecond), point)
	stat := store.Observe(now.Add(2*time.Second), point)

	assert.Equal(t, uint64(1), stat.Count, "a sample after the TTL starts a fresh materialized-view row")
	assert.Equal(t, now.Add(2*time.Second), stat.FirstSeen)
}

func TestMilestone4SeriesStatsStoreUsesShardLocalLocks(t *testing.T) {
	store := NewStore(Options{ShardCount: 2, MaxContexts: 10, TTL: time.Hour})
	blockedShardKey := ckey.ContextKey(0)
	unblockedShardKey := ckey.ContextKey(1)

	blockedShard := store.shardForKey(blockedShardKey)
	blockedShard.Lock()
	defer blockedShard.Unlock()

	done := make(chan struct{})
	go func() {
		store.Observe(time.Unix(100, 0), testPoint(unblockedShardKey, "unblocked"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "storing into another shard should not wait on the blocked shard")
	}
}

func TestMilestone4SeriesStatsStoreTopIsDeterministic(t *testing.T) {
	store := NewStore(Options{ShardCount: 4, MaxContexts: 10, TTL: time.Hour})
	now := time.Unix(100, 0)

	for i := 0; i < 5; i++ {
		store.Observe(now.Add(time.Duration(i)*time.Millisecond), testPoint(1, "alpha"))
	}
	for i := 0; i < 3; i++ {
		store.Observe(now.Add(time.Duration(i)*time.Millisecond), testPoint(2, "beta"))
	}
	for i := 0; i < 3; i++ {
		store.Observe(now.Add(time.Duration(i)*time.Millisecond), testPoint(3, "aardvark"))
	}

	top := store.Top(now.Add(time.Second), 2)
	require.Len(t, top, 2)
	assert.Equal(t, "alpha", top[0].Name)
	assert.Equal(t, uint64(5), top[0].Count)
	assert.Equal(t, "aardvark", top[1].Name, "ties are resolved by stable display fields")
	assert.Equal(t, uint64(3), top[1].Count)
}

func TestMilestone4SeriesStatsStoreTopWithRatesSummarizesRetainedRows(t *testing.T) {
	store := NewStore(Options{ShardCount: 2, MaxContexts: 10, TTL: time.Hour})
	now := time.Unix(100, 0)
	point := testPoint(1, "rate")

	store.Observe(now, point)
	store.Observe(now.Add(2*time.Second), point)
	store.Observe(now.Add(4*time.Second), point)

	top := store.TopWithRates(now.Add(6*time.Second), 1)
	require.Len(t, top, 1)
	assert.Equal(t, "rate", top[0].Name)
	assert.Equal(t, uint64(3), top[0].Count)
	assert.InDelta(t, 0.5, top[0].RatePerSecond, 0.001)
}

func TestMilestone4SeriesStatsStoreTelemetryReportsBounds(t *testing.T) {
	telemetry := &recordingTelemetry{}
	store := NewStore(Options{ShardCount: 1, MaxContexts: 2, TTL: time.Hour, Telemetry: telemetry})
	now := time.Unix(100, 0)

	store.Observe(now, testPoint(1, "first"))
	store.Observe(now.Add(time.Nanosecond), testPoint(2, "second"))
	store.Observe(now.Add(2*time.Nanosecond), testPoint(3, "third"))

	telemetry.assert(t, recordingTelemetry{
		storedContexts:  2,
		budgetEvictions: 1,
	})

	snapshot := store.Snapshot(now.Add(2 * time.Nanosecond))
	require.Len(t, snapshot, 2)
	telemetry.assert(t, recordingTelemetry{
		storedContexts:   2,
		budgetEvictions:  1,
		snapshots:        1,
		snapshotContexts: 2,
	})

	snapshot = store.Snapshot(now.Add(time.Hour + 3*time.Nanosecond))
	require.Empty(t, snapshot)
	telemetry.assert(t, recordingTelemetry{
		storedContexts:   0,
		budgetEvictions:  1,
		ttlPrunes:        2,
		snapshots:        2,
		snapshotContexts: 0,
	})
}

func BenchmarkMilestone4SeriesStatsStoreObserve(b *testing.B) {
	points := make([]Point, 8192)
	for i := range points {
		points[i] = testPoint(ckey.ContextKey(i), "identity.metric."+strconv.Itoa(i))
	}
	store := NewStore(Options{ShardCount: 32, MaxContexts: 65536, TTL: time.Hour})
	now := time.Unix(100, 0)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			store.Observe(now, points[i%len(points)])
			i++
		}
	})
}

func testPoint(key ckey.ContextKey, name string) Point {
	return Point{
		Key:  key,
		Name: name,
		Tags: "env:test",
	}
}

type recordingTelemetry struct {
	sync.Mutex
	storedContexts   int
	budgetEvictions  int
	ttlPrunes        int
	snapshots        int
	snapshotContexts int
}

func (t *recordingTelemetry) SetStoredContexts(count int) {
	t.Lock()
	defer t.Unlock()
	t.storedContexts = count
}

func (t *recordingTelemetry) IncBudgetEvictions() {
	t.Lock()
	defer t.Unlock()
	t.budgetEvictions++
}

func (t *recordingTelemetry) AddTTLPrunes(count int) {
	t.Lock()
	defer t.Unlock()
	t.ttlPrunes += count
}

func (t *recordingTelemetry) IncSnapshots() {
	t.Lock()
	defer t.Unlock()
	t.snapshots++
}

func (t *recordingTelemetry) SetSnapshotContexts(count int) {
	t.Lock()
	defer t.Unlock()
	t.snapshotContexts = count
}

func (t *recordingTelemetry) assert(tb testing.TB, expected recordingTelemetry) {
	tb.Helper()
	t.Lock()
	defer t.Unlock()
	assert.Equal(tb, expected.storedContexts, t.storedContexts, "stored contexts gauge")
	assert.Equal(tb, expected.budgetEvictions, t.budgetEvictions, "budget eviction counter")
	assert.Equal(tb, expected.ttlPrunes, t.ttlPrunes, "TTL prune counter")
	assert.Equal(tb, expected.snapshots, t.snapshots, "snapshot counter")
	assert.Equal(tb, expected.snapshotContexts, t.snapshotContexts, "snapshot contexts gauge")
}
