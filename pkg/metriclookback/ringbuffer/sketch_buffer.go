// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ringbuffer

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

var (
	tlmRingAppendedSketches    = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "ring_appended_sketches", []string{"source"}, "Count of sketch points appended to the metric lookback ring")
	tlmRingOverwrittenSketches = telemetryimpl.GetCompatComponent().NewSimpleCounter("metric_lookback", "ring_overwritten_sketches", "Count of sketch points overwritten in the metric lookback ring")
	tlmRingSketchRecords       = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_sketch_records", nil, "Current number of sketch records retained in the metric lookback ring")
	tlmRingSketchCapacity      = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "ring_sketch_capacity", nil, "Total sketch record capacity of the metric lookback ring")
)

// SketchBuffer is a bounded in-memory ring for recent finalized sketch points.
// It is intentionally separate from Buffer so scalar records keep their compact
// representation when distribution lookback is not used.
type SketchBuffer struct {
	now func() time.Time

	contexts *contextStore
	shards   []sketchShard

	nextSequence        atomic.Uint64
	appendedSketches    atomic.Uint64
	overwrittenSketches atomic.Uint64
	records             atomic.Int64
}

// NewSketchBuffer creates a bounded in-memory sketch buffer.
func NewSketchBuffer(options Options) *SketchBuffer {
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

	b := &SketchBuffer{
		now:      now,
		contexts: newContextStore(shardCount),
		shards:   make([]sketchShard, shardCount),
	}

	baseCapacity := capacity / shardCount
	extraSlots := capacity % shardCount
	for i := range b.shards {
		shardCapacity := baseCapacity
		if i < extraSlots {
			shardCapacity++
		}
		b.shards[i].records = make([]sketchRecord, shardCapacity)
	}
	tlmRingSketchCapacity.Set(float64(capacity))
	tlmRingSketchRecords.Set(0)

	return b
}

// AppendSketchSeries stores every sketch point in series as an already-finalized
// retained sketch point.
func (b *SketchBuffer) AppendSketchSeries(ctx context.Context, source Source, series *metrics.SketchSeries) error {
	if b == nil || series == nil || len(series.Points) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	hasSketch := false
	for i := range series.Points {
		if series.Points[i].Sketch != nil {
			hasSketch = true
			break
		}
	}
	if !hasSketch {
		return nil
	}

	for i := range series.Points {
		if err := ctx.Err(); err != nil {
			return err
		}
		point := series.Points[i]
		if point.Sketch == nil {
			continue
		}
		contextID, shardID := b.contexts.retainSketchSeries(source, series)
		timestampUnixMicro := sketchTimestampUnixMicro(point.Ts)
		if timestampUnixMicro == 0 {
			timestampUnixMicro = b.now().UnixMicro()
		}
		rec := sketchRecord{
			contextID:          contextID,
			timestampUnixMicro: timestampUnixMicro,
			sequence:           b.nextSequence.Add(1),
			sketch:             retainSketchData(point.Sketch),
		}
		b.appendRecord(source, shardID, rec)
	}

	return nil
}

func (b *SketchBuffer) appendRecord(source Source, shardID int, rec sketchRecord) {
	overwrittenContextID, overwritten := b.shards[shardID].append(rec)
	b.appendedSketches.Add(1)
	tlmRingAppendedSketches.Inc(string(source.Kind))
	if overwritten {
		b.overwrittenSketches.Add(1)
		tlmRingOverwrittenSketches.Inc()
		b.contexts.release(overwrittenContextID)
	} else {
		b.records.Add(1)
	}
	tlmRingSketchRecords.Set(float64(b.records.Load()))
}

// SketchSeriesBetween reconstructs retained sketch points whose original
// timestamps fall in the inclusive [from, to] window, ordered by append sequence.
func (b *SketchBuffer) SketchSeriesBetween(from, to time.Time) metrics.SketchSeriesList {
	if b == nil || invalidRange(from, to) {
		return nil
	}

	records := b.snapshotSortedRecords()
	if len(records) == 0 {
		return nil
	}
	contexts := b.contexts.snapshot()

	series := make(metrics.SketchSeriesList, 0, len(records))
	for i := range records {
		rec := &records[i]
		if !sketchRecordInRange(rec, from, to) {
			continue
		}
		ctx, found := contexts[rec.contextID]
		if !found || rec.sketch == nil {
			continue
		}
		series = append(series, &metrics.SketchSeries{
			DistributionMetadata: metrics.DistributionMetadata{
				Name:     ctx.name,
				Tags:     tagset.CompositeTagsFromSlice(append([]string(nil), ctx.tags...)),
				Host:     ctx.host,
				Interval: ctx.interval,
				NoIndex:  ctx.noIndex,
				Source:   ctx.metricSource,
			},
			Points: []metrics.SketchPoint{{
				Ts:     rec.timestampUnixMicro / 1e6,
				Sketch: cloneSketchData(rec.sketch),
			}},
		})
	}
	return series
}

