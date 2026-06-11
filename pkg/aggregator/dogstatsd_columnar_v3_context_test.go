// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package aggregator

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggercomp "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypespkg "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func TestDogstatsdColumnarV3ResolvesOriginTags(t *testing.T) {
	configmock.New(t)
	fakeTagger := setupTagger(t)
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{tagger: fakeTagger})

	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "origin.metric",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"client:tag"},
		SampleRate: 1,
		OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container1", Cardinality: "low"},
	}, 101))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(111, false, &sink))
	require.Len(t, sink.rows, 1)
	require.Equal(t, "origin.metric", sink.rows[0].Name)
	require.Equal(t, []string{"client:tag", "env:prod", "image_name:image", "pod_name:thing1"}, sink.rows[0].Tags.UnsafeToReadOnlySliceString())
}

func TestDogstatsdColumnarV3AppliesMetricTagFilterList(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("metric_tag_filterlist_adp_only", false)
	matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"counter.metric": {Tags: []string{"drop"}, Action: "exclude"},
	}, logmock.New(t))
	samples := []metrics.MetricSample{
		{
			Name:       "counter.metric",
			Value:      1,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:a", "keep:x"},
			SampleRate: 1,
		},
		{
			Name:       "counter.metric",
			Value:      2,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:b", "keep:x"},
			SampleRate: 1,
		},
	}
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{
		tagger:        nooptagger.NewComponent(),
		tagFilterList: matcher,
	})

	for idx := range samples {
		require.True(t, store.insert(ckey.ContextKey(idx+1), samples[idx], 101+float64(idx)))
	}

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(111, false, &sink))
	require.Len(t, sink.rows, 1)
	require.Equal(t, "counter.metric", sink.rows[0].Name)
	require.Equal(t, metrics.APIRateType, sink.rows[0].MType)
	require.Equal(t, []string{"keep:x"}, sink.rows[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 0.3}}, sink.rows[0].Points)
}

func TestDogstatsdColumnarV3CompactDescriptorRefsUseResolvedContext(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("metric_tag_filterlist_adp_only", false)
	matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"counter.metric": {Tags: []string{"drop"}, Action: "exclude"},
	}, logmock.New(t))
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{
		tagger:        nooptagger.NewComponent(),
		tagFilterList: matcher,
	})
	stateA := &metrics.DogStatsDCompactIdentityState{}
	stateB := &metrics.DogStatsDCompactIdentityState{}
	sampleA := metrics.MetricSample{
		Name:       "counter.metric",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"drop:a", "keep:x"},
		Host:       "host-a",
		SampleRate: 1,
		Source:     metrics.MetricSourceDogstatsd,
	}
	sampleB := sampleA
	sampleB.Value = 2
	sampleB.Tags = []string{"drop:b", "keep:x"}
	sampleAAgain := sampleA
	sampleAAgain.Value = 3

	store.insertAcceptedRow(0, NewDogStatsDColumnarV3SampleFromMetricSample(ckey.ContextKey(1), 10, stateA, sampleA, true), 101)
	store.insertAcceptedRow(0, NewDogStatsDColumnarV3SampleFromMetricSample(ckey.ContextKey(2), 11, stateB, sampleB, true), 102)
	store.insertAcceptedRow(0, NewDogStatsDColumnarV3SampleFromMetricSample(ckey.ContextKey(1), 10, stateA, sampleAAgain, false), 103)

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(111, false, &sink))
	require.Len(t, sink.rows, 1)
	require.Equal(t, []string{"keep:x"}, sink.rows[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 0.6}}, sink.rows[0].Points)
}

func TestDogstatsdColumnarV3ParityWithTimeSamplerOriginTags(t *testing.T) {
	configmock.New(t)
	fakeTagger := setupTagger(t)
	samples := []metrics.MetricSample{
		{
			Name:       "origin.metric",
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"client:tag"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container1", Cardinality: "low", ProductOrigin: origindetection.ProductOriginDogStatsD},
		},
	}

	require.Equal(t,
		comparableRows(flushLegacyColumnarParityRows(t, samples, fakeTagger, nil)),
		comparableRows(flushColumnarParityRows(t, samples, fakeTagger, nil)),
	)
}

