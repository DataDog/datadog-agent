// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ringbuffer stores recent metric lookback observations in bounded
// in-memory rings. Scalar points and finalized sketch points use separate record
// layouts so the common scalar path stays compact while sharing the same source
// and context conventions.
package ringbuffer

import (
	"context"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

var (
	tlmRingAppendedSamples    = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "ring_appended_samples", []string{"source"}, "Count of samples appended to the metric lookback ring")
	tlmRingOverwrittenSamples = telemetryimpl.GetCompatComponent().NewSimpleCounter("metric_lookback", "ring_overwritten_samples", "Count of samples overwritten in the metric lookback ring")
	tlmRingRecords            = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_records", nil, "Current number of records retained in the metric lookback ring")
	tlmRingCapacity           = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_capacity", nil, "Total record capacity of the metric lookback ring")
	tlmRingOldestTimestamp    = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_oldest_timestamp", nil, "Oldest retained metric lookback timestamp, in Unix seconds")
	tlmRingNewestTimestamp    = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_newest_timestamp", nil, "Newest retained metric lookback timestamp, in Unix seconds")
)

const (
	// DefaultCapacity is the total number of samples retained when Options.Capacity
	// is not set.
	DefaultCapacity = 262_144

	// DefaultShardCount is the number of independent rings used when
	// Options.ShardCount is not set.
	DefaultShardCount = 16

	// checkSeriesSourceTypeName matches the normal check sampler's v1 source type
	// name for check-originated metric series. The numeric Source field still
	// carries the more specific source/origin metadata for v2 payloads.
	checkSeriesSourceTypeName = "System"

	// dogStatsDNoAggregationInterval matches the fixed aggregator bucket interval
	// used by the no-aggregation serializer for rate-like DogStatsD samples.
	// Keep this package independent from pkg/aggregator so the retention substrate
	// can remain outside the hot aggregation package.
	dogStatsDNoAggregationInterval = 10
)

// SourceKind identifies the retention producer. It is provenance for lookback
// accounting and source-specific egress semantics; it is not emitted as an extra
// backend metric tag.
type SourceKind string

const (
	// SourceDogStatsDBucketed identifies selected normal DogStatsD samples after
	// they have been materialized into short-width lookback buckets.
	SourceDogStatsDBucketed SourceKind = "dogstatsd_bucketed"
	// SourceDogStatsDNoAggregation identifies selected timestamped DogStatsD
	// samples that bypass local aggregation.
	SourceDogStatsDNoAggregation SourceKind = "dogstatsd_no_aggregation"
	// SourceCheck identifies samples emitted by a normal check path.
	SourceCheck SourceKind = "check"
	// SourceCheckShadow identifies samples emitted by a future lookback shadow
	// check path.
	SourceCheckShadow SourceKind = "check_shadow"
)

// Source describes the producer of retained points.
type Source struct {
	Kind SourceKind
	// ID is producer-specific: a check ID for check/shadow-check samples, or empty
	// for the first-stage selected DogStatsD no-aggregation stream.
	ID string
}

type sampleFlags uint8

const (
	flagFlushFirstValue sampleFlags = 1 << iota
)

// Options controls Buffer construction.
type Options struct {
	// Capacity is the total number of sample slots allocated across all shards.
	// Retention is shard-local: samples are assigned to shards by metric context,
	// so a hot shard can overwrite its oldest samples while other shards still
	// have unused slots. When zero or negative, DefaultCapacity is used.
	Capacity int

	// ShardCount is the number of independent rings. When zero or negative,
	// DefaultShardCount is used. Values larger than Capacity are clamped so every
	// shard has at least one slot.
	ShardCount int

	// Now supplies timestamps for samples without an explicit MetricSample
	// timestamp. When nil, time.Now is used.
	Now func() time.Time
}

// Stats describes the current state of a Buffer.
type Stats struct {
	Capacity             int
	ShardCount           int
	Records              int
	ActiveContexts       int
	TotalContextsCreated uint64
	AppendedSamples      uint64
	OverwrittenSamples   uint64
	OldestUnixMicro      int64
	NewestUnixMicro      int64
}

// Point is a retained scalar point returned to local readers such as monitors.
type Point struct {
	Ts    time.Time
	Value float64
	Tags  []string
}

