// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metrics

import (
	// stdlib
	"math"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextMetricsGaugeSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := "context_key"
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey, &mSample, 1, 10)
	series := metrics.Flush(12345)

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
	contextKey := "context_key"
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey, &mSample, 1, 10)
	series := metrics.Flush(12345)

	assert.Equal(t, 1, len(series))

	series = metrics.Flush(12355)
	// No series flushed since there's no new sample since last flush
	assert.Equal(t, 0, len(series))
}

// No series should be flushed when the samples have values of +Inf/-Inf
func TestContextMetricsGaugeSamplingInfinity(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey1 := "context_key1"
	contextKey2 := "context_key2"
	mSample1 := MetricSample{
		Value: math.Inf(1),
		Mtype: GaugeType,
	}
	mSample2 := MetricSample{
		Value: math.Inf(-1),
		Mtype: GaugeType,
	}

	metrics.AddSample(contextKey1, &mSample1, 1, 10)
	metrics.AddSample(contextKey2, &mSample2, 1, 10)
	series := metrics.Flush(12345)

	assert.Equal(t, 0, len(series))
}

// No series should be flushed when the rate has been sampled only once overall
// Important for check metrics aggregation
func TestContextMetricsRateSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := "context_key"

	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 1}, 12340, 10)
	series := metrics.Flush(12345)

	// No series flushed since the rate was sampled once only
	assert.Equal(t, 0, len(series))

	metrics.AddSample(contextKey, &MetricSample{Mtype: RateType, Value: 2}, 12350, 10)
	series = metrics.Flush(12351)
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

func TestContextMetricsCountSampling(t *testing.T) {
	metrics := MakeContextMetrics()
	contextKey := "context_key"

	metrics.AddSample(contextKey, &MetricSample{Mtype: CountType, Value: 1}, 12340, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: CountType, Value: 5}, 12345, 10)
	series := metrics.Flush(12350)
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
	contextKey := "context_key"

	metrics.AddSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 1}, 12340, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 5}, 12345, 10)
	series := metrics.Flush(12350)
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
	contextKey := "context_key"

	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12340, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 2}, 12342, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12350, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 6}, 12350, 10)
	series := metrics.Flush(12351)

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
	contextKey := "context_key"

	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 1}, 12340, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 2}, 12341, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12342, 10)
	metrics.AddSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12343, 10)
	series := metrics.Flush(12351)

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
