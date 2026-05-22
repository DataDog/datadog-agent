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
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container1", Cardinality: "low"},
		},
	}

	require.Equal(t,
		comparableRows(flushLegacyColumnarParityRows(t, samples, fakeTagger, nil)),
		comparableRows(flushColumnarParityRows(t, samples, fakeTagger, nil)),
	)
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