// Buffer is a bounded in-memory ring for recent scalar metric samples.
type Buffer struct {
	now func() time.Time

	contexts *contextStore
	shards   []shard

	nextSequence       atomic.Uint64
	appendedSamples    atomic.Uint64
	overwrittenSamples atomic.Uint64
	records            atomic.Int64
	newestUnixMicro    atomic.Int64
}

// New creates a bounded in-memory ring buffer.
func New(options Options) *Buffer {
	capacity := options.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	shardCount := options.ShardCount
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	if shardCount > capacity {
		shardCount = capacity
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	b := &Buffer{
		now:      now,
		contexts: newContextStore(shardCount),
		shards:   make([]shard, shardCount),
	}

	baseCapacity := capacity / shardCount
	extraSlots := capacity % shardCount
	for i := range b.shards {
		shardCapacity := baseCapacity
		if i < extraSlots {
			shardCapacity++
		}
		b.shards[i].records = make([]record, shardCapacity)
	}
	tlmRingCapacity.Set(float64(capacity))
	tlmRingRecords.Set(0)

	return b
}

// AppendSamples stores scalar metric samples from the given source. If the
// context is cancelled before or during the call, AppendSamples returns the
// context error after retaining any samples already appended.
func (b *Buffer) AppendSamples(ctx context.Context, source Source, samples []metrics.MetricSample) error {
	if b == nil || len(samples) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	nowUnixMicro := int64(0)
	for i := range samples {
		if err := ctx.Err(); err != nil {
			return err
		}

		sample := samples[i]
		timestampUnixMicro := sampleTimestampUnixMicro(sample.Timestamp)
		if timestampUnixMicro == 0 {
			if nowUnixMicro == 0 {
				nowUnixMicro = b.now().UnixMicro()
			}
			timestampUnixMicro = nowUnixMicro
		}

		contextID, shardID := b.contexts.retain(source, sample)
		sequence := b.nextSequence.Add(1)
		rec := record{
			contextID:          contextID,
			timestampUnixMicro: timestampUnixMicro,
			sequence:           sequence,
			value:              recordValue(source, sample),
			sampleRate:         sample.SampleRate,
			flags:              flagsForSample(sample),
		}

		b.appendRecord(source, shardID, rec)
	}

	return nil
}

// AppendSerie stores every point in serie as already-normalized retained points.
// This is used by DogStatsD no-aggregation after the worker has constructed the
// same series it sends to the normal serializer, preserving tag enrichment and
// type/value mapping.
func (b *Buffer) AppendSerie(ctx context.Context, source Source, serie *metrics.Serie) error {
	if b == nil || serie == nil || len(serie.Points) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for i := range serie.Points {
		if err := ctx.Err(); err != nil {
			return err
		}
		contextID, shardID := b.contexts.retainSerie(source, serie)
		point := serie.Points[i]
		timestampUnixMicro := sampleTimestampUnixMicro(point.Ts)
		if timestampUnixMicro == 0 {
			timestampUnixMicro = b.now().UnixMicro()
		}
		rec := record{
			contextID:          contextID,
			timestampUnixMicro: timestampUnixMicro,
			sequence:           b.nextSequence.Add(1),
			value:              point.Value,
		}
		b.appendRecord(source, shardID, rec)
	}

	return nil
}

func (b *Buffer) appendRecord(source Source, shardID int, rec record) {
	overwrittenContextID, overwritten := b.shards[shardID].append(rec)
	b.appendedSamples.Add(1)
	tlmRingAppendedSamples.Inc(string(source.Kind))
	if overwritten {
		b.overwrittenSamples.Add(1)
		tlmRingOverwrittenSamples.Inc()
		b.contexts.release(overwrittenContextID)
	} else {
		b.records.Add(1)
	}
	b.setNewestUnixMicro(rec.timestampUnixMicro)
	tlmRingRecords.Set(float64(b.records.Load()))
	tlmRingNewestTimestamp.Set(float64(b.newestUnixMicro.Load()) / 1e6)
}

func (b *Buffer) setNewestUnixMicro(timestampUnixMicro int64) {
	for {
		current := b.newestUnixMicro.Load()
		if timestampUnixMicro <= current {
			return
		}
		if b.newestUnixMicro.CompareAndSwap(current, timestampUnixMicro) {
			return
		}
	}
}

// Series reconstructs every retained sample into a metrics.Series, ordered by
// the global append sequence.
func (b *Buffer) Series() metrics.Series {
	return b.SeriesBetween(time.Time{}, time.Time{})
}

// SeriesBetween reconstructs retained samples whose original timestamps fall in
// the inclusive [from, to] window, ordered by the global append sequence. A zero
// from or to leaves that side of the window unbounded. Each retained sample
// becomes a single-point serie carrying the canonicalized metric context (name,
// host, tags, source, unit, no-index) recorded at append time. This is a
// non-destructive snapshot: the buffer keeps its samples so forwarding can be
// retried or repeated.
func (b *Buffer) SeriesBetween(from, to time.Time) metrics.Series {
	if b == nil || invalidRange(from, to) {
		return nil
	}

	records := b.snapshotSortedRecords()
	if len(records) == 0 {
		return nil
	}
	contexts := b.contexts.snapshot()

	series := make(metrics.Series, 0, len(records))
	for i := range records {
		rec := &records[i]
		if !recordInRange(rec, from, to) {
			continue
		}
		ctx, found := contexts[rec.contextID]
		if !found {
			// The context was evicted between snapshots; skip the orphan record.
			continue
		}
		serie := &metrics.Serie{
			Name: ctx.name,
			Points: []metrics.Point{{
				Ts:    float64(rec.timestampUnixMicro) / 1e6,
				Value: rec.value,
			}},
			Tags:     tagset.CompositeTagsFromSlice(append([]string(nil), ctx.tags...)),
			Host:     ctx.host,
			MType:    ctx.apiType,
			Interval: ctx.interval,
		}
		if ctx.source.Kind != SourceDogStatsDNoAggregation {
			serie.NoIndex = ctx.noIndex
			serie.Source = ctx.metricSource
			serie.SourceTypeName = ctx.sourceTypeName
			serie.Unit = ctx.unit
		}
		series = append(series, serie)
	}
	return series
}

// PointsBetween returns retained points for a metric/source in the inclusive
// [from, to] window, ordered by point timestamp. A zero from or to leaves that
// side of the window unbounded. The returned points are a stable snapshot.
func (b *Buffer) PointsBetween(source Source, metricName string, from, to time.Time) []Point {
	return b.PointsBetweenSources([]Source{source}, metricName, from, to)
}

// PointsBetweenSources returns retained points for a metric from any of the
// provided sources in the inclusive [from, to] window, ordered by point
// timestamp. A zero from or to leaves that side of the window unbounded. The
// returned points are a stable snapshot. A source with an empty ID matches every
// source with the same kind, which lets monitors read all shadow-check instances.
func (b *Buffer) PointsBetweenSources(sources []Source, metricName string, from, to time.Time) []Point {
	if b == nil || len(sources) == 0 || metricName == "" || invalidRange(from, to) {
		return nil
	}

	records := b.snapshotSortedRecords()
	if len(records) == 0 {
		return nil
	}
	contexts := b.contexts.snapshot()

	points := make([]Point, 0)
	for i := range records {
		rec := &records[i]
		if !recordInRange(rec, from, to) {
			continue
		}
		ctx, found := contexts[rec.contextID]
		if !found || ctx.name != metricName {
			continue
		}
		if !sourceMatchesAny(sources, ctx.source) {
			continue
		}
		points = append(points, Point{
			Ts:    time.UnixMicro(rec.timestampUnixMicro),
			Value: rec.value,
			Tags:  append([]string(nil), ctx.tags...),
		})
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Ts.Equal(points[j].Ts) {
			return points[i].Value < points[j].Value
		}
		return points[i].Ts.Before(points[j].Ts)
	})
	return points
}

