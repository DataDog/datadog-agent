// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverdebugimpl

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMilestone3DebugStatsViewEvictsOldestWhenBudgetExceeded(t *testing.T) {
	view := newDebugStatsView(1, 2, time.Hour)
	now := time.Unix(100, 0)

	view.store(now, testShardIdentity(1, "oldest"))
	view.store(now.Add(time.Second), testShardIdentity(2, "middle"))
	view.store(now.Add(2*time.Second), testShardIdentity(3, "newest"))

	snapshot := view.snapshot(now.Add(2 * time.Second))
	require.Len(t, snapshot, 2)
	assert.NotContains(t, snapshot, ckey.ContextKey(1))
	assert.Contains(t, snapshot, ckey.ContextKey(2))
	assert.Contains(t, snapshot, ckey.ContextKey(3))
}

func TestMilestone3DebugStatsViewExpiresStaleContexts(t *testing.T) {
	view := newDebugStatsView(1, 10, time.Second)
	now := time.Unix(100, 0)

	view.store(now, testShardIdentity(1, "stale"))
	view.store(now, testShardIdentity(2, "fresh"))

	snapshot := view.snapshot(now.Add(time.Second))
	require.Len(t, snapshot, 2, "entries at the TTL boundary are still retained")

	snapshot = view.snapshot(now.Add(time.Second + time.Nanosecond))
	require.Empty(t, snapshot, "snapshot prunes entries older than the TTL")
	assert.Zero(t, view.len())
}

func TestMilestone3DebugStatsViewResetsExpiredContextCount(t *testing.T) {
	view := newDebugStatsView(1, 10, time.Second)
	now := time.Unix(100, 0)
	key := testShardIdentity(1, "reset")

	view.store(now, key)
	view.store(now.Add(500*time.Millisecond), key)
	view.store(now.Add(2*time.Second), key)

	snapshot := view.snapshot(now.Add(2 * time.Second))
	require.Len(t, snapshot, 1)
	assert.Equal(t, uint64(1), snapshot[key.ContextKey].Count, "a sample after the TTL starts a fresh materialized-view row")
}

func TestMilestone3SpikeCountersUseTimeBucketsWithoutMetricChannel(t *testing.T) {
	buckets := newMetricsCountBuckets(2)
	start := time.Unix(100, 0)

	for i := 0; i < 10; i++ {
		buckets.record(ckey.ContextKey(i), start)
	}
	for i := 0; i < 3; i++ {
		buckets.record(ckey.ContextKey(i), start.Add(time.Second))
	}
	assert.False(t, buckets.hasSpikeAt(start.Add(time.Second)))

	for i := 0; i < 20; i++ {
		buckets.record(ckey.ContextKey(i), start.Add(2*time.Second))
	}
	assert.True(t, buckets.hasSpikeAt(start.Add(2*time.Second)))

	counts := buckets.countsEndingAt(start.Add(2 * time.Second))
	assert.Equal(t, uint64(20), counts[0])
	assert.Equal(t, uint64(3), counts[1])
	assert.Equal(t, uint64(10), counts[2])
}

func TestMilestone3bServerDebugComponentEnforcesContextBudget(t *testing.T) {
	debug := fulfillDeps(t, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.view = newDebugStatsViewWithTelemetry(1, 8, time.Hour, nil)
	d.metricsCounts = newMetricsCountBuckets(1)
	d.SetMetricStatsEnabled(true)
	defer d.SetMetricStatsEnabled(false)

	builder := identity.NewBuilder()
	for i := 0; i < 32; i++ {
		sample := metrics.MetricSample{
			Name: "bounded.metric",
			Tags: []string{"instance:" + strconv.Itoa(i)},
		}
		context := builder.ResolveHotPath(sample)
		d.StoreMetricStatsWithShardIdentity(context.Shard)
	}

	payload, err := d.GetJSONDebugStats()
	require.NoError(t, err)
	var stats map[ckey.ContextKey]metricStat
	require.NoError(t, json.Unmarshal(payload, &stats))
	assert.LessOrEqual(t, len(stats), 8)
	assert.LessOrEqual(t, d.view.len(), 8)
}

func TestMilestone3bDebugStatsViewTelemetryReportsBounds(t *testing.T) {
	telemetry := &recordingDebugStatsTelemetry{}
	view := newDebugStatsViewWithTelemetry(1, 2, time.Hour, telemetry)
	now := time.Unix(100, 0)

	view.store(now, testShardIdentity(1, "first"))
	view.store(now.Add(time.Nanosecond), testShardIdentity(2, "second"))
	view.store(now.Add(2*time.Nanosecond), testShardIdentity(3, "third"))

	telemetry.assert(t, recordingDebugStatsTelemetry{
		storedContexts:  2,
		budgetEvictions: 1,
	})

	snapshot := view.snapshot(now.Add(2 * time.Nanosecond))
	require.Len(t, snapshot, 2)
	telemetry.assert(t, recordingDebugStatsTelemetry{
		storedContexts:   2,
		budgetEvictions:  1,
		snapshots:        1,
		snapshotContexts: 2,
	})

	snapshot = view.snapshot(now.Add(time.Hour + 3*time.Nanosecond))
	require.Empty(t, snapshot)
	telemetry.assert(t, recordingDebugStatsTelemetry{
		storedContexts:   0,
		budgetEvictions:  1,
		ttlPrunes:        2,
		snapshots:        2,
		snapshotContexts: 0,
	})
}

func BenchmarkMilestone3StoreMetricStatsWithShardIdentity(b *testing.B) {
	contexts := make([]identity.HotPathContext, 8192)
	builder := identity.NewBuilder()
	for i := range contexts {
		sample := metrics.MetricSample{
			Name: "identity.metric",
			Tags: []string{"env:prod", "service:web", "instance:" + strconv.Itoa(i)},
		}
		contexts[i] = builder.ResolveHotPath(sample)
	}

	debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.SetMetricStatsEnabled(true)
	defer d.SetMetricStatsEnabled(false)

	b.Run("parallel_precomputed_shard_identity", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				context := contexts[i%len(contexts)]
				d.StoreMetricStatsWithShardIdentity(context.Shard)
				i++
			}
		})
	})

	b.Run("snapshot_high_cardinality", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := d.GetJSONDebugStats()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMilestone3bDebugStatsContention(b *testing.B) {
	contexts := make([]identity.HotPathContext, 8192)
	builder := identity.NewBuilder()
	for i := range contexts {
		sample := metrics.MetricSample{
			Name: "identity.metric",
			Tags: []string{"env:prod", "service:web", "instance:" + strconv.Itoa(i)},
		}
		contexts[i] = builder.ResolveHotPath(sample)
	}
	now := time.Unix(100, 0)

	b.Run("legacy_global_lock_unbuffered_spike_channel", func(b *testing.B) {
		legacy := newLegacyDebugStatsStore()
		defer legacy.stop()

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				legacy.store(now, contexts[i%len(contexts)].Shard)
				i++
			}
		})
	})

	b.Run("bounded_sharded_materialized_view", func(b *testing.B) {
		view := newDebugStatsViewWithTelemetry(32, defaultDebugStatsMaxContexts, time.Hour, nil)
		buckets := newMetricsCountBuckets(32)

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				shard := contexts[i%len(contexts)].Shard
				view.store(now, shard)
				buckets.record(shard.ContextKey, now)
				i++
			}
		})
	})
}

