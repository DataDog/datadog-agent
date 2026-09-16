// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metriclookbackimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	collectormetriclookback "github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	metriclookbackdogstatsd "github.com/DataDog/datadog-agent/pkg/metriclookback/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermocks "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestMetricLookbackDogStatsDFactoryDisabled(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled": false,
	})
	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))

	lookback := factory(serializermocks.NewMetricSerializer(t))

	require.Nil(t, lookback)
}

func TestMetricLookbackComponentProvidesDogStatsDFactory(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":                true,
		"metric_lookback.capacity":               256,
		"metric_lookback.shard_count":            1,
		"metric_lookback.dogstatsd.metric_names": []string{"target.metric"},
		"metric_lookback.monitor.mode":           "disabled",
	})

	provides, err := NewComponent(Requires{Config: cfg, Log: logmock.New(t)})

	require.NoError(t, err)
	require.NotNil(t, provides.Comp)
	require.NotNil(t, provides.DogStatsDLookbackFactory)

	shadowManager := provides.Comp.NewSenderManager(context.Background(), "default-host")
	require.NotNil(t, shadowManager)
	shadowSender, err := shadowManager.GetSender(checkid.ID("gpu:shadow"))
	require.NoError(t, err)
	shadowSender.Gauge("shadow.metric", 42, "", nil)
	shadowSender.Commit()
	retained := provides.Comp.(component).retention.Series()
	require.Len(t, retained, 1)
	require.Equal(t, "shadow.metric", retained[0].Name)
	require.Equal(t, "default-host", retained[0].Host)

	lookback := provides.DogStatsDLookbackFactory(serializermocks.NewMetricSerializer(t))
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)
	require.True(t, lookback.WantsDogStatsDMetric("target.metric"))
}

func TestMetricLookbackDogStatsDFactoryMetricNamesEnableDogStatsDLookback(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":                true,
		"metric_lookback.capacity":               256,
		"metric_lookback.shard_count":            1,
		"metric_lookback.dogstatsd.metric_names": []string{"target.metric"},
		"metric_lookback.monitor.mode":           "disabled",
	})
	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))

	lookback := factory(serializermocks.NewMetricSerializer(t))

	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)
	require.True(t, lookback.WantsDogStatsDMetric("target.metric"))
	require.False(t, lookback.WantsDogStatsDMetric("other.metric"))
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

	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))
	lookback := factory(serializer)
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)

	appendFactoryNoAggTestWindow(lookback, start, 0, 30, 2)

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor egress")
	}
}

func TestMetricLookbackDogStatsDFactoryDryRunForwardsHealthyMonitorWindow(t *testing.T) {
	start := time.Unix(100, 0)
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":                true,
		"metric_lookback.capacity":               256,
		"metric_lookback.shard_count":            1,
		"metric_lookback.dogstatsd.metric_names": []string{},
		"metric_lookback.monitor.mode":           "dry_run",
		"metric_lookback.monitor.metric_name":    "target.metric",
		"metric_lookback.monitor.range_epsilon":  0.05,
	})
	serializer := serializermocks.NewMetricSerializer(t)
	forwarded := make(chan struct{}, 1)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Greater(t, source.Count(), uint64(0))
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))
	lookback := factory(serializer)
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)

	for second := 0; second <= 30; second++ {
		lookback.AppendDogStatsDNoAggSerie(&metrics.Serie{
			Name:     "target.metric",
			Points:   []metrics.Point{{Ts: float64(start.Add(time.Duration(second) * time.Second).Unix()), Value: 2}},
			Tags:     tagset.CompositeTagsFromSlice(nil),
			MType:    metrics.APIGaugeType,
			Interval: 10,
		})
	}

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for dry-run monitor egress")
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

	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))
	lookback := factory(serializer)
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)

	appendFactoryBucketedTestWindow(lookback, start, 0, 30, 2)

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
			series, ok := source.Current().(*metrics.SketchSeries)
			require.True(t, ok)
			if series.Name == "target.metric" {
				foundSketch = true
			}
		}
		require.True(t, foundSketch)
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := requireMetricLookbackDogStatsDFactory(t, cfg, newMetricLookbackRetention(cfg))
	lookback := factory(serializer)
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)

	appendFactoryDistributionTestWindow(lookback, start, 0, 30, 2)

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor egress")
	}
}

func TestMetricLookbackMonitorEgressAdapterFromShadowSenderSamples(t *testing.T) {
	start := time.Unix(100, 0)
	cfg := metricLookbackMonitorFactoryConfig(t)
	retention := newMetricLookbackRetention(cfg)
	serializer := serializermocks.NewMetricSerializer(t)
	forwarded := make(chan struct{}, 1)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Greater(t, source.Count(), uint64(0))
		foundShadowPoint := false
		for source.MoveNext() {
			serie := source.Current()
			if serie.Name == "target.metric" && serie.Host == "default-host" {
				foundShadowPoint = true
			}
		}
		require.True(t, foundShadowPoint)
		forwarded <- struct{}{}
	}).Return(nil).Maybe()

	factory := requireMetricLookbackDogStatsDFactory(t, cfg, retention)
	lookback := factory(serializer)
	// The DogStatsD adapter is still created because the monitor metric is auto-admitted,
	// but this test only writes through the shadow-check sender manager.
	require.NotNil(t, lookback)
	stopLookbackOnCleanup(t, lookback)

	manager := collectormetriclookback.NewSenderManager(context.Background(), "default-host", retention)
	sender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	for second := 0; second <= 30; second++ {
		ts := float64(start.Add(time.Duration(second) * time.Second).Unix())
		value := float64(2)
		if second == 30 {
			value = 3
		}
		require.NoError(t, sender.GaugeWithTimestamp("target.metric", value, "", nil, ts))
		sender.Commit()
	}

	select {
	case <-forwarded:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for shadow sender monitor egress")
	}
}

