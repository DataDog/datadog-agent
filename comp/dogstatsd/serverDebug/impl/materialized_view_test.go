// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverdebugimpl

import (
	"strconv"
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

	view.store(now, testDebugViewKey(1, "oldest"))
	view.store(now.Add(time.Second), testDebugViewKey(2, "middle"))
	view.store(now.Add(2*time.Second), testDebugViewKey(3, "newest"))

	snapshot := view.snapshot(now.Add(2 * time.Second))
	require.Len(t, snapshot, 2)
	assert.NotContains(t, snapshot, ckey.ContextKey(1))
	assert.Contains(t, snapshot, ckey.ContextKey(2))
	assert.Contains(t, snapshot, ckey.ContextKey(3))
}

func TestMilestone3DebugStatsViewExpiresStaleContexts(t *testing.T) {
	view := newDebugStatsView(1, 10, time.Second)
	now := time.Unix(100, 0)

	view.store(now, testDebugViewKey(1, "stale"))
	view.store(now, testDebugViewKey(2, "fresh"))

	snapshot := view.snapshot(now.Add(time.Second))
	require.Len(t, snapshot, 2, "entries at the TTL boundary are still retained")

	snapshot = view.snapshot(now.Add(time.Second + time.Nanosecond))
	require.Empty(t, snapshot, "snapshot prunes entries older than the TTL")
	assert.Zero(t, view.len())
}

func TestMilestone3DebugStatsViewResetsExpiredContextCount(t *testing.T) {
	view := newDebugStatsView(1, 10, time.Second)
	now := time.Unix(100, 0)
	key := testDebugViewKey(1, "reset")

	view.store(now, key)
	view.store(now.Add(500*time.Millisecond), key)
	view.store(now.Add(2*time.Second), key)

	snapshot := view.snapshot(now.Add(2 * time.Second))
	require.Len(t, snapshot, 1)
	assert.Equal(t, uint64(1), snapshot[key.Key].Count, "a sample after the TTL starts a fresh materialized-view row")
}

func TestMilestone3DebugStatsViewUsesShardLocalLocks(t *testing.T) {
	view := newDebugStatsView(2, 10, time.Hour)
	blockedShardKey := ckey.ContextKey(0)
	unblockedShardKey := ckey.ContextKey(1)

	blockedShard := view.shardForKey(blockedShardKey)
	blockedShard.Lock()
	defer blockedShard.Unlock()

	done := make(chan struct{})
	go func() {
		view.store(time.Unix(100, 0), testDebugViewKey(unblockedShardKey, "unblocked"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "storing into another shard should not wait on the blocked shard")
	}
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

func BenchmarkMilestone3StoreMetricStatsWithDebugViewKey(b *testing.B) {
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

	b.Run("parallel_precomputed_debug_view_key", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				context := contexts[i%len(contexts)]
				d.StoreMetricStatsWithDebugViewKey(metrics.MetricSample{}, context.DebugView)
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

func testDebugViewKey(key ckey.ContextKey, name string) identity.DebugViewKey {
	return identity.DebugViewKey{
		Client: identity.ClientSeriesIdentity{
			Name: name,
			Tags: []string{"env:test"},
		},
		Key:         key,
		DisplayTags: "env:test",
	}
}
