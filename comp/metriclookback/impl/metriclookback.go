// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookbackimpl implements metric lookback wiring for the Agent.
package metriclookbackimpl

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	metriclookbackdef "github.com/DataDog/datadog-agent/comp/metriclookback/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectormetriclookback "github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	metriclookbackdogstatsd "github.com/DataDog/datadog-agent/pkg/metriclookback/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Requires defines the metric lookback component dependencies.
type Requires struct {
	Config config.Component
	Log    log.Component
}

// Provides defines the values provided by the metric lookback component.
type Provides struct {
	Comp                     metriclookbackdef.Component
	DogStatsDLookbackFactory aggregator.DogStatsDLookbackFactory
}

type component struct {
	retention *metriclookback.Retention
}

// NewSenderManager returns a shadow-check sender manager backed by the shared
// metric lookback retention ring.
func (c component) NewSenderManager(ctx context.Context, defaultHostname string) sender.SenderManager {
	return collectormetriclookback.NewSenderManager(ctx, defaultHostname, c.retention)
}

// NewComponent creates the metric lookback component.
func NewComponent(req Requires) (Provides, error) {
	retention := newMetricLookbackRetention(req.Config)
	factory, err := newMetricLookbackDogStatsDFactory(req.Config, req.Log, retention)
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: component{retention: retention}, DogStatsDLookbackFactory: factory}, nil
}

const (
	metricLookbackMonitorModeDisabled = "disabled"
	metricLookbackMonitorModeDryRun   = "dry_run"
	metricLookbackMonitorModeEnabled  = "enabled"
)

func newMetricLookbackRetention(cfg config.Component) *metriclookback.Retention {
	// Construct the retention owner eagerly, but keep the backing rings lazy. This
	// lets per-instance shadow-check opt-in share the same retention path without
	// reserving ring memory when no selected metric is written.
	return metriclookback.NewRetention(ringbuffer.Options{
		Capacity:   cfg.GetInt("metric_lookback.capacity"),
		ShardCount: cfg.GetInt("metric_lookback.shard_count"),
	})
}

func newMetricLookbackDogStatsDFactory(cfg config.Component, logger log.Component, retention *metriclookback.Retention) (aggregator.DogStatsDLookbackFactory, error) {
	if err := validateMetricLookbackMonitorConfig(cfg); err != nil {
		return nil, err
	}

	return func(metricSerializer serializer.MetricSerializer) aggregator.DogStatsDLookback {
		if retention == nil || !cfg.GetBool("metric_lookback.enabled") {
			return nil
		}

		metricNames := cfg.GetStringSlice("metric_lookback.dogstatsd.metric_names")
		monitorMode := metricLookbackMonitorMode(cfg)
		monitorEnabled := monitorMode != metricLookbackMonitorModeDisabled
		if len(metricNames) == 0 && !monitorEnabled {
			return nil
		}

		var egressController *metriclookback.EgressController
		if monitorEnabled {
			egressController = metriclookback.NewEgressController(retention, metricSerializer, metriclookback.EgressControllerOptions{
				PreTriggerWindow:             cfg.GetDuration("metric_lookback.egress.pre_trigger_window"),
				PostRecoveryWindow:           cfg.GetDuration("metric_lookback.egress.post_recovery_window"),
				DryRun:                       monitorMode == metricLookbackMonitorModeDryRun,
				MonitorStateTransitionLogger: metricLookbackMonitorStateTransitionLogger(logger),
			})
		}
		watcher, err := newMetricLookbackMonitor(cfg, logger, retention, egressController)
		if err != nil {
			logger.Errorf("invalid metric_lookback monitor configuration: %v", err)
			return nil
		}
		retention.SetMonitor(watcher)
		materializer := metriclookbackdogstatsd.NewDogStatsDBucketMaterializer(retention, metriclookbackdogstatsd.DogStatsDBucketMaterializerOptions{
			Monitor: watcher,
		})

		adapter := metriclookbackdogstatsd.NewDogStatsDAdapter(retention, metriclookbackdogstatsd.DogStatsDOptions{
			MetricNames:        metricNames,
			Monitor:            watcher,
			BucketMaterializer: materializer,
			EgressController:   egressController,
		})
		if adapter == nil {
			logger.Warn("metric_lookback is enabled but no DogStatsD metric names or monitor metric are configured; lookback inactive")
			return nil
		}
		if egressController != nil && watcher != nil {
			egressController.Start()
		}
		return adapter
	}, nil
}

func metricLookbackMonitorStateTransitionLogger(logger log.Component) metriclookback.MonitorStateTransitionLogger {
	return func(transition metriclookback.MonitorStateTransition) {
		if logger == nil {
			return
		}
		decision := transition.Decision
		if transition.Initial {
			logger.Infof("metric_lookback monitor state initialized: metric_name=%q state=%s dry_run=%t egress_mode=%s window_from=%s window_to=%s point_count=%d min=%v max=%v range=%v range_epsilon=%v partition_tags=%v partition_key=%q partition_count=%d",
				transition.MetricName,
				transition.To.String(),
				transition.DryRun,
				transition.EgressMode.String(),
				decision.WindowFrom.Format(time.RFC3339Nano),
				decision.WindowTo.Format(time.RFC3339Nano),
				decision.PointCount,
				decision.Min,
				decision.Max,
				decision.Range,
				decision.RangeEpsilon,
				decision.PartitionTags,
				decision.PartitionKey,
				decision.PartitionCount,
			)
			return
		}
		logger.Infof("metric_lookback monitor state transition: metric_name=%q from=%s to=%s dry_run=%t egress_mode=%s window_from=%s window_to=%s point_count=%d min=%v max=%v range=%v range_epsilon=%v partition_tags=%v partition_key=%q partition_count=%d",
			transition.MetricName,
			transition.From.String(),
			transition.To.String(),
			transition.DryRun,
			transition.EgressMode.String(),
			decision.WindowFrom.Format(time.RFC3339Nano),
			decision.WindowTo.Format(time.RFC3339Nano),
			decision.PointCount,
			decision.Min,
			decision.Max,
			decision.Range,
			decision.RangeEpsilon,
			decision.PartitionTags,
			decision.PartitionKey,
			decision.PartitionCount,
		)
	}
}

