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

func columnarV3SkipLegacyFlushEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_SKIP_LEGACY_FLUSH"))
	return err == nil && enabled
}

// DogStatsDColumnarV3Inserter is implemented by demultiplexers that can accept
// parsed DogStatsD metric samples into the experimental v3-aligned columnar
// aggregation table. Supported on-time metric samples bypass TimeSampler,
// ContextMetrics, metrics.Metric, metrics.Serie, and iterable serializer paths.
type DogStatsDColumnarV3Inserter interface {
	DogStatsDColumnarV3Enabled() bool
	AcceptDogStatsDColumnarV3Sample(sample metrics.MetricSample) bool
	GetDogStatsDColumnarV3SamplePool() *DogStatsDColumnarV3SamplePool
	AggregateDogStatsDColumnarV3Samples(shard TimeSamplerID, samples DogStatsDColumnarV3SampleBatch)
	InsertDogStatsDColumnarV3Sample(shardKey ckey.ContextKey, sample metrics.MetricSample) bool
}

// DogStatsDColumnarV3Sample is the parser-to-columnar-worker handoff row for
// the local-only vertical-slice experiment. It carries the already resolved
// backend context key alongside the enriched DogStatsD sample.
type DogStatsDColumnarV3Sample struct {
	ContextKey ckey.ContextKey
	Sample     metrics.MetricSample
}

// DogStatsDColumnarV3SampleBatch is owned by the columnar v3 sample pool.
type DogStatsDColumnarV3SampleBatch []DogStatsDColumnarV3Sample

// DogStatsDColumnarV3SamplePool reuses parser-to-columnar-worker handoff
// batches, mirroring the existing MetricSamplePool ownership pattern.
type DogStatsDColumnarV3SamplePool struct {
	batchSize int
	pool      sync.Pool
}

func newDogStatsDColumnarV3SamplePool(batchSize int) *DogStatsDColumnarV3SamplePool {
	if batchSize <= 0 {
		batchSize = MetricSamplePoolBatchSize
	}
	return &DogStatsDColumnarV3SamplePool{batchSize: batchSize}
}

// GetBatch returns an empty batch with fixed capacity.
func (p *DogStatsDColumnarV3SamplePool) GetBatch() DogStatsDColumnarV3SampleBatch {
	if p == nil {
		return make(DogStatsDColumnarV3SampleBatch, MetricSamplePoolBatchSize)
	}
	if batch, ok := p.pool.Get().(DogStatsDColumnarV3SampleBatch); ok {
		return batch[:p.batchSize]
	}
	return make(DogStatsDColumnarV3SampleBatch, p.batchSize)
}