// SerieSource returns a metrics.SerieSource over all current retained samples,
// suitable for passing directly to serializer.SendIterableSeries. The snapshot
// is taken eagerly when SerieSource is called.
func (b *Buffer) SerieSource() metrics.SerieSource {
	return b.SerieSourceBetween(time.Time{}, time.Time{})
}

// SerieSourceBetween returns a metrics.SerieSource over retained samples whose
// original timestamps fall in the inclusive [from, to] window. A zero from or to
// leaves that side of the window unbounded. The snapshot is taken eagerly when
// SerieSourceBetween is called.
func (b *Buffer) SerieSourceBetween(from, to time.Time) metrics.SerieSource {
	return &serieSliceSource{series: b.SeriesBetween(from, to), index: -1}
}

func invalidRange(from, to time.Time) bool {
	return !from.IsZero() && !to.IsZero() && from.After(to)
}

func recordInRange(rec *record, from, to time.Time) bool {
	if !from.IsZero() && rec.timestampUnixMicro < from.UnixMicro() {
		return false
	}
	if !to.IsZero() && rec.timestampUnixMicro > to.UnixMicro() {
		return false
	}
	return true
}

// snapshotSortedRecords returns a copy of every retained record ordered by the
// global append sequence.
func (b *Buffer) snapshotSortedRecords() []record {
	var out []record
	for i := range b.shards {
		out = b.shards[i].appendRecordsTo(out)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].sequence < out[j].sequence
	})
	return out
}

