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
	"strings"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

func columnarV3NativeSerializerEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_NATIVE_SERIALIZER"))
	return err == nil && enabled
}

func columnarV3DirectSeriesSerializerEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_SERIES_SERIALIZER"))
	return err == nil && enabled
}

func columnarV3DescriptorExpirySeconds() int64 {
	if raw := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DESCRIPTOR_EXPIRY_SECONDS"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			return value
		}
	}
	return pkgconfigsetup.Datadog().GetInt64("dogstatsd_context_expiry_seconds")
}

func columnarV3DescriptorInterningEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_INTERN_DESCRIPTORS"))
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
	// byDescriptor is intentionally lazy. The normal DogStatsD columnar path uses
	// monotonically increasing arrival-time buckets, so descriptor-local
	// lastBucket/lastRow is enough to find the current row. The map is built only
	// for rare non-monotonic descriptor/bucket access.
	byDescriptor map[int]int
	descriptors  []int
	values       []float64
	sampled      []bool
	sets         []map[string]struct{}
}

type dogstatsdColumnarStringEntry struct {
	value string
	refs  int
}

type dogstatsdColumnarTagsetEntry struct {
	tags []string
	refs int
}

type dogstatsdColumnarDictionary struct {
	strings map[string]*dogstatsdColumnarStringEntry
	tagsets map[string]*dogstatsdColumnarTagsetEntry
}

type dogstatsdColumnarShard struct {
	mu      sync.Mutex
	buckets map[int64]*dogstatsdColumnarBucket

	insertedSamples uint64

	descriptorByKey        map[dogstatsdColumnarKey]int
	freeDescriptors        []int
	descriptorActive       []bool
	descriptorNonMonotonic []bool
	contextKeys            []ckey.ContextKey
	names                  []string
	hosts                  []string
	tags                   [][]string
	tagKeys                []string
	mtypes                 []metrics.MetricType
	noIndex                []bool
	sources                []metrics.MetricSource
	units                  []string
	lastSeen               []int64
	lastBucket             []int64
	lastRow                []int
	flushGeneration        uint64
	flushSeen              []uint64
	flushRow               []int
	dictionary             dogstatsdColumnarDictionary
}

type dogstatsdColumnarStore struct {
	interval            int64
	descriptorExpiry    int64
	descriptorInterning bool
	shards              []dogstatsdColumnarShard
}

func newDogstatsdColumnarDictionary() dogstatsdColumnarDictionary {
	return dogstatsdColumnarDictionary{
		strings: make(map[string]*dogstatsdColumnarStringEntry),
		tagsets: make(map[string]*dogstatsdColumnarTagsetEntry),
	}
}

func (d *dogstatsdColumnarDictionary) internString(value string) string {
	if value == "" {
		return ""
	}
	if entry, ok := d.strings[value]; ok {
		entry.refs++
		return entry.value
	}
	d.strings[value] = &dogstatsdColumnarStringEntry{value: value, refs: 1}
	tlmDogstatsdColumnarStats.Inc("created_dictionary_strings")
	return value
}

func (d *dogstatsdColumnarDictionary) releaseString(value string) {
	if value == "" {
		return
	}
	entry, ok := d.strings[value]
	if !ok {
		return
	}
	entry.refs--
	if entry.refs > 0 {
		return
	}
	delete(d.strings, value)
	tlmDogstatsdColumnarStats.Inc("released_dictionary_strings")
}

func (d *dogstatsdColumnarDictionary) internTags(tags []string) ([]string, string) {
	if len(tags) == 0 {
		return nil, ""
	}
	cloned := append([]string(nil), tags...)
	slices.Sort(cloned)
	cloned = dedupeSortedTags(cloned)
	key := strings.Join(cloned, "\x00")
	if entry, ok := d.tagsets[key]; ok {
		entry.refs++
		tlmDogstatsdColumnarStats.Inc("reused_dictionary_tagsets")
		return entry.tags, key
	}
	for i, tag := range cloned {
		cloned[i] = d.internString(tag)
	}
	d.tagsets[key] = &dogstatsdColumnarTagsetEntry{tags: cloned, refs: 1}
	tlmDogstatsdColumnarStats.Inc("created_dictionary_tagsets")
	return cloned, key
}

func (d *dogstatsdColumnarDictionary) releaseTags(key string) {
	if key == "" {
		return
	}
	entry, ok := d.tagsets[key]
	if !ok {
		return
	}
	entry.refs--
	if entry.refs > 0 {
		return
	}
	for _, tag := range entry.tags {
		d.releaseString(tag)
	}
	delete(d.tagsets, key)
	tlmDogstatsdColumnarStats.Inc("released_dictionary_tagsets")
}

