// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	// stdlib
	"sort"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// project
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type OrderedSeries struct {
	series []*metrics.Serie
}

func (os OrderedSeries) Len() int {
	return len(os.series)
}

func (os OrderedSeries) Less(i, j int) bool {
	return ckey.Compare(os.series[i].ContextKey, os.series[j].ContextKey) == -1
}

func (os OrderedSeries) Swap(i, j int) {
	os.series[j], os.series[i] = os.series[i], os.series[j]
}

// TimeSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := NewTimeSampler(10, "")

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456.5))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460.5))

}

func TestBucketSampling(t *testing.T) {
	sampler := NewTimeSampler(10, "")

	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample, 12345.0)
	sampler.addSample(&mSample, 12355.0)
	sampler.addSample(&mSample, 12365.0)

	series := sampler.flush(12360.0)

	expectedSerie := &metrics.Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     []metrics.Point{{Ts: 12340.0, Value: mSample.Value}, {Ts: 12350.0, Value: mSample.Value}},
		MType:      metrics.APIGaugeType,
		Interval:   10,
		NameSuffix: "",
	}

	assert.Equal(t, 1, len(sampler.metricsByTimestamp))
	if assert.Equal(t, 1, len(series)) {
		metrics.AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextSampling(t *testing.T) {
	sampler := NewTimeSampler(10, "default-hostname")

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name1",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name2",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name3",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	sampler.addSample(&mSample1, 12346.0)
	sampler.addSample(&mSample2, 12346.0)
	sampler.addSample(&mSample3, 12346.0)

	orderedSeries := OrderedSeries{sampler.flush(12360.0)}
	sort.Sort(orderedSeries)

	series := orderedSeries.series

	expectedSerie1 := &metrics.Serie{
		Name:     "my.metric.name1",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie2 := &metrics.Serie{
		Name:     "my.metric.name3",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "metric-hostname",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie3 := &metrics.Serie{
		Name:     "my.metric.name2",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}

	require.Equal(t, 3, len(series))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])
	metrics.AssertSerieEqual(t, expectedSerie3, series[2])
}

func TestCounterExpirySeconds(t *testing.T) {
	sampler := NewTimeSampler(10, "default-hostname")

	sampleCounter1 := &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	contextCounter1 := generateContextKey(sampleCounter1)

	sampleCounter2 := &metrics.MetricSample{
		Name:       "my.counter2",
		Value:      2,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	contextCounter2 := generateContextKey(sampleCounter2)

	sampleGauge3 := &metrics.MetricSample{
		Name:       "my.gauge",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampler.addSample(sampleCounter1, 1004.0)
	sampler.addSample(sampleCounter2, 1002.0)
	sampler.addSample(sampleGauge3, 1003.0)
	// counterLastSampledByContext should be populated when a sample is added
	assert.Equal(t, 2, len(sampler.counterLastSampledByContext))

	orderedSeries := OrderedSeries{sampler.flush(1010.0)}

	sort.Sort(orderedSeries)

	series := orderedSeries.series

	expectedSerie1 := &metrics.Serie{
		Name:     "my.counter1",
		Points:   []metrics.Point{{Ts: 1000.0, Value: .1}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	expectedSerie2 := &metrics.Serie{
		Name:     "my.counter2",
		Points:   []metrics.Point{{Ts: 1000.0, Value: .2}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	require.Equal(t, 3, len(series))
	require.Equal(t, 2, len(sampler.counterLastSampledByContext))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])
	assert.Equal(t, 1004.0, sampler.counterLastSampledByContext[contextCounter1])
	assert.Equal(t, 1002.0, sampler.counterLastSampledByContext[contextCounter2])

	sampleCounter1 = &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampler.addSample(sampleCounter2, 1034.0)
	sampler.addSample(sampleCounter1, 1010.0)
	sampler.addSample(sampleCounter2, 1020.0)

	orderedSeries = OrderedSeries{sampler.flush(1040.0)}
	sort.Sort(orderedSeries)

	series = orderedSeries.series

	expectedSerie1 = &metrics.Serie{
		Name:     "my.counter1",
		Points:   []metrics.Point{{Ts: 1010.0, Value: .1}, {Ts: 1020.0, Value: 0.0}, {Ts: 1030.0, Value: 0.0}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	expectedSerie2 = &metrics.Serie{
		Name:     "my.counter2",
		Points:   []metrics.Point{{Ts: 1010, Value: 0}, {Ts: 1020.0, Value: .2}, {Ts: 1030.0, Value: .2}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	require.Equal(t, 2, len(series))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])

	// We shouldn't get any empty counter since the last flush was during the same interval
	series = sampler.flush(1045.0)
	assert.Equal(t, 0, len(series))

	// Now we should get the empty counters
	series = sampler.flush(1050.0)
	assert.Equal(t, 2, len(series))

	series = sampler.flush(1329.0)
	// Counter1 should have stopped reporting but the context is not expired yet
	// Counter2 should still report
	assert.Equal(t, 1, len(series))
	assert.Equal(t, 1, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 2, len(sampler.contextResolver.contextsByKey))

	series = sampler.flush(1800.0)
	// Everything stopped reporting and is expired
	assert.Equal(t, 0, len(series))
	assert.Equal(t, 0, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 0, len(sampler.contextResolver.contextsByKey))
}

//func TestOne(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestFormatter(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestCounterNormalization(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestHistogramNormalization(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestCounter(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestSampledCounter(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestGauge(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestSets(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestStringSets(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestRate(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestRateErrors(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestGaugeSampleRate(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestHistogram(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestSampledHistogram(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestBatchSubmission(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestMonokeyBatchingNoTags(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestMonokeyBatchingWithTags(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestMonokeyBatchingWithTagsWithSampling(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestBadPacketsThrowErrors(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestMetricsExpiry(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestDiagnosticStats(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestHistogramCounter(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestEventTags(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestServiceCheckBasic(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestServiceCheckTags(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
//
//func TestRecentPointThreshold(t *testing.T) {
//	assert.Equal(t, 1, 1)
//}