// serieSliceSource is a metrics.SerieSource backed by a fixed slice of series.
type serieSliceSource struct {
	series metrics.Series
	index  int
}

func (s *serieSliceSource) MoveNext() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *serieSliceSource) Current() *metrics.Serie {
	return s.series[s.index]
}

func (s *serieSliceSource) Count() uint64 {
	return uint64(len(s.series))
}

func sourceMatchesAny(queries []Source, actual Source) bool {
	for _, query := range queries {
		if query == actual || (query.ID == "" && query.Kind == actual.Kind) {
			return true
		}
	}
	return false
}

func recordValue(source Source, sample metrics.MetricSample) float64 {
	if source.Kind == SourceDogStatsDNoAggregation && isDogStatsDNoAggRate(sample.Mtype) {
		return sample.Value / dogStatsDNoAggregationInterval
	}
	return sample.Value
}

func apiMetricType(source Source, mtype metrics.MetricType) metrics.APIMetricType {
	if source.Kind == SourceDogStatsDNoAggregation {
		switch mtype {
		case metrics.CounterType, metrics.RateType:
			return metrics.APIRateType
		case metrics.CountType, metrics.CountWithTimestampType:
			return metrics.APICountType
		default:
			return metrics.APIGaugeType
		}
	}

	switch mtype {
	case metrics.CountType, metrics.CounterType, metrics.CountWithTimestampType:
		return metrics.APICountType
	default:
		// GaugeType, GaugeWithTimestampType, RateType, MonotonicCountType,
		// HistogramType, HistorateType, and unsupported/non-scalar types.
		return metrics.APIGaugeType
	}
}

func apiInterval(source Source, _ metrics.MetricType) int64 {
	if source.Kind == SourceDogStatsDNoAggregation {
		return dogStatsDNoAggregationInterval
	}
	return 0
}

func isDogStatsDNoAggRate(mtype metrics.MetricType) bool {
	return mtype == metrics.CounterType || mtype == metrics.RateType
}

func sourceTypeName(source Source) string {
	switch source.Kind {
	case SourceCheck, SourceCheckShadow:
		return checkSeriesSourceTypeName
	default:
		return ""
	}
}

// Stats returns a point-in-time summary of the buffer.
func (b *Buffer) Stats() Stats {
	if b == nil {
		return Stats{}
	}

	records := 0
	capacity := 0
	oldestUnixMicro := int64(0)
	newestUnixMicro := int64(0)
	for i := range b.shards {
		recordCount, shardCapacity, shardOldest, shardNewest := b.shards[i].stats()
		records += recordCount
		capacity += shardCapacity
		if shardOldest > 0 && (oldestUnixMicro == 0 || shardOldest < oldestUnixMicro) {
			oldestUnixMicro = shardOldest
		}
		if shardNewest > newestUnixMicro {
			newestUnixMicro = shardNewest
		}
	}

	activeContexts, totalContextsCreated := b.contexts.stats()
	tlmRingCapacity.Set(float64(capacity))
	tlmRingRecords.Set(float64(records))
	tlmRingOldestTimestamp.Set(float64(oldestUnixMicro) / 1e6)
	tlmRingNewestTimestamp.Set(float64(newestUnixMicro) / 1e6)
	return Stats{
		Capacity:             capacity,
		ShardCount:           len(b.shards),
		Records:              records,
		ActiveContexts:       activeContexts,
		TotalContextsCreated: totalContextsCreated,
		AppendedSamples:      b.appendedSamples.Load(),
		OverwrittenSamples:   b.overwrittenSamples.Load(),
		OldestUnixMicro:      oldestUnixMicro,
		NewestUnixMicro:      newestUnixMicro,
	}
}

