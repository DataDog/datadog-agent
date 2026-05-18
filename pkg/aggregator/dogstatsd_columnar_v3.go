// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDogstatsdColumnarStats = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_columnar_v3", "stats",
		[]string{"stat"}, "Experimental DogStatsD columnar v3 table stats")
	tlmDogstatsdColumnarFallbacks = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_columnar_v3", "fallbacks",
		[]string{"reason"}, "Experimental DogStatsD columnar v3 fallback counts")
	tlmDogstatsdColumnarDuration = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_columnar_v3", "duration_ns",
		[]string{"phase"}, "Experimental DogStatsD columnar v3 duration by phase, in nanoseconds")
)

func columnarV3ExperimentEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3"))
	return err == nil && enabled
}

// DogStatsDColumnarV3Inserter is implemented by demultiplexers that can accept
// parsed DogStatsD metric samples directly into the experimental v3-aligned
// columnar aggregation table. This bypasses the batcher, time-sampler worker,
// ContextMetrics, metrics.Metric, metrics.Serie, and iterable serializer paths
// for supported on-time metric samples.
type DogStatsDColumnarV3Inserter interface {
	DogStatsDColumnarV3Enabled() bool
	InsertDogStatsDColumnarV3Sample(shardKey ckey.ContextKey, sample metrics.MetricSample) bool
}

type dogstatsdColumnarKey struct {
	contextKey ckey.ContextKey
	mtype      metrics.MetricType
}

type dogstatsdColumnarBucket struct {
	byDescriptor map[int]int
	descriptors  []int
	values       []float64
	sampled      []bool
	sets         []map[string]struct{}
}

type dogstatsdColumnarShard struct {
	mu      sync.Mutex
	buckets map[int64]*dogstatsdColumnarBucket

	insertedSamples uint64

	descriptorByKey map[dogstatsdColumnarKey]int
	contextKeys     []ckey.ContextKey
	names           []string
	hosts           []string
	tags            [][]string
	mtypes          []metrics.MetricType
	noIndex         []bool
	sources         []metrics.MetricSource
	units           []string
}

type dogstatsdColumnarStore struct {
	interval int64
	shards   []dogstatsdColumnarShard
}

func newDogStatsDColumnarStore(interval int64, shardCount int) *dogstatsdColumnarStore {
	if interval == 0 {
		interval = bucketSize
	}
	if shardCount <= 0 {
		shardCount = 1
	}

	store := &dogstatsdColumnarStore{
		interval: interval,
		shards:   make([]dogstatsdColumnarShard, shardCount),
	}
	for i := range store.shards {
		store.shards[i].buckets = make(map[int64]*dogstatsdColumnarBucket)
		store.shards[i].descriptorByKey = make(map[dogstatsdColumnarKey]int)
	}
	return store
}

func (s *dogstatsdColumnarStore) insert(shardKey ckey.ContextKey, sample metrics.MetricSample, timestamp float64) bool {
	if s == nil {
		return false
	}
	if sample.Timestamp > 0 {
		recordDogstatsdColumnarFallback("timestamp")
		return false
	}
	if math.IsInf(sample.Value, 0) || math.IsNaN(sample.Value) {
		recordDogstatsdColumnarFallback("invalid_value")
		return false
	}
	if !dogstatsdColumnarSupportedMetric(sample.Mtype) {
		recordDogstatsdColumnarFallback("metric_type")
		return false
	}
	if sample.FlushFirstValue {
		recordDogstatsdColumnarFallback("flush_first_value")
		return false
	}

	shardIdx := dogstatsdColumnarShardIndex(shardKey, len(s.shards))
	bucketStart := s.calculateBucketStart(timestamp)
	shard := &s.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	bucket := shard.buckets[bucketStart]
	if bucket == nil {
		bucket = newDogstatsdColumnarBucket()
		shard.buckets[bucketStart] = bucket
	}

	key := dogstatsdColumnarKey{contextKey: shardKey, mtype: sample.Mtype}
	descriptorID, ok := shard.descriptorByKey[key]
	if !ok {
		descriptorID = shard.appendDescriptor(key, sample)
	}

	idx, ok := bucket.byDescriptor[descriptorID]
	if !ok {
		idx = bucket.appendRow(descriptorID)
	}

	switch sample.Mtype {
	case metrics.GaugeType:
		bucket.values[idx] = sample.Value
		bucket.sampled[idx] = true
	case metrics.CounterType:
		sampleRate := sample.SampleRate
		if sampleRate == 0 {
			sampleRate = 1
		}
		bucket.values[idx] += sample.Value * (1 / sampleRate)
		bucket.sampled[idx] = true
	case metrics.CountType:
		bucket.values[idx] += sample.Value
		bucket.sampled[idx] = true
	case metrics.SetType:
		if bucket.sets[idx] == nil {
			bucket.sets[idx] = make(map[string]struct{})
		}
		bucket.sets[idx][sample.RawValue] = struct{}{}
		bucket.sampled[idx] = true
	}

	shard.insertedSamples++
	return true
}