func metricLookbackMonitorMode(cfg config.Component) string {
	if cfg == nil {
		return metricLookbackMonitorModeDisabled
	}
	mode := cfg.GetString("metric_lookback.monitor.mode")
	if mode == "" {
		return metricLookbackMonitorModeDisabled
	}
	return mode
}

func validateMetricLookbackMonitorConfig(cfg config.Component) error {
	if cfg == nil || !cfg.GetBool("metric_lookback.enabled") {
		return nil
	}
	mode := metricLookbackMonitorMode(cfg)
	switch mode {
	case metricLookbackMonitorModeDisabled:
		return nil
	case metricLookbackMonitorModeDryRun, metricLookbackMonitorModeEnabled:
		if cfg.GetString("metric_lookback.monitor.metric_name") == "" {
			return fmt.Errorf("metric_lookback.monitor.metric_name is required when metric_lookback.monitor.mode is %q", mode)
		}
		if rangeEpsilon := cfg.GetFloat64("metric_lookback.monitor.range_epsilon"); rangeEpsilon < 0 {
			return fmt.Errorf("metric_lookback.monitor.range_epsilon must be non-negative, got %v", rangeEpsilon)
		}
		if evaluationInterval := cfg.GetDuration("metric_lookback.monitor.evaluation_interval"); evaluationInterval < 0 {
			return fmt.Errorf("metric_lookback.monitor.evaluation_interval must be non-negative, got %v", evaluationInterval)
		}
		if preTriggerWindow := cfg.GetDuration("metric_lookback.egress.pre_trigger_window"); preTriggerWindow < 0 {
			return fmt.Errorf("metric_lookback.egress.pre_trigger_window must be non-negative, got %v", preTriggerWindow)
		}
		if postRecoveryWindow := cfg.GetDuration("metric_lookback.egress.post_recovery_window"); postRecoveryWindow < 0 {
			return fmt.Errorf("metric_lookback.egress.post_recovery_window must be non-negative, got %v", postRecoveryWindow)
		}
		return nil
	default:
		return fmt.Errorf("metric_lookback.monitor.mode must be one of %q, %q, or %q; got %q", metricLookbackMonitorModeDisabled, metricLookbackMonitorModeDryRun, metricLookbackMonitorModeEnabled, mode)
	}
}

func newMetricLookbackMonitor(cfg config.Component, logger log.Component, retention *metriclookback.Retention, sink monitor.DecisionSink) (*monitor.Watcher, error) {
	if metricLookbackMonitorMode(cfg) == metricLookbackMonitorModeDisabled {
		return nil, nil
	}
	if sink == nil {
		logger.Warn("metric_lookback.monitor.mode is active but egress controller is not available; monitor inactive")
		return nil, nil
	}

	metricName := cfg.GetString("metric_lookback.monitor.metric_name")
	monitorSources := []ringbuffer.Source{
		{Kind: ringbuffer.SourceDogStatsDBucketed},
		{Kind: ringbuffer.SourceDogStatsDNoAggregation},
		{Kind: ringbuffer.SourceCheckShadow},
	}
	reader := monitor.PointReaderFunc(func(metricName string, from, to time.Time) []monitor.Point {
		points := retention.PointsBetweenSources(monitorSources, metricName, from, to)
		// PlaceholderAverageSketchProjection is intentionally isolated from retention
		// and egress serialization. It only lets the current scalar monitor evaluate
		// selected distribution sketches until a sketch-aware monitor design is chosen.
		sketchPoints := retention.ProjectedSketchPointsBetweenSources(monitorSources, metricName, from, to, metriclookback.PlaceholderAverageSketchProjection{})
		out := make([]monitor.Point, 0, len(points)+len(sketchPoints))
		for _, point := range points {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value, Tags: point.Tags})
		}
		for _, point := range sketchPoints {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value, Tags: point.Tags})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Ts.Equal(out[j].Ts) {
				return out[i].Value < out[j].Value
			}
			return out[i].Ts.Before(out[j].Ts)
		})
		return out
	})
	watcher, err := monitor.New(monitor.Config{
		MetricName:         metricName,
		RangeEpsilon:       cfg.GetFloat64("metric_lookback.monitor.range_epsilon"),
		PartitionTags:      cfg.GetStringSlice("metric_lookback.monitor.partition_tags"),
		EvaluationInterval: cfg.GetDuration("metric_lookback.monitor.evaluation_interval"),
	}, reader, sink)
	if err != nil {
		return nil, err
	}
	if watcher == nil {
		logger.Warn("metric_lookback.monitor.mode is active but metric_lookback.monitor.metric_name is empty; monitor inactive")
	}
	return watcher, nil
}
