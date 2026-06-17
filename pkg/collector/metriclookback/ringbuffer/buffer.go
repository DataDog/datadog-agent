// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ringbuffer stores recent scalar check metric samples in a bounded
// in-memory ring. It is intended to satisfy the metric lookback sender Writer
// API without exposing a query surface yet.
package ringbuffer

import (
	"context"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"go.uber.org/atomic"
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
)

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
}

// Buffer is a bounded in-memory ring for recent check metric samples.
type Buffer struct {
	now func() time.Time

	contexts *contextStore
	shards   []shard

	nextSequence       *atomic.Uint64
	appendedSamples    *atomic.Uint64
	overwrittenSamples *atomic.Uint64
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
		now:                now,
		contexts:           newContextStore(shardCount),
		shards:             make([]shard, shardCount),
		nextSequence:       atomic.NewUint64(0),
		appendedSamples:    atomic.NewUint64(0),
		overwrittenSamples: atomic.NewUint64(0),
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

	return b
}

// Append stores scalar metric samples emitted by a check. It implements the
// lookbacksender.Writer shape. If the context is cancelled before or during the
// call, Append returns the context error after retaining any samples already
// appended.
func (b *Buffer) Append(ctx context.Context, checkID checkid.ID, samples []metrics.MetricSample) error {
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

		contextID, shardID := b.contexts.retain(checkID, sample)
		sequence := b.nextSequence.Inc()
		rec := record{
			contextID:          contextID,
			timestampUnixMicro: timestampUnixMicro,
			sequence:           sequence,
			value:              sample.Value,
			sampleRate:         sample.SampleRate,
			flags:              flagsForSample(sample),
		}

		overwrittenContextID, overwritten := b.shards[shardID].append(rec)
		b.appendedSamples.Inc()
		if overwritten {
			b.overwrittenSamples.Inc()
			b.contexts.release(overwrittenContextID)
		}
	}

	return nil
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
// non-destructive snapshot: the buffer keeps its samples so a dump can be
// retried or repeated.
//
// MetricType is mapped to the API metric type according to the lookback raw
// scalar semantics: count-like sender submissions stay counts, while rate,
// monotonic-count, histogram, and historate submissions are emitted as gauges
// because the dump intentionally does not compute backend rates, monotonic
// deltas, or histogram rollups. Raw retained values are sent as-is at their
// original timestamps.
func (b *Buffer) SeriesBetween(from, to time.Time) metrics.Series {
	if b == nil || invalidRange(from, to) {
		return nil
	}

	// Collect a stable snapshot of records and the contexts they reference.
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
			Tags:           tagset.CompositeTagsFromSlice(append([]string(nil), ctx.tags...)),
			Host:           ctx.host,
			MType:          apiMetricType(ctx.mtype),
			NoIndex:        ctx.noIndex,
			Source:         ctx.source,
			SourceTypeName: checkSeriesSourceTypeName,
			Unit:           ctx.unit,
		}
		series = append(series, serie)
	}
	return series
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

// apiMetricType maps an aggregator MetricType to the API metric type used in
// serialized series, on a best-effort basis for lookback dumps.
func apiMetricType(mtype metrics.MetricType) metrics.APIMetricType {
	switch mtype {
	case metrics.CountType, metrics.CounterType, metrics.CountWithTimestampType:
		return metrics.APICountType
	default:
		// GaugeType, GaugeWithTimestampType, RateType, MonotonicCountType,
		// HistogramType, HistorateType, and unsupported/non-scalar types.
		return metrics.APIGaugeType
	}
}

// Stats returns a point-in-time summary of the buffer.
func (b *Buffer) Stats() Stats {
	if b == nil {
		return Stats{}
	}

	records := 0
	capacity := 0
	for i := range b.shards {
		recordCount, shardCapacity := b.shards[i].stats()
		records += recordCount
		capacity += shardCapacity
	}

	activeContexts, totalContextsCreated := b.contexts.stats()
	return Stats{
		Capacity:             capacity,
		ShardCount:           len(b.shards),
		Records:              records,
		ActiveContexts:       activeContexts,
		TotalContextsCreated: totalContextsCreated,
		AppendedSamples:      b.appendedSamples.Load(),
		OverwrittenSamples:   b.overwrittenSamples.Load(),
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

func (s *shard) stats() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.length, len(s.records)
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
	id      uint64
	checkID checkid.ID
	name    string
	host    string
	tags    []string
	mtype   metrics.MetricType
	noIndex bool
	source  metrics.MetricSource
	unit    string
	shardID int
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

func (s *contextStore) retain(checkID checkid.ID, sample metrics.MetricSample) (uint64, int) {
	tags := canonicalTags(sample.Tags)
	key := buildContextKey(checkID, sample, tags)

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, found := s.byKey[key]; found {
		entry.refs++
		return entry.ctx.id, entry.ctx.shardID
	}

	contextID := s.nextID
	s.nextID++
	shardID := shardIndex(hashString(key), s.shardCount)
	entry := &contextEntry{
		key: key,
		ctx: metricContext{
			id:      contextID,
			checkID: checkID,
			name:    sample.Name,
			host:    sample.Host,
			tags:    tags,
			mtype:   sample.Mtype,
			noIndex: sample.NoIndex,
			source:  sample.Source,
			unit:    sample.Unit,
			shardID: shardID,
		},
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

func buildContextKey(checkID checkid.ID, sample metrics.MetricSample, tags []string) string {
	var builder strings.Builder
	appendStringField(&builder, string(checkID))
	appendStringField(&builder, sample.Name)
	appendStringField(&builder, sample.Host)
	appendIntField(&builder, int64(sample.Mtype))
	appendBoolField(&builder, sample.NoIndex)
	appendIntField(&builder, int64(sample.Source))
	appendStringField(&builder, sample.Unit)
	appendIntField(&builder, int64(len(tags)))
	for _, tag := range tags {
		appendStringField(&builder, tag)
	}
	return builder.String()
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