func TestDogstatsdColumnarV3ParityWithTimeSamplerClientOriginFields(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("origin_detection_unified", true)
	fakeTagger := setupTagger(t)
	samples := []metrics.MetricSample{
		{
			Name:       "client.local.container",
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"client:tag"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypespkg.OriginInfo{
				LocalData:     origindetection.LocalData{ContainerID: "container2"},
				Cardinality:   "low",
				ProductOrigin: origindetection.ProductOriginDogStatsD,
			},
		},
		{
			Name:       "client.external.pod",
			Value:      2,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"client:tag"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypespkg.OriginInfo{
				ExternalData:  origindetection.ExternalData{PodUID: "pod1", ContainerName: "sidecar"},
				Cardinality:   "low",
				ProductOrigin: origindetection.ProductOriginDogStatsD,
			},
		},
		{
			Name:       "client.cardinality.none",
			Value:      3,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"client:tag"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypespkg.OriginInfo{
				LocalData:     origindetection.LocalData{ContainerID: "container1"},
				Cardinality:   "none",
				ProductOrigin: origindetection.ProductOriginDogStatsD,
			},
		},
	}

	legacyRows := comparableRows(flushLegacyColumnarParityRows(t, samples, fakeTagger, nil))
	columnarRows := comparableRows(flushColumnarParityRows(t, samples, fakeTagger, nil))
	require.Equal(t, legacyRows, columnarRows)
	require.Equal(t, []string{"client:tag", "env:staging", "image_name:image", "pod_name:thing2"}, tagsForComparableRow(t, columnarRows, "client.local.container"))
	require.Equal(t, []string{"client:tag", "kube_namespace:default", "pod_name:pod1", "pod_phase:running"}, tagsForComparableRow(t, columnarRows, "client.external.pod"))
	require.Equal(t, []string{"client:tag"}, tagsForComparableRow(t, columnarRows, "client.cardinality.none"))
}

func TestDogstatsdColumnarV3ParityWithTimeSamplerCounterTagFilter(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("metric_tag_filterlist_adp_only", false)
	matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"counter.metric": {Tags: []string{"drop"}, Action: "exclude"},
	}, logmock.New(t))
	samples := []metrics.MetricSample{
		{
			Name:       "counter.metric",
			Value:      1,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:a", "keep:x"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
		},
		{
			Name:       "counter.metric",
			Value:      2,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:b", "keep:x"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
		},
	}

	require.Equal(t,
		comparableRows(flushLegacyColumnarParityRows(t, samples, nooptagger.NewComponent(), matcher)),
		comparableRows(flushColumnarParityRows(t, samples, nooptagger.NewComponent(), matcher)),
	)
}

func TestDogstatsdColumnarV3ParityWithTimeSamplerCounterIdleZerosAndExpiry(t *testing.T) {
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS", "true")
	cfg := configmock.New(t)
	cfg.SetWithoutSource("dogstatsd_context_expiry_seconds", 20)
	cfg.SetWithoutSource("dogstatsd_expiry_seconds", 25)

	matcher := filterlistimpl.NewNoopTagMatcher()
	tagger := nooptagger.NewComponent()
	legacy := NewTimeSampler(TimeSamplerID(0), 10, tags.NewStore(true, "legacy-counter-idle-zero"), tagger, "host")
	columnar := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{tagger: tagger})
	sample := metrics.MetricSample{
		Name:       "idle.counter",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"env:prod"},
		Host:       "host-a",
		SampleRate: 1,
		Source:     metrics.MetricSourceDogstatsd,
	}
	legacy.sample(&sample, 101, matcher)
	require.True(t, columnar.insert(ckey.ContextKey(1), sample, 101))

	assertParityFlush := func(timestamp float64, expected []metrics.Point) {
		t.Helper()
		legacyRows, columnarRows := flushLegacyAndColumnarRows(t, legacy, columnar, timestamp)
		require.Equal(t, comparableRows(legacyRows), comparableRows(columnarRows))
		if expected == nil {
			require.Empty(t, columnarRows)
			return
		}
		require.Len(t, columnarRows, 1)
		require.Equal(t, expected, columnarRows[0].Points)
	}

	assertParityFlush(111, []metrics.Point{{Ts: 100, Value: 0.1}})
	assertParityFlush(121, []metrics.Point{{Ts: 110, Value: 0}})
	assertParityFlush(131, []metrics.Point{{Ts: 120, Value: 0}})
	assertParityFlush(141, nil)
	assertParityFlush(147, nil)
	require.Zero(t, legacy.contextResolver.length())
	require.Zero(t, columnar.shards[0].contextResolver.length())
}

