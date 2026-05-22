// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	taggercomp "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
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

func columnarV3CompactHintSize() int {
	if raw := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_COMPACT_HINT_SIZE"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	return 65536
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
// the local-only vertical-slice experiment. It carries the parser-side shard
// key plus the scalar fields needed for aggregation. When descriptor fields are
// present, the columnar worker resolves the final backend context with the same
// tagger/tag-filter path as the legacy TimeSampler before storing the row.
// Compact identity consumers may omit descriptor fields once the columnar
// worker has acknowledged descriptor state for compatibility-safe rows.
type DogStatsDColumnarV3Sample struct {
	ContextKey   ckey.ContextKey
	CompactID    uint64
	CompactState *metrics.DogStatsDCompactIdentityState

	DescriptorID         int
	DescriptorGeneration uint32
	HasDescriptorRef     bool

	Value      float64
	SampleRate float64
	RawValue   string
	Mtype      metrics.MetricType

	Name          string
	Tags          []string
	Host          string
	Unit          string
	NoIndex       bool
	Source        metrics.MetricSource
	OriginInfo    taggertypes.OriginInfo
	HasDescriptor bool
}

// NewDogStatsDColumnarV3SampleFromMetricSample projects a full MetricSample into
// the narrower parser-to-columnar row used by the v3 vertical slice.
func NewDogStatsDColumnarV3SampleFromMetricSample(contextKey ckey.ContextKey, compactID uint64, compactState *metrics.DogStatsDCompactIdentityState, sample metrics.MetricSample, includeDescriptor bool) DogStatsDColumnarV3Sample {
	row := DogStatsDColumnarV3Sample{
		ContextKey:   contextKey,
		CompactID:    compactID,
		CompactState: compactState,
		Value:        sample.Value,
		SampleRate:   sample.SampleRate,
		RawValue:     sample.RawValue,
		Mtype:        sample.Mtype,
	}
	if descriptorID, generation, ok := compactState.ColumnarDescriptorRef(sample.Mtype); ok {
		row.DescriptorID = descriptorID
		row.DescriptorGeneration = generation
		row.HasDescriptorRef = true
		return row
	}
	if includeDescriptor {
		row.Name = sample.Name
		row.Tags = sample.Tags
		row.Host = sample.Host
		row.Unit = sample.Unit
		row.NoIndex = sample.NoIndex
		row.Source = sample.Source
		row.OriginInfo = sample.OriginInfo
		row.HasDescriptor = true
	}
	return row
}

// GetName implements metrics.MetricSampleContext so the columnar table can
// reuse the same backend context resolver as the legacy TimeSampler.
func (r *DogStatsDColumnarV3Sample) GetName() string { return r.Name }

// GetHost implements metrics.MetricSampleContext.
func (r *DogStatsDColumnarV3Sample) GetHost() string { return r.Host }

// GetTags implements metrics.MetricSampleContext. Client-provided DogStatsD
// tags remain metric tags; origin-derived tags are appended through the tagger
// buffer, matching metrics.MetricSample.GetTags.
func (r *DogStatsDColumnarV3Sample) GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, tagger taggercomp.Component) {
	metricBuffer.Append(r.Tags...)
	if tagger != nil {
		tagger.EnrichTags(taggerBuffer, r.OriginInfo)
	}
}

// GetMetricType implements metrics.MetricSampleContext.
func (r *DogStatsDColumnarV3Sample) GetMetricType() metrics.MetricType { return r.Mtype }

// IsNoIndex implements metrics.MetricSampleContext.
func (r *DogStatsDColumnarV3Sample) IsNoIndex() bool { return r.NoIndex }

// GetSource implements metrics.MetricSampleContext.
func (r *DogStatsDColumnarV3Sample) GetSource() metrics.MetricSource { return r.Source }

var _ metrics.MetricSampleContext = (*DogStatsDColumnarV3Sample)(nil)

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
	mu              sync.Mutex
	buckets         map[int64]*dogstatsdColumnarBucket
	freeBuckets     []*dogstatsdColumnarBucket
	contextResolver *timestampContextResolver
	tagsStore       *tags.Store

	serieRowsScratch []metrics.SerieRow
	v3RowsScratch    []metrics.V3MetricPointRow

	insertedSamples uint64

	descriptorByKey        map[dogstatsdColumnarKey]int
	descriptorByCompactID  map[uint64]int
	compactIDRing          []uint64
	compactIDNext          int
	freeDescriptors        []int
	descriptorActive       []bool
	descriptorNonMonotonic []bool
	compactStates          []map[*metrics.DogStatsDCompactIdentityState]struct{}
	contextKeys            []ckey.ContextKey
	descriptorGenerations  []uint32
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
	compactHintSize     int
	tagFilterList       filterlist.TagMatcher
	shards              []dogstatsdColumnarShard
}