func newDogStatsDColumnarStore(interval int64, shardCount int) *dogstatsdColumnarStore {
	if interval == 0 {
		interval = bucketSize
	}
	if shardCount <= 0 {
		shardCount = 1
	}

	store := &dogstatsdColumnarStore{
		interval:            interval,
		descriptorExpiry:    columnarV3DescriptorExpirySeconds(),
		descriptorInterning: columnarV3DescriptorInterningEnabled(),
		shards:              make([]dogstatsdColumnarShard, shardCount),
	}
	if store.descriptorExpiry > 0 && store.descriptorExpiry < store.interval {
		store.descriptorExpiry = store.interval
	}
	for i := range store.shards {
		store.shards[i].buckets = make(map[int64]*dogstatsdColumnarBucket)
		store.shards[i].descriptorByKey = make(map[dogstatsdColumnarKey]int)
		if store.descriptorInterning {
			store.shards[i].dictionary = newDogstatsdColumnarDictionary()
		}
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
	cutoffTime     int64
	forceFlushAll  bool
	rowSink        metrics.SerieRowSink
	v3PointRowSink metrics.V3MetricPointRowSink
	blockChan      chan uint64
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
	shadow := newOptionalDirectRowShadowBuilder()
	var shadowStart time.Time
	if shadow != nil {
		shadowStart = time.Now()
	}
	var rows uint64
	if trigger.v3PointRowSink != nil {
		rows = w.store.flushShardToV3MetricPointSink(&w.store.shards[int(w.shardID)], trigger.cutoffTime, trigger.forceFlushAll, trigger.v3PointRowSink, shadow)
	} else {
		rows = w.store.flushShard(&w.store.shards[int(w.shardID)], trigger.cutoffTime, trigger.forceFlushAll, trigger.rowSink, shadow)
	}
	finishDirectRowShadow(shadow, "columnar_v3", shadowStart)
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
	if !ok || !shard.descriptorActive[descriptorID] {
		descriptorID = shard.appendDescriptor(key, sample, s.descriptorInterning)
	}
	shard.lastSeen[descriptorID] = bucketStart

	idx := shard.rowIndex(bucket, bucketStart, descriptorID)

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
	return &dogstatsdColumnarBucket{}
}

func (s *dogstatsdColumnarShard) appendDescriptor(key dogstatsdColumnarKey, sample metrics.MetricSample, internDescriptorStrings bool) int {
	idx := -1
	if len(s.freeDescriptors) > 0 {
		last := len(s.freeDescriptors) - 1
		idx = s.freeDescriptors[last]
		s.freeDescriptors = s.freeDescriptors[:last]
		tlmDogstatsdColumnarStats.Inc("reused_descriptor_slots")
	} else {
		idx = len(s.names)
		s.contextKeys = append(s.contextKeys, 0)
		s.names = append(s.names, "")
		s.hosts = append(s.hosts, "")
		s.tags = append(s.tags, nil)
		s.tagKeys = append(s.tagKeys, "")
		s.mtypes = append(s.mtypes, 0)
		s.noIndex = append(s.noIndex, false)
		s.sources = append(s.sources, 0)
		s.units = append(s.units, "")
		s.lastSeen = append(s.lastSeen, 0)
		s.lastBucket = append(s.lastBucket, 0)
		s.lastRow = append(s.lastRow, -1)
		s.descriptorActive = append(s.descriptorActive, false)
		s.descriptorNonMonotonic = append(s.descriptorNonMonotonic, false)
		s.flushSeen = append(s.flushSeen, 0)
		s.flushRow = append(s.flushRow, 0)
	}

	name := sample.Name
	host := sample.Host
	tags := cloneSortedTags(sample.Tags)
	tagKey := ""
	unit := sample.Unit
	if internDescriptorStrings {
		name = s.dictionary.internString(sample.Name)
		host = s.dictionary.internString(sample.Host)
		tags, tagKey = s.dictionary.internTags(sample.Tags)
		unit = s.dictionary.internString(sample.Unit)
	}

	s.descriptorByKey[key] = idx
	s.descriptorActive[idx] = true
	s.descriptorNonMonotonic[idx] = false
	s.contextKeys[idx] = key.contextKey
	s.names[idx] = name
	s.hosts[idx] = host
	s.tags[idx] = tags
	s.tagKeys[idx] = tagKey
	s.mtypes[idx] = sample.Mtype
	s.noIndex[idx] = sample.NoIndex
	s.sources[idx] = sample.Source
	s.units[idx] = unit
	s.lastSeen[idx] = 0
	s.lastBucket[idx] = 0
	s.lastRow[idx] = -1
	s.flushSeen[idx] = 0
	s.flushRow[idx] = 0
	tlmDogstatsdColumnarStats.Inc("created_descriptors")
	return idx
}

func (s *dogstatsdColumnarShard) rowIndex(bucket *dogstatsdColumnarBucket, bucketStart int64, descriptorID int) int {
	if s.lastRow[descriptorID] >= 0 && s.lastBucket[descriptorID] == bucketStart {
		return s.lastRow[descriptorID]
	}

	if s.lastRow[descriptorID] >= 0 && s.lastBucket[descriptorID] > bucketStart && !s.descriptorNonMonotonic[descriptorID] {
		s.descriptorNonMonotonic[descriptorID] = true
		tlmDogstatsdColumnarStats.Inc("non_monotonic_descriptors")
	}

	if s.descriptorNonMonotonic[descriptorID] {
		bucket.ensureDescriptorIndex()
		if idx, ok := bucket.byDescriptor[descriptorID]; ok {
			s.lastBucket[descriptorID] = bucketStart
			s.lastRow[descriptorID] = idx
			return idx
		}
	}

	idx := bucket.appendRow(descriptorID)
	if bucket.byDescriptor != nil {
		bucket.byDescriptor[descriptorID] = idx
	}
	s.lastBucket[descriptorID] = bucketStart
	s.lastRow[descriptorID] = idx
	return idx
}

func (b *dogstatsdColumnarBucket) ensureDescriptorIndex() {
	if b.byDescriptor != nil {
		return
	}
	b.byDescriptor = make(map[int]int, len(b.descriptors))
	for idx, descriptorID := range b.descriptors {
		b.byDescriptor[descriptorID] = idx
	}
	tlmDogstatsdColumnarStats.Inc("created_bucket_indexes")
}

func (b *dogstatsdColumnarBucket) appendRow(descriptorID int) int {
	idx := len(b.descriptors)
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
	return dedupeSortedTags(cloned)
}

func dedupeSortedTags(tags []string) []string {
	if len(tags) < 2 {
		return tags
	}
	j := 0
	for i := 1; i < len(tags); i++ {
		if tags[i] == tags[j] {
			continue
		}
		j++
		tags[j] = tags[i]
	}
	return tags[:j+1]
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
	shadow := newOptionalDirectRowShadowBuilder()
	var rows uint64

	for shardIdx := range s.shards {
		shard := &s.shards[shardIdx]
		rows += s.flushShard(shard, cutoffTime, forceFlushAll, rowSink, shadow)
	}

	duration := time.Since(start)
	shadow.finish("columnar_v3", duration)
	tlmDogstatsdColumnarDuration.Add(float64(duration.Nanoseconds()), "flush")
	tlmDogstatsdColumnarStats.Inc("flushes")
	tlmDogstatsdColumnarStats.Add(float64(rows), "flushed_rows")
	return rows
}

func (s *dogstatsdColumnarStore) flushShard(shard *dogstatsdColumnarShard, cutoffTime int64, forceFlushAll bool, rowSink metrics.SerieRowSink, shadow *directRowShadowBuilder) uint64 {
	var rows []metrics.SerieRow

	shard.mu.Lock()
	defer shard.mu.Unlock()

	generation := shard.nextFlushGeneration()
	if shard.insertedSamples > 0 {
		tlmDogstatsdColumnarStats.Add(float64(shard.insertedSamples), "inserted_samples")
		shard.insertedSamples = 0
	}

	for bucketTimestamp, bucket := range shard.buckets {
		if bucketTimestamp+s.interval > cutoffTime && !forceFlushAll {
			continue
		}
		s.collectBucket(shard, bucketTimestamp, bucket, &rows, generation)
		delete(shard.buckets, bucketTimestamp)
		tlmDogstatsdColumnarStats.Inc("flushed_buckets")
	}
	s.expireDescriptors(shard, cutoffTime)

	for i := range rows {
		shadow.observeSerieRow(&rows[i])
		rowSink.AppendSerieRow(rows[i])
	}
	return uint64(len(rows))
}

func (s *dogstatsdColumnarStore) flushShardToV3MetricPointSink(shard *dogstatsdColumnarShard, cutoffTime int64, forceFlushAll bool, sink metrics.V3MetricPointRowSink, shadow *directRowShadowBuilder) uint64 {
	var rows []metrics.V3MetricPointRow

	shard.mu.Lock()
	defer shard.mu.Unlock()

	generation := shard.nextFlushGeneration()
	if shard.insertedSamples > 0 {
		tlmDogstatsdColumnarStats.Add(float64(shard.insertedSamples), "inserted_samples")
		shard.insertedSamples = 0
	}

	for bucketTimestamp, bucket := range shard.buckets {
		if bucketTimestamp+s.interval > cutoffTime && !forceFlushAll {
			continue
		}
		s.collectBucketToV3MetricPointRows(shard, bucketTimestamp, bucket, &rows, generation)
		delete(shard.buckets, bucketTimestamp)
		tlmDogstatsdColumnarStats.Inc("flushed_buckets")
	}
	s.expireDescriptors(shard, cutoffTime)

	for i := range rows {
		shadow.observeV3MetricPointRow(&rows[i])
		sink.AppendV3MetricPointRow(rows[i])
	}
	return uint64(len(rows))
}

func (s *dogstatsdColumnarStore) collectBucket(shard *dogstatsdColumnarShard, bucketTimestamp int64, bucket *dogstatsdColumnarBucket, rows *[]metrics.SerieRow, generation uint64) {
	for idx, descriptorID := range bucket.descriptors {
		if !bucket.sampled[idx] || !shard.descriptorActive[descriptorID] {
			continue
		}

		value, apiType, ok := s.flushValue(shard, bucket, idx, descriptorID)
		if !ok {
			continue
		}

		point := metrics.Point{Ts: float64(bucketTimestamp), Value: value}
		if shard.flushSeen[descriptorID] == generation {
			rowIdx := shard.flushRow[descriptorID]
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
			shard.flushSeen[descriptorID] = generation
			shard.flushRow[descriptorID] = len(*rows)
			*rows = append(*rows, row)
		}
		tlmDogstatsdColumnarStats.Inc("flushed_points")
	}
}

func (s *dogstatsdColumnarStore) collectBucketToV3MetricPointRows(shard *dogstatsdColumnarShard, bucketTimestamp int64, bucket *dogstatsdColumnarBucket, rows *[]metrics.V3MetricPointRow, generation uint64) {
	for idx, descriptorID := range bucket.descriptors {
		if !bucket.sampled[idx] || !shard.descriptorActive[descriptorID] {
			continue
		}

		value, apiType, ok := s.flushValue(shard, bucket, idx, descriptorID)
		if !ok {
			continue
		}

		if shard.flushSeen[descriptorID] == generation {
			rowIdx := shard.flushRow[descriptorID]
			(*rows)[rowIdx].AppendPoint(bucketTimestamp, value)
		} else {
			row := metrics.NewV3MetricPointRow(
				shard.names[descriptorID],
				bucketTimestamp,
				value,
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
			shard.flushSeen[descriptorID] = generation
			shard.flushRow[descriptorID] = len(*rows)
			*rows = append(*rows, row)
		}
		tlmDogstatsdColumnarStats.Inc("flushed_points")
	}
}

func (s *dogstatsdColumnarShard) nextFlushGeneration() uint64 {
	s.flushGeneration++
	if s.flushGeneration != 0 {
		return s.flushGeneration
	}
	for i := range s.flushSeen {
		s.flushSeen[i] = 0
	}
	s.flushGeneration = 1
	return s.flushGeneration
}

func (s *dogstatsdColumnarStore) expireDescriptors(shard *dogstatsdColumnarShard, cutoffTime int64) {
	if s.descriptorExpiry <= 0 {
		return
	}
	expireBefore := cutoffTime - s.descriptorExpiry
	for descriptorID, active := range shard.descriptorActive {
		if !active || shard.lastSeen[descriptorID] >= expireBefore {
			continue
		}
		key := dogstatsdColumnarKey{contextKey: shard.contextKeys[descriptorID], mtype: shard.mtypes[descriptorID]}
		delete(shard.descriptorByKey, key)
		shard.releaseDescriptor(descriptorID)
	}
}

func (s *dogstatsdColumnarShard) releaseDescriptor(descriptorID int) {
	s.dictionary.releaseString(s.names[descriptorID])
	s.dictionary.releaseString(s.hosts[descriptorID])
	s.dictionary.releaseTags(s.tagKeys[descriptorID])
	s.dictionary.releaseString(s.units[descriptorID])

	s.descriptorActive[descriptorID] = false
	s.descriptorNonMonotonic[descriptorID] = false
	s.contextKeys[descriptorID] = 0
	s.names[descriptorID] = ""
	s.hosts[descriptorID] = ""
	s.tags[descriptorID] = nil
	s.tagKeys[descriptorID] = ""
	s.mtypes[descriptorID] = 0
	s.noIndex[descriptorID] = false
	s.sources[descriptorID] = 0
	s.units[descriptorID] = ""
	s.lastSeen[descriptorID] = 0
	s.lastBucket[descriptorID] = 0
	s.lastRow[descriptorID] = -1
	s.flushSeen[descriptorID] = 0
	s.flushRow[descriptorID] = 0
	s.freeDescriptors = append(s.freeDescriptors, descriptorID)
	tlmDogstatsdColumnarStats.Inc("expired_descriptors")
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
