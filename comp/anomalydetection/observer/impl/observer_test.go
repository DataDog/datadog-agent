// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

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

type countingSeriesDetector struct {
	anomalies []observerdef.Anomaly
}

func (d *countingSeriesDetector) Name() string { return "counting" }

func (d *countingSeriesDetector) Detect(_ observerdef.Series) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: append([]observerdef.Anomaly(nil), d.anomalies...),
	}
}
