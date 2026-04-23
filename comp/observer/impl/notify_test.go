// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// sumRangeStorage is a minimal StorageReader that answers SumRange calls via a
// user-supplied function and panics on all other methods (they are unused in
// notify_test.go).
type sumRangeStorage struct {
	fn func(handle observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate) float64
}

func (s *sumRangeStorage) SumRange(handle observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate) float64 {
	return s.fn(handle, start, end, agg)
}
func (s *sumRangeStorage) ListSeries(_ observerdef.SeriesFilter) []observerdef.SeriesMeta {
	panic("not implemented")
}
func (s *sumRangeStorage) GetSeriesRange(_ observerdef.SeriesRef, _, _ int64, _ observerdef.Aggregate) *observerdef.Series {
	panic("not implemented")
}
func (s *sumRangeStorage) ForEachPoint(_ observerdef.SeriesRef, _, _ int64, _ observerdef.Aggregate, _ func(*observerdef.Series, observerdef.Point)) bool {
	panic("not implemented")
}
func (s *sumRangeStorage) PointCount(_ observerdef.SeriesRef) int { panic("not implemented") }
func (s *sumRangeStorage) PointCountUpTo(_ observerdef.SeriesRef, _ int64) int {
	panic("not implemented")
}
func (s *sumRangeStorage) WriteGeneration(_ observerdef.SeriesRef) int64 { panic("not implemented") }
func (s *sumRangeStorage) SeriesGeneration() uint64                      { panic("not implemented") }

func TestIsLogDerivedAnomaly_LogMetricsExtractorWithPattern(t *testing.T) {
	a := observerdef.Anomaly{
		Type:   observerdef.AnomalyTypeMetric,
		Source: observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
		Context: &observerdef.MetricContext{
			Pattern: "C3:C8_C1",
			Example: "ERROR: connection refused to db.prod:5432",
		},
	}
	assert.True(t, isLogDerivedAnomaly(a))
}

func TestIsLogDerivedAnomaly_LogMetricsExtractorExampleOnlyNoPattern(t *testing.T) {
	// Even with an empty pattern, a non-empty example qualifies.
	a := observerdef.Anomaly{
		Type:   observerdef.AnomalyTypeMetric,
		Source: observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
		Context: &observerdef.MetricContext{
			Pattern: "",
			Example: "some log line",
		},
	}
	assert.True(t, isLogDerivedAnomaly(a))
}

func TestIsLogDerivedAnomaly_LogMetricsExtractorNoContext(t *testing.T) {
	a := observerdef.Anomaly{
		Type:    observerdef.AnomalyTypeMetric,
		Source:  observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
		Context: nil,
	}
	assert.False(t, isLogDerivedAnomaly(a))
}

func TestBuildChangeMessage_LogMetricsExtractorUsesExample(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
				Context: &observerdef.MetricContext{
					Pattern: "C3:C8_C1",
					Example: "ERROR: connection refused to db.prod:5432",
				},
			},
		},
	}
	msg := buildChangeMessage(c, nil)
	assert.Contains(t, msg, "Log frequency change detected")
	assert.Contains(t, msg, "ERROR: connection refused to db.prod:5432")
	assert.NotContains(t, msg, "C3:C8_C1") // tokenized signature should not appear
}

func TestBuildChangeMessage_LogMetricsExtractorFallsBackToPatternWhenNoExample(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
				Context: &observerdef.MetricContext{
					Pattern: "C3:C8_C1",
					Example: "",
				},
			},
		},
	}
	msg := buildChangeMessage(c, nil)
	assert.Contains(t, msg, "Log frequency change detected")
	assert.Contains(t, msg, "C3:C8_C1")
}

func TestBuildEventTags_LogMetricsExtractorTreatedAsLog(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
				Context: &observerdef.MetricContext{
					Pattern: "C3:C8_C1",
					Example: "some log line",
				},
			},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "anomaly_type:log")
	assert.NotContains(t, tags, "anomaly_type:metric")
}

func TestBuildEventTags_BaseTagsAlwaysPresent(t *testing.T) {
	c := observerdef.ActiveCorrelation{Pattern: "kernel_bottleneck"}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "source:agent-q-branch-observer")
	assert.Contains(t, tags, "pattern:kernel_bottleneck")
}

