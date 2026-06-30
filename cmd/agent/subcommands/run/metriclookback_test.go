// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermocks "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestMetricLookbackDogStatsDFactoryDisabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled": false,
	})
	factory := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t))

	lookback := factory(serializermocks.NewMetricSerializer(t))

	require.Nil(t, lookback)
}

func TestMetricLookbackDogStatsDFactoryBuildsMonitorEgressAdapterFromNoAggSeries(t *testing.T) {
	start := time.Unix(100, 0)
	cfg := metricLookbackMonitorFactoryConfig(t)
	serializer := serializermocks.NewMetricSerializer(t)
	forwarded := make(chan struct{}, 1)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Greater(t, source.Count(), uint64(0))
		found := false
		for source.MoveNext() {
			if source.Current().Name == "target.metric" {
				found = true
			}
		}
		require.True(t, found)
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t))
	lookback := factory(serializer)
	require.NotNil(t, lookback)

	appendFactoryNoAggTestWindow(lookback, start, 0, 15, 2)

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor egress")
	}
}

func TestMetricLookbackDogStatsDFactoryBuildsMonitorEgressAdapterFromBucketedSamples(t *testing.T) {
	start := time.Unix(100, 0)
	cfg := metricLookbackMonitorFactoryConfig(t)
	serializer := serializermocks.NewMetricSerializer(t)
	forwarded := make(chan struct{}, 1)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Greater(t, source.Count(), uint64(0))
		foundBucketedPoint := false
		for source.MoveNext() {
			serie := source.Current()
			if serie.Name == "target.metric" && serie.Interval == 1 {
				foundBucketedPoint = true
			}
		}
		require.True(t, foundBucketedPoint)
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t))
	lookback := factory(serializer)
	require.NotNil(t, lookback)

	appendFactoryBucketedTestWindow(lookback, start, 0, 15, 2)

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor egress")
	}
}

func TestMetricLookbackDogStatsDFactoryBuildsMonitorEgressAdapterFromDistributionSamples(t *testing.T) {
	start := time.Unix(100, 0)
	cfg := metricLookbackMonitorFactoryConfig(t)
	serializer := serializermocks.NewMetricSerializer(t)
	forwarded := make(chan struct{}, 1)
	serializer.On("SendSketch", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SketchesSource)
		require.Greater(t, source.Count(), uint64(0))
		foundSketch := false
		for source.MoveNext() {
			series := source.Current()
			if series.Name == "target.metric" {
				foundSketch = true
			}
		}
		require.True(t, foundSketch)
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t))
	lookback := factory(serializer)
	require.NotNil(t, lookback)

	appendFactoryDistributionTestWindow(lookback, start, 0, 15, 2)

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor egress")
	}
}

func metricLookbackMonitorFactoryConfig(t testing.TB) configmock.Component {
	return configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":                true,
		"metric_lookback.capacity":               256,
		"metric_lookback.shard_count":            1,
		"metric_lookback.dogstatsd.enabled":      false,
		"metric_lookback.dogstatsd.metric_names": []string{},
		"metric_lookback.monitor.enabled":        true,
		"metric_lookback.monitor.metric_name":    "target.metric",
		"metric_lookback.monitor.baseline_min":   1.0,
	})
}

func appendFactoryNoAggTestWindow(lookback interface {
	AppendDogStatsDNoAggSerie(*metrics.Serie)
}, start time.Time, fromSecond, toSecond int, value float64) {
	for second := fromSecond; second <= toSecond; second++ {
		lookback.AppendDogStatsDNoAggSerie(&metrics.Serie{
			Name:     "target.metric",
			Points:   []metrics.Point{{Ts: float64(start.Add(time.Duration(second) * time.Second).Unix()), Value: value}},
			Tags:     tagset.CompositeTagsFromSlice(nil),
			MType:    metrics.APIGaugeType,
			Interval: 10,
		})
	}
}

func appendFactoryBucketedTestWindow(lookback aggregator.DogStatsDLookback, start time.Time, fromSecond, toSecond int, value float64) {
	ctx := aggregator.DogStatsDLookbackContext{
		ContextKey: ckey.ContextKey(1),
		Name:       "target.metric",
		Host:       "host",
	}
	for second := fromSecond; second <= toSecond; second++ {
		ts := float64(start.Add(time.Duration(second) * time.Second).Unix())
		lookback.ObserveDogStatsDSample(&metrics.MetricSample{
			Name:       "target.metric",
			Value:      value,
			Mtype:      metrics.GaugeType,
			SampleRate: 1,
		}, ts, ctx)
		lookback.FlushDogStatsDBuckets(ts+metriclookback.DefaultDogStatsDSealDelay.Seconds()+1, false)
	}
}

func appendFactoryDistributionTestWindow(lookback aggregator.DogStatsDLookback, start time.Time, fromSecond, toSecond int, value float64) {
	ctx := aggregator.DogStatsDLookbackContext{
		ContextKey: ckey.ContextKey(2),
		Name:       "target.metric",
		Host:       "host",
	}
	for second := fromSecond; second <= toSecond; second++ {
		ts := float64(start.Add(time.Duration(second) * time.Second).Unix())
		lookback.ObserveDogStatsDSample(&metrics.MetricSample{
			Name:       "target.metric",
			Value:      value,
			Mtype:      metrics.DistributionType,
			SampleRate: 1,
		}, ts, ctx)
		lookback.FlushDogStatsDBuckets(ts+metriclookback.DefaultDogStatsDSealDelay.Seconds()+1, false)
	}
}
