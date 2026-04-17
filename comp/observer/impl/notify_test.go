// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

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