func newDogstatsdColumnarBucket() *dogstatsdColumnarBucket {
	return &dogstatsdColumnarBucket{
		byDescriptor: make(map[int]int),
	}
}

func (s *dogstatsdColumnarShard) appendDescriptor(key dogstatsdColumnarKey, sample metrics.MetricSample) int {
	idx := len(s.names)
	s.descriptorByKey[key] = idx
	s.contextKeys = append(s.contextKeys, key.contextKey)
	s.names = append(s.names, sample.Name)
	s.hosts = append(s.hosts, sample.Host)
	s.tags = append(s.tags, cloneSortedTags(sample.Tags))
	s.mtypes = append(s.mtypes, sample.Mtype)
	s.noIndex = append(s.noIndex, sample.NoIndex)
	s.sources = append(s.sources, sample.Source)
	s.units = append(s.units, sample.Unit)
	tlmDogstatsdColumnarStats.Inc("created_descriptors")
	return idx
}

func (b *dogstatsdColumnarBucket) appendRow(descriptorID int) int {
	idx := len(b.descriptors)
	b.byDescriptor[descriptorID] = idx
	b.descriptors = append(b.descriptors, descriptorID)
	b.values = append(b.values, 0)
	b.sampled = append(b.sampled, false)
	b.sets = append(b.sets, nil)

	tlmDogstatsdColumnarStats.Inc("created_rows")
	return idx
}

func cloneSortedTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	cloned := append([]string(nil), tags...)
	slices.Sort(cloned)
	if len(cloned) < 2 {
		return cloned
	}
	j := 0
	for i := 1; i < len(cloned); i++ {
		if cloned[i] == cloned[j] {
			continue
		}
		j++
		cloned[j] = cloned[i]
	}
	return cloned[:j+1]
}

func dogstatsdColumnarShardIndex(key ckey.ContextKey, shardCount int) int {
	return int((uint64(key>>32) * uint64(shardCount)) >> 32)
}

func dogstatsdColumnarSupportedMetric(mtype metrics.MetricType) bool {
	switch mtype {
	case metrics.GaugeType, metrics.CounterType, metrics.CountType, metrics.SetType:
		return true
	default:
		return false
	}
}

func (s *dogstatsdColumnarStore) calculateBucketStart(timestamp float64) int64 {
	return int64(timestamp) - int64(timestamp)%s.interval
}

func (s *dogstatsdColumnarStore) flush(cutoffTime int64, forceFlushAll bool, rowSink metrics.SerieRowSink) uint64 {
	if s == nil || rowSink == nil {
		return 0
	}

	start := time.Now()
	shadow := newDirectRowShadowBuilder()
	var rows uint64

	for shardIdx := range s.shards {
		shard := &s.shards[shardIdx]
		rows += s.flushShard(shard, cutoffTime, forceFlushAll, rowSink, shadow)
	}

	shadow.finish("columnar_v3", time.Since(start))
	tlmDogstatsdColumnarDuration.Add(float64(time.Since(start).Nanoseconds()), "flush")
	tlmDogstatsdColumnarStats.Inc("flushes")
	tlmDogstatsdColumnarStats.Add(float64(rows), "flushed_rows")
	return rows
}