func TestBuildEventTags_MetricAnomalyType(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{Type: observerdef.AnomalyTypeMetric, Source: observerdef.SeriesDescriptor{Namespace: "dogstatsd"}},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "anomaly_type:metric")
	assert.NotContains(t, tags, "anomaly_type:log")
}

func TestBuildEventTags_LogAnomalyType(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{Type: observerdef.AnomalyTypeLog, Source: observerdef.SeriesDescriptor{Namespace: "log_detector"}},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "anomaly_type:log")
	assert.NotContains(t, tags, "anomaly_type:metric")
}

func TestBuildEventTags_LogDerivedMetricAnomaly(t *testing.T) {
	// A metric anomaly originating from log_pattern_extractor with a pattern
	// context is treated as a log anomaly.
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type: observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{
					Namespace: LogPatternExtractorName,
				},
				Context: &observerdef.MetricContext{
					Pattern: "some log pattern",
				},
			},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "anomaly_type:log")
	assert.NotContains(t, tags, "anomaly_type:metric")
}

func TestBuildEventTags_BothTypes(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{Type: observerdef.AnomalyTypeMetric, Source: observerdef.SeriesDescriptor{Namespace: "dogstatsd"}},
			{Type: observerdef.AnomalyTypeLog, Source: observerdef.SeriesDescriptor{Namespace: "log_detector"}},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "anomaly_type:metric")
	assert.Contains(t, tags, "anomaly_type:log")
}

func TestBuildEventTags_DimensionalTagsFromSourceTags(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type: observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{
					Namespace: "dogstatsd",
					Tags:      []string{"service:web", "env:prod", "host:h1", "version:1.0"},
				},
			},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "service:web")
	assert.Contains(t, tags, "env:prod")
	assert.Contains(t, tags, "host:h1")
	assert.NotContains(t, tags, "version:1.0") // non-dimensional tags not propagated
}

func TestBuildEventTags_DimensionalTagsFromSplitTags(t *testing.T) {
	// Log-derived anomalies carry dimensional info in Context.SplitTags.
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
				Context: &observerdef.MetricContext{
					Pattern: "some log pattern",
					SplitTags: map[string]string{
						"service": "api",
						"env":     "staging",
						"host":    "h2",
					},
				},
			},
		},
	}
	tags := buildEventTags(c)
	assert.Contains(t, tags, "service:api")
	assert.Contains(t, tags, "env:staging")
	assert.Contains(t, tags, "host:h2")
}

func TestBuildEventTags_DeduplicatesDimensions(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "p",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: "ns", Tags: []string{"service:web"}},
			},
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: "ns", Tags: []string{"service:web"}},
			},
		},
	}
	tags := buildEventTags(c)
	count := 0
	for _, t := range tags {
		if t == "service:web" {
			count++
		}
	}
	assert.Equal(t, 1, count, "service:web should appear exactly once")
}

func TestBuildEventTags_SourceAndPatternAreFirstTwo(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "mypat",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: "ns", Tags: []string{"service:svc"}},
			},
		},
	}
	tags := buildEventTags(c)
	assert.Equal(t, "source:agent-q-branch-observer", tags[0])
	assert.Equal(t, "pattern:mypat", tags[1])
	// Remaining tags are sorted
	rest := tags[2:]
	sorted := make([]string, len(rest))
	copy(sorted, rest)
	sort.Strings(sorted)
	assert.Equal(t, sorted, rest)
}

// --- isSignificantRateChange ---

func TestIsSignificantRateChange_BothAboveThresholds(t *testing.T) {
	// curr is 3× prev → relative change = 2/3 > 0.5, absolute > 0.1
	assert.True(t, isSignificantRateChange(1.0, 3.0))
}

func TestIsSignificantRateChange_RelativeBelowThreshold(t *testing.T) {
	// 10 → 13: relative change = 3/13 ≈ 0.23 < 0.5
	assert.False(t, isSignificantRateChange(10.0, 13.0))
}

