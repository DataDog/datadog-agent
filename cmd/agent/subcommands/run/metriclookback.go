// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func newMetricLookbackDogStatsDFactory(cfg config.Component, logger log.Component) aggregator.DogStatsDLookbackFactory {
	return func(metricSerializer serializer.MetricSerializer) aggregator.DogStatsDLookback {
		if !cfg.GetBool("metric_lookback.enabled") {
			return nil
		}

		dogstatsdEnabled := cfg.GetBool("metric_lookback.dogstatsd.enabled")
		monitorEnabled := cfg.GetBool("metric_lookback.monitor.enabled")
		if !dogstatsdEnabled && !monitorEnabled {
			return nil
		}

		retention := metriclookback.NewRetention(ringbuffer.Options{
			Capacity:   cfg.GetInt("metric_lookback.capacity"),
			ShardCount: cfg.GetInt("metric_lookback.shard_count"),
		})

		var egressController *metriclookback.EgressController
		if monitorEnabled {
			egressController = metriclookback.NewEgressController(retention, metricSerializer, metriclookback.EgressControllerOptions{})
		}
		watcher := newMetricLookbackMonitor(cfg, logger, retention, egressController)
		materializer := metriclookback.NewDogStatsDBucketMaterializer(retention, metriclookback.DogStatsDBucketMaterializerOptions{
			Monitor: watcher,
		})

		metricNames := []string(nil)
		if dogstatsdEnabled {
			metricNames = cfg.GetStringSlice("metric_lookback.dogstatsd.metric_names")
		}
		adapter := metriclookback.NewDogStatsDAdapter(retention, metriclookback.DogStatsDOptions{
			MetricNames:        metricNames,
			Monitor:            watcher,
			BucketMaterializer: materializer,
		})
		if adapter == nil {
			logger.Warn("metric_lookback is enabled but no DogStatsD metric names or monitor metric are configured; lookback inactive")
			return nil
		}
		if egressController != nil && watcher != nil {
			egressController.Start()
		}
		return adapter
	}
}

func newMetricLookbackMonitor(cfg config.Component, logger log.Component, retention *metriclookback.Retention, sink monitor.DecisionSink) *monitor.Watcher {
	if !cfg.GetBool("metric_lookback.monitor.enabled") {
		return nil
	}
	if sink == nil {
		logger.Warn("metric_lookback.monitor.enabled is set but egress controller is not available; monitor inactive")
		return nil
	}

	metricName := cfg.GetString("metric_lookback.monitor.metric_name")
	dogstatsdSources := []ringbuffer.Source{
		{Kind: ringbuffer.SourceDogStatsDBucketed},
		{Kind: ringbuffer.SourceDogStatsDNoAggregation},
	}
	reader := monitor.PointReaderFunc(func(metricName string, from, to time.Time) []monitor.Point {
		points := retention.PointsBetweenSources(dogstatsdSources, metricName, from, to)
		// PlaceholderAverageSketchProjection is intentionally isolated from retention
		// and egress serialization. It only lets the current scalar monitor evaluate
		// selected distribution sketches until a sketch-aware monitor design is chosen.
		sketchPoints := retention.ProjectedSketchPointsBetweenSources(dogstatsdSources, metricName, from, to, metriclookback.PlaceholderAverageSketchProjection{})
		out := make([]monitor.Point, 0, len(points)+len(sketchPoints))
		for _, point := range points {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value})
		}
		for _, point := range sketchPoints {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Ts.Equal(out[j].Ts) {
				return out[i].Value < out[j].Value
			}
			return out[i].Ts.Before(out[j].Ts)
		})
		return out
	})
	watcher := monitor.New(monitor.Config{
		MetricName:   metricName,
		RangeEpsilon: cfg.GetFloat64("metric_lookback.monitor.range_epsilon"),
	}, reader, sink)
	if watcher == nil {
		logger.Warn("metric_lookback.monitor.enabled is set but metric_lookback.monitor.metric_name is empty; monitor inactive")
	}
	return watcher
}
