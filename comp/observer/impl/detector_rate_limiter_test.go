// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

type fixedSourceDetector struct {
	anomalies []observerdef.Anomaly
	idx       int
}

func (f *fixedSourceDetector) Name() string { return "fixed" }
func (f *fixedSourceDetector) Detect(_ observerdef.StorageReader, ts int64) observerdef.DetectionResult {
	a := f.anomalies[f.idx%len(f.anomalies)]
	f.idx++
	a.Timestamp = ts
	return observerdef.DetectionResult{Anomalies: []observerdef.Anomaly{a}}
}

func makeRLAnomaly(name string) observerdef.Anomaly {
	return observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: name, Aggregate: observerdef.AggregateAverage},
		DetectorName: "fixed",
	}
}

// TestRateLimitedDetector_SingleSource checks that a single noisy source is
// capped at rateLimitBurst anomalies within the rate window.
func TestRateLimitedDetector_SingleSource(t *testing.T) {
	inner := &fixedSourceDetector{anomalies: []observerdef.Anomaly{makeRLAnomaly("cpu.user")}}
	rld := newRateLimitedDetector(inner)

	passed := 0
	for i := 0; i < 20; i++ {
		passed += len(rld.Detect(nil, int64(i)).Anomalies)
	}
	assert.Equal(t, rateLimitBurst, passed)
}

// TestRateLimitedDetector_TwoSources checks that two distinct metric series
// each get their own independent burst quota.
func TestRateLimitedDetector_TwoSources(t *testing.T) {
	inner := &fixedSourceDetector{anomalies: []observerdef.Anomaly{
		makeRLAnomaly("cpu.user"),
		makeRLAnomaly("mem.used"),
	}}
	rld := newRateLimitedDetector(inner)

	passed := 0
	for i := 0; i < 20; i++ {
		passed += len(rld.Detect(nil, int64(i)).Anomalies)
	}
	assert.Equal(t, rateLimitBurst*2, passed)
}

// TestRateLimitedDetector_NoDropNoTelemetry checks that no telemetry is emitted
// when no anomalies are dropped (avoids noise in the state view).
func TestRateLimitedDetector_NoDropNoTelemetry(t *testing.T) {
	inner := &fixedSourceDetector{anomalies: []observerdef.Anomaly{makeRLAnomaly("cpu.user")}}
	rld := newRateLimitedDetector(inner)

	// Only rateLimitBurst calls — none should be dropped.
	for i := 0; i < rateLimitBurst; i++ {
		result := rld.Detect(nil, int64(i))
		assert.Empty(t, result.Telemetry, "no telemetry expected when nothing is dropped")
	}
}