// PointsBetweenSources projects retained sketch points for a metric from any of
// the provided sources into scalar points in the inclusive [from, to] window.
// A source with an empty ID matches every source with the same kind, which lets
// monitors read all shadow-check instances. The projection is deliberately
// supplied by the caller so the sketch retention layer does not own monitor
// semantics.
func (b *SketchBuffer) PointsBetweenSources(sources []Source, metricName string, from, to time.Time, project func(metrics.SketchPoint) (float64, bool)) []Point {
	if b == nil || len(sources) == 0 || metricName == "" || project == nil || invalidRange(from, to) {
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
		if !sketchRecordInRange(rec, from, to) {
			continue
		}
		ctx, found := contexts[rec.contextID]
		if !found || ctx.name != metricName {
			continue
		}
		if !sourceMatchesAny(sources, ctx.source) {
			continue
		}
		value, ok := project(metrics.SketchPoint{Ts: rec.timestampUnixMicro / 1e6, Sketch: rec.sketch})
		if !ok {
			continue
		}
		points = append(points, Point{
			Ts:    time.UnixMicro(rec.timestampUnixMicro),
			Value: value,
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

// SketchSourceBetween returns a metrics.SketchesSource over retained sketches.
func (b *SketchBuffer) SketchSourceBetween(from, to time.Time) metrics.SketchesSource {
	return &sketchSliceSource{series: b.SketchSeriesBetween(from, to), index: -1}
}

// Stats returns a point-in-time summary of the sketch buffer.
func (b *SketchBuffer) Stats() Stats {
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
	tlmRingSketchRecords.Set(float64(records))
	return Stats{
		Capacity:             capacity,
		ShardCount:           len(b.shards),
		Records:              records,
		ActiveContexts:       activeContexts,
		TotalContextsCreated: totalContextsCreated,
		AppendedSamples:      b.appendedSketches.Load(),
		OverwrittenSamples:   b.overwrittenSketches.Load(),
		OldestUnixMicro:      oldestUnixMicro,
		NewestUnixMicro:      newestUnixMicro,
	}
}

func (b *SketchBuffer) snapshotSortedRecords() []sketchRecord {
	var out []sketchRecord
	for i := range b.shards {
		out = b.shards[i].appendRecordsTo(out)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].sequence < out[j].sequence
	})
	return out
}

type sketchSliceSource struct {
	series metrics.SketchSeriesList
	index  int
}

func (s *sketchSliceSource) MoveNext() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *sketchSliceSource) Current() metrics.Distribution {
	return s.series[s.index]
}

func (s *sketchSliceSource) Count() uint64 {
	return uint64(len(s.series))
}

func (s *sketchSliceSource) WaitForValue() bool {
	return s.index+1 < len(s.series)
}

type sketchRecord struct {
	contextID          uint64
	timestampUnixMicro int64
	sequence           uint64
	sketch             *quantile.Sketch
}

type sketchShard struct {
	mu      sync.Mutex
	records []sketchRecord
	start   int
	length  int
	next    int
}

func (s *sketchShard) append(rec sketchRecord) (uint64, bool) {
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

func (s *sketchShard) appendRecordsTo(out []sketchRecord) []sketchRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < s.length; i++ {
		idx := (s.start + i) % len(s.records)
		out = append(out, s.records[idx])
	}
	return out
}

func (s *sketchShard) stats() (count, capacity int, oldestUnixMicro, newestUnixMicro int64) {
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

func (s *contextStore) retainSketchSeries(source Source, series *metrics.SketchSeries) (uint64, int) {
	rawTags := append([]string(nil), series.Tags.UnsafeToReadOnlySliceString()...)
	keyTags := canonicalTags(rawTags)
	key := buildSketchContextKey(source, series, keyTags)
	ctx := metricContext{
		source:       source,
		name:         series.Name,
		host:         series.Host,
		tags:         rawTags,
		interval:     series.Interval,
		noIndex:      series.NoIndex,
		metricSource: series.Source,
	}
	return s.retainContext(key, ctx)
}

func buildSketchContextKey(source Source, series *metrics.SketchSeries, tags []string) string {
	var builder strings.Builder
	appendContextKeyPrefix(&builder, source, series.Name, series.Host)
	appendIntField(&builder, series.Interval)
	appendBoolField(&builder, series.NoIndex)
	appendIntField(&builder, int64(series.Source))
	appendTagsField(&builder, tags)
	return builder.String()
}

func sketchTimestampUnixMicro(timestamp int64) int64 {
	if timestamp <= 0 {
		return 0
	}
	return timestamp * 1e6
}

func sketchRecordInRange(rec *sketchRecord, from, to time.Time) bool {
	if !from.IsZero() && rec.timestampUnixMicro < from.UnixMicro() {
		return false
	}
	if !to.IsZero() && rec.timestampUnixMicro > to.UnixMicro() {
		return false
	}
	return true
}

func retainSketchData(data *quantile.Sketch) *quantile.Sketch {
	if data == nil {
		return nil
	}
	return data.Copy()
}

func cloneSketchData(data *quantile.Sketch) *quantile.Sketch {
	return retainSketchData(data)
}
