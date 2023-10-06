// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestPromCounterInitializer(t *testing.T) {
	promTelemetry := prometheus.NewRegistry()

	counter := promCounter{
		pc: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "subsystem",
				Name:      "test",
				Help:      "help docs",
			},
			[]string{"check_name", "state"},
		),
	}

	promTelemetry.MustRegister(counter.pc)

	// Sanity check that we don't have any metrics
	startMetrics, err := promTelemetry.Gather()
	assert.NoError(t, err)
	if err != nil {
		return
	}

	assert.Zero(t, len(startMetrics))

	// Set some values and ensure that we have those counters
	counter.InitializeToZero("mycheck", "mystate")

	endMetrics, err := promTelemetry.Gather()
	if !assert.NoError(t, err) {
		return
	}

	if !assert.Equal(t, len(endMetrics), 1) {
		return
	}

	metricFamily := endMetrics[0]
	if !assert.Equal(t, len(metricFamily.GetMetric()), 1) {
		return
	}

	assert.Equal(t, metricFamily.GetName(), "subsystem_test")

	metric := metricFamily.GetMetric()[0]
	assert.Equal(t, metric.GetLabel()[0].GetName(), "check_name")
	assert.Equal(t, metric.GetLabel()[0].GetValue(), "mycheck")

	assert.Equal(t, metric.GetLabel()[1].GetName(), "state")
	assert.Equal(t, metric.GetLabel()[1].GetValue(), "mystate")

	assert.Equal(t, metric.GetCounter().GetValue(), 0.0)
}

func TestPromCounterAdd(t *testing.T) {
	promTelemetry := prometheus.NewRegistry()

	counter := promCounter{
		pc: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "subsystem",
				Name:      "test",
				Help:      "help docs",
			},
			[]string{},
		),
	}

	promTelemetry.MustRegister(counter.pc)

	get := func() float64 {
		metrics, err := promTelemetry.Gather()
		assert.NoError(t, err)

		metricFamily := metrics[0]
		metric := metricFamily.GetMetric()[0]
		return metric.GetCounter().GetValue()
	}
	counter.Add(10.0)
	assert.Equal(t, get(), 10.0)

	counter.Add(0.0)
	assert.Equal(t, get(), 10.0)

	counter.Add(-10.0)
	assert.Equal(t, get(), 10.0)

}
