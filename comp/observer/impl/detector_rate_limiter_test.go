// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync/atomic"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

type fixedSourceDetector struct {
	anomalies []observerdef.Anomaly
	idx       atomic.Int64
}

func (f *fixedSourceDetector) Name() string { return "fixed" }
func (f *fixedSourceDetector) Detect(_ observerdef.StorageReader, ts int64) observerdef.DetectionResult {
	i := int(f.idx.Add(1) - 1)
	a := f.anomalies[i%len(f.anomalies)]
	a.Timestamp = ts
	return observerdef.DetectionResult{Anomalies: []observerdef.Anomaly{a}}
}

func makeAnomaly(name string) observerdef.Anomaly {
	return observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: name, Aggregate: observerdef.AggregateAverage},
		DetectorName: "fixed",
	}
}

func TestRateLimitedDetector_PassCount(t *testing.T) {
	tests := []struct {
		name      string
		anomalies []observerdef.Anomaly
		calls     int
		want      int
	}{
		{
			name:      "single source capped at burst",
			anomalies: []observerdef.Anomaly{makeAnomaly("cpu.user")},
			calls:     20,
			want:      rateLimitBurst,
		},
		{
			name:      "two distinct sources each get independent burst quota",
			anomalies: []observerdef.Anomaly{makeAnomaly("cpu.user"), makeAnomaly("mem.used")},
			calls:     20,
			want:      rateLimitBurst * 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &fixedSourceDetector{anomalies: tt.anomalies}
			rld := newRateLimitedDetector(inner)
			passed := 0
			for i := 0; i < tt.calls; i++ {
				passed += len(rld.Detect(nil, int64(i)).Anomalies)
			}
			if passed != tt.want {
				t.Errorf("Detect() passed count = %d, want %d", passed, tt.want)
			}
		})
	}
}

// TestRateLimitedDetector_NoDropNoTelemetry checks that no telemetry is emitted
// when no anomalies are dropped (avoids noise in the state view).
func TestRateLimitedDetector_NoDropNoTelemetry(t *testing.T) {
	inner := &fixedSourceDetector{anomalies: []observerdef.Anomaly{makeAnomaly("cpu.user")}}
	rld := newRateLimitedDetector(inner)

	// Only rateLimitBurst calls — none should be dropped.
	for i := 0; i < rateLimitBurst; i++ {
		result := rld.Detect(nil, int64(i))
		if len(result.Telemetry) != 0 {
			t.Errorf("Detect(nil, %d).Telemetry = %v, want empty (no anomalies dropped)", i, result.Telemetry)
		}
	}
}
