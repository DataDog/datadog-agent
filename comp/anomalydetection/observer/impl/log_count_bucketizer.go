// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const (
	defaultLogCountBucketSeconds    = int64(5)
	defaultLogCountIdleTTLSeconds   = int64(300)
	defaultLogCountRetentionSeconds = int64(600)
)

// LogCountBucketConfig controls materialization of fixed-width log count
// buckets. It applies only to .count metrics emitted by log extractors.
type LogCountBucketConfig struct {
	Enabled          bool
	BucketSeconds    int64
	IdleTTLSeconds   int64
	RetentionSeconds int64
}

// DefaultLogCountBucketConfig returns the evaluated five-second cadence. The
// feature remains disabled until explicitly enabled in Agent configuration.
func DefaultLogCountBucketConfig() LogCountBucketConfig {
	return LogCountBucketConfig{
		BucketSeconds:    defaultLogCountBucketSeconds,
		IdleTTLSeconds:   defaultLogCountIdleTTLSeconds,
		RetentionSeconds: defaultLogCountRetentionSeconds,
	}
}

type logCountBucketInterval struct {
	firstEnd int64
	lastEnd  int64
}

type logCountBucketSeries struct {
	namespace string
	name      string
	tags      []string
	context   *observerdef.MetricContext
	anchor    int64
	// lastObserved is the latest real log timestamp. Synthetic zero buckets do
	// not advance it, so storage can evict genuinely idle series first.
	lastObserved int64
	storageRef   observerdef.SeriesRef
	values       map[int64]float64
	intervals    []logCountBucketInterval
}

// materializedLogCountBucketizer incrementally turns sparse log occurrences
// into one stored count per completed bucket. Empty buckets are written as
// zero only after the series has been discovered and only through its idle TTL.
// It is confined to the engine's single dispatch goroutine.
type materializedLogCountBucketizer struct {
	config         LogCountBucketConfig
	series         map[uint64]*logCountBucketSeries
	flushedThrough int64
}

func newMaterializedLogCountBucketizer(config LogCountBucketConfig) *materializedLogCountBucketizer {
	defaults := DefaultLogCountBucketConfig()
	if config.BucketSeconds <= 0 {
		config.BucketSeconds = defaults.BucketSeconds
	}
	if config.IdleTTLSeconds < 0 {
		config.IdleTTLSeconds = defaults.IdleTTLSeconds
	}
	if config.RetentionSeconds < 0 {
		config.RetentionSeconds = defaults.RetentionSeconds
	}
	return &materializedLogCountBucketizer{
		config: config,
		series: make(map[uint64]*logCountBucketSeries),
	}
}

func (b *materializedLogCountBucketizer) handlesMetric(name string) bool {
	return strings.HasSuffix(name, ".count")
}

// observe adds one extractor output to its pending bucket. False means the
// bucket was already flushed and the late observation cannot be incorporated
// without rewriting history.
func (b *materializedLogCountBucketizer) observe(
	namespace string,
	metric observerdef.MetricOutput,
	timestamp int64,
	tags []string,
) bool {
	if timestamp <= b.flushedThrough {
		return false
	}
	key := seriesKeyHash(namespace, metric.Name, tags)
	state := b.series[key]
	if state == nil {
		state = &logCountBucketSeries{
			namespace:    namespace,
			name:         metric.Name,
			tags:         append([]string(nil), tags...),
			context:      metric.Context,
			anchor:       timestamp,
			lastObserved: timestamp,
			storageRef:   -1,
			values:       make(map[int64]float64),
		}
		b.series[key] = state
	} else {
		state.lastObserved = max(state.lastObserved, timestamp)
		if metric.Context != nil {
			state.context = metric.Context
		}
	}

	bucketEnd := logCountBucketEnd(timestamp, state.anchor, b.config.BucketSeconds)
	if bucketEnd <= b.flushedThrough {
		return false
	}
	state.values[bucketEnd] += metric.Value

	lastEnd := bucketEnd
	activeThrough := timestamp + b.config.IdleTTLSeconds
	if activeThrough >= bucketEnd {
		lastEnd += ((activeThrough - bucketEnd) / b.config.BucketSeconds) * b.config.BucketSeconds
	}
	state.intervals = mergeLogCountBucketInterval(
		state.intervals,
		logCountBucketInterval{firstEnd: bucketEnd, lastEnd: lastEnd},
		b.config.BucketSeconds,
	)
	return true
}

