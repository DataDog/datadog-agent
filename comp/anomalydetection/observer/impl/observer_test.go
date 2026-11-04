// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/require"
)

func TestObserverResetActivatesScorerCorrelationWatcher(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	if err != nil {
		t.Fatalf("newDefaultMetricsFilterRules() returned error: %v", err)
	}

	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		catalog:      defaultCatalog(),
		metricsCh:    make(chan observation, 1),
		logsCh:       make(chan observation, 1),
		metricFilter: filter,
	}
	done := make(chan struct{})
	go func() {
		obs.run()
		close(done)
	}()
	t.Cleanup(func() {
		close(obs.metricsCh)
		close(obs.logsCh)
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
	storageCfg.TrackAnomalyHistory = true
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
			Timestamp: 100,
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
			Timestamp: 100,
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
	telComp := telemetryimpl.NewMock(t)

	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		catalog:      defaultCatalog(),
		metricsCh:    make(chan observation, 4),
		logsCh:       make(chan observation, 4),
		telemetry:    newObserverTelemetry(telComp),
		metricFilter: filter,
	}
	done := make(chan struct{})
	go func() {
		obs.run()
		close(done)
	}()
	t.Cleanup(func() {
		close(obs.metricsCh)
		close(obs.logsCh)
		<-done
	})

	// The first observation creates a series but does not advance analysis.
	obs.metricsCh <- observation{
		source:    "ns",
		metric:    metricHandoff{name: "requests", value: 1, timestamp: 1},
		hasMetric: true,
	}
	obs.Flush()
	requireSeriesCountTelemetry(t, telComp, 0)

	// A later observation advances analysis and publishes the current count.
	obs.metricsCh <- observation{
		source:    "ns",
		metric:    metricHandoff{name: "requests", value: 1, timestamp: 2},
		hasMetric: true,
	}
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
	metricFamilies, err := telemetryComp.Gather(telemetry.NoFilter)
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
	// finishes without finding any additional noisy series.
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

func TestHandleQueuesIsolateMetricAndLogBursts(t *testing.T) {
	for _, burstKind := range []string{"metrics", "logs"} {
		t.Run(burstKind, func(t *testing.T) {
			h := &handle{
				metricsCh: make(chan observation, 1),
				logsCh:    make(chan observation, 1),
				source:    "check",
			}
			metric := &metricObs{name: "system.cpu.user", value: 1, timestamp: 1000}
			log := &logObs{content: "request failed", status: "error", timestampMs: 1_000_000}
			if burstKind == "metrics" {
				require.False(t, h.ObserveMetricAndReportDrop(metric, 0))
				require.True(t, h.ObserveMetricAndReportDrop(metric, 0))
				h.ObserveLog(log)
			} else {
				h.ObserveLog(log)
				h.ObserveLog(log)
				require.False(t, h.ObserveMetricAndReportDrop(metric, 0))
			}
			require.Len(t, h.metricsCh, 1)
			require.Len(t, h.logsCh, 1)
			require.Equal(t, int64(1), h.dropCount.Load())
		})
	}
}

func TestObserverFlushProcessesBothQueues(t *testing.T) {
	storage := newTimeSeriesStorage()
	extractor := NewLogMetricsExtractor(DefaultLogMetricsExtractorConfig())
	obs := &observerImpl{
		engine: newEngine(engineConfig{
			storage:    storage,
			extractors: []observerdef.LogMetricsExtractor{extractor},
		}),
		metricsCh: make(chan observation, 1),
		logsCh:    make(chan observation, 1),
	}
	h := &handle{metricsCh: obs.metricsCh, logsCh: obs.logsCh, source: "check"}
	h.ObserveMetric(&metricObs{name: "system.cpu.user", value: 1, timestamp: 1000}, 0)
	h.ObserveLog(&logObs{content: "Request failed with unexpected error", status: "error", timestampMs: 1_000_000})
	done := make(chan struct{})
	go func() {
		obs.run()
		close(done)
	}()
	t.Cleanup(func() {
		close(obs.metricsCh)
		close(obs.logsCh)
		<-done
	})

	obs.Flush()
	require.NotEmpty(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"}))
	require.NotEmpty(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()}))
	// An empty flush must also finish, including on repeated calls.
	obs.Flush()
}

func TestObserverRunDrainsRemainingQueue(t *testing.T) {
	for _, otherQueueState := range []string{"nil", "closed"} {
		for _, remainingKind := range []string{"metrics", "logs"} {
			t.Run(remainingKind+"_with_"+otherQueueState+"_other_queue", func(t *testing.T) {
				storage := newTimeSeriesStorage()
				extractor := NewLogMetricsExtractor(DefaultLogMetricsExtractorConfig())
				obs := &observerImpl{
					engine: newEngine(engineConfig{
						storage:    storage,
						extractors: []observerdef.LogMetricsExtractor{extractor},
					}),
					metricsCh: make(chan observation, 1),
					logsCh:    make(chan observation, 1),
				}
				remainingCh, otherCh := obs.metricsCh, &obs.logsCh
				namespace := "check"
				if remainingKind == "logs" {
					remainingCh, otherCh = obs.logsCh, &obs.metricsCh
					namespace = extractor.Name()
				}
				if otherQueueState == "nil" {
					*otherCh = nil
				} else {
					close(*otherCh)
				}

				done := make(chan struct{})
				go func() {
					obs.run()
					close(done)
				}()
				t.Cleanup(func() {
					close(remainingCh)
					// Both queues are now nil or closed; the worker must exit.
					<-done
				})

				for i, sample := range []struct{ metricName, logContent string }{
					{"requests", "database connection refused"},
					{"errors", "request failed with unexpected error"},
				} {
					next := observation{source: "check"}
					if remainingKind == "metrics" {
						next.hasMetric = true
						next.metric = metricHandoff{name: sample.metricName, value: 1, timestamp: 1000}
					} else {
						next.log = &logObs{content: sample.logContent, status: "error", timestampMs: 1_000_000}
					}
					remainingCh <- next

					// Acknowledge processing on the surviving queue before sending
					// another observation. Flush() requires both queues to be open.
					processed := make(chan struct{})
					remainingCh <- observation{flush: processed}
					<-processed
					require.Len(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: namespace}), i+1)
				}
			})
		}
	}
}
