// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverdebugimpl

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

const (
	defaultDebugStatsShardCount  = 32
	defaultDebugStatsMaxContexts = 65536
	defaultDebugStatsTTL         = 10 * time.Minute
	metricsCountWindow           = 5
)

type debugStatsView struct {
	shards              []debugStatsShard
	maxContextsPerShard int
	ttl                 time.Duration
	storedContexts      *atomic.Int64
	telemetry           debugStatsViewTelemetry
}

type debugStatsShard struct {
	sync.Mutex
	stats map[ckey.ContextKey]metricStat
}

func newDefaultDebugStatsView() *debugStatsView {
	return newDebugStatsView(defaultDebugStatsShardCount, defaultDebugStatsMaxContexts, defaultDebugStatsTTL)
}

func newDebugStatsView(shardCount, maxContexts int, ttl time.Duration) *debugStatsView {
	return newDebugStatsViewWithTelemetry(shardCount, maxContexts, ttl, defaultDebugStatsViewTelemetry)
}

func newDebugStatsViewWithTelemetry(shardCount, maxContexts int, ttl time.Duration, telemetry debugStatsViewTelemetry) *debugStatsView {
	if shardCount <= 0 {
		shardCount = 1
	}
	if maxContexts <= 0 {
		maxContexts = shardCount
	}

	maxContextsPerShard := (maxContexts + shardCount - 1) / shardCount
	if maxContextsPerShard <= 0 {
		maxContextsPerShard = 1
	}

	view := &debugStatsView{
		shards:              make([]debugStatsShard, shardCount),
		maxContextsPerShard: maxContextsPerShard,
		ttl:                 ttl,
		storedContexts:      atomic.NewInt64(0),
		telemetry:           telemetry,
	}
	for i := range view.shards {
		view.shards[i].stats = make(map[ckey.ContextKey]metricStat)
	}
	view.setStoredContextsTelemetry()
	return view
}

func (v *debugStatsView) store(now time.Time, debugViewKey identity.DebugViewKey) metricStat {
	shard := v.shardForKey(debugViewKey.Key)
	shard.Lock()
	defer shard.Unlock()

	stat, exists := shard.stats[debugViewKey.Key]
	if exists && v.isExpired(now, stat) {
		stat = metricStat{}
	}

	if !exists && len(shard.stats) >= v.maxContextsPerShard {
		v.recordTTLPrunes(shard.pruneExpiredLocked(now, v.ttl))
		if len(shard.stats) >= v.maxContextsPerShard && shard.evictOldestLocked() {
			v.storedContexts.Dec()
			if v.telemetry != nil {
				v.telemetry.incBudgetEvictions()
			}
		}
	}

	stat.Count++
	stat.LastSeen = now
	stat.Name = debugViewKey.Client.Name
	stat.Tags = debugViewKey.DisplayTags
	shard.stats[debugViewKey.Key] = stat
	if !exists {
		v.storedContexts.Inc()
		v.setStoredContextsTelemetry()
	}
	return stat
}

func (v *debugStatsView) snapshot(now time.Time) map[ckey.ContextKey]metricStat {
	snapshot := make(map[ckey.ContextKey]metricStat)
	for i := range v.shards {
		shard := &v.shards[i]
		shard.Lock()
		v.recordTTLPrunes(shard.pruneExpiredLocked(now, v.ttl))
		for key, stat := range shard.stats {
			snapshot[key] = stat
		}
		shard.Unlock()
	}
	if v.telemetry != nil {
		v.telemetry.incSnapshots()
		v.telemetry.setSnapshotContexts(len(snapshot))
	}
	v.setStoredContextsTelemetry()
	return snapshot
}

func (v *debugStatsView) len() int {
	var total int
	for i := range v.shards {
		shard := &v.shards[i]
		shard.Lock()
		total += len(shard.stats)
		shard.Unlock()
	}
	return total
}

func (v *debugStatsView) shardForKey(key ckey.ContextKey) *debugStatsShard {
	return &v.shards[uint64(key)%uint64(len(v.shards))]
}

func (v *debugStatsView) isExpired(now time.Time, stat metricStat) bool {
	return v.ttl > 0 && now.Sub(stat.LastSeen) > v.ttl
}

func (v *debugStatsView) recordTTLPrunes(count int) {
	if count <= 0 {
		return
	}
	v.storedContexts.Sub(int64(count))
	if v.telemetry != nil {
		v.telemetry.addTTLPrunes(count)
	}
}

func (v *debugStatsView) setStoredContextsTelemetry() {
	if v.telemetry != nil {
		v.telemetry.setStoredContexts(int(v.storedContexts.Load()))
	}
}

func (s *debugStatsShard) pruneExpiredLocked(now time.Time, ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}
	var pruned int
	for key, stat := range s.stats {
		if now.Sub(stat.LastSeen) > ttl {
			delete(s.stats, key)
			pruned++
		}
	}
	return pruned
}

func (s *debugStatsShard) evictOldestLocked() bool {
	var oldestKey ckey.ContextKey
	var oldestSeen time.Time
	first := true
	for key, stat := range s.stats {
		if first || stat.LastSeen.Before(oldestSeen) {
			oldestKey = key
			oldestSeen = stat.LastSeen
			first = false
		}
	}
	if first {
		return false
	}
	delete(s.stats, oldestKey)
	return true
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
