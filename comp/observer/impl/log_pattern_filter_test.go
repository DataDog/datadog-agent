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
// Helpers
// ---------------------------------------------------------------------------

// logPatternAnomalyWithRef builds a minimal log_pattern_extractor anomaly with a
// storage-backed SourceRef for the given series ref and aggregation.
func logPatternAnomalyWithRef(ref observerdef.SeriesRef, agg observerdef.Aggregate) observerdef.Anomaly {
	return observerdef.Anomaly{
		Source: observerdef.SeriesDescriptor{
			Namespace: LogPatternExtractorName,
			Aggregate: agg,
		},
		SourceRef: &observerdef.QueryHandle{Ref: ref, Aggregate: agg},
	}
}

// populateLogPatternStorage adds `logsPerSec` count-1 writes per second for
// [startSec, endSec) into the storage under the log_pattern_extractor namespace.
// Returns the SeriesRef for the written series.
func populateLogPatternStorage(s *timeSeriesStorage, metricName string, startSec, endSec, logsPerSec int64) observerdef.SeriesRef {
	for sec := startSec; sec < endSec; sec++ {
		for i := int64(0); i < logsPerSec; i++ {
			s.Add(LogPatternExtractorName, metricName, 1.0, sec, nil)
		}
	}
	metas := s.ListSeries(observerdef.SeriesFilter{Namespace: LogPatternExtractorName})
	return metas[0].Ref
}

// ---------------------------------------------------------------------------
// logPatternRateFilter unit tests — fallback path (no SourceRef)
// ---------------------------------------------------------------------------

func TestLogPatternRateFilter_Name(t *testing.T) {
	f := NewLogPatternRateFilter()
	assert.Equal(t, "log_pattern_rate_filter", f.Name())
}

func TestLogPatternRateFilter_PassesNonLogPatternNamespace(t *testing.T) {
	f := NewLogPatternRateFilter()
	storage := newTimeSeriesStorage()

	for _, ns := range []string{"dogstatsd", "cpu_detector", "", "log_pattern_extractor_v2"} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: ns},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 0.0},
		}
		assert.False(t, f.ShouldFilterOut(a, storage, 100), "unexpected filter for namespace %q", ns)
	}
}

// Fallback: no SourceRef, use CurrentValue.
func TestLogPatternRateFilter_FallbackPassesAboveThreshold(t *testing.T) {
	f := NewLogPatternRateFilter()
	storage := newTimeSeriesStorage()

	for _, rate := range []float64{1.0, 1.01, 5.0, 100.0} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: rate},
		}
		assert.False(t, f.ShouldFilterOut(a, storage, 100), "should not filter CurrentValue=%.2f (>= 1 log/s)", rate)
	}
}

func TestLogPatternRateFilter_FallbackFiltersBelowThreshold(t *testing.T) {
	f := NewLogPatternRateFilter()
	storage := newTimeSeriesStorage()

	for _, rate := range []float64{0.0, 0.1, 0.5, 0.99} {
		a := observerdef.Anomaly{
			Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: rate},
		}
		assert.True(t, f.ShouldFilterOut(a, storage, 100), "should filter CurrentValue=%.2f (< 1 log/s)", rate)
	}
}

func TestLogPatternRateFilter_FallbackPassesNilDebugInfo(t *testing.T) {
	f := NewLogPatternRateFilter()
	storage := newTimeSeriesStorage()

	a := observerdef.Anomaly{
		Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
		DebugInfo: nil,
	}
	assert.False(t, f.ShouldFilterOut(a, storage, 100))
}

// ---------------------------------------------------------------------------
// logPatternRateFilter unit tests — windowed average path (with SourceRef)
// ---------------------------------------------------------------------------

