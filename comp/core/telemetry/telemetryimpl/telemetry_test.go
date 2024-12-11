// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCounterInitializer(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, MockModule())

	counter := telemetry.NewCounter("subsystem", "test", []string{"check_name", "state"}, "help docs")

	// Set some values and ensure that we have those counters
	counter.InitializeToZero("mycheck", "mystate")

	startMetrics, err := telemetry.GetRegistry().Gather()
	if !assert.NoError(t, err) {
		return
	}

	if !assert.Equal(t, len(startMetrics), 1) {
		return
	}

	metrics, err := telemetry.GetCountMetric("subsystem", "test")
	assert.NoError(t, err)
	require.Len(t, metrics, 1)

	metricLabels := metrics[0].Tags()
	assert.Equal(t, metricLabels["check_name"], "mycheck")
	assert.Equal(t, metricLabels["state"], "mystate")

	assert.Equal(t, metrics[0].Value(), 0.0)
}

func TestGetCounterValue(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, MockModule())

	counter := telemetry.NewCounter("subsystem", "test", []string{"state"}, "help docs")
	assert.Equal(t, counter.WithValues("ok").Get(), 0.0)
	assert.Equal(t, counter.WithValues("error").Get(), 0.0)

	counter.Inc("ok")
	assert.Equal(t, counter.WithValues("ok").Get(), 1.0)
	assert.Equal(t, counter.WithValues("error").Get(), 0.0)

	counter.Add(123, "error")
	assert.Equal(t, counter.WithValues("error").Get(), 123.0)
}

func TestGetGaugeValue(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, MockModule())

	gauge := telemetry.NewGauge("subsystem", "test", []string{"state"}, "help docs")
	assert.Equal(t, gauge.WithValues("ok").Get(), 0.0)
	assert.Equal(t, gauge.WithValues("error").Get(), 0.0)

	gauge.Inc("ok")
	assert.Equal(t, gauge.WithValues("ok").Get(), 1.0)
	assert.Equal(t, gauge.WithValues("error").Get(), 0.0)

	gauge.Add(123, "error")
	assert.Equal(t, gauge.WithValues("error").Get(), 123.0)
}

func TestGetSimpleHistogramValue(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, MockModule())

	hist := telemetry.NewSimpleHistogram("subsystem", "test", "help docs", []float64{1, 2, 3, 4})

	assert.Equal(t, 4, len(hist.Get().Buckets))

	hist.Observe(1)
	hist.Observe(1)

	hist.Observe(3)
	hist.Observe(3)
	hist.Observe(3)

	assert.Equal(t, uint64(2), hist.Get().Buckets[0].Count)
	assert.Equal(t, uint64(2), hist.Get().Buckets[1].Count)
	assert.Equal(t, uint64(5), hist.Get().Buckets[2].Count)
	assert.Equal(t, uint64(5), hist.Get().Buckets[3].Count)

	assert.Equal(t, uint64(5), hist.Get().Count)
	assert.Equal(t, float64(11), hist.Get().Sum)
}

func TestGetHistogramValue(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, MockModule())

	hist := telemetry.NewHistogram("subsystem", "test", []string{"state"}, "help docs", []float64{1, 2, 3, 4})

	assert.Equal(t, uint64(0), hist.WithValues("ok").Get().Buckets[0].Count)
	assert.Equal(t, uint64(0), hist.WithValues("ok").Get().Buckets[1].Count)
	hist.Observe(1, "ok")

	assert.Equal(t, uint64(1), hist.WithValues("ok").Get().Buckets[0].Count)
	assert.Equal(t, uint64(1), hist.WithValues("ok").Get().Buckets[1].Count)

	hist.Observe(2, "ok")
	assert.Equal(t, uint64(1), hist.WithValues("ok").Get().Buckets[0].Count)
	assert.Equal(t, uint64(2), hist.WithValues("ok").Get().Buckets[1].Count)

	assert.Equal(t, uint64(1), hist.WithTags(map[string]string{"state": "ok"}).Get().Buckets[0].Count)
	assert.Equal(t, uint64(2), hist.WithTags(map[string]string{"state": "ok"}).Get().Buckets[1].Count)
}

func TestGoMetrics(t *testing.T) {
	// Read the default global registry
	metrics, err := registry.Gather()
	assert.NoError(t, err)

	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}
	// Make sure we have one for each category at least.
	assert.Contains(t, metricNames, "go_goroutines")
	assert.Contains(t, metricNames, "go_memstats_alloc_bytes")
	assert.Contains(t, metricNames, "go_sched_goroutines_goroutines")
	assert.Contains(t, metricNames, "go_threads")
	assert.Contains(t, metricNames, "go_gc_duration_seconds")
}