// flush writes every completed bucket through upTo into shared storage.
func (b *materializedLogCountBucketizer) flush(storage *timeSeriesStorage, upTo int64) {
	if upTo <= b.flushedThrough {
		return
	}
	for key, state := range b.series {
		remaining := state.intervals[:0]
		for _, interval := range state.intervals {
			nextEnd := interval.firstEnd
			for nextEnd <= interval.lastEnd && nextEnd <= upTo {
				result := storage.Add(
					state.namespace,
					state.name,
					state.values[nextEnd],
					nextEnd,
					state.tags,
				)
				if state.context != nil && result.Ref >= 0 {
					storage.SetContext(result.Ref, state.context)
				}
				if result.Ref >= 0 {
					state.storageRef = result.Ref
					storage.SetSupportedAggregations(result.Ref, observerdef.AggregateAverage)
					storage.SetSeriesRetention(result.Ref, b.config.RetentionSeconds)
					storage.SetSeriesActivityTimestamp(result.Ref, state.lastObserved)
				}
				delete(state.values, nextEnd)
				nextEnd += b.config.BucketSeconds
			}
			if nextEnd <= interval.lastEnd {
				interval.firstEnd = nextEnd
				remaining = append(remaining, interval)
			}
		}
		state.intervals = remaining
		if len(state.intervals) == 0 && len(state.values) == 0 {
			delete(b.series, key)
		}
	}
	b.flushedThrough = upTo
}

func (b *materializedLogCountBucketizer) removeMetricName(namespace, name string) {
	for key, state := range b.series {
		if state.namespace == namespace && state.name == name {
			delete(b.series, key)
		}
	}
}

func (b *materializedLogCountBucketizer) removeSeriesByHashes(hashes map[uint64]struct{}) {
	for hash := range hashes {
		delete(b.series, hash)
	}
}

func (b *materializedLogCountBucketizer) removeSeriesByRefs(refs []observerdef.SeriesRef) {
	if len(refs) == 0 {
		return
	}
	removed := make(map[observerdef.SeriesRef]struct{}, len(refs))
	for _, ref := range refs {
		removed[ref] = struct{}{}
	}
	for key, state := range b.series {
		if _, ok := removed[state.storageRef]; ok {
			delete(b.series, key)
		}
	}
}

func (b *materializedLogCountBucketizer) reset() {
	clear(b.series)
	b.flushedThrough = 0
}

func logCountBucketEnd(timestamp, anchor, bucketSeconds int64) int64 {
	return anchor + ((timestamp-anchor)/bucketSeconds+1)*bucketSeconds - 1
}

func mergeLogCountBucketInterval(
	intervals []logCountBucketInterval,
	candidate logCountBucketInterval,
	bucketSeconds int64,
) []logCountBucketInterval {
	insertAt := sort.Search(len(intervals), func(i int) bool {
		return intervals[i].lastEnd+bucketSeconds >= candidate.firstEnd
	})
	if insertAt == len(intervals) {
		return append(intervals, candidate)
	}
	if candidate.lastEnd+bucketSeconds < intervals[insertAt].firstEnd {
		intervals = append(intervals, logCountBucketInterval{})
		copy(intervals[insertAt+1:], intervals[insertAt:])
		intervals[insertAt] = candidate
		return intervals
	}

	first := min(candidate.firstEnd, intervals[insertAt].firstEnd)
	last := max(candidate.lastEnd, intervals[insertAt].lastEnd)
	mergeThrough := insertAt + 1
	for mergeThrough < len(intervals) && intervals[mergeThrough].firstEnd <= last+bucketSeconds {
		last = max(last, intervals[mergeThrough].lastEnd)
		mergeThrough++
	}
	intervals[insertAt] = logCountBucketInterval{firstEnd: first, lastEnd: last}
	return append(intervals[:insertAt+1], intervals[mergeThrough:]...)
}
