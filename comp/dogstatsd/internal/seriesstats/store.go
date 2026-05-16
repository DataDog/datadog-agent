// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seriesstats provides bounded, shard-local materialized views of
// DogStatsD series activity.
package seriesstats

import (
	"sort"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

// Point identifies one series update in a materialized stats view.
type Point struct {
	Key  ckey.ContextKey
	Name string
	Tags string
}

// Stat is the materialized state retained for one series.
type Stat struct {
	Name      string
	Count     uint64
	FirstSeen time.Time
	LastSeen  time.Time
	Tags      string
}

// StatSummary is a query-friendly view of one retained series.
type StatSummary struct {
	Stat
	RatePerSecond float64
}

// Options configures a Store.
type Options struct {
	ShardCount  int
	MaxContexts int
	TTL         time.Duration
	Telemetry   Telemetry
}

// Telemetry observes Store boundedness and snapshot activity.
type Telemetry interface {
	SetStoredContexts(count int)
	IncBudgetEvictions()
	AddTTLPrunes(count int)
	IncSnapshots()
	SetSnapshotContexts(count int)
}

// Store is a bounded, shard-local materialized view of series activity.
type Store struct {
	shards              []shard
	maxContextsPerShard int
	ttl                 time.Duration
	storedContexts      *atomic.Int64
	telemetry           Telemetry
}

type shard struct {
	sync.Mutex
	stats map[ckey.ContextKey]Stat
}

// NewStore returns a bounded series stats store.
func NewStore(opts Options) *Store {
	shardCount := opts.ShardCount
	if shardCount <= 0 {
		shardCount = 1
	}
	maxContexts := opts.MaxContexts
	if maxContexts <= 0 {
		maxContexts = shardCount
	}

	maxContextsPerShard := (maxContexts + shardCount - 1) / shardCount
	if maxContextsPerShard <= 0 {
		maxContextsPerShard = 1
	}

	store := &Store{
		shards:              make([]shard, shardCount),
		maxContextsPerShard: maxContextsPerShard,
		ttl:                 opts.TTL,
		storedContexts:      atomic.NewInt64(0),
		telemetry:           opts.Telemetry,
	}
	for i := range store.shards {
		store.shards[i].stats = make(map[ckey.ContextKey]Stat)
	}
	store.setStoredContextsTelemetry()
	return store
}

// Observe records one series point and returns the updated materialized stat.
func (s *Store) Observe(now time.Time, point Point) Stat {
	shard := s.shardForKey(point.Key)
	shard.Lock()
	defer shard.Unlock()

	stat, exists := shard.stats[point.Key]
	if exists && s.isExpired(now, stat) {
		stat = Stat{}
	}

	if !exists && len(shard.stats) >= s.maxContextsPerShard {
		s.recordTTLPrunes(shard.pruneExpiredLocked(now, s.ttl))
		if len(shard.stats) >= s.maxContextsPerShard && shard.evictOldestLocked() {
			s.storedContexts.Dec()
			if s.telemetry != nil {
				s.telemetry.IncBudgetEvictions()
			}
		}
	}

	if stat.Count == 0 {
		stat.FirstSeen = now
	}
	stat.Count++
	stat.LastSeen = now
	stat.Name = point.Name
	stat.Tags = point.Tags
	shard.stats[point.Key] = stat
	if !exists {
		s.storedContexts.Inc()
		s.setStoredContextsTelemetry()
	}
	return stat
}

// Snapshot returns a merged copy of all currently retained series stats.
func (s *Store) Snapshot(now time.Time) map[ckey.ContextKey]Stat {
	snapshot := make(map[ckey.ContextKey]Stat)
	for i := range s.shards {
		shard := &s.shards[i]
		shard.Lock()
		s.recordTTLPrunes(shard.pruneExpiredLocked(now, s.ttl))
		for key, stat := range shard.stats {
			snapshot[key] = stat
		}
		shard.Unlock()
	}
	if s.telemetry != nil {
		s.telemetry.IncSnapshots()
		s.telemetry.SetSnapshotContexts(len(snapshot))
	}
	s.setStoredContextsTelemetry()
	return snapshot
}

// Top returns up to limit currently retained series, ordered by count descending
// and then by stable display fields.
func (s *Store) Top(now time.Time, limit int) []Stat {
	stats := s.sortedSnapshot(now, limit)
	return stats
}

// TopWithRates returns top series with average per-second rates over each
// retained row's observed lifetime.
func (s *Store) TopWithRates(now time.Time, limit int) []StatSummary {
	stats := s.sortedSnapshot(now, limit)
	summaries := make([]StatSummary, 0, len(stats))
	for _, stat := range stats {
		summaries = append(summaries, StatSummary{
			Stat:          stat,
			RatePerSecond: ratePerSecond(now, stat),
		})
	}
	return summaries
}

func (s *Store) sortedSnapshot(now time.Time, limit int) []Stat {
	if limit <= 0 {
		return nil
	}
	snapshot := s.Snapshot(now)
	stats := make([]Stat, 0, len(snapshot))
	for _, stat := range snapshot {
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		if stats[i].Name != stats[j].Name {
			return stats[i].Name < stats[j].Name
		}
		if stats[i].Tags != stats[j].Tags {
			return stats[i].Tags < stats[j].Tags
		}
		return stats[i].LastSeen.Before(stats[j].LastSeen)
	})
	if len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}

func ratePerSecond(now time.Time, stat Stat) float64 {
	elapsed := now.Sub(stat.FirstSeen).Seconds()
	if elapsed <= 0 {
		return float64(stat.Count)
	}
	return float64(stat.Count) / elapsed
}

// Len returns the current number of retained series contexts.
func (s *Store) Len() int {
	var total int
	for i := range s.shards {
		shard := &s.shards[i]
		shard.Lock()
		total += len(shard.stats)
		shard.Unlock()
	}
	return total
}

func (s *Store) shardForKey(key ckey.ContextKey) *shard {
	return &s.shards[uint64(key)%uint64(len(s.shards))]
}

func (s *Store) isExpired(now time.Time, stat Stat) bool {
	return s.ttl > 0 && now.Sub(stat.LastSeen) > s.ttl
}

func (s *Store) recordTTLPrunes(count int) {
	if count <= 0 {
		return
	}
	s.storedContexts.Sub(int64(count))
	if s.telemetry != nil {
		s.telemetry.AddTTLPrunes(count)
	}
}

func (s *Store) setStoredContextsTelemetry() {
	if s.telemetry != nil {
		s.telemetry.SetStoredContexts(int(s.storedContexts.Load()))
	}
}

func (s *shard) pruneExpiredLocked(now time.Time, ttl time.Duration) int {
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

func (s *shard) evictOldestLocked() bool {
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
