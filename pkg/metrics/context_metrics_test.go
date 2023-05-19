// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	// stdlib
	"math"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

func TestContextMetricsGaugeSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey, &mSample, 1, 10, nil)
	series, err := metrics.Flush(12345)

	assert.Len(t, err, 0)
	expectedSerie := &Serie{
		ContextKey: contextKey,
		Points:     []Point{{12345.0, mSample.Value}},
		MType:      APIGaugeType,
		NameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

// No series should be flushed when there's no new sample btw 2 flushes
// Important for check metrics aggregation
func TestContextMetricsGaugeSamplingNoSample(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey, &mSample, 1, 10, nil)
	series, err := metrics.Flush(12345)

	assert.Len(t, err, 0)
	assert.Equal(t, 1, len(series))

	series, err = metrics.Flush(12355)
	assert.Len(t, err, 0)
	// No series flushed since there's no new sample since last flush
	assert.Equal(t, 0, len(series))
}

// Samples with values of +Inf/-Inf/NaN should be ignored
func TestContextMetricsGaugeSamplingInvalidSamples(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey1 := ckey.ContextKey(0xaaffffffffffffff)
	contextKey2 := ckey.ContextKey(0xbbffffffffffffff)

	// +/-Inf
	mSample1 := MetricSample{
		Value: math.Inf(1),
		Mtype: GaugeType,
	}
	mSample2 := MetricSample{
		Value: math.Inf(-1),
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey1, &mSample1, 1, 10, nil)
	metrics.AddSample(contextKey2, &mSample2, 1, 10, nil)
	series, err := metrics.Flush(20)
	assert.Len(t, err, 0)
	assert.Equal(t, 0, len(series))

	// NaN
	mSample3 := MetricSample{
		Value: math.NaN(),
		Mtype: GaugeType,
	}
	metrics.AddSample(contextKey1, &mSample3, 1, 30, nil)
	series, err = metrics.Flush(40)
	assert.Len(t, err, 0)
	assert.Equal(t, 0, len(series))

	// Regular value, should flush a series
	mSample4 := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}
	metrics.AddSample(contextKey1, &mSample4, 1, 50, nil)
	series, err = metrics.Flush(60)
	assert.Len(t, err, 0)
	expectedSerie := &Serie{
		ContextKey: contextKey1,
		Points:     []Point{{60., 1.}},
		MType:      APIGaugeType,
		NameSuffix: "",
	}
	require.Equal(t, 1, len(series))
	AssertSerieEqual(t, expectedSerie, series[0])
}

// No series should be flushed when the rate has been sampled only once overall
// Important for check metrics aggregation
func TestContextMetricsSingleRateSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 1}, 12340, 10, nil)
	series, err := metrics.Flush(12345)

	assert.Len(t, err, 0)
	// No series flushed since the rate was sampled once only
	assert.Equal(t, 0, len(series))

	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 2}, 12350, 10, nil)
	series, err = metrics.Flush(12351)

	assert.Len(t, err, 0)
	expectedSerie := &Serie{
		ContextKey: contextKey,
		Points:     []Point{{12350.0, 1. / 10.}},
		MType:      APIGaugeType,
		NameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

// No series should be flushed when the rate is negative, and an error should be returned
// Important for check metrics aggregation
func TestContextMetricsNegativeRateSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 2}, 12340, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 1}, 12350, 10, nil)
	series, err := metrics.Flush(12351)

	assert.Len(t, series, 0)
	require.Len(t, err, 1)
	assert.Contains(t, err, contextKey)
}

func TestContextMetricsCountSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: CountType, Value: 1}, 12340, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: CountType, Value: 5}, 12345, 10, nil)
	series, err := metrics.Flush(12350)

	assert.Len(t, err, 0)
	expectedSerie := &Serie{
		ContextKey: contextKey,
		Points:     []Point{{12350.0, 6.}},
		MType:      APICountType,
		NameSuffix: "",
	}

	if assert.Len(t, series, 1) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextMetricsMonotonicCountSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 1}, 12340, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 5}, 12345, 10, nil)
	series, err := metrics.Flush(12350)

	assert.Len(t, err, 0)
	expectedSerie := &Serie{
		ContextKey: contextKey,
		Points:     []Point{{12350.0, 4.}},
		MType:      APICountType,
		NameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextMetricsHistogramSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12340, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 2}, 12342, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12350, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 6}, 12350, 10, nil)
	series, err := metrics.Flush(12351)

	assert.Len(t, err, 0)
	expectedSeries := []*Serie{
		{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 6.}},
			MType:      APIGaugeType,
			NameSuffix: ".max",
		},
		{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 1.}},
			MType:      APIGaugeType,
			NameSuffix: ".median",
		},
		{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 2.5}},
			MType:      APIGaugeType,
			NameSuffix: ".avg",
		},
		{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 0.4}},
			MType:      APIRateType,
			NameSuffix: ".count",
		},
		{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 6.}},
			MType:      APIGaugeType,
			NameSuffix: ".95percentile",
		},
	}

	if assert.Len(t, series, len(expectedSeries)) {
		for i := range expectedSeries {
			AssertSerieEqual(t, expectedSeries[i], series[i])
		}
	}
}

func TestContextMetricsHistorateSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := ckey.ContextKey(0xffffffffffffffff)

	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 1}, 12340, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 2}, 12341, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12342, 10, nil)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12343, 10, nil)
	series, err := metrics.Flush(12351)

	assert.Len(t, err, 0)
	require.Len(t, series, 5)
	AssertSerieEqual(t,
		&Serie{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 2.}},
			MType:      APIGaugeType,
			NameSuffix: ".max",
		},
		series[0])

	AssertSerieEqual(t,
		&Serie{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 1.}},
			MType:      APIGaugeType,
			NameSuffix: ".median",
		},
		series[1])

	AssertSerieEqual(t,
		&Serie{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 1.0}},
			MType:      APIGaugeType,
			NameSuffix: ".avg",
		},
		series[2])

	AssertSerieEqual(t,
		&Serie{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 0.3}},
			MType:      APIRateType,
			NameSuffix: ".count",
		},
		series[3])

	AssertSerieEqual(t,
		&Serie{
			ContextKey: contextKey,
			Points:     []Point{{12351.0, 2.}},
			MType:      APIGaugeType,
			NameSuffix: ".95percentile",
		},
		series[4])
}