func (s *dogstatsdColumnarStore) flushShard(shard *dogstatsdColumnarShard, cutoffTime int64, forceFlushAll bool, rowSink metrics.SerieRowSink, shadow *directRowShadowBuilder) uint64 {
	var rows []metrics.SerieRow
	rowByKey := make(map[dogstatsdColumnarKey]int)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.insertedSamples > 0 {
		tlmDogstatsdColumnarStats.Add(float64(shard.insertedSamples), "inserted_samples")
		shard.insertedSamples = 0
	}

	for bucketTimestamp, bucket := range shard.buckets {
		if bucketTimestamp+s.interval > cutoffTime && !forceFlushAll {
			continue
		}
		s.collectBucket(shard, bucketTimestamp, bucket, &rows, rowByKey)
		delete(shard.buckets, bucketTimestamp)
		tlmDogstatsdColumnarStats.Inc("flushed_buckets")
	}

	for i := range rows {
		shadow.observeSerieRow(&rows[i])
		rowSink.AppendSerieRow(rows[i])
	}
	return uint64(len(rows))
}

func (s *dogstatsdColumnarStore) collectBucket(shard *dogstatsdColumnarShard, bucketTimestamp int64, bucket *dogstatsdColumnarBucket, rows *[]metrics.SerieRow, rowByKey map[dogstatsdColumnarKey]int) {
	for idx, descriptorID := range bucket.descriptors {
		if !bucket.sampled[idx] {
			continue
		}

		value, apiType, ok := s.flushValue(shard, bucket, idx, descriptorID)
		if !ok {
			continue
		}

		point := metrics.Point{Ts: float64(bucketTimestamp), Value: value}
		key := dogstatsdColumnarKey{contextKey: shard.contextKeys[descriptorID], mtype: shard.mtypes[descriptorID]}
		if rowIdx, ok := rowByKey[key]; ok {
			(*rows)[rowIdx].Points = append((*rows)[rowIdx].Points, point)
		} else {
			row := metrics.NewSerieRow(
				shard.names[descriptorID],
				[]metrics.Point{point},
				tagset.CompositeTagsFromSlice(shard.tags[descriptorID]),
				shard.hosts[descriptorID],
				"",
				apiType,
				s.interval,
				"",
				shard.units[descriptorID],
				shard.noIndex[descriptorID],
				nil,
				shard.sources[descriptorID],
			)
			rowByKey[key] = len(*rows)
			*rows = append(*rows, row)
		}
		tlmDogstatsdColumnarStats.Inc("flushed_points")
	}
}

func (s *dogstatsdColumnarStore) flushValue(shard *dogstatsdColumnarShard, bucket *dogstatsdColumnarBucket, idx int, descriptorID int) (float64, metrics.APIMetricType, bool) {
	switch shard.mtypes[descriptorID] {
	case metrics.GaugeType:
		return bucket.values[idx], metrics.APIGaugeType, true
	case metrics.CounterType:
		return bucket.values[idx] / float64(s.interval), metrics.APIRateType, true
	case metrics.CountType:
		return bucket.values[idx], metrics.APICountType, true
	case metrics.SetType:
		return float64(len(bucket.sets[idx])), metrics.APIGaugeType, true
	default:
		log.Debugf("DogStatsD columnar v3: unsupported metric type in flush: %s", shard.mtypes[descriptorID])
		return 0, metrics.APIGaugeType, false
	}
}

func recordDogstatsdColumnarFallback(reason string) {
	tlmDogstatsdColumnarFallbacks.Inc(reason)
	tlmDogstatsdColumnarStats.Inc("fallback_samples")
}