func testShardIdentity(key ckey.ContextKey, name string) identity.ShardIdentity {
	return identity.ShardIdentity{
		Client: identity.ClientSeriesIdentity{
			Name: name,
			Tags: []string{"env:test"},
		},
		ContextKey:  key,
		DisplayTags: "env:test",
	}
}

type recordingDebugStatsTelemetry struct {
	sync.Mutex
	storedContexts   int
	budgetEvictions  int
	ttlPrunes        int
	snapshots        int
	snapshotContexts int
}

func (t *recordingDebugStatsTelemetry) SetStoredContexts(count int) {
	t.Lock()
	defer t.Unlock()
	t.storedContexts = count
}

func (t *recordingDebugStatsTelemetry) IncBudgetEvictions() {
	t.Lock()
	defer t.Unlock()
	t.budgetEvictions++
}

func (t *recordingDebugStatsTelemetry) AddTTLPrunes(count int) {
	t.Lock()
	defer t.Unlock()
	t.ttlPrunes += count
}

func (t *recordingDebugStatsTelemetry) IncSnapshots() {
	t.Lock()
	defer t.Unlock()
	t.snapshots++
}

func (t *recordingDebugStatsTelemetry) SetSnapshotContexts(count int) {
	t.Lock()
	defer t.Unlock()
	t.snapshotContexts = count
}

func (t *recordingDebugStatsTelemetry) assert(tb testing.TB, expected recordingDebugStatsTelemetry) {
	tb.Helper()
	t.Lock()
	defer t.Unlock()
	assert.Equal(tb, expected.storedContexts, t.storedContexts, "stored contexts gauge")
	assert.Equal(tb, expected.budgetEvictions, t.budgetEvictions, "budget eviction counter")
	assert.Equal(tb, expected.ttlPrunes, t.ttlPrunes, "TTL prune counter")
	assert.Equal(tb, expected.snapshots, t.snapshots, "snapshot counter")
	assert.Equal(tb, expected.snapshotContexts, t.snapshotContexts, "snapshot contexts gauge")
}

type legacyDebugStatsStore struct {
	sync.Mutex
	stats      map[ckey.ContextKey]metricStat
	metricChan chan struct{}
	stopChan   chan struct{}
	done       chan struct{}
}

func newLegacyDebugStatsStore() *legacyDebugStatsStore {
	store := &legacyDebugStatsStore{
		stats:      make(map[ckey.ContextKey]metricStat),
		metricChan: make(chan struct{}),
		stopChan:   make(chan struct{}),
		done:       make(chan struct{}),
	}
	go store.runMetricsCountLoop()
	return store
}

func (s *legacyDebugStatsStore) store(now time.Time, shard identity.ShardIdentity) {
	s.Lock()
	stat := s.stats[shard.ContextKey]
	stat.Count++
	stat.LastSeen = now
	stat.Name = shard.Client.Name
	stat.Tags = shard.DisplayTags
	s.stats[shard.ContextKey] = stat
	s.Unlock()

	s.metricChan <- struct{}{}
}

func (s *legacyDebugStatsStore) runMetricsCountLoop() {
	defer close(s.done)
	for {
		select {
		case <-s.metricChan:
		case <-s.stopChan:
			return
		}
	}
}

func (s *legacyDebugStatsStore) stop() {
	close(s.stopChan)
	<-s.done
}
