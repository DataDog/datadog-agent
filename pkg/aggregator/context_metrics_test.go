package aggregator

import (
	// stdlib
	"math"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextMetricsGaugeSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.addSample(contextKey, &mSample, 1, 10)
	series := metrics.flush(12345)

	expectedSerie := &Serie{
		contextKey: contextKey,
		Points:     []Point{{int64(12345), mSample.Value}},
		MType:      APIGaugeType,
		nameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

// No series should be flushed when there's no new sample btw 2 flushes
// Important for check metrics aggregation
func TestContextMetricsGaugeSamplingNoSample(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"
	mSample := MetricSample{
		Value: 1,
		Mtype: GaugeType,
	}

	metrics.addSample(contextKey, &mSample, 1, 10)
	series := metrics.flush(12345)

	assert.Equal(t, 1, len(series))

	series = metrics.flush(12355)
	// No series flushed since there's no new sample since last flush
	assert.Equal(t, 0, len(series))
}

// No series should be flushed when the samples have values of +Inf/-Inf
func TestContextMetricsGaugeSamplingInfinity(t *testing.T) {
	metrics := makeContextMetrics()
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

	metrics.addSample(contextKey1, &mSample1, 1, 10)
	metrics.addSample(contextKey2, &mSample2, 1, 10)
	series := metrics.flush(12345)

	assert.Equal(t, 0, len(series))
}

// No series should be flushed when the rate has been sampled only once overall
// Important for check metrics aggregation
func TestContextMetricsRateSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"

	metrics.addSample(contextKey, &MetricSample{Mtype: RateType, Value: 1}, 12340, 10)
	series := metrics.flush(12345)

	// No series flushed since the rate was sampled once only
	assert.Equal(t, 0, len(series))

	metrics.addSample(contextKey, &MetricSample{Mtype: RateType, Value: 2}, 12350, 10)
	series = metrics.flush(12351)
	expectedSerie := &Serie{
		contextKey: contextKey,
		Points:     []Point{{int64(12350), 1. / 10.}},
		MType:      APIGaugeType,
		nameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextMetricsCountSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"

	metrics.addSample(contextKey, &MetricSample{Mtype: CountType, Value: 1}, 12340, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: CountType, Value: 5}, 12345, 10)
	series := metrics.flush(12350)
	expectedSerie := &Serie{
		contextKey: contextKey,
		Points:     []Point{{int64(12350), 6.}},
		MType:      APICountType,
		nameSuffix: "",
	}

	if assert.Len(t, series, 1) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextMetricsMonotonicCountSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"

	metrics.addSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 1}, 12340, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: MonotonicCountType, Value: 5}, 12345, 10)
	series := metrics.flush(12350)
	expectedSerie := &Serie{
		contextKey: contextKey,
		Points:     []Point{{int64(12350), 4.}},
		MType:      APICountType,
		nameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextMetricsHistogramSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"

	metrics.addSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12340, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 2}, 12342, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 1}, 12350, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistogramType, Value: 6}, 12350, 10)
	series := metrics.flush(12351)

	expectedSeries := []*Serie{
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 6.}},
			MType:      APIGaugeType,
			nameSuffix: ".max",
		},
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 1.}},
			MType:      APIGaugeType,
			nameSuffix: ".median",
		},
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 2.5}},
			MType:      APIGaugeType,
			nameSuffix: ".avg",
		},
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 4.}},
			MType:      APIRateType,
			nameSuffix: ".count",
		},
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 6.}},
			MType:      APIGaugeType,
			nameSuffix: ".95percentile",
		},
	}

	if assert.Len(t, series, len(expectedSeries)) {
		for i := range expectedSeries {
			AssertSerieEqual(t, expectedSeries[i], series[i])
		}
	}
}

func TestContextMetricsHistorateSampling(t *testing.T) {
	metrics := makeContextMetrics()
	contextKey := "context_key"

	metrics.addSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 1}, 12340, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 2}, 12341, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12342, 10)
	metrics.addSample(contextKey, &MetricSample{Mtype: HistorateType, Value: 4}, 12343, 10)
	series := metrics.flush(12351)

	require.Len(t, series, 5)
	AssertSerieEqual(t,
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 2.}},
			MType:      APIGaugeType,
			nameSuffix: ".max",
		},
		series[0])

	AssertSerieEqual(t,
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 1.}},
			MType:      APIGaugeType,
			nameSuffix: ".median",
		},
		series[1])

	AssertSerieEqual(t,
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 1.0}},
			MType:      APIGaugeType,
			nameSuffix: ".avg",
		},
		series[2])

	AssertSerieEqual(t,
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 3.}},
			MType:      APIRateType,
			nameSuffix: ".count",
		},
		series[3])

	AssertSerieEqual(t,
		&Serie{
			contextKey: contextKey,
			Points:     []Point{{int64(12351), 2.}},
			MType:      APIGaugeType,
			nameSuffix: ".95percentile",
		},
		series[4])
}
