// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ---------------------------------------------------------------------------
// logPatternRateFilter unit tests
// ---------------------------------------------------------------------------

func TestLogPatternRateFilter_Name(t *testing.T) {
	f := NewLogPatternRateFilter()
	assert.Equal(t, "log_pattern_rate_filter", f.Name())
}

func TestLogPatternRateFilter_PassesNonLogPatternNamespace(t *testing.T) {
	f := NewLogPatternRateFilter()

	// Anomalies from any other namespace must never be filtered out,
	// regardless of CurrentValue.
	for _, ns := range []string{"dogstatsd", "cpu_detector", "", "log_pattern_extractor_v2"} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: ns},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 0.0},
		}
		assert.False(t, f.ShouldFilterOut(a), "unexpected filter for namespace %q", ns)
	}
}

func TestLogPatternRateFilter_PassesAboveThreshold(t *testing.T) {
	f := NewLogPatternRateFilter()

	for _, rate := range []float64{1.0, 1.01, 5.0, 100.0} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: rate},
		}
		assert.False(t, f.ShouldFilterOut(a), "should not filter rate=%.2f (>= 1 log/s)", rate)
	}
}

func TestLogPatternRateFilter_FiltersBelowThreshold(t *testing.T) {
	f := NewLogPatternRateFilter()

	for _, rate := range []float64{0.0, 0.1, 0.5, 0.99} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: rate},
		}
		assert.True(t, f.ShouldFilterOut(a), "should filter rate=%.2f (< 1 log/s)", rate)
	}
}

func TestLogPatternRateFilter_PassesNilDebugInfo(t *testing.T) {
	f := NewLogPatternRateFilter()

	// When DebugInfo is absent we cannot evaluate the rate, so the anomaly
	// must pass through to avoid silently dropping unrecognised detectors.
	a := observerdef.Anomaly{
		Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
		DebugInfo: nil,
	}
	assert.False(t, f.ShouldFilterOut(a))
}

// ---------------------------------------------------------------------------
// Engine integration: detectorFilters wire-up
// ---------------------------------------------------------------------------

// alwaysFilterDetectorFilter is a filter that always discards every anomaly.
type alwaysFilterDetectorFilter struct{}

func (f *alwaysFilterDetectorFilter) Name() string                               { return "always_filter" }
func (f *alwaysFilterDetectorFilter) ShouldFilterOut(_ observerdef.Anomaly) bool { return true }

// neverFilterDetectorFilter is a filter that never discards any anomaly.
type neverFilterDetectorFilter struct{}

func (f *neverFilterDetectorFilter) Name() string                               { return "never_filter" }
func (f *neverFilterDetectorFilter) ShouldFilterOut(_ observerdef.Anomaly) bool { return false }

func TestEngine_DetectorFilterSuppressesAnomalies(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{Source: observerdef.SeriesDescriptor{Name: "cpu", Aggregate: observerdef.AggregateAverage}, DetectorName: "test", Timestamp: 99},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test", anomalies: anomalies},
		},
		detectorFilters: []observerdef.DetectorFilter{&alwaysFilterDetectorFilter{}},
	})

	result := e.Advance(100)

	assert.Empty(t, result.anomalies, "all anomalies should have been filtered out")
	assert.Equal(t, 0, e.TotalAnomalyCount(), "filtered anomalies should not increment total count")
}

func TestEngine_DetectorFilterNeverFilterPassesAnomalies(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{Source: observerdef.SeriesDescriptor{Name: "cpu", Aggregate: observerdef.AggregateAverage}, DetectorName: "test", Timestamp: 99},
		{Source: observerdef.SeriesDescriptor{Name: "mem", Aggregate: observerdef.AggregateAverage}, DetectorName: "test", Timestamp: 99},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test", anomalies: anomalies},
		},
		detectorFilters: []observerdef.DetectorFilter{&neverFilterDetectorFilter{}},
	})

	result := e.Advance(100)

	assert.Len(t, result.anomalies, 2, "no anomalies should have been filtered out")
}

func TestEngine_NoFiltersPassesAllAnomalies(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{Source: observerdef.SeriesDescriptor{Name: "cpu", Aggregate: observerdef.AggregateAverage}, DetectorName: "test", Timestamp: 99},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test", anomalies: anomalies},
		},
		// detectorFilters intentionally left nil
	})

	result := e.Advance(100)

	assert.Len(t, result.anomalies, 1)
}

func TestEngine_FilteredAnomalyDoesNotEmitEvent(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{Source: observerdef.SeriesDescriptor{Name: "cpu", Aggregate: observerdef.AggregateAverage}, DetectorName: "test", Timestamp: 99},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test", anomalies: anomalies},
		},
		detectorFilters: []observerdef.DetectorFilter{&alwaysFilterDetectorFilter{}},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)
	e.Advance(100)

	assert.Empty(t, sink.eventsOfKind(eventAnomalyCreated), "filtered anomaly should not emit an event")
}

// ---------------------------------------------------------------------------
// defaultDetectorFilters
// ---------------------------------------------------------------------------

func TestDefaultDetectorFilters_ContainsLogPatternRateFilter(t *testing.T) {
	filters := defaultDetectorFilters()
	require.NotEmpty(t, filters)

	var found bool
	for _, f := range filters {
		if f.Name() == "log_pattern_rate_filter" {
			found = true
			break
		}
	}
	assert.True(t, found, "defaultDetectorFilters should contain log_pattern_rate_filter")
}
