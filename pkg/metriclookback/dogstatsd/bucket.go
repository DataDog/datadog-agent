// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

const (
	// DefaultDogStatsDBucketWidth is the initial short-resolution bucket width for
	// normal DogStatsD lookback materialization. It is an internal default so the
	// code is not architecturally tied to exactly one second.
	DefaultDogStatsDBucketWidth = time.Second
	// DefaultDogStatsDSealDelay is the internal delay before open normal-DogStatsD
	// lookback buckets are sealed into the retention ring.
	DefaultDogStatsDSealDelay = 10 * time.Second
	// DefaultDogStatsDMaterializerShardCount is the number of independent locks
	// used by the selected DogStatsD bucket materializer.
	DefaultDogStatsDMaterializerShardCount = 16
)

var (
	tlmDogStatsDBucketSamples  = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "dogstatsd_bucket_samples", []string{"state"}, "Count of selected normal DogStatsD samples handled by the metric lookback bucket materializer")
	tlmDogStatsDBucketSealed   = telemetryimpl.GetCompatComponent().NewSimpleCounter("metric_lookback", "dogstatsd_bucket_sealed", "Count of normal DogStatsD lookback buckets sealed into the retention ring")
	tlmDogStatsDBucketPoints   = telemetryimpl.GetCompatComponent().NewSimpleCounter("metric_lookback", "dogstatsd_bucket_points", "Count of normal DogStatsD lookback scalar points sealed into the retention ring")
	tlmDogStatsDBucketSketches = telemetryimpl.GetCompatComponent().NewSimpleCounter("metric_lookback", "dogstatsd_bucket_sketches", "Count of normal DogStatsD lookback sketch points sealed into the retention ring")
)

// DogStatsDBucketMaterializerOptions controls selected normal-DogStatsD bucket
// materialization. These are intentionally code-level options for now; the
// public config only enables DogStatsD lookback and selects metric names.
type DogStatsDBucketMaterializerOptions struct {
	// BucketWidth controls the lookback materialization resolution. It defaults to
	// DefaultDogStatsDBucketWidth and is normalized to a positive whole number of
	// seconds because metrics.Serie.Interval is second-granularity.
	BucketWidth time.Duration
	// SealDelay delays sealing recent buckets so in-flight samples have time to
	// arrive. It defaults to DefaultDogStatsDSealDelay. Set it to a negative value
	// in tests to disable the delay.
	SealDelay time.Duration
	// ShardCount controls materializer lock sharding. It defaults to
	// DefaultDogStatsDMaterializerShardCount.
	ShardCount int
	// ContextExpiry controls how long selected series descriptors stay available
	// after their last real sample. It defaults to dogstatsd_context_expiry_seconds.
	ContextExpiry time.Duration
	// CounterExpiry controls how long selected counters keep emitting zero buckets
	// after their last real sample. It defaults to dogstatsd_expiry_seconds.
	CounterExpiry time.Duration
	// Monitor is updated after sealed points have been appended to retention.
	Monitor *monitor.Watcher
}

// DogStatsDBucketMaterializer maintains a selected short-width materialized view
// of normal DogStatsD samples. Open buckets are mutable aggregation/sketch state;
// only sealed series and sketch series are appended to the shared retention ring.
type DogStatsDBucketMaterializer struct {
	retention *metriclookback.Retention
	monitor   *monitor.Watcher

	bucketWidth        time.Duration
	bucketWidthSeconds int64
	sealDelay          time.Duration
	contextExpiry      time.Duration
	counterExpiry      time.Duration

	shards []dogStatsDBucketShard
}

type dogStatsDBucketShard struct {
	mu sync.Mutex

	initialized    bool
	nextSealBucket int64
	buckets        map[int64]metrics.ContextMetrics
	sketchBuckets  map[int64]map[ckey.ContextKey]*quantile.Agent
	descriptors    map[ckey.ContextKey]dogStatsDDescriptor
}

type dogStatsDDescriptor struct {
	name     string
	host     string
	tags     []string
	mtype    metrics.MetricType
	noIndex  bool
	source   metrics.MetricSource
	lastSeen int64
}

