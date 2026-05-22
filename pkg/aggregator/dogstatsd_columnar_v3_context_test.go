// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
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
	store := newDogStatsDColumnarStore(10, 1, dogstatsdColumnarStoreConfig{
		tagger:        nooptagger.NewComponent(),
		tagFilterList: matcher,
	})

	require.True(t, store.insert(ckey.ContextKey(1), metrics.MetricSample{
		Name:       "counter.metric",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"drop:a", "keep:x"},
		SampleRate: 1,
	}, 101))
	require.True(t, store.insert(ckey.ContextKey(2), metrics.MetricSample{
		Name:       "counter.metric",
		Value:      2,
		Mtype:      metrics.CounterType,
		Tags:       []string{"drop:b", "keep:x"},
		SampleRate: 1,
	}, 102))

	var sink columnarRowCaptureSink
	require.Equal(t, uint64(1), store.flush(111, false, &sink))
	require.Len(t, sink.rows, 1)
	require.Equal(t, "counter.metric", sink.rows[0].Name)
	require.Equal(t, metrics.APIRateType, sink.rows[0].MType)
	require.Equal(t, []string{"keep:x"}, sink.rows[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 0.3}}, sink.rows[0].Points)
}
