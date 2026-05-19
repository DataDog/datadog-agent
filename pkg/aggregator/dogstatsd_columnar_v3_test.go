// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type columnarRowCaptureSink struct {
	rows []metrics.SerieRow
}

func (s *columnarRowCaptureSink) AppendSerieRow(row metrics.SerieRow) {
	cloned := row
	cloned.Points = append([]metrics.Point(nil), row.Points...)
	cloned.Resources = append([]metrics.Resource(nil), row.Resources...)
	if tags := row.Tags.UnsafeToReadOnlySliceString(); tags != nil {
		cloned.Tags = row.Tags
	}
	s.rows = append(s.rows, cloned)
}

type columnarPointRowCaptureSink struct {
	rows []metrics.V3MetricPointRow
}

func (s *columnarPointRowCaptureSink) AppendV3MetricPointRow(row metrics.V3MetricPointRow) {
	cloned := row
	cloned.Resources = append([]metrics.Resource(nil), row.Resources...)
	if tags := row.Tags.UnsafeToReadOnlySliceString(); tags != nil {
		cloned.Tags = row.Tags
	}
	s.rows = append(s.rows, cloned)
}

func TestDogstatsdColumnarV3InsertAndFlushRows(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 2)

	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "gauge.metric",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"z:last", "a:first", "a:first"},
		Host:       "host-a",
		SampleRate: 1,
	}, 101))
	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "gauge.metric",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"z:last", "a:first"},
		Host:       "host-a",
		SampleRate: 1,
	}, 102))
	require.True(t, store.insert(ckey.ContextKey(2), metrics.MetricSample{
		Name:       "counter.metric",
		Value:      3,
		Mtype:      metrics.CounterType,
		Tags:       []string{"env:test"},
		SampleRate: 0.5,
	}, 103))
	require.True(t, store.insert(ckey.ContextKey(3), metrics.MetricSample{
		Name:       "count.metric",
		Value:      4,
		Mtype:      metrics.CountType,
		SampleRate: 1,
	}, 104))
	require.True(t, store.insert(ckey.ContextKey(4), metrics.MetricSample{
		Name:       "set.metric",
		RawValue:   "a",
		Mtype:      metrics.SetType,
		SampleRate: 1,
	}, 105))
	require.True(t, store.insert(ckey.ContextKey(4), metrics.MetricSample{
		Name:       "set.metric",
		RawValue:   "a",
		Mtype:      metrics.SetType,
		SampleRate: 1,
	}, 106))
	require.True(t, store.insert(ckey.ContextKey(4), metrics.MetricSample{
		Name:       "set.metric",
		RawValue:   "b",
		Mtype:      metrics.SetType,
		SampleRate: 1,
	}, 107))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(4), store.flush(111, false, &sink))
	require.Len(t, sink.rows, 4)

	rowsByName := map[string]metrics.SerieRow{}
	for _, row := range sink.rows {
		rowsByName[row.Name] = row
	}

	gauge := rowsByName["gauge.metric"]
	require.Equal(t, metrics.APIGaugeType, gauge.MType)
	require.Equal(t, "host-a", gauge.Host)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 2}}, gauge.Points)
	require.Equal(t, []string{"a:first", "z:last"}, gauge.Tags.UnsafeToReadOnlySliceString())

	counter := rowsByName["counter.metric"]
	require.Equal(t, metrics.APIRateType, counter.MType)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 0.6}}, counter.Points)

	count := rowsByName["count.metric"]
	require.Equal(t, metrics.APICountType, count.MType)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 4}}, count.Points)

	set := rowsByName["set.metric"]
	require.Equal(t, metrics.APIGaugeType, set.MType)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 2}}, set.Points)

	sink.rows = nil
	require.Equal(t, uint64(0), store.flush(121, false, &sink))
	require.Empty(t, sink.rows)
}

func TestDogstatsdColumnarV3FlushMergesPointsAcrossBuckets(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	sample := metrics.MetricSample{
		Name:       "gauge.metric",
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	}
	sample.Value = 1
	require.True(t, store.insert(ckey.ContextKey(1), sample, 101))
	sample.Value = 2
	require.True(t, store.insert(ckey.ContextKey(1), sample, 112))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(125, false, &sink))
	require.Len(t, sink.rows, 1)
	require.ElementsMatch(t, []metrics.Point{{Ts: 100, Value: 1}, {Ts: 110, Value: 2}}, sink.rows[0].Points)
}

func TestDogstatsdColumnarV3FlushNativePointRows(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "gauge.metric",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"z:last", "a:first", "a:first"},
		Host:       "host-a",
		SampleRate: 1,
	}, 101))
	require.True(t, store.insert(ckey.ContextKey(2), metrics.MetricSample{
		Name:       "counter.metric",
		Value:      3,
		Mtype:      metrics.CounterType,
		Tags:       []string{"env:test"},
		SampleRate: 0.5,
	}, 103))

	var sink columnarPointRowCaptureSink
	shadow := newDirectRowShadowBuilder()
	require.Equal(t, uint64(2), store.flushShardToV3MetricPointSink(&store.shards[0], 111, false, &sink, shadow))
	require.Len(t, sink.rows, 2)

	rowsByName := map[string]metrics.V3MetricPointRow{}
	for _, row := range sink.rows {
		rowsByName[row.Name] = row
	}

	gauge := rowsByName["gauge.metric"]
	require.Equal(t, metrics.APIGaugeType, gauge.MType)
	require.Equal(t, "host-a", gauge.Host)
	require.Equal(t, int64(100), gauge.Timestamp)
	require.Equal(t, float64(1), gauge.Value)
	require.Equal(t, []string{"a:first", "z:last"}, gauge.Tags.UnsafeToReadOnlySliceString())

	counter := rowsByName["counter.metric"]
	require.Equal(t, metrics.APIRateType, counter.MType)
	require.Equal(t, int64(100), counter.Timestamp)
	require.Equal(t, 0.6, counter.Value)
}

