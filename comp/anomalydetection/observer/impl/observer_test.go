// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/require"
)

func TestResetForReplayPreservesDroppedValueCallback(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	var reasons []string
	e.storage.onDroppedValue = func(reason string) { reasons = append(reasons, reason) }

	e.ResetForReplay(nil, nil, nil, nil, DefaultStorageConfig(), BaselineConfig{})
	e.storage.Add("ns", "metric", math.NaN(), 1, nil)

	require.Equal(t, []string{"non_finite"}, reasons)
}

func TestObserverResetActivatesScorerCorrelationWatcher(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	if err != nil {
		t.Fatalf("newDefaultMetricsFilterRules() returned error: %v", err)
	}

	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		catalog:      defaultCatalog(),
		obsCh:        make(chan observation, 1),
		metricFilter: filter,
	}
	done := make(chan struct{})
	go func() {
		obs.run()
		close(done)
	}()
	t.Cleanup(func() {
		close(obs.obsCh)
		<-done
	})

	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	settings := ComponentSettings{
		Enabled: map[string]bool{"anomaly_scorer": true},
		configs: map[string]any{"anomaly_scorer": cfg},
	}
	storageCfg := DefaultStorageConfig()
	storageCfg.TrackCorrelationHistory = true
	obs.Reset(settings, storageCfg)

	scorer := obs.engine.scorer
	if scorer == nil {
		t.Fatal("expected replay scorer to be configured")
	}
	seedAndCrossHighThreshold(scorer, 1000)
	if got := scorer.ActiveCorrelations(); len(got) != 1 {
		t.Fatalf("expected replay scorer watcher to open one correlation episode, got %d", len(got))
	}
}

func TestSeriesDetectorAdapter_DoesNotReemitOutputsWithoutNewData(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)

	adapter := newSeriesDetectorAdapter(&countingSeriesDetector{
		anomalies: []observerdef.Anomaly{{
			Title:       "spike",
			Description: "detected spike",
			Timestamp:   100,
		}},
	}, []observerdef.Aggregate{observerdef.AggregateAverage})

	first := adapter.Detect(storage, 100)
	if len(first.Anomalies) != 1 {
		t.Fatalf("expected 1 anomaly on first detect, got %d", len(first.Anomalies))
	}
	second := adapter.Detect(storage, 101)
	if len(second.Anomalies) != 0 {
		t.Fatalf("expected 0 anomalies without new data, got %d", len(second.Anomalies))
	}
}

func TestSeriesDetectorAdapter_ResetClearsVisibleCountCache(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)

	adapter := newSeriesDetectorAdapter(&countingSeriesDetector{
		anomalies: []observerdef.Anomaly{{
			Title:       "spike",
			Description: "detected spike",
			Timestamp:   100,
		}},
	}, []observerdef.Aggregate{observerdef.AggregateAverage})

	first := adapter.Detect(storage, 100)
	if len(first.Anomalies) != 1 {
		t.Fatalf("expected 1 anomaly on first detect, got %d", len(first.Anomalies))
	}

	adapter.Reset()

	afterReset := adapter.Detect(storage, 100)
	if len(afterReset.Anomalies) != 1 {
		t.Fatalf("expected 1 anomaly after reset, got %d", len(afterReset.Anomalies))
	}
}

func TestObserverPublishesSeriesCountOnAdvanceAndReplayBoundaries(t *testing.T) {
	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		catalog:      defaultCatalog(),
		obsCh:        make(chan observation, 4),
		telemetry:    newObserverTelemetry(telComp),
		metricFilter: filter,
	}
	done := make(chan struct{})
	go func() {
		obs.run()
		close(done)
	}()
	t.Cleanup(func() {
		close(obs.obsCh)
		<-done
	})

	// The first observation creates a series but does not advance analysis.
	obs.obsCh <- observation{source: "ns", metric: &metricObs{name: "requests", value: 1, timestamp: 0}}
	obs.Flush()
	requireSeriesCountTelemetry(t, telComp, 0)

	// A later observation advances analysis and publishes the current count.
	obs.obsCh <- observation{source: "ns", metric: &metricObs{name: "requests", value: 1, timestamp: 2}}
	obs.Flush()
	requireSeriesCountTelemetry(t, telComp, 1)

	// Replacing storage during reset clears the gauge immediately.
	obs.Reset(ComponentSettings{}, DefaultStorageConfig())
	requireSeriesCountTelemetry(t, telComp, 0)

	// Replay publishes once at its completion, not when data is preloaded.
	obs.engine.storage.Add("ns", "replayed", 1, 10, nil)
	requireSeriesCountTelemetry(t, telComp, 0)
	obs.ReplayStoredData()
	requireSeriesCountTelemetry(t, telComp, 1)

	// The final stream flush publishes even when no advance is needed.
	obs.engine.storage.RemoveSeriesByMetricName("ns", "replayed")
	obs.FinishReplayStream()
	requireSeriesCountTelemetry(t, telComp, 0)
}