type record struct {
	contextID          uint64
	timestampUnixMicro int64
	sequence           uint64
	value              float64
	sampleRate         float64
	flags              sampleFlags
}

type shard struct {
	mu      sync.Mutex
	records []record
	start   int
	length  int
	next    int
}

func (s *shard) append(rec record) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.records) == 0 {
		return 0, false
	}

	overwrittenContextID := uint64(0)
	overwritten := s.length == len(s.records)
	if overwritten {
		overwrittenContextID = s.records[s.next].contextID
	} else {
		s.length++
	}

	s.records[s.next] = rec
	s.next = (s.next + 1) % len(s.records)
	if overwritten {
		s.start = s.next
	}

	return overwrittenContextID, overwritten
}

func (s *shard) stats() (count, capacity int, oldestUnixMicro, newestUnixMicro int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < s.length; i++ {
		idx := (s.start + i) % len(s.records)
		timestamp := s.records[idx].timestampUnixMicro
		if timestamp > 0 && (oldestUnixMicro == 0 || timestamp < oldestUnixMicro) {
			oldestUnixMicro = timestamp
		}
		if timestamp > newestUnixMicro {
			newestUnixMicro = timestamp
		}
	}
	return s.length, len(s.records), oldestUnixMicro, newestUnixMicro
}

// appendRecordsTo appends the shard's live records, oldest first, to out.
func (s *shard) appendRecordsTo(out []record) []record {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < s.length; i++ {
		idx := (s.start + i) % len(s.records)
		out = append(out, s.records[idx])
	}
	return out
}

type metricContext struct {
	id             uint64
	source         Source
	name           string
	host           string
	tags           []string
	mtype          metrics.MetricType
	apiType        metrics.APIMetricType
	interval       int64
	noIndex        bool
	metricSource   metrics.MetricSource
	sourceTypeName string
	unit           string
	shardID        int
}

type contextEntry struct {
	key  string
	ctx  metricContext
	refs int
}

type contextStore struct {
	mu                   sync.Mutex
	shardCount           int
	nextID               uint64
	byKey                map[string]*contextEntry
	byID                 map[uint64]*contextEntry
	totalContextsCreated uint64
}

func newContextStore(shardCount int) *contextStore {
	return &contextStore{
		shardCount: shardCount,
		nextID:     1,
		byKey:      make(map[string]*contextEntry),
		byID:       make(map[uint64]*contextEntry),
	}
}

func (s *contextStore) retain(source Source, sample metrics.MetricSample) (uint64, int) {
	tags := canonicalTags(sample.Tags)
	key := buildContextKey(source, sample, tags)
	ctx := metricContext{
		source:         source,
		name:           sample.Name,
		host:           sample.Host,
		tags:           tags,
		mtype:          sample.Mtype,
		apiType:        apiMetricType(source, sample.Mtype),
		interval:       apiInterval(source, sample.Mtype),
		noIndex:        sample.NoIndex,
		metricSource:   sample.Source,
		sourceTypeName: sourceTypeName(source),
		unit:           sample.Unit,
	}
	return s.retainContext(key, ctx)
}

func (s *contextStore) retainSerie(source Source, serie *metrics.Serie) (uint64, int) {
	rawTags := append([]string(nil), serie.Tags.UnsafeToReadOnlySliceString()...)
	keyTags := canonicalTags(rawTags)
	key := buildSerieContextKey(source, serie, keyTags)
	ctx := metricContext{
		source:         source,
		name:           serie.Name,
		host:           serie.Host,
		tags:           rawTags,
		apiType:        serie.MType,
		interval:       serie.Interval,
		noIndex:        serie.NoIndex,
		metricSource:   serie.Source,
		sourceTypeName: serie.SourceTypeName,
		unit:           serie.Unit,
	}
	return s.retainContext(key, ctx)
}