func TestDogstatsdColumnarV3ParityWithTimeSamplerCounterTagFilterIdleZeros(t *testing.T) {
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS", "true")
	cfg := configmock.New(t)
	cfg.SetWithoutSource("metric_tag_filterlist_adp_only", false)
	cfg.SetWithoutSource("dogstatsd_context_expiry_seconds", 20)
	cfg.SetWithoutSource("dogstatsd_expiry_seconds", 25)
	matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
		"counter.metric": {Tags: []string{"drop"}, Action: "exclude"},
	}, logmock.New(t))

	tagger := nooptagger.NewComponent()
	legacy := NewTimeSampler(TimeSamplerID(0), 10, tags.NewStore(true, "legacy-counter-tagfilter-zero"), tagger, "host")
	columnar := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{
		tagger:        tagger,
		tagFilterList: matcher,
	})
	samples := []metrics.MetricSample{
		{
			Name:       "counter.metric",
			Value:      1,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:a", "keep:x"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
		},
		{
			Name:       "counter.metric",
			Value:      2,
			Mtype:      metrics.CounterType,
			Tags:       []string{"drop:b", "keep:x"},
			Host:       "host-a",
			SampleRate: 1,
			Source:     metrics.MetricSourceDogstatsd,
		},
	}
	for idx := range samples {
		legacy.sample(&samples[idx], 101+float64(idx), matcher)
		require.True(t, columnar.insert(ckey.ContextKey(idx+1), samples[idx], 101+float64(idx)))
	}

	legacyRows, columnarRows := flushLegacyAndColumnarRows(t, legacy, columnar, 111)
	require.Equal(t, comparableRows(legacyRows), comparableRows(columnarRows))
	require.Len(t, columnarRows, 1)
	require.Equal(t, []string{"keep:x"}, columnarRows[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 0.3}}, columnarRows[0].Points)

	legacyRows, columnarRows = flushLegacyAndColumnarRows(t, legacy, columnar, 121)
	require.Equal(t, comparableRows(legacyRows), comparableRows(columnarRows))
	require.Len(t, columnarRows, 1)
	require.Equal(t, []string{"keep:x"}, columnarRows[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, []metrics.Point{{Ts: 110, Value: 0}}, columnarRows[0].Points)
}

func TestDogstatsdColumnarV3NativePointRowsEmitCounterIdleZeros(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("dogstatsd_context_expiry_seconds", 20)
	cfg.SetWithoutSource("dogstatsd_expiry_seconds", 25)

	tagger := nooptagger.NewComponent()
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{tagger: tagger})
	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "native.idle.counter",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"env:prod"},
		Host:       "host-a",
		SampleRate: 1,
		Source:     metrics.MetricSourceDogstatsd,
	}, 101))

	var sink columnarPointRowCaptureSink
	shadow := newDirectRowShadowBuilder()
	require.Equal(t, uint64(1), store.flushShardToV3MetricPointSink(&store.shards[0], 111, false, &sink, shadow))
	require.Len(t, sink.rows, 1)
	require.Equal(t, int64(100), sink.rows[0].Timestamp)
	require.Equal(t, 0.1, sink.rows[0].Value)

	sink.rows = nil
	require.Equal(t, uint64(1), store.flushShardToV3MetricPointSink(&store.shards[0], 121, false, &sink, shadow))
	require.Len(t, sink.rows, 1)
	require.Equal(t, int64(110), sink.rows[0].Timestamp)
	require.Equal(t, 0.0, sink.rows[0].Value)
}

