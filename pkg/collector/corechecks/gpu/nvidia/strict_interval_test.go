// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const testStrictInterval = 15 * time.Second

func processAt(t *testing.T, p *StrictIntervalProcessor, start time.Time, offset time.Duration) []time.Duration {
	t.Helper()

	metric := &Metric{Name: "device.total", Value: 1, Type: metrics.GaugeType, StrictInterval: testStrictInterval}
	samples := p.ProcessSamples([]Sample{metric}, start.Add(offset), "gpu-1")

	offsets := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		offsets = append(offsets, sample.(*Metric).Timestamp.Sub(start))
	}
	return offsets
}

func TestStrictIntervalPassesThroughOtherSamples(t *testing.T) {
	metric := &Metric{Name: "temperature", Value: 42, Type: metrics.GaugeType}
	histogram := &HistogramSample{Name: "latency", Value: 1}

	samples := NewStrictIntervalProcessor(0).ProcessSamples([]Sample{metric, histogram}, time.Unix(1000, 0), "gpu-1")

	require.Equal(t, []Sample{metric, histogram}, samples)
	require.True(t, metric.Timestamp.IsZero(), "samples without a strict interval keep the execution time")
}

func TestStaticMetricsUseConfiguredInterval(t *testing.T) {
	processor := NewStrictIntervalProcessor(testStrictInterval)
	now := time.Unix(1000, 0)
	samples := []Sample{
		&Metric{Name: "core.limit"},
		&Metric{Name: "device.total"},
		&Metric{Name: "memory.bar1.total"},
		&Metric{Name: "memory.limit"},
		&Metric{Name: "nvlink.count.total"},
		&Metric{Name: "temperature"},
	}

	require.Len(t, processor.ProcessSamples(samples, now, "gpu-1"), len(samples))
	require.Len(t, processor.ProcessSamples(samples, now.Add(5*time.Second), "gpu-1"), 2)
}

func TestStrictIntervalTimestamps(t *testing.T) {
	tests := []struct {
		offset   time.Duration
		expected []time.Duration
	}{
		{offset: 5 * time.Second, expected: []time.Duration{}},
		{offset: 13 * time.Second, expected: []time.Duration{}},
		{offset: 13500 * time.Millisecond, expected: []time.Duration{testStrictInterval}},
		{offset: testStrictInterval, expected: []time.Duration{testStrictInterval}},
		{offset: 16500 * time.Millisecond, expected: []time.Duration{testStrictInterval}},
		{offset: 20 * time.Second, expected: []time.Duration{testStrictInterval}},
		{offset: 50 * time.Second, expected: []time.Duration{15 * time.Second, 30 * time.Second, 45 * time.Second}},
	}

	for _, test := range tests {
		t.Run(test.offset.String(), func(t *testing.T) {
			processor := NewStrictIntervalProcessor(0)
			require.Equal(t, []time.Duration{0}, processAt(t, processor, time.Unix(1000, 0), 0))
			require.Equal(t, test.expected, processAt(t, processor, time.Unix(1000, 0), test.offset))
		})
	}
}

func TestStrictIntervalCapsFilledPoints(t *testing.T) {
	processor := NewStrictIntervalProcessor(0)
	start := time.Unix(1000, 0)
	processAt(t, processor, start, 0)

	require.Len(t, processAt(t, processor, start, time.Hour), maxStrictIntervalPoints)

	require.Equal(t, []time.Duration{time.Hour + testStrictInterval}, processAt(t, processor, start, time.Hour+testStrictInterval))
}

func TestStrictIntervalTracksSeriesIndependently(t *testing.T) {
	processor := NewStrictIntervalProcessor(0)
	now := time.Unix(1000, 0)
	next := now.Add(testStrictInterval)

	batch := func() []Sample {
		return []Sample{
			&Metric{Name: "device.total", Value: 1, Type: metrics.GaugeType, StrictInterval: testStrictInterval},
			&Metric{Name: "device.unhealthy", Value: 0, Type: metrics.GaugeType, StrictInterval: testStrictInterval},
		}
	}

	require.Len(t, processor.ProcessSamples(batch(), now, "gpu-1"), 2)
	require.Len(t, processor.ProcessSamples(batch(), now, "gpu-2"), 2)
	require.Len(t, processor.ProcessSamples(batch(), next, "gpu-1"), 2)
	require.Len(t, processor.ProcessSamples(batch(), next, "gpu-2"), 2)
}

func TestStrictIntervalKeepsExactSpacingUnderJitter(t *testing.T) {
	processor := NewStrictIntervalProcessor(0)
	start := time.Unix(1000, 0)

	var emitted []time.Duration
	for run := range 200 {
		offset := time.Duration(run) * (testStrictInterval + 400*time.Millisecond)
		emitted = append(emitted, processAt(t, processor, start, offset)...)
	}

	require.Greater(t, len(emitted), 190)
	for i := 1; i < len(emitted); i++ {
		require.Equal(t, testStrictInterval, emitted[i]-emitted[i-1], "points %d and %d are not one interval apart", i-1, i)
	}
}
