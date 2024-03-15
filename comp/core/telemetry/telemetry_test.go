// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"context"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCounterInitializer(t *testing.T) {

	telemetry := fxutil.Test[Mock](t, MockModule())

	counter := telemetry.NewCounter("subsystem", "test", []string{"check_name", "state"}, "help docs")

	// Sanity check that we don't have any metrics
	startMetrics, err := telemetry.GetRegistry().Gather()
	assert.NoError(t, err)
	if err != nil {
		return
	}

	// 1 because OTEL adds a target_info gauge by default
	assert.Equal(t, 1, len(startMetrics))

	// Set some values and ensure that we have those counters
	counter.InitializeToZero("mycheck", "mystate")

	endMetrics, err := telemetry.GetRegistry().Gather()
	if !assert.NoError(t, err) {
		return
	}

	// 2 because OTEL adds a target_info gauge by default
	if !assert.Equal(t, len(endMetrics), 2) {
		return
	}

	var metricFamily *dto.MetricFamily

	for _, m := range endMetrics {
		if m.GetName() == "subsystem__test" {
			metricFamily = m
		}
	}

	if !assert.Equal(t, len(metricFamily.GetMetric()), 1) {
		return
	}

	metric := metricFamily.GetMetric()[0]
	assert.Equal(t, metric.GetLabel()[0].GetName(), "check_name")
	assert.Equal(t, metric.GetLabel()[0].GetValue(), "mycheck")

	assert.Equal(t, metric.GetLabel()[1].GetName(), "state")
	assert.Equal(t, metric.GetLabel()[1].GetValue(), "mystate")

	assert.Equal(t, metric.GetCounter().GetValue(), 0.0)
}

func TestGetCounterValue(t *testing.T) {
	telemetry := fxutil.Test[Mock](t, MockModule())

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
	telemetry := fxutil.Test[Mock](t, MockModule())

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
	telemetry := fxutil.Test[Mock](t, MockModule())

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
	telemetry := fxutil.Test[Mock](t, MockModule())

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

func TestMeterProvider(t *testing.T) {

	telemetry := fxutil.Test[Mock](t, MockModule())

	counter, _ := telemetry.Meter("foo").Int64Counter("bar")
	counter.Add(context.TODO(), 123)

	_ = telemetry.GetMeterProvider().ForceFlush(context.TODO())

	metrics, err := telemetry.GetRegistry().Gather()
	assert.NoError(t, err)

	var metricFamily *dto.MetricFamily

	for _, m := range metrics {
		if m.GetName() == "bar_total" {
			metricFamily = m
		}
	}

	metric := metricFamily.GetMetric()[0]
	assert.Equal(t, metric.GetCounter().GetValue(), 123.0)
	assert.Equal(t, *metric.GetLabel()[0].Value, "foo")
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
