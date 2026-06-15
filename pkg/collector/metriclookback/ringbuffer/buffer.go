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
	"go.uber.org/atomic"
)

const (
	// DefaultCapacity is the total number of samples retained when Options.Capacity
	// is not set.
	DefaultCapacity = 262_144

	// DefaultShardCount is the number of independent rings used when
	// Options.ShardCount is not set.
	DefaultShardCount = 16
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