type dogstatsdColumnarStoreConfig struct {
	tagger        taggercomp.Component
	tagFilterList filterlist.TagMatcher
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

func newDogStatsDColumnarStore(interval int64, shardCount int, configs ...dogstatsdColumnarStoreConfig) *dogstatsdColumnarStore {
	if interval == 0 {
		interval = bucketSize
	}
	if shardCount <= 0 {
		shardCount = 1
	}

	var config dogstatsdColumnarStoreConfig
	if len(configs) > 0 {
		config = configs[0]
	}

	store := &dogstatsdColumnarStore{
		interval:            interval,
		descriptorExpiry:    columnarV3DescriptorExpirySeconds(),
		descriptorInterning: columnarV3DescriptorInterningEnabled(),
		compactHintSize:     columnarV3CompactHintSize(),
		tagFilterList:       config.tagFilterList,
		shards:              make([]dogstatsdColumnarShard, shardCount),
	}
	if store.descriptorExpiry > 0 && store.descriptorExpiry < store.interval {
		store.descriptorExpiry = store.interval
	}
	cfg := pkgconfigsetup.Datadog()
	contextExpireTime := cfg.GetInt64("dogstatsd_context_expiry_seconds")
	counterExpireTime := contextExpireTime + cfg.GetInt64("dogstatsd_expiry_seconds")
	useTagsStore := cfg.GetBool("aggregator_use_tags_store")
	for i := range store.shards {
		store.shards[i].buckets = make(map[int64]*dogstatsdColumnarBucket)
		store.shards[i].descriptorByKey = make(map[dogstatsdColumnarKey]int)
		if config.tagger != nil {
			id := fmt.Sprintf("columnar_v3 #%d", i)
			store.shards[i].tagsStore = tags.NewStore(useTagsStore, id)
			store.shards[i].contextResolver = newTimestampContextResolver(config.tagger, store.shards[i].tagsStore, id, contextExpireTime, counterExpireTime)
		}
		if store.descriptorInterning {
			store.shards[i].dictionary = newDogstatsdColumnarDictionary()
		}
	}
	return store
}

type dogstatsdColumnarWorker struct {
	store         *dogstatsdColumnarStore
	shardID       TimeSamplerID
	samplePool    *DogStatsDColumnarV3SamplePool
	tagFilterList filterlist.TagMatcher

	samplesChan       chan DogStatsDColumnarV3SampleBatch
	flushChan         chan dogstatsdColumnarFlushTrigger
	tagFilterListChan chan filterlist.TagMatcher
	stopChan          chan struct{}
}

type dogstatsdColumnarFlushTrigger struct {
	cutoffTime     int64
	forceFlushAll  bool
	rowSink        metrics.SerieRowSink
	v3PointRowSink metrics.V3MetricPointRowSink
	blockChan      chan uint64
}

func newDogStatsDColumnarWorker(store *dogstatsdColumnarStore, shardID TimeSamplerID, bufferSize int, samplePool *DogStatsDColumnarV3SamplePool, tagFilterList filterlist.TagMatcher) *dogstatsdColumnarWorker {
	return &dogstatsdColumnarWorker{
		store:             store,
		shardID:           shardID,
		samplePool:        samplePool,
		tagFilterList:     tagFilterList,
		samplesChan:       make(chan DogStatsDColumnarV3SampleBatch, bufferSize),
		flushChan:         make(chan dogstatsdColumnarFlushTrigger),
		tagFilterListChan: make(chan filterlist.TagMatcher),
		stopChan:          make(chan struct{}),
	}
}

func (w *dogstatsdColumnarWorker) run() {
	for {
		select {
		case <-w.stopChan:
			return
		case samples := <-w.samplesChan:
			w.processSamples(samples)
		case matcher := <-w.tagFilterListChan:
			w.setTagFilterList(matcher)
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
		w.store.insertAcceptedRowUnlocked(shardIdx, samples[i], timestamp, w.tagFilterList)
	}
	w.samplePool.PutBatch(samples)
}

func (w *dogstatsdColumnarWorker) setTagFilterList(matcher filterlist.TagMatcher) {
	w.tagFilterList = matcher
	if w.store == nil || int(w.shardID) >= len(w.store.shards) {
		return
	}
	resolver := w.store.shards[int(w.shardID)].contextResolver
	if resolver != nil {
		resolver.resolver.clearTagFilterCache()
	}
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
	s.insertAccepted(shardIdx, shardKey, 0, sample, timestamp)
	return true
}

func (s *dogstatsdColumnarStore) insertAccepted(shardIdx int, shardKey ckey.ContextKey, compactID uint64, sample metrics.MetricSample, timestamp float64) {
	row := NewDogStatsDColumnarV3SampleFromMetricSample(shardKey, compactID, nil, sample, true)
	s.insertAcceptedRow(shardIdx, row, timestamp)
}

func (s *dogstatsdColumnarStore) insertAcceptedRow(shardIdx int, row DogStatsDColumnarV3Sample, timestamp float64) {
	if s == nil || shardIdx < 0 || shardIdx >= len(s.shards) {
		return
	}
	shard := &s.shards[shardIdx]
	bucketStart := s.calculateBucketStart(timestamp)

	shard.mu.Lock()
	defer shard.mu.Unlock()
	s.insertAcceptedRowInShard(shard, bucketStart, row, int64(timestamp), s.tagFilterList)
}

func (s *dogstatsdColumnarStore) insertAcceptedRowUnlocked(shardIdx int, row DogStatsDColumnarV3Sample, timestamp float64, tagFilterList filterlist.TagMatcher) {
	if s == nil || shardIdx < 0 || shardIdx >= len(s.shards) {
		return
	}
	shard := &s.shards[shardIdx]
	bucketStart := s.calculateBucketStart(timestamp)
	s.insertAcceptedRowInShard(shard, bucketStart, row, int64(timestamp), tagFilterList)
}

func (s *dogstatsdColumnarStore) insertAcceptedInShard(shard *dogstatsdColumnarShard, bucketStart int64, shardKey ckey.ContextKey, compactID uint64, sample metrics.MetricSample) {
	row := NewDogStatsDColumnarV3SampleFromMetricSample(shardKey, compactID, nil, sample, true)
	s.insertAcceptedRowInShard(shard, bucketStart, row, bucketStart, s.tagFilterList)
}

func (s *dogstatsdColumnarStore) resolveRowContext(shard *dogstatsdColumnarShard, row DogStatsDColumnarV3Sample, timestamp int64, tagFilterList filterlist.TagMatcher) DogStatsDColumnarV3Sample {
	if shard == nil || shard.contextResolver == nil || !row.HasDescriptor {
		return row
	}

	contextKey := shard.contextResolver.trackContext(&row, timestamp, tagFilterList)
	context, ok := shard.contextResolver.get(contextKey)
	if !ok || context == nil {
		return row
	}

	row.ContextKey = contextKey
	row.Name = context.Name
	row.Host = context.Host
	row.Tags = context.Tags().UnsafeToReadOnlySliceString()
	row.NoIndex = context.noIndex
	row.Source = context.source
	return row
}

func (s *dogstatsdColumnarStore) insertAcceptedRowInShard(shard *dogstatsdColumnarShard, bucketStart int64, row DogStatsDColumnarV3Sample, timestamp int64, tagFilterList filterlist.TagMatcher) {
	row = s.resolveRowContext(shard, row, timestamp, tagFilterList)

	bucket := shard.buckets[bucketStart]
	if bucket == nil {
		bucket = shard.getBucket()
		shard.buckets[bucketStart] = bucket
	}

	key := dogstatsdColumnarKey{contextKey: row.ContextKey, mtype: row.Mtype}
	descriptorID, ok := shard.lookupDescriptorByRef(row.DescriptorID, row.DescriptorGeneration, key, row.HasDescriptorRef)
	if !ok {
		descriptorID, ok = shard.lookupDescriptorByCompactID(row.CompactID, key)
		if !ok {
			descriptorID, ok = shard.descriptorByKey[key]
			if !ok || !shard.descriptorActive[descriptorID] {
				if !row.HasDescriptor {
					row.CompactState.ClearColumnarDescriptorKnown(row.Mtype)
					recordDogstatsdColumnarFallback("compact_descriptor_missing")
					return
				}
				descriptorID = shard.appendDescriptorFromRow(key, row, s.descriptorInterning)
			}
			shard.rememberCompactDescriptor(row.CompactID, descriptorID, s.compactHintSize)
		}
	}
	if row.CompactID != 0 {
		row.CompactState.MarkColumnarDescriptorRef(row.Mtype, descriptorID, shard.descriptorGenerations[descriptorID])
		shard.rememberCompactState(descriptorID, row.CompactState)
	}
	shard.lastSeen[descriptorID] = bucketStart

	idx := shard.rowIndex(bucket, bucketStart, descriptorID)

	switch row.Mtype {
	case metrics.GaugeType:
		bucket.values[idx] = row.Value
		bucket.sampled[idx] = true
	case metrics.CounterType:
		sampleRate := row.SampleRate
		if sampleRate == 0 {
			sampleRate = 1
		}
		bucket.values[idx] += row.Value * (1 / sampleRate)
		bucket.sampled[idx] = true
	case metrics.CountType:
		bucket.values[idx] += row.Value
		bucket.sampled[idx] = true
	case metrics.SetType:
		if bucket.sets[idx] == nil {
			bucket.sets[idx] = make(map[string]struct{})
		}
		bucket.sets[idx][row.RawValue] = struct{}{}
		bucket.sampled[idx] = true
	}

	shard.insertedSamples++
}

func newDogstatsdColumnarBucket() *dogstatsdColumnarBucket {
	return &dogstatsdColumnarBucket{}
}

func (s *dogstatsdColumnarShard) getBucket() *dogstatsdColumnarBucket {
	if len(s.freeBuckets) == 0 {
		tlmDogstatsdColumnarStats.Inc("created_buckets")
		return newDogstatsdColumnarBucket()
	}
	last := len(s.freeBuckets) - 1
	bucket := s.freeBuckets[last]
	s.freeBuckets[last] = nil
	s.freeBuckets = s.freeBuckets[:last]
	tlmDogstatsdColumnarStats.Inc("reused_buckets")
	return bucket
}

func (s *dogstatsdColumnarShard) releaseBucket(bucket *dogstatsdColumnarBucket) {
	if bucket == nil {
		return
	}
	if bucket.byDescriptor != nil {
		for descriptorID := range bucket.byDescriptor {
			delete(bucket.byDescriptor, descriptorID)
		}
	}
	for i := range bucket.sets {
		bucket.sets[i] = nil
	}
	bucket.descriptors = bucket.descriptors[:0]
	bucket.values = bucket.values[:0]
	bucket.sampled = bucket.sampled[:0]
	bucket.sets = bucket.sets[:0]
	s.freeBuckets = append(s.freeBuckets, bucket)
}

func (s *dogstatsdColumnarShard) lookupDescriptorByRef(descriptorID int, generation uint32, key dogstatsdColumnarKey, hasRef bool) (int, bool) {
	if !hasRef || generation == 0 || descriptorID < 0 || descriptorID >= len(s.descriptorActive) {
		return 0, false
	}
	if s.descriptorActive[descriptorID] && s.descriptorGenerations[descriptorID] == generation && s.contextKeys[descriptorID] == key.contextKey && s.mtypes[descriptorID] == key.mtype {
		return descriptorID, true
	}
	return 0, false
}

func (s *dogstatsdColumnarShard) lookupDescriptorByCompactID(compactID uint64, key dogstatsdColumnarKey) (int, bool) {
	if compactID == 0 || s.descriptorByCompactID == nil {
		return 0, false
	}
	descriptorID, ok := s.descriptorByCompactID[compactID]
	if !ok {
		return 0, false
	}
	if descriptorID >= 0 && descriptorID < len(s.descriptorActive) && s.descriptorActive[descriptorID] && s.contextKeys[descriptorID] == key.contextKey && s.mtypes[descriptorID] == key.mtype {
		return descriptorID, true
	}
	delete(s.descriptorByCompactID, compactID)
	return 0, false
}

func (s *dogstatsdColumnarShard) rememberCompactDescriptor(compactID uint64, descriptorID int, maxSize int) {
	if compactID == 0 || maxSize <= 0 {
		return
	}
	if s.descriptorByCompactID == nil || len(s.compactIDRing) == 0 {
		s.descriptorByCompactID = make(map[uint64]int)
		s.compactIDRing = make([]uint64, maxSize)
	}
	if existing, ok := s.descriptorByCompactID[compactID]; ok {
		if existing != descriptorID {
			s.descriptorByCompactID[compactID] = descriptorID
		}
		return
	}
	evicted := s.compactIDRing[s.compactIDNext]
	if evicted != 0 {
		delete(s.descriptorByCompactID, evicted)
	}
	s.compactIDRing[s.compactIDNext] = compactID
	s.compactIDNext = (s.compactIDNext + 1) % len(s.compactIDRing)
	s.descriptorByCompactID[compactID] = descriptorID
}

func (s *dogstatsdColumnarShard) appendDescriptor(key dogstatsdColumnarKey, sample metrics.MetricSample, internDescriptorStrings bool) int {
	row := NewDogStatsDColumnarV3SampleFromMetricSample(key.contextKey, 0, nil, sample, true)
	return s.appendDescriptorFromRow(key, row, internDescriptorStrings)
}

func (s *dogstatsdColumnarShard) rememberCompactState(descriptorID int, state *metrics.DogStatsDCompactIdentityState) {
	if state == nil || descriptorID < 0 || descriptorID >= len(s.compactStates) {
		return
	}
	states := s.compactStates[descriptorID]
	if states == nil {
		states = make(map[*metrics.DogStatsDCompactIdentityState]struct{}, 1)
		s.compactStates[descriptorID] = states
	}
	states[state] = struct{}{}
}

func (s *dogstatsdColumnarShard) appendDescriptorFromRow(key dogstatsdColumnarKey, row DogStatsDColumnarV3Sample, internDescriptorStrings bool) int {
	idx := -1
	if len(s.freeDescriptors) > 0 {
		last := len(s.freeDescriptors) - 1
		idx = s.freeDescriptors[last]
		s.freeDescriptors = s.freeDescriptors[:last]
		tlmDogstatsdColumnarStats.Inc("reused_descriptor_slots")
	} else {
		idx = len(s.names)
		s.contextKeys = append(s.contextKeys, 0)
		s.descriptorGenerations = append(s.descriptorGenerations, 0)
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
		s.compactStates = append(s.compactStates, nil)
		s.flushSeen = append(s.flushSeen, 0)
		s.flushRow = append(s.flushRow, 0)
	}

	name := row.Name
	host := row.Host
	tags := cloneSortedTags(row.Tags)
	tagKey := ""
	unit := row.Unit
	if internDescriptorStrings {
		name = s.dictionary.internString(row.Name)
		host = s.dictionary.internString(row.Host)
		tags, tagKey = s.dictionary.internTags(row.Tags)
		unit = s.dictionary.internString(row.Unit)
	}

	s.descriptorGenerations[idx]++
	if s.descriptorGenerations[idx] == 0 {
		s.descriptorGenerations[idx] = 1
	}

	s.descriptorByKey[key] = idx
	s.descriptorActive[idx] = true
	s.descriptorNonMonotonic[idx] = false
	s.contextKeys[idx] = key.contextKey
	s.names[idx] = name
	s.hosts[idx] = host
	s.tags[idx] = tags
	s.tagKeys[idx] = tagKey
	s.mtypes[idx] = row.Mtype
	s.noIndex[idx] = row.NoIndex
	s.sources[idx] = row.Source
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
	shard.mu.Lock()
	defer shard.mu.Unlock()

	rows := shard.serieRowsScratch[:0]
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
		shard.releaseBucket(bucket)
		tlmDogstatsdColumnarStats.Inc("flushed_buckets")
	}
	s.expireDescriptors(shard, cutoffTime)
	s.expireContexts(shard, cutoffTime)

	for i := range rows {
		shadow.observeSerieRow(&rows[i])
		rowSink.AppendSerieRow(rows[i])
		rows[i] = metrics.SerieRow{}
	}
	rowCount := uint64(len(rows))
	shard.serieRowsScratch = rows[:0]
	return rowCount
}

func (s *dogstatsdColumnarStore) flushShardToV3MetricPointSink(shard *dogstatsdColumnarShard, cutoffTime int64, forceFlushAll bool, sink metrics.V3MetricPointRowSink, shadow *directRowShadowBuilder) uint64 {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	rows := shard.v3RowsScratch[:0]
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
		shard.releaseBucket(bucket)
		tlmDogstatsdColumnarStats.Inc("flushed_buckets")
	}
	s.expireDescriptors(shard, cutoffTime)
	s.expireContexts(shard, cutoffTime)

	for i := range rows {
		shadow.observeV3MetricPointRow(&rows[i])
		sink.AppendV3MetricPointRow(&rows[i])
		rows[i] = metrics.V3MetricPointRow{}
	}
	rowCount := uint64(len(rows))
	shard.v3RowsScratch = rows[:0]
	return rowCount
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

func (s *dogstatsdColumnarStore) expireContexts(shard *dogstatsdColumnarShard, cutoffTime int64) {
	if shard == nil || shard.contextResolver == nil {
		return
	}
	shard.contextResolver.expireContexts(cutoffTime)
	if shard.tagsStore != nil {
		shard.tagsStore.Shrink()
	}
}

func (s *dogstatsdColumnarShard) releaseDescriptor(descriptorID int) {
	mtype := s.mtypes[descriptorID]
	if descriptorID >= 0 && descriptorID < len(s.compactStates) {
		for state := range s.compactStates[descriptorID] {
			state.ClearColumnarDescriptorKnown(mtype)
		}
		s.compactStates[descriptorID] = nil
	}

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