func requireSeriesCountTelemetry(t *testing.T, telemetryComp telemetry.Component, want float64) {
	t.Helper()
	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)
	for _, family := range metricFamilies {
		if family.GetName() != "observer__"+telemetrySeriesCount {
			continue
		}
		require.Len(t, family.GetMetric(), 1)
		require.Equal(t, want, family.GetMetric()[0].GetGauge().GetValue())
		return
	}
	if want == 0 {
		return
	}
	t.Fatalf("series-count telemetry gauge was not registered")
}

func TestBaselineCompletedCallbackSink_AccumulatesGroupsUntilAllBaselinesComplete(t *testing.T) {
	storage := newTimeSeriesStorage()
	ref := storage.Add("ns", "cpu", 1.0, 100, nil).Ref

	type callbackResult struct {
		endSec int64
		groups []string
	}
	var callbacks []callbackResult
	sink := &baselineCompletedCallbackSink{
		engine: newEngine(engineConfig{storage: storage}),
		callback: func(endSec int64, groups []string) {
			callbacks = append(callbacks, callbackResult{endSec: endSec, groups: groups})
		},
	}

	// The first detector finds a metric-backed anomaly. The final detector
	// models a detector such as RRCF, which has no per-series source refs.
	sink.onEngineEvent(engineEvent{
		kind: eventBaselineCompleted,
		baselineCompleted: &baselineCompletedEvent{
			mutedRefs: []observerdef.SeriesRef{ref},
		},
	})
	sink.onEngineEvent(engineEvent{
		kind:      eventBaselineCompleted,
		timestamp: 200,
		baselineCompleted: &baselineCompletedEvent{
			allComplete: true,
		},
	})

	if len(callbacks) != 1 || callbacks[0].endSec != 200 {
		t.Fatalf("callbacks = %v, want one callback at 200", callbacks)
	}
	if len(callbacks[0].groups) != 1 || callbacks[0].groups[0] != "ns/cpu" {
		t.Fatalf("first callback groups = %v, want [ns/cpu]", callbacks[0].groups)
	}

	// A second replay must not inherit groups accumulated for the first one.
	secondRef := storage.Add("ns", "memory", 1.0, 300, nil).Ref
	sink.onEngineEvent(engineEvent{
		kind: eventBaselineCompleted,
		baselineCompleted: &baselineCompletedEvent{
			mutedRefs: []observerdef.SeriesRef{secondRef},
		},
	})
	sink.onEngineEvent(engineEvent{
		kind:      eventBaselineCompleted,
		timestamp: 400,
		baselineCompleted: &baselineCompletedEvent{
			allComplete: true,
		},
	})

	if len(callbacks) != 2 || callbacks[1].endSec != 400 {
		t.Fatalf("callbacks = %v, want second callback at 400", callbacks)
	}
	if len(callbacks[1].groups) != 1 || callbacks[1].groups[0] != "ns/memory" {
		t.Fatalf("second callback groups = %v, want [ns/memory]", callbacks[1].groups)
	}
}

type countingSeriesDetector struct {
	anomalies []observerdef.Anomaly
}

func (d *countingSeriesDetector) Name() string { return "counting" }
func (*countingSeriesDetector) Ready() bool    { return true }

func (d *countingSeriesDetector) Detect(_ observerdef.Series) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: append([]observerdef.Anomaly(nil), d.anomalies...),
	}
}