// PutBatch clears and returns a batch to the pool.
func (p *DogStatsDColumnarV3SamplePool) PutBatch(batch DogStatsDColumnarV3SampleBatch) {
	if p == nil || cap(batch) < p.batchSize {
		return
	}
	batch = batch[:p.batchSize]
	for i := range batch {
		batch[i] = DogStatsDColumnarV3Sample{}
	}
	p.pool.Put(batch)
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
	lastBucket      []int64
	lastRow         []int
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

type dogstatsdColumnarWorker struct {
	store      *dogstatsdColumnarStore
	shardID    TimeSamplerID
	samplePool *DogStatsDColumnarV3SamplePool

	samplesChan chan DogStatsDColumnarV3SampleBatch
	flushChan   chan dogstatsdColumnarFlushTrigger
	stopChan    chan struct{}
}

type dogstatsdColumnarFlushTrigger struct {
	cutoffTime    int64
	forceFlushAll bool
	rowSink       metrics.SerieRowSink
	blockChan     chan uint64
}

func newDogStatsDColumnarWorker(store *dogstatsdColumnarStore, shardID TimeSamplerID, bufferSize int, samplePool *DogStatsDColumnarV3SamplePool) *dogstatsdColumnarWorker {
	return &dogstatsdColumnarWorker{
		store:       store,
		shardID:     shardID,
		samplePool:  samplePool,
		samplesChan: make(chan DogStatsDColumnarV3SampleBatch, bufferSize),
		flushChan:   make(chan dogstatsdColumnarFlushTrigger),
		stopChan:    make(chan struct{}),
	}
}

func (w *dogstatsdColumnarWorker) run() {
	for {
		select {
		case <-w.stopChan:
			return
		case samples := <-w.samplesChan:
			w.processSamples(samples)
		case trigger := <-w.flushChan:
			w.triggerFlush(trigger)
		}
	}
}

func (w *dogstatsdColumnarWorker) processSamples(samples DogStatsDColumnarV3SampleBatch) {
	if len(samples) == 0 {
		w.samplePool.PutBatch(samples)
		return
	}
	timestamp := timeNowNano()
	shardIdx := int(w.shardID)
	for i := range samples {
		w.store.insertAcceptedUnlocked(shardIdx, samples[i].ContextKey, samples[i].Sample, timestamp)
	}
	w.samplePool.PutBatch(samples)
}

func (w *dogstatsdColumnarWorker) triggerFlush(trigger dogstatsdColumnarFlushTrigger) {
	if w.store == nil || int(w.shardID) >= len(w.store.shards) {
		trigger.blockChan <- 0
		return
	}
	start := time.Now()
	shadow := newDirectRowShadowBuilder()
	rows := w.store.flushShard(&w.store.shards[int(w.shardID)], trigger.cutoffTime, trigger.forceFlushAll, trigger.rowSink, shadow)
	shadow.finish("columnar_v3", time.Since(start))
	trigger.blockChan <- rows
}

func (w *dogstatsdColumnarWorker) stop() {
	w.stopChan <- struct{}{}
}

func (s *dogstatsdColumnarStore) accept(sample metrics.MetricSample) bool {
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
	return true
}

func (s *dogstatsdColumnarStore) insert(shardKey ckey.ContextKey, sample metrics.MetricSample, timestamp float64) bool {
	if !s.accept(sample) {
		return false
	}
	shardIdx := dogstatsdColumnarShardIndex(shardKey, len(s.shards))
	s.insertAccepted(shardIdx, shardKey, sample, timestamp)
	return true
}

func (s *dogstatsdColumnarStore) insertAccepted(shardIdx int, shardKey ckey.ContextKey, sample metrics.MetricSample, timestamp float64) {
	if s == nil || shardIdx < 0 || shardIdx >= len(s.shards) {
		return
	}
	shard := &s.shards[shardIdx]
	bucketStart := s.calculateBucketStart(timestamp)

	shard.mu.Lock()
	defer shard.mu.Unlock()
	s.insertAcceptedInShard(shard, bucketStart, shardKey, sample)
}

func (s *dogstatsdColumnarStore) insertAcceptedUnlocked(shardIdx int, shardKey ckey.ContextKey, sample metrics.MetricSample, timestamp float64) {
	if s == nil || shardIdx < 0 || shardIdx >= len(s.shards) {
		return
	}
	shard := &s.shards[shardIdx]
	bucketStart := s.calculateBucketStart(timestamp)
	s.insertAcceptedInShard(shard, bucketStart, shardKey, sample)
}

func (s *dogstatsdColumnarStore) insertAcceptedInShard(shard *dogstatsdColumnarShard, bucketStart int64, shardKey ckey.ContextKey, sample metrics.MetricSample) {
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

	idx := -1
	if shard.lastRow[descriptorID] >= 0 && shard.lastBucket[descriptorID] == bucketStart {
		idx = shard.lastRow[descriptorID]
	} else {
		var ok bool
		idx, ok = bucket.byDescriptor[descriptorID]
		if !ok {
			idx = bucket.appendRow(descriptorID)
		}
		shard.lastBucket[descriptorID] = bucketStart
		shard.lastRow[descriptorID] = idx
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
	s.lastBucket = append(s.lastBucket, 0)
	s.lastRow = append(s.lastRow, -1)
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