func TestIsSignificantRateChange_AbsoluteBelowThreshold(t *testing.T) {
	// 0.01 → 0.05: absolute change = 0.04 < 0.1, even though relative is large
	assert.False(t, isSignificantRateChange(0.01, 0.05))
}

func TestIsSignificantRateChange_BothZero(t *testing.T) {
	assert.False(t, isSignificantRateChange(0.0, 0.0))
}

func TestIsSignificantRateChange_RateDrops(t *testing.T) {
	// 5 → 1: large drop, should be significant
	assert.True(t, isSignificantRateChange(5.0, 1.0))
}

// --- rate display in buildChangeMessage ---

// makeStorageWithRates returns a sumRangeStorage whose SumRange returns
// prevTotal for the earlier window and currTotal for the current window,
// identified by the end timestamp.
func makeStorageWithRates(ts int64, prevTotal, currTotal float64) *sumRangeStorage {
	return &sumRangeStorage{
		fn: func(_ observerdef.SeriesRef, start, end int64, _ observerdef.Aggregate) float64 {
			if end == ts-logPatternRateWindowSec {
				return prevTotal
			}
			return currTotal
		},
	}
}

func makeLogPatternAnomaly(ts int64) observerdef.Anomaly {
	return observerdef.Anomaly{
		Type:      observerdef.AnomalyTypeMetric,
		Timestamp: ts,
		Source:    observerdef.SeriesDescriptor{Namespace: LogPatternExtractorName},
		SourceRef: &observerdef.QueryHandle{Ref: observerdef.SeriesRef(42)},
		Context: &observerdef.MetricContext{
			Pattern: "connection refused",
			Example: "ERROR: connection refused to db:5432",
		},
	}
}

func TestBuildChangeMessage_RateChangedDisplay(t *testing.T) {
	const ts = int64(10000)
	// prev window: 6 logs/s, curr window: 0.5 log/s — large drop, should show "changed from"
	storage := makeStorageWithRates(ts, 6.0*logPatternPrevRateWindowSec, 0.5*logPatternRateWindowSec)
	c := observerdef.ActiveCorrelation{
		Pattern:   "p",
		Anomalies: []observerdef.Anomaly{makeLogPatternAnomaly(ts)},
	}
	msg := buildChangeMessage(c, storage)
	assert.Contains(t, msg, fmt.Sprintf("rate: %.1flog/s (was %.1flog/s last minutes)", 0.5, 6.0))
}

func TestBuildChangeMessage_RateUnchanged_ShowsPlainRate(t *testing.T) {
	const ts = int64(10000)
	// Both windows return the same rate → not significant, plain display
	storage := makeStorageWithRates(ts, 3.0*logPatternPrevRateWindowSec, 3.0*logPatternRateWindowSec)
	c := observerdef.ActiveCorrelation{
		Pattern:   "p",
		Anomalies: []observerdef.Anomaly{makeLogPatternAnomaly(ts)},
	}
	msg := buildChangeMessage(c, storage)
	assert.Contains(t, msg, fmt.Sprintf("\n\trate: %.1flog/s", 3.0))
	// No previous rate display
	assert.NotContains(t, msg, "was")
	assert.NotContains(t, msg, "last minutes")
}

func TestBuildChangeMessage_LogFrequency_RateChangedDisplay(t *testing.T) {
	const ts = int64(10000)
	// prev window: 0.2 log/s, curr window: 5.0 log/s — large jump
	storage := makeStorageWithRates(ts, 0.2*logPatternPrevRateWindowSec, 5.0*logPatternRateWindowSec)
	a := observerdef.Anomaly{
		Type:      observerdef.AnomalyTypeMetric,
		Timestamp: ts,
		Source:    observerdef.SeriesDescriptor{Namespace: LogMetricsExtractorName},
		SourceRef: &observerdef.QueryHandle{Ref: observerdef.SeriesRef(7)},
		Context: &observerdef.MetricContext{
			Example: "panic: runtime error",
		},
	}
	c := observerdef.ActiveCorrelation{Pattern: "p", Anomalies: []observerdef.Anomaly{a}}
	msg := buildChangeMessage(c, storage)
	assert.Contains(t, msg, fmt.Sprintf("rate: %.1flog/s (was %.1flog/s last minutes)", 5.0, 0.2))
}