func TestMetricLookbackDogStatsDFactoryRejectsNegativeRangeEpsilon(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":               true,
		"metric_lookback.monitor.mode":          "enabled",
		"metric_lookback.monitor.metric_name":   "target.metric",
		"metric_lookback.monitor.range_epsilon": -0.01,
	})

	factory, err := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t), newMetricLookbackRetention(cfg))

	require.Nil(t, factory)
	require.ErrorContains(t, err, "metric_lookback.monitor.range_epsilon")
}

func TestMetricLookbackDogStatsDFactoryRejectsNegativeWindowDurations(t *testing.T) {
	for _, key := range []string{
		"metric_lookback.monitor.evaluation_interval",
		"metric_lookback.egress.pre_trigger_window",
		"metric_lookback.egress.post_recovery_window",
	} {
		t.Run(key, func(t *testing.T) {
			cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
				"metric_lookback.enabled":               true,
				"metric_lookback.monitor.mode":          "enabled",
				"metric_lookback.monitor.metric_name":   "target.metric",
				"metric_lookback.monitor.range_epsilon": 0.05,
				key:                                     -time.Second,
			})

			factory, err := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t), newMetricLookbackRetention(cfg))

			require.Nil(t, factory)
			require.ErrorContains(t, err, key)
		})
	}
}

func TestMetricLookbackDogStatsDFactoryRejectsInvalidMonitorMode(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":      true,
		"metric_lookback.monitor.mode": "observe",
	})

	factory, err := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t), newMetricLookbackRetention(cfg))

	require.Nil(t, factory)
	require.ErrorContains(t, err, "metric_lookback.monitor.mode")
}

func TestMetricLookbackDogStatsDFactoryRequiresMetricNameWhenMonitorModeActive(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":      true,
		"metric_lookback.monitor.mode": "enabled",
	})

	factory, err := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t), newMetricLookbackRetention(cfg))

	require.Nil(t, factory)
	require.ErrorContains(t, err, "metric_lookback.monitor.metric_name")
}

func requireMetricLookbackDogStatsDFactory(t testing.TB, cfg configmock.Component, retention *metriclookback.Retention) aggregator.DogStatsDLookbackFactory {
	t.Helper()
	factory, err := newMetricLookbackDogStatsDFactory(cfg, logmock.New(t), retention)
	require.NoError(t, err)
	require.NotNil(t, factory)
	return factory
}

func stopLookbackOnCleanup(t testing.TB, lookback aggregator.DogStatsDLookback) {
	t.Helper()
	stopper, ok := lookback.(aggregator.DogStatsDLookbackStopper)
	if !ok {
		return
	}
	t.Cleanup(stopper.Stop)
}

func metricLookbackMonitorFactoryConfig(t testing.TB) configmock.Component {
	return configmock.NewMockWithOverrides(t, map[string]interface{}{
		"metric_lookback.enabled":                     true,
		"metric_lookback.capacity":                    256,
		"metric_lookback.shard_count":                 1,
		"metric_lookback.dogstatsd.metric_names":      []string{},
		"metric_lookback.monitor.mode":                "enabled",
		"metric_lookback.monitor.metric_name":         "target.metric",
		"metric_lookback.monitor.evaluation_interval": 30 * time.Second,
		"metric_lookback.monitor.range_epsilon":       0.05,
		"metric_lookback.egress.pre_trigger_window":   0 * time.Second,
		"metric_lookback.egress.post_recovery_window": 30 * time.Second,
	})
}

func appendFactoryNoAggTestWindow(lookback interface {
	AppendDogStatsDNoAggSerie(*metrics.Serie)
}, start time.Time, fromSecond, toSecond int, value float64) {
	for second := fromSecond; second <= toSecond; second++ {
		sampleValue := value
		if second == toSecond {
			sampleValue = value + 1
		}
		lookback.AppendDogStatsDNoAggSerie(&metrics.Serie{
			Name:     "target.metric",
			Points:   []metrics.Point{{Ts: float64(start.Add(time.Duration(second) * time.Second).Unix()), Value: sampleValue}},
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
		sampleValue := value
		if second == toSecond {
			sampleValue = value + 1
		}
		lookback.ObserveDogStatsDSample(&metrics.MetricSample{
			Name:       "target.metric",
			Value:      sampleValue,
			Mtype:      metrics.GaugeType,
			SampleRate: 1,
		}, ts, ctx)
		lookback.FlushDogStatsDBuckets(ts+metriclookbackdogstatsd.DefaultDogStatsDSealDelay.Seconds()+1, false)
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
		sampleValue := value
		if second == toSecond {
			sampleValue = value + 1
		}
		lookback.ObserveDogStatsDSample(&metrics.MetricSample{
			Name:       "target.metric",
			Value:      sampleValue,
			Mtype:      metrics.DistributionType,
			SampleRate: 1,
		}, ts, ctx)
		lookback.FlushDogStatsDBuckets(ts+metriclookbackdogstatsd.DefaultDogStatsDSealDelay.Seconds()+1, false)
	}
}
