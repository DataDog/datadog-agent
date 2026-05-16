// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverdebugimpl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/seriesstats"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

const (
	defaultDebugStatsShardCount  = 32
	defaultDebugStatsMaxContexts = 65536
	defaultDebugStatsTTL         = 10 * time.Minute
	metricsCountWindow           = 5
)

type debugStatsView struct {
	seriesStore *seriesstats.Store
}

func newDefaultDebugStatsView() *debugStatsView {
	return newDebugStatsView(defaultDebugStatsShardCount, defaultDebugStatsMaxContexts, defaultDebugStatsTTL)
}

func newDebugStatsView(shardCount, maxContexts int, ttl time.Duration) *debugStatsView {
	return newDebugStatsViewWithTelemetry(shardCount, maxContexts, ttl, defaultDebugStatsViewTelemetry)
}

func newDebugStatsViewWithTelemetry(shardCount, maxContexts int, ttl time.Duration, telemetry seriesstats.Telemetry) *debugStatsView {
	return &debugStatsView{
		seriesStore: seriesstats.NewStore(seriesstats.Options{
			ShardCount:  shardCount,
			MaxContexts: maxContexts,
			TTL:         ttl,
			Telemetry:   telemetry,
		}),
	}
}

func (v *debugStatsView) store(now time.Time, debugViewKey identity.DebugViewKey) metricStat {
	stat := v.seriesStore.Observe(now, seriesstats.Point{
		Key:  debugViewKey.Key,
		Name: debugViewKey.Client.Name,
		Tags: debugViewKey.DisplayTags,
	})
	return metricStatFromSeriesStat(stat)
}

func (v *debugStatsView) snapshot(now time.Time) map[ckey.ContextKey]metricStat {
	seriesSnapshot := v.seriesStore.Snapshot(now)
	snapshot := make(map[ckey.ContextKey]metricStat, len(seriesSnapshot))
	for key, stat := range seriesSnapshot {
		snapshot[key] = metricStatFromSeriesStat(stat)
	}
	return snapshot
}

func (v *debugStatsView) len() int {
	return v.seriesStore.Len()
}

func metricStatFromSeriesStat(stat seriesstats.Stat) metricStat {
	return metricStat{
		Name:     stat.Name,
		Count:    stat.Count,
		LastSeen: stat.LastSeen,
		Tags:     stat.Tags,
	}
}

type metricsCountBuckets struct {
	shards []metricsCountShard
}

type metricsCountShard struct {
	sync.Mutex
	seconds [metricsCountWindow]int64
	counts  [metricsCountWindow]uint64
}

func newMetricsCountBuckets(shardCount int) metricsCountBuckets {
	if shardCount <= 0 {
		shardCount = 1
	}
	return metricsCountBuckets{shards: make([]metricsCountShard, shardCount)}
}

func (b *metricsCountBuckets) record(key ckey.ContextKey, now time.Time) {
	if len(b.shards) == 0 {
		return
	}
	shard := &b.shards[uint64(key)%uint64(len(b.shards))]
	shard.record(now.Truncate(time.Second).Unix())
}

func (b *metricsCountBuckets) hasSpikeAt(now time.Time) bool {
	counts := b.countsEndingAt(now)
	current := counts[0]

	var previous uint64
	for _, count := range counts[1:] {
		previous += count
	}

	return current > previous
}

func (b *metricsCountBuckets) countsEndingAt(now time.Time) [metricsCountWindow]uint64 {
	var counts [metricsCountWindow]uint64
	if len(b.shards) == 0 {
		return counts
	}

	currentSec := now.Truncate(time.Second).Unix()
	for i := range b.shards {
		shard := &b.shards[i]
		shard.Lock()
		for offset := 0; offset < metricsCountWindow; offset++ {
			sec := currentSec - int64(offset)
			idx := bucketIndex(sec)
			if shard.seconds[idx] == sec {
				counts[offset] += shard.counts[idx]
			}
		}
		shard.Unlock()
	}
	return counts
}

func (s *metricsCountShard) record(sec int64) {
	idx := bucketIndex(sec)
	s.Lock()
	defer s.Unlock()

	current := s.seconds[idx]
	if current > sec {
		return
	}
	if current != sec {
		s.seconds[idx] = sec
		s.counts[idx] = 0
	}
	s.counts[idx]++
}

func bucketIndex(sec int64) int {
	idx := sec % metricsCountWindow
	if idx < 0 {
		idx += metricsCountWindow
	}
	return int(idx)
}