func TestDogstatsdColumnarV3FlushNativePointRowsMergesPointsAcrossBuckets(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	sample := metrics.MetricSample{
		Name:       "gauge.metric",
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	}
	sample.Value = 1
	require.True(t, store.insert(ckey.ContextKey(1), sample, 101))
	sample.Value = 2
	require.True(t, store.insert(ckey.ContextKey(1), sample, 112))

	var sink columnarPointRowCaptureSink
	shadow := newDirectRowShadowBuilder()
	require.Equal(t, uint64(1), store.flushShardToV3MetricPointSink(&store.shards[0], 125, false, &sink, shadow))
	require.Len(t, sink.rows, 1)
	require.Equal(t, map[int64]float64{100: 1, 110: 2}, nativePointsByTimestamp(sink.rows[0]))
}

func nativePointsByTimestamp(row metrics.V3MetricPointRow) map[int64]float64 {
	points := make(map[int64]float64, row.NumPoints())
	if len(row.Values) == 0 {
		points[row.Timestamp] = row.Value
		return points
	}
	for i, value := range row.Values {
		points[row.Timestamps[i]] = value
	}
	return points
}

func TestDogstatsdColumnarV3UnsupportedSamplesFallback(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)

	require.False(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "histogram.metric",
		Value:      1,
		Mtype:      metrics.HistogramType,
		SampleRate: 1,
	}, 101))
	require.False(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "late.metric",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Timestamp:  123,
		SampleRate: 1,
	}, 101))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(0), store.flush(111, false, &sink))
}

func TestDogstatsdColumnarV3KeepsOpenBuckets(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "gauge.metric",
		Value:      1,
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	}, 101))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(0), store.flush(109, false, &sink))
	require.Empty(t, sink.rows)
	require.Equal(t, uint64(1), store.flush(109, true, &sink))
	require.Len(t, sink.rows, 1)
}

func TestDogstatsdColumnarV3UsesDescriptorRowCacheForMonotonicBuckets(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	sample := metrics.MetricSample{Name: "gauge.metric", Mtype: metrics.GaugeType, SampleRate: 1}

	sample.Value = 1
	require.True(t, store.insert(ckey.ContextKey(1), sample, 101))
	sample.Value = 2
	require.True(t, store.insert(ckey.ContextKey(1), sample, 102))
	sample.Value = 3
	require.True(t, store.insert(ckey.ContextKey(1), sample, 112))

	for _, bucket := range store.shards[0].buckets {
		require.Nil(t, bucket.byDescriptor)
	}
	require.Len(t, store.shards[0].buckets[100].descriptors, 1)
	require.Len(t, store.shards[0].buckets[110].descriptors, 1)
}

func TestDogstatsdColumnarV3BuildsBucketIndexOnlyForNonMonotonicBuckets(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	sample := metrics.MetricSample{Name: "gauge.metric", Mtype: metrics.GaugeType, SampleRate: 1}

	sample.Value = 1
	require.True(t, store.insert(ckey.ContextKey(1), sample, 112))
	sample.Value = 2
	require.True(t, store.insert(ckey.ContextKey(1), sample, 101))
	sample.Value = 3
	require.True(t, store.insert(ckey.ContextKey(1), sample, 113))

	require.NotNil(t, store.shards[0].buckets[100].byDescriptor)
	require.NotNil(t, store.shards[0].buckets[110].byDescriptor)
	require.Len(t, store.shards[0].buckets[110].descriptors, 1)

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(125, false, &sink))
	require.Len(t, sink.rows, 1)
	require.ElementsMatch(t, []metrics.Point{{Ts: 100, Value: 2}, {Ts: 110, Value: 3}}, sink.rows[0].Points)
}

func TestDogstatsdColumnarV3InternsAndExpiresDescriptorDictionaries(t *testing.T) {
	store := newDogStatsDColumnarStore(10, 1)
	store.descriptorExpiry = 10
	store.descriptorInterning = true
	store.shards[0].dictionary = newDogstatsdColumnarDictionary()
	sharedTags := []string{"z:last", "a:first", "a:first"}

	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "gauge.one",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       sharedTags,
		SampleRate: 1,
	}, 101))
	require.True(t, store.insert(ckey.ContextKey(2), metrics.MetricSample{
		Name:       "gauge.two",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       sharedTags,
		SampleRate: 1,
	}, 102))

	shard := &store.shards[0]
	require.Len(t, shard.dictionary.tagsets, 1)
	require.Len(t, shard.dictionary.strings, 4) // two names plus two unique tags
	require.Same(t, &shard.tags[0][0], &shard.tags[1][0])

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(2), store.flush(200, false, &sink))
	require.Len(t, sink.rows, 2)
	require.Empty(t, shard.descriptorByKey)
	require.Len(t, shard.freeDescriptors, 2)
	require.Empty(t, shard.dictionary.tagsets)
	require.Empty(t, shard.dictionary.strings)
}
