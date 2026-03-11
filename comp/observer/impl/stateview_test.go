// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

func TestStateView_StorageAccess(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)
	storage.Add("ns", "cpu", 2.0, 101, nil)
	storage.Add("ns", "mem", 512.0, 100, []string{"host:a"})

	e := newEngine(engineConfig{storage: storage})
	sv := e.StateView()

	// ListSeries
	keys := sv.ListSeries(observerdef.SeriesFilter{Namespace: "ns"})
	if len(keys) != 2 {
		t.Fatalf("expected 2 series, got %d", len(keys))
	}

	// GetSeriesRange
	series := sv.GetSeriesRange(observerdef.SeriesKey{Namespace: "ns", Name: "cpu"}, 0, 200, observerdef.AggregateAverage)
	if series == nil {
		t.Fatal("expected series data, got nil")
	}
	if len(series.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(series.Points))
	}

	// ScenarioBounds
	start, end, ok := sv.ScenarioBounds()
	if !ok {
		t.Fatal("expected bounds to be available")
	}
	if start != 100 || end != 101 {
		t.Fatalf("expected bounds [100, 101], got [%d, %d]", start, end)
	}
}

func TestStateView_Anomalies(t *testing.T) {
	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
	})
	sv := e.StateView()

	// Initially empty
	if len(sv.Anomalies()) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(sv.Anomalies()))
	}
	if sv.TotalAnomalyCount() != 0 {
		t.Fatalf("expected 0 total anomalies, got %d", sv.TotalAnomalyCount())
	}

	// Add some anomalies via the engine
	e.captureRawAnomaly(observerdef.Anomaly{
		Source:       "cpu",
		DetectorName: "cusum",
		Timestamp:    100,
	})
	e.captureRawAnomaly(observerdef.Anomaly{
		Source:       "mem",
		DetectorName: "bocpd",
		Timestamp:    101,
	})

	if len(sv.Anomalies()) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(sv.Anomalies()))
	}
	if sv.TotalAnomalyCount() != 2 {
		t.Fatalf("expected 2 total anomalies, got %d", sv.TotalAnomalyCount())
	}
	if sv.UniqueAnomalySourceCount() != 2 {
		t.Fatalf("expected 2 unique sources, got %d", sv.UniqueAnomalySourceCount())
	}

	// DetectorAnomalies filters correctly
	cusumAnomalies := sv.DetectorAnomalies("cusum")
	if len(cusumAnomalies) != 1 {
		t.Fatalf("expected 1 cusum anomaly, got %d", len(cusumAnomalies))
	}
	if cusumAnomalies[0].DetectorName != "cusum" {
		t.Fatalf("expected cusum, got %s", cusumAnomalies[0].DetectorName)
	}

	// AnomaliesByDetector groups correctly
	byDetector := sv.AnomaliesByDetector()
	if len(byDetector) != 2 {
		t.Fatalf("expected 2 detector groups, got %d", len(byDetector))
	}
	if len(byDetector["cusum"]) != 1 {
		t.Fatalf("expected 1 cusum anomaly, got %d", len(byDetector["cusum"]))
	}
	if len(byDetector["bocpd"]) != 1 {
		t.Fatalf("expected 1 bocpd anomaly, got %d", len(byDetector["bocpd"]))
	}

	// AnomaliesForSeries filters by SourceSeriesID
	e.captureRawAnomaly(observerdef.Anomaly{
		Source:         "disk",
		DetectorName:   "cusum",
		Timestamp:      102,
		SourceSeriesID: "ns|disk:avg|",
	})
	diskAnomalies := sv.AnomaliesForSeries("ns|disk:avg|")
	if len(diskAnomalies) != 1 {
		t.Fatalf("expected 1 disk anomaly, got %d", len(diskAnomalies))
	}
	if diskAnomalies[0].Source != "disk" {
		t.Fatalf("expected disk source, got %s", diskAnomalies[0].Source)
	}
	// Empty SourceSeriesID should not match
	emptyAnomalies := sv.AnomaliesForSeries("")
	if len(emptyAnomalies) != 2 {
		t.Fatalf("expected 2 anomalies with empty SourceSeriesID, got %d", len(emptyAnomalies))
	}
}

func TestStateView_DetectorsAndCorrelators(t *testing.T) {
	detector := &mockDetector{name: "mock_det"}
	correlator := &mockCorrelator{name: "mock_corr"}

	e := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		detectors:   []observerdef.Detector{detector},
		correlators: []observerdef.Correlator{correlator},
	})
	sv := e.StateView()

	detectors := sv.ListDetectors()
	if len(detectors) != 1 || detectors[0].Name != "mock_det" {
		t.Fatalf("unexpected detectors: %+v", detectors)
	}

	correlators := sv.ListCorrelators()
	if len(correlators) != 1 || correlators[0].Name != "mock_corr" {
		t.Fatalf("unexpected correlators: %+v", correlators)
	}
}

func TestStateView_SchedulingState(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)
	storage.Add("ns", "cpu", 2.0, 200, nil)

	e := newEngine(engineConfig{storage: storage})
	sv := e.StateView()

	if sv.LastAnalyzedTime() != 0 {
		t.Fatalf("expected 0, got %d", sv.LastAnalyzedTime())
	}

	e.Advance(150)
	if sv.LastAnalyzedTime() != 150 {
		t.Fatalf("expected 150, got %d", sv.LastAnalyzedTime())
	}
}

func TestStateView_Telemetry(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	sv := e.StateView()

	// Initially empty
	if len(sv.Telemetry()) != 0 {
		t.Fatalf("expected 0 telemetry, got %d", len(sv.Telemetry()))
	}

	// Manually accumulate telemetry
	e.telemetryMu.Lock()
	e.accumulatedTelemetry = append(e.accumulatedTelemetry, observerdef.ObserverTelemetry{
		DetectorName: "test",
	})
	e.telemetryMu.Unlock()

	tel := sv.Telemetry()
	if len(tel) != 1 || tel[0].DetectorName != "test" {
		t.Fatalf("unexpected telemetry: %+v", tel)
	}
}

// mockDetector is a minimal Detector for testing.
type mockDetector struct {
	name string
}

func (d *mockDetector) Name() string { return d.name }
func (d *mockDetector) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}

// mockCorrelator is a minimal Correlator for testing.
type mockCorrelator struct {
	name string
}

func (c *mockCorrelator) Name() string                                        { return c.name }
func (c *mockCorrelator) ProcessAnomaly(_ observerdef.Anomaly)                {}
func (c *mockCorrelator) Advance(_ int64)                                     {}
func (c *mockCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation { return nil }
func (c *mockCorrelator) Reset()                                              {}
