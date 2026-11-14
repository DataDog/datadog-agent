// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricsevaluatorimpl

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

func TestNewMonitorStateSizesWindow(t *testing.T) {
	for _, tc := range []struct {
		window time.Duration
		size   int
	}{
		{window: time.Minute, size: 2},
		{window: 5 * time.Minute, size: 6},
		{window: 90 * time.Second, size: 3}, // rounded up to 2 intervals
	} {
		ms := newMonitorState(metricsevaluator.Monitor{Window: tc.window}, time.Minute)
		assert.Len(t, ms.samples, tc.size, "window %s", tc.window)
		assert.Equal(t, metricsevaluator.StateNoData, ms.state)
	}
}

func TestAggregate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		aggregation metricsevaluator.Aggregation
		samples     []sample
		value       float64
		ok          bool
	}{
		{
			name:        "window not full",
			aggregation: metricsevaluator.AggregationAvg,
			samples:     []sample{{1, true}, {2, true}},
			ok:          false,
		},
		{
			name:        "avg",
			aggregation: metricsevaluator.AggregationAvg,
			samples:     []sample{{1, true}, {2, true}, {6, true}},
			value:       3,
			ok:          true,
		},
		{
			name:        "avg skips missing samples",
			aggregation: metricsevaluator.AggregationAvg,
			samples:     []sample{{2, true}, {0, false}, {4, true}},
			value:       3,
			ok:          true,
		},
		{
			name:        "max",
			aggregation: metricsevaluator.AggregationMax,
			samples:     []sample{{1, true}, {7, true}, {3, true}},
			value:       7,
			ok:          true,
		},
		{
			name:        "min",
			aggregation: metricsevaluator.AggregationMin,
			samples:     []sample{{4, true}, {7, true}, {3, true}},
			value:       3,
			ok:          true,
		},
		{
			name:        "delta",
			aggregation: metricsevaluator.AggregationDelta,
			samples:     []sample{{10, true}, {12, true}, {15, true}},
			value:       5,
			ok:          true,
		},
		{
			name:        "delta after counter reset",
			aggregation: metricsevaluator.AggregationDelta,
			samples:     []sample{{10, true}, {1, true}, {4, true}},
			value:       4,
			ok:          true,
		},
		{
			name:        "delta with a reset in the middle of the window",
			aggregation: metricsevaluator.AggregationDelta,
			samples:     []sample{{100, true}, {1, true}, {150, true}},
			value:       150,
			ok:          true,
		},
		{
			name:        "no sample has a value",
			aggregation: metricsevaluator.AggregationMax,
			samples:     []sample{{0, false}, {0, false}, {0, false}},
			ok:          false,
		},
		{
			name:        "ring wraps and drops the oldest samples",
			aggregation: metricsevaluator.AggregationDelta,
			samples:     []sample{{100, true}, {1, true}, {2, true}, {3, true}, {9, true}},
			value:       7,
			ok:          true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMonitorState(metricsevaluator.Monitor{Window: 2 * time.Minute, Aggregation: tc.aggregation}, time.Minute)
			for _, s := range tc.samples {
				ms.push(s)
			}
			value, ok := ms.aggregate()
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.value, value)
			}
		})
	}
}

func TestSeriesValue(t *testing.T) {
	reg := prometheus.NewRegistry()
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "memory"}, []string{"pool", "kind"})
	reg.MustRegister(gauge)
	gauge.WithLabelValues("a", "heap").Set(1)
	gauge.WithLabelValues("b", "heap").Set(2)
	gauge.WithLabelValues("b", "stack").Set(4)

	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	mf := families[0]

	for _, tc := range []struct {
		name  string
		tags  map[string]string
		value float64
		ok    bool
	}{
		{name: "no filter sums every series", tags: nil, value: 7, ok: true},
		{name: "single label", tags: map[string]string{"pool": "b"}, value: 6, ok: true},
		{name: "all labels", tags: map[string]string{"pool": "b", "kind": "stack"}, value: 4, ok: true},
		{name: "no matching series", tags: map[string]string{"pool": "c"}, ok: false},
		{name: "unknown label", tags: map[string]string{"zone": "a"}, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := seriesValue(mf, tc.tags)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.value, value)
		})
	}

	_, ok := seriesValue((*telemetry.MetricFamily)(nil), nil)
	assert.False(t, ok)
}

func TestMonitorSampleRatio(t *testing.T) {
	reg := prometheus.NewRegistry()
	usage := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "memory_usage"}, []string{"kind"})
	limit := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "memory_limit"}, []string{"kind"})
	reg.MustRegister(usage, limit)
	usage.WithLabelValues("agent").Set(300)
	limit.WithLabelValues("agent").Set(400)
	usage.WithLabelValues("cluster-agent").Set(100)
	limit.WithLabelValues("cluster-agent").Set(0)

	families, err := reg.Gather()
	require.NoError(t, err)
	byName := make(map[string]*telemetry.MetricFamily)
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}

	m := metricsevaluator.Monitor{Metric: "memory_usage", DivideBy: "memory_limit", Tags: map[string]string{"kind": "agent"}}
	assert.Equal(t, sample{value: 0.75, ok: true}, monitorSample(m, byName))

	m.Tags = map[string]string{"kind": "cluster-agent"}
	assert.Equal(t, sample{}, monitorSample(m, byName), "a zero divisor has no value")

	m.DivideBy = "missing"
	assert.Equal(t, sample{}, monitorSample(m, byName), "a missing divisor has no value")

	m.DivideBy = ""
	assert.Equal(t, sample{value: 100, ok: true}, monitorSample(m, byName))
}