func (s *contextStore) retainContext(key string, ctx metricContext) (uint64, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, found := s.byKey[key]; found {
		entry.refs++
		return entry.ctx.id, entry.ctx.shardID
	}

	contextID := s.nextID
	s.nextID++
	shardID := shardIndex(hashString(key), s.shardCount)
	ctx.id = contextID
	ctx.shardID = shardID
	entry := &contextEntry{
		key:  key,
		ctx:  ctx,
		refs: 1,
	}
	s.byKey[key] = entry
	s.byID[contextID] = entry
	s.totalContextsCreated++
	return contextID, shardID
}

func (s *contextStore) release(contextID uint64) {
	if contextID == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, found := s.byID[contextID]
	if !found {
		return
	}
	entry.refs--
	if entry.refs > 0 {
		return
	}
	delete(s.byID, contextID)
	delete(s.byKey, entry.key)
}

func (s *contextStore) stats() (int, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID), s.totalContextsCreated
}

// snapshot returns a copy of the active metric contexts keyed by context ID.
// Tag slices are cloned so callers can retain them safely.
func (s *contextStore) snapshot() map[uint64]metricContext {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[uint64]metricContext, len(s.byID))
	for id, entry := range s.byID {
		ctx := entry.ctx
		ctx.tags = append([]string(nil), entry.ctx.tags...)
		out[id] = ctx
	}
	return out
}

func sampleTimestampUnixMicro(timestamp float64) int64 {
	if timestamp <= 0 {
		return 0
	}
	return int64(timestamp * 1e6)
}

func flagsForSample(sample metrics.MetricSample) sampleFlags {
	var flags sampleFlags
	if sample.FlushFirstValue {
		flags |= flagFlushFirstValue
	}
	return flags
}

func canonicalTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	out := append([]string(nil), tags...)
	sort.Strings(out)
	return dedupeSortedStrings(out)
}

func dedupeSortedStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}

	writeIdx := 0
	for readIdx := 1; readIdx < len(values); readIdx++ {
		if values[readIdx] == values[writeIdx] {
			continue
		}
		writeIdx++
		values[writeIdx] = values[readIdx]
	}
	return values[:writeIdx+1]
}

func buildContextKey(source Source, sample metrics.MetricSample, tags []string) string {
	var builder strings.Builder
	appendContextKeyPrefix(&builder, source, sample.Name, sample.Host)
	appendIntField(&builder, int64(sample.Mtype))
	appendBoolField(&builder, sample.NoIndex)
	appendIntField(&builder, int64(sample.Source))
	appendStringField(&builder, sample.Unit)
	appendTagsField(&builder, tags)
	return builder.String()
}

func buildSerieContextKey(source Source, serie *metrics.Serie, tags []string) string {
	var builder strings.Builder
	appendContextKeyPrefix(&builder, source, serie.Name, serie.Host)
	appendIntField(&builder, int64(serie.MType))
	appendIntField(&builder, serie.Interval)
	appendBoolField(&builder, serie.NoIndex)
	appendIntField(&builder, int64(serie.Source))
	appendStringField(&builder, serie.SourceTypeName)
	appendStringField(&builder, serie.Unit)
	appendTagsField(&builder, tags)
	return builder.String()
}

func appendContextKeyPrefix(builder *strings.Builder, source Source, name, host string) {
	appendStringField(builder, string(source.Kind))
	appendStringField(builder, source.ID)
	appendStringField(builder, name)
	appendStringField(builder, host)
}

func appendTagsField(builder *strings.Builder, tags []string) {
	appendIntField(builder, int64(len(tags)))
	for _, tag := range tags {
		appendStringField(builder, tag)
	}
}

func appendStringField(builder *strings.Builder, value string) {
	builder.WriteString(strconv.Itoa(len(value)))
	builder.WriteByte(':')
	builder.WriteString(value)
	builder.WriteByte('|')
}

func appendIntField(builder *strings.Builder, value int64) {
	builder.WriteString(strconv.FormatInt(value, 10))
	builder.WriteByte('|')
}

func appendBoolField(builder *strings.Builder, value bool) {
	if value {
		builder.WriteByte('1')
	} else {
		builder.WriteByte('0')
	}
	builder.WriteByte('|')
}

func hashString(value string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return h.Sum64()
}

func shardIndex(hash uint64, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}
	return int(hash % uint64(shardCount))
}