// NewDogStatsDBucketMaterializer creates a selected normal-DogStatsD bucket
// materializer. It returns nil when retention is nil.
func NewDogStatsDBucketMaterializer(retention *metriclookback.Retention, opts DogStatsDBucketMaterializerOptions) *DogStatsDBucketMaterializer {
	if retention == nil {
		return nil
	}

	bucketWidth := normalizeDogStatsDBucketWidth(opts.BucketWidth)
	shardCount := opts.ShardCount
	if shardCount <= 0 {
		shardCount = DefaultDogStatsDMaterializerShardCount
	}
	contextExpiry := opts.ContextExpiry
	if contextExpiry <= 0 {
		contextExpiry = time.Duration(pkgconfigsetup.Datadog().GetInt64("dogstatsd_context_expiry_seconds")) * time.Second
	}
	counterExpiry := opts.CounterExpiry
	if counterExpiry <= 0 {
		counterExpiry = time.Duration(pkgconfigsetup.Datadog().GetInt64("dogstatsd_expiry_seconds")) * time.Second
	}
	sealDelay := opts.SealDelay
	if sealDelay == 0 {
		sealDelay = DefaultDogStatsDSealDelay
	}
	if sealDelay < 0 {
		sealDelay = 0
	}

	m := &DogStatsDBucketMaterializer{
		retention:          retention,
		monitor:            opts.Monitor,
		bucketWidth:        bucketWidth,
		bucketWidthSeconds: int64(bucketWidth / time.Second),
		sealDelay:          sealDelay,
		contextExpiry:      contextExpiry,
		counterExpiry:      counterExpiry,
		shards:             make([]dogStatsDBucketShard, shardCount),
	}
	for i := range m.shards {
		m.shards[i].buckets = make(map[int64]metrics.ContextMetrics)
		m.shards[i].sketchBuckets = make(map[int64]map[ckey.ContextKey]*quantile.Agent)
		m.shards[i].descriptors = make(map[ckey.ContextKey]dogStatsDDescriptor)
	}
	return m
}

func normalizeDogStatsDBucketWidth(width time.Duration) time.Duration {
	if width <= 0 {
		return DefaultDogStatsDBucketWidth
	}
	if width < time.Second {
		return time.Second
	}
	if remainder := width % time.Second; remainder != 0 {
		width += time.Second - remainder
	}
	return width
}

// Observe updates the open lookback bucket for a selected normal DogStatsD
// sample using the effective series context resolved by the TimeSampler.
func (m *DogStatsDBucketMaterializer) Observe(sample *metrics.MetricSample, timestamp float64, dogctx aggregator.DogStatsDLookbackContext) {
	if m == nil || sample == nil {
		return
	}
	if sample.Mtype != metrics.GaugeType && sample.Mtype != metrics.CounterType && sample.Mtype != metrics.DistributionType {
		tlmDogStatsDBucketSamples.Inc("unsupported_type")
		return
	}

	bucketStart := m.bucketStart(timestamp)
	shard := &m.shards[m.shardIndex(dogctx.ContextKey)]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.initialized && bucketStart < shard.nextSealBucket {
		tlmDogStatsDBucketSamples.Inc("late_drop")
		return
	}
	if !shard.initialized {
		shard.initialized = true
		shard.nextSealBucket = bucketStart
	}

	desc, found := shard.descriptors[dogctx.ContextKey]
	if !found {
		desc = dogStatsDDescriptor{
			name:    dogctx.Name,
			host:    dogctx.Host,
			tags:    append([]string(nil), dogctx.Tags...),
			mtype:   sample.Mtype,
			noIndex: dogctx.NoIndex,
			source:  dogctx.Source,
		}
	}
	desc.lastSeen = int64(timestamp)
	shard.descriptors[dogctx.ContextKey] = desc

	if sample.Mtype == metrics.DistributionType {
		if math.IsInf(sample.Value, 0) || math.IsNaN(sample.Value) {
			tlmDogStatsDBucketSamples.Inc("error")
			return
		}
		bucketSketches := shard.sketchBuckets[bucketStart]
		if bucketSketches == nil {
			bucketSketches = make(map[ckey.ContextKey]*quantile.Agent)
			shard.sketchBuckets[bucketStart] = bucketSketches
		}
		agent := bucketSketches[dogctx.ContextKey]
		if agent == nil {
			agent = &quantile.Agent{}
			bucketSketches[dogctx.ContextKey] = agent
		}
		agent.Insert(sample.Value, sample.SampleRate)
		tlmDogStatsDBucketSamples.Inc("accepted")
		return
	}

	bucketMetrics := shard.buckets[bucketStart]
	if bucketMetrics == nil {
		bucketMetrics = metrics.MakeContextMetrics()
		shard.buckets[bucketStart] = bucketMetrics
	}
	if err := bucketMetrics.AddSample(dogctx.ContextKey, sample, timestamp, m.bucketWidthSeconds, nil, pkgconfigsetup.Datadog()); err != nil {
		tlmDogStatsDBucketSamples.Inc("error")
		return
	}
	tlmDogStatsDBucketSamples.Inc("accepted")
}