type comparableSerieRow struct {
	Name     string
	Tags     []string
	Host     string
	Points   []metrics.Point
	MType    metrics.APIMetricType
	Interval int64
	Unit     string
	NoIndex  bool
	Source   metrics.MetricSource
}

func flushLegacyColumnarParityRows(t *testing.T, samples []metrics.MetricSample, tagger taggercomp.Component, matcher filterlist.TagMatcher) []metrics.SerieRow {
	t.Helper()
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS", "true")
	sampler := NewTimeSampler(TimeSamplerID(0), 10, tags.NewStore(true, "legacy-columnar-parity"), tagger, "host")
	for idx := range samples {
		sample := samples[idx]
		sampler.sample(&sample, 101+float64(idx), matcher)
	}

	var sink rowCaptureSerieSink
	var sketches metrics.SketchSeriesList
	sampler.flush(111, &sink, &sketches, nil, true)
	require.Empty(t, sink.series)
	require.Empty(t, sketches)
	return sink.rows
}

func flushColumnarParityRows(t *testing.T, samples []metrics.MetricSample, tagger taggercomp.Component, matcher filterlist.TagMatcher) []metrics.SerieRow {
	t.Helper()
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{
		tagger:        tagger,
		tagFilterList: matcher,
	})
	for idx := range samples {
		require.True(t, store.insert(ckey.ContextKey(idx+1), samples[idx], 101+float64(idx)))
	}

	var sink columnarRowCaptureSink
	store.flush(111, false, &sink)
	return sink.rows
}

func flushLegacyAndColumnarRows(t *testing.T, legacy *TimeSampler, columnar *dogstatsdColumnarStore, timestamp float64) ([]metrics.SerieRow, []metrics.SerieRow) {
	t.Helper()
	var legacySink rowCaptureSerieSink
	var sketches metrics.SketchSeriesList
	legacy.flush(timestamp, &legacySink, &sketches, nil, false)
	require.Empty(t, legacySink.series)
	require.Empty(t, sketches)

	var columnarSink columnarRowCaptureSink
	columnar.flush(int64(timestamp), false, &columnarSink)
	return legacySink.rows, columnarSink.rows
}

func tagsForComparableRow(t *testing.T, rows []comparableSerieRow, name string) []string {
	t.Helper()
	for _, row := range rows {
		if row.Name == name {
			return row.Tags
		}
	}
	require.Failf(t, "missing row", "row %q not found in %#v", name, rows)
	return nil
}

func comparableRows(rows []metrics.SerieRow) []comparableSerieRow {
	out := make([]comparableSerieRow, 0, len(rows))
	for _, row := range rows {
		tags := append([]string(nil), row.Tags.UnsafeToReadOnlySliceString()...)
		slices.Sort(tags)
		points := append([]metrics.Point(nil), row.Points...)
		slices.SortFunc(points, func(a, b metrics.Point) int {
			if a.Ts < b.Ts {
				return -1
			}
			if a.Ts > b.Ts {
				return 1
			}
			if a.Value < b.Value {
				return -1
			}
			if a.Value > b.Value {
				return 1
			}
			return 0
		})
		out = append(out, comparableSerieRow{
			Name:     row.Name,
			Tags:     tags,
			Host:     row.Host,
			Points:   points,
			MType:    row.MType,
			Interval: row.Interval,
			Unit:     row.Unit,
			NoIndex:  row.NoIndex,
			Source:   row.Source,
		})
	}
	slices.SortFunc(out, func(a, b comparableSerieRow) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		if a.Host < b.Host {
			return -1
		}
		if a.Host > b.Host {
			return 1
		}
		if a.MType < b.MType {
			return -1
		}
		if a.MType > b.MType {
			return 1
		}
		return 0
	})
	return out
}
