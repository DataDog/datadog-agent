// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

func TestSeriesDetectorAdapter_DoesNotReemitOutputsWithoutNewData(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)

	adapter := newSeriesDetectorAdapter(&countingSeriesDetector{
		anomalies: []observerdef.Anomaly{{
			Title:       "spike",
			Description: "detected spike",
			Timestamp:   100,
		}},
		telemetry: []observerdef.ObserverTelemetry{{
			DetectorName: "counting",
		}},
	}, []observerdef.Aggregate{observerdef.AggregateAverage})

	first := adapter.Detect(storage, 100)
	if len(first.Anomalies) != 1 {
		t.Fatalf("expected 1 anomaly on first detect, got %d", len(first.Anomalies))
	}
	if len(first.Telemetry) != 1 {
		t.Fatalf("expected 1 telemetry event on first detect, got %d", len(first.Telemetry))
	}

	second := adapter.Detect(storage, 101)
	if len(second.Anomalies) != 0 {
		t.Fatalf("expected 0 anomalies without new data, got %d", len(second.Anomalies))
	}
	if len(second.Telemetry) != 0 {
		t.Fatalf("expected 0 telemetry events without new data, got %d", len(second.Telemetry))
	}
}

type countingSeriesDetector struct {
	anomalies []observerdef.Anomaly
	telemetry []observerdef.ObserverTelemetry
}

func (d *countingSeriesDetector) Name() string { return "counting" }

func (d *countingSeriesDetector) Detect(_ observerdef.Series) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: append([]observerdef.Anomaly(nil), d.anomalies...),
		Telemetry: append([]observerdef.ObserverTelemetry(nil), d.telemetry...),
	}
}