// Flush seals all buckets that are old enough relative to timestamp.
func (m *DogStatsDBucketMaterializer) Flush(timestamp float64) {
	if m == nil {
		return
	}
	last, ok := m.lastSealableBucketStart(timestamp)
	if !ok {
		return
	}

	var observations []monitorObservation
	for i := range m.shards {
		result := m.shards[i].flushThrough(last, m)
		observations = m.appendSealedSeries(result.series, observations)
		observations = m.appendSealedSketchSeries(result.sketches, observations)
	}
	m.observeSealedPoints(observations)
}

// FlushAll seals all currently open buckets, ignoring the normal seal delay.
// This mirrors the TimeSampler force-flush path used during shutdown when the
// Agent is configured to flush incomplete DogStatsD buckets.
func (m *DogStatsDBucketMaterializer) FlushAll(_ float64) {
	if m == nil {
		return
	}
	var observations []monitorObservation
	for i := range m.shards {
		last, ok := m.shards[i].lastOpenBucketStart()
		if !ok {
			continue
		}
		result := m.shards[i].flushThrough(last, m)
		observations = m.appendSealedSeries(result.series, observations)
		observations = m.appendSealedSketchSeries(result.sketches, observations)
	}
	m.observeSealedPoints(observations)
}

type dogStatsDFlushResult struct {
	series   []*metrics.Serie
	sketches []*metrics.SketchSeries
}

type monitorObservation struct {
	name string
	at   time.Time
}

func (m *DogStatsDBucketMaterializer) appendSealedSeries(series []*metrics.Serie, observations []monitorObservation) []monitorObservation {
	for _, serie := range series {
		if serie == nil || len(serie.Points) == 0 {
			continue
		}
		_ = m.retention.AppendSerie(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, serie)
		tlmDogStatsDBucketPoints.Add(float64(len(serie.Points)))
		if !m.shouldObserveMetric(serie.Name) {
			continue
		}
		for _, point := range serie.Points {
			observations = append(observations, monitorObservation{name: serie.Name, at: pointObservedAt(point)})
		}
	}
	return observations
}

func (m *DogStatsDBucketMaterializer) appendSealedSketchSeries(series []*metrics.SketchSeries, observations []monitorObservation) []monitorObservation {
	for _, serie := range series {
		if serie == nil || len(serie.Points) == 0 {
			continue
		}
		_ = m.retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, serie)
		tlmDogStatsDBucketSketches.Add(float64(len(serie.Points)))
		if !m.shouldObserveMetric(serie.Name) {
			continue
		}
		for _, point := range serie.Points {
			observations = append(observations, monitorObservation{name: serie.Name, at: sketchPointObservedAt(point)})
		}
	}
	return observations
}

func (m *DogStatsDBucketMaterializer) shouldObserveMetric(name string) bool {
	return m.monitor != nil && name == m.monitor.MetricName()
}

func (m *DogStatsDBucketMaterializer) observeSealedPoints(observations []monitorObservation) {
	if m.monitor == nil || len(observations) == 0 {
		return
	}
	sort.SliceStable(observations, func(i, j int) bool {
		if observations[i].at.Equal(observations[j].at) {
			return observations[i].name < observations[j].name
		}
		return observations[i].at.Before(observations[j].at)
	})
	for _, observation := range observations {
		m.monitor.Observe(observation.name, observation.at)
	}
}

func sketchPointObservedAt(point metrics.SketchPoint) time.Time {
	if point.Ts > 0 {
		return time.Unix(point.Ts, 0)
	}
	return time.Now()
}

func (s *dogStatsDBucketShard) lastOpenBucketStart() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		return 0, false
	}
	last := int64(0)
	found := false
	for bucketStart := range s.buckets {
		if bucketStart < s.nextSealBucket {
			continue
		}
		if !found || bucketStart > last {
			last = bucketStart
			found = true
		}
	}
	for bucketStart := range s.sketchBuckets {
		if bucketStart < s.nextSealBucket {
			continue
		}
		if !found || bucketStart > last {
			last = bucketStart
			found = true
		}
	}
	return last, found
}

