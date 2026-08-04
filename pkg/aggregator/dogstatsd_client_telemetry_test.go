// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type recordingSimpleCounter struct {
	value float64
}

func (c *recordingSimpleCounter) Inc() {
	c.value++
}

func (c *recordingSimpleCounter) Add(value float64) {
	c.value += value
}

func (c *recordingSimpleCounter) Get() float64 {
	return c.value
}

func newTestDogStatsDClientTelemetry() (*dogStatsDClientTelemetry, [4]*recordingSimpleCounter) {
	counters := [4]*recordingSimpleCounter{{}, {}, {}, {}}
	return newDogStatsDClientTelemetry(counters[0], counters[1], counters[2], counters[3]), counters
}

func TestDogStatsDClientTelemetryRecordsSupportedRateBuckets(t *testing.T) {
	telemetry, counters := newTestDogStatsDClientTelemetry()

	for _, test := range []struct {
		name         string
		counterIndex int
	}{
		{name: "datadog.dogstatsd.client.bytes_sent", counterIndex: 0},
		{name: "datadog.dogstatsd.client.bytes_dropped", counterIndex: 1},
		{name: "datadog.dogstatsd.client.bytes_dropped_queue", counterIndex: 2},
		{name: "datadog.dogstatsd.client.bytes_dropped_writer", counterIndex: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			telemetry.observe(&metrics.Serie{
				Name:     test.name,
				MType:    metrics.APIRateType,
				Interval: 10,
				Points: []metrics.Point{
					{Ts: 100, Value: 0.7},
					{Ts: 110, Value: 0.3},
				},
			})

			assert.Equal(t, 10.0, counters[test.counterIndex].Get())
		})
	}
}

func TestDogStatsDClientTelemetryRecordsFractionalRateBuckets(t *testing.T) {
	telemetry, counters := newTestDogStatsDClientTelemetry()

	telemetry.observe(&metrics.Serie{
		Name:     dogStatsDClientBytesSentMetric,
		MType:    metrics.APIRateType,
		Interval: 10,
		Points:   []metrics.Point{{Value: 0.75}},
	})

	assert.Equal(t, 7.5, counters[0].Get())
}

func TestDogStatsDClientTelemetryIgnoresUnsupportedSeries(t *testing.T) {
	telemetry, counters := newTestDogStatsDClientTelemetry()

	for _, serie := range []*metrics.Serie{
		{
			Name:     "datadog.dogstatsd.client.metrics",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIGaugeType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points: []metrics.Point{
				{Value: -0.7},
				{Value: math.NaN()},
				{Value: math.Inf(1)},
				{Value: math.Ldexp(1, 64) / 10},
				{Value: 1e20},
			},
		},
	} {
		telemetry.observe(serie)
	}

	for _, counter := range counters {
		assert.Zero(t, counter.Get())
	}
}