func TestLogPatternRateFilter_WindowedAverageFiltersLowRate(t *testing.T) {
	// 30 logs over 60 seconds = 0.5 log/s average → should be filtered.
	s := newTimeSeriesStorage()
	ref := populateLogPatternStorage(s, "log.pattern.count", 1, 61, 1) // 1 log/s × 60s = 60 logs... but
	// Override: spread only 30 logs over 60s by writing 1 log every other second.
	s2 := newTimeSeriesStorage()
	for sec := int64(1); sec <= 60; sec += 2 {
		s2.Add(LogPatternExtractorName, "log.pattern.count", 1.0, sec, nil)
	}
	metas := s2.ListSeries(observerdef.SeriesFilter{Namespace: LogPatternExtractorName})
	ref = metas[0].Ref

	a := logPatternAnomalyWithRef(ref, observerdef.AggregateCount)
	f := NewLogPatternRateFilter()

	assert.True(t, f.ShouldFilterOut(a, s2, 60), "0.5 log/s average should be filtered")
}

func TestLogPatternRateFilter_WindowedAveragePassesHighRate(t *testing.T) {
	// 5 logs/s × 60 seconds = 300 total → average 5 log/s → should pass.
	s := newTimeSeriesStorage()
	ref := populateLogPatternStorage(s, "log.pattern.count", 1, 61, 5)

	a := logPatternAnomalyWithRef(ref, observerdef.AggregateCount)
	f := NewLogPatternRateFilter()

	assert.False(t, f.ShouldFilterOut(a, s, 60), "5 log/s average should not be filtered")
}

func TestLogPatternRateFilter_WindowedAverageExactlyAtThreshold(t *testing.T) {
	// Exactly 1 log/s (60 logs over 60s) → average == 1.0 → not < 1.0, should pass.
	s := newTimeSeriesStorage()
	ref := populateLogPatternStorage(s, "log.pattern.count", 1, 61, 1)

	a := logPatternAnomalyWithRef(ref, observerdef.AggregateCount)
	f := NewLogPatternRateFilter()

	assert.False(t, f.ShouldFilterOut(a, s, 60), "exactly 1 log/s should not be filtered")
}

func TestLogPatternRateFilter_WindowedAverageOnlyLooksAtLastMinute(t *testing.T) {
	// Seconds 1-60: 5 logs/s (busy). Seconds 61-120: 0 logs/s (silent).
	// At dataTime=120, the 60s window covers [61,120] where rate=0 → filtered.
	// At dataTime=60,  the 60s window covers [1,60]  where rate=5 → passes.
	s := newTimeSeriesStorage()
	ref := populateLogPatternStorage(s, "log.pattern.count", 1, 61, 5)

	a := logPatternAnomalyWithRef(ref, observerdef.AggregateCount)
	f := NewLogPatternRateFilter()

	assert.False(t, f.ShouldFilterOut(a, s, 60), "window [1,60] at 5 log/s should not be filtered")
	assert.True(t, f.ShouldFilterOut(a, s, 120), "window [61,120] with no data should be filtered")
}

// ---------------------------------------------------------------------------
// Engine integration: detectorFilters wire-up
// ---------------------------------------------------------------------------

// alwaysFilterDetectorFilter is a filter that always discards every anomaly.
type alwaysFilterDetectorFilter struct{}

func (f *alwaysFilterDetectorFilter) Name() string { return "always_filter" }
func (f *alwaysFilterDetectorFilter) ShouldFilterOut(_ observerdef.Anomaly, _ observerdef.StorageReader, _ int64) bool {
	return true
}

// neverFilterDetectorFilter is a filter that never discards any anomaly.
type neverFilterDetectorFilter struct{}

func (f *neverFilterDetectorFilter) Name() string { return "never_filter" }
func (f *neverFilterDetectorFilter) ShouldFilterOut(_ observerdef.Anomaly, _ observerdef.StorageReader, _ int64) bool {
	return false
}

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
// Catalog integration: filters are wired through defaultCatalog
// ---------------------------------------------------------------------------

func TestCatalog_DefaultFiltersContainLogPatternRateFilter(t *testing.T) {
	catalog := defaultCatalog()
	_, _, _, filters, _ := catalog.Instantiate(ComponentSettings{})
	require.NotEmpty(t, filters)

	var found bool
	for _, f := range filters {
		if f.Name() == "log_pattern_rate_filter" {
			found = true
			break
		}
	}
	assert.True(t, found, "default catalog should produce a log_pattern_rate_filter")
}