func (s *dogStatsDBucketShard) flushThrough(lastBucketStart int64, m *DogStatsDBucketMaterializer) dogStatsDFlushResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized || lastBucketStart < s.nextSealBucket {
		return dogStatsDFlushResult{}
	}

	result := dogStatsDFlushResult{}
	for bucketStart := s.nextSealBucket; bucketStart <= lastBucketStart; bucketStart += m.bucketWidthSeconds {
		sealedBucket := false

		bucketMetrics := s.buckets[bucketStart]
		delete(s.buckets, bucketStart)
		if bucketMetrics == nil && s.hasActiveCounter(bucketStart, m.counterExpiry) {
			bucketMetrics = metrics.MakeContextMetrics()
		}
		if bucketMetrics != nil {
			s.sampleCounterZeroValues(bucketStart, bucketMetrics, m)
			rawSeries, _ := bucketMetrics.Flush(float64(bucketStart))
			for _, serie := range rawSeries {
				desc, found := s.descriptors[serie.ContextKey]
				if !found {
					continue
				}
				serie.Name = desc.name + serie.NameSuffix
				serie.Tags = tagset.CompositeTagsFromSlice(append([]string(nil), desc.tags...))
				serie.Host = desc.host
				serie.NoIndex = desc.noIndex
				serie.Source = desc.source
				serie.Interval = m.bucketWidthSeconds
				result.series = append(result.series, serie)
				sealedBucket = true
			}
		}

		bucketSketches := s.sketchBuckets[bucketStart]
		delete(s.sketchBuckets, bucketStart)
		for contextKey, agent := range bucketSketches {
			if agent == nil {
				continue
			}
			sketch := agent.Finish()
			if sketch == nil {
				continue
			}
			desc, found := s.descriptors[contextKey]
			if !found {
				continue
			}
			result.sketches = append(result.sketches, &metrics.SketchSeries{
				DistributionMetadata: metrics.DistributionMetadata{
					Name:     desc.name,
					Tags:     tagset.CompositeTagsFromSlice(append([]string(nil), desc.tags...)),
					Host:     desc.host,
					Interval: m.bucketWidthSeconds,
					NoIndex:  desc.noIndex,
					Source:   desc.source,
				},
				Points: []metrics.SketchPoint{{
					Ts:     bucketStart,
					Sketch: sketch,
				}},
			})
			sealedBucket = true
		}

		if sealedBucket {
			tlmDogStatsDBucketSealed.Add(1)
		}
	}
	s.expireDescriptors(lastBucketStart, m)
	s.nextSealBucket = lastBucketStart + m.bucketWidthSeconds
	return result
}

func (s *dogStatsDBucketShard) hasActiveCounter(bucketStart int64, counterExpiry time.Duration) bool {
	for _, desc := range s.descriptors {
		if desc.mtype == metrics.CounterType && counterActiveAt(desc, bucketStart, counterExpiry) {
			return true
		}
	}
	return false
}

func (s *dogStatsDBucketShard) sampleCounterZeroValues(bucketStart int64, bucketMetrics metrics.ContextMetrics, m *DogStatsDBucketMaterializer) {
	for contextKey, desc := range s.descriptors {
		if desc.mtype != metrics.CounterType || !counterActiveAt(desc, bucketStart, m.counterExpiry) {
			continue
		}
		sample := &metrics.MetricSample{
			Name:       desc.name,
			Value:      0,
			RawValue:   "0.0",
			Mtype:      metrics.CounterType,
			Tags:       nil,
			Host:       desc.host,
			SampleRate: 1,
			Timestamp:  float64(bucketStart),
			NoIndex:    desc.noIndex,
			Source:     desc.source,
		}
		_ = bucketMetrics.AddSample(contextKey, sample, float64(bucketStart), m.bucketWidthSeconds, nil, pkgconfigsetup.Datadog())
	}
}

func (s *dogStatsDBucketShard) expireDescriptors(cutoffBucketStart int64, m *DogStatsDBucketMaterializer) {
	for contextKey, desc := range s.descriptors {
		expiry := m.contextExpiry
		if desc.mtype == metrics.CounterType {
			expiry += m.counterExpiry
		}
		if desc.lastSeen+int64(expiry/time.Second) < cutoffBucketStart {
			delete(s.descriptors, contextKey)
		}
	}
}

func counterActiveAt(desc dogStatsDDescriptor, bucketStart int64, counterExpiry time.Duration) bool {
	return desc.lastSeen+int64(counterExpiry/time.Second) > bucketStart
}

func (m *DogStatsDBucketMaterializer) bucketStart(timestamp float64) int64 {
	unixSeconds := int64(timestamp)
	return unixSeconds - unixSeconds%m.bucketWidthSeconds
}

func (m *DogStatsDBucketMaterializer) lastSealableBucketStart(timestamp float64) (int64, bool) {
	watermark := timestamp - m.sealDelay.Seconds()
	if watermark < float64(m.bucketWidthSeconds) {
		return 0, false
	}
	lastCandidate := int64(math.Floor(watermark)) - m.bucketWidthSeconds
	if lastCandidate < 0 {
		return 0, false
	}
	return lastCandidate - lastCandidate%m.bucketWidthSeconds, true
}

func (m *DogStatsDBucketMaterializer) shardIndex(contextKey ckey.ContextKey) int {
	if len(m.shards) <= 1 {
		return 0
	}
	return int(uint64(contextKey) % uint64(len(m.shards)))
}
