package aggregator

import (
	// stdlib
	"sort"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"

	// datadog
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

// Helper(s)
func AssertSerieEqual(t *testing.T, expected, actual *Serie) {
	assert.Equal(t, expected.Name, actual.Name)
	if expected.Tags != actual.Tags {
		assert.NotNil(t, actual.Tags)
		assert.NotNil(t, expected.Tags)
		assert.Equal(t, *(expected.Tags), *(actual.Tags))
	}
	assert.Equal(t, expected.Host, actual.Host)
	assert.Equal(t, expected.DeviceName, actual.DeviceName)
	assert.Equal(t, expected.Mtype, actual.Mtype)
	assert.Equal(t, expected.Interval, actual.Interval)
	if expected.contextKey != "" {
		// Only test the contextKey if it's set in the expected Serie
		assert.Equal(t, expected.contextKey, actual.contextKey)
	}
	assert.Equal(t, expected.nameSuffix, actual.nameSuffix)
	AssertPointsEqual(t, expected.Points, actual.Points)
}

func AssertPointsEqual(t *testing.T, expected, actual [][]interface{}) {
	if assert.Equal(t, len(expected), len(actual)) {
		for _, point := range expected {
			assert.Contains(t, actual, point)
		}
	}
}

type OrderedSeries struct {
	series []*Serie
}

func (os OrderedSeries) Len() int {
	return len(os.series)
}

func (os OrderedSeries) Less(i, j int) bool {
	return os.series[i].contextKey < os.series[j].contextKey
}

func (os OrderedSeries) Swap(i, j int) {
	os.series[j], os.series[i] = os.series[i], os.series[j]
}

// MetricSample
func TestGenerateContextKey(t *testing.T) {
	mSample := dogstatsd.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      dogstatsd.Gauge,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}

	context := generateContextKey(&mSample)
	assert.Equal(t, "bar,foo,my.metric.name", context)
}

// Sampler

// IntervalSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := IntervalSampler{10, map[int64]*Metrics{}}

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460))

}

func TestMetricsGaugeSampling(t *testing.T) {
	metrics := newMetrics()
	contextKey := "context_key"
	mSample := dogstatsd.MetricSample{
		Value: 1,
		Mtype: dogstatsd.Gauge,
	}

	metrics.addSample(contextKey, mSample.Mtype, mSample.Value)
	series := metrics.flush(12345)

	expectedSerie := &Serie{
		contextKey: contextKey,
		Points:     [][]interface{}{{int64(12345), mSample.Value}},
		Mtype:      "gauge",
		nameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestBucketSampling(t *testing.T) {
	intervalSampler := IntervalSampler{10, map[int64]*Metrics{}}

	mSample := dogstatsd.MetricSample{
		Value: 1,
		Mtype: dogstatsd.Gauge,
	}
	contextKey := "context_key"

	intervalSampler.addSample(contextKey, mSample.Mtype, mSample.Value, 12345)
	intervalSampler.addSample(contextKey, mSample.Mtype, mSample.Value, 12355)
	intervalSampler.addSample(contextKey, mSample.Mtype, mSample.Value, 12365)

	series := intervalSampler.flush(12360)

	expectedSerie := &Serie{
		Points:     [][]interface{}{{int64(12340), mSample.Value}, {int64(12350), mSample.Value}},
		Mtype:      "gauge",
		Interval:   10,
		nameSuffix: "",
		contextKey: contextKey,
	}

	assert.Equal(t, 1, len(intervalSampler.metricsByTimestamp))
	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

//
//// Sampler
func TestIntervalSampling(t *testing.T) {
	sampler := NewSampler()

	mSample1 := dogstatsd.MetricSample{
		Name:       "my.metric.name1",
		Value:      1,
		Mtype:      dogstatsd.Gauge,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Interval:   10,
	}
	mSample2 := dogstatsd.MetricSample{
		Name:       "my.metric.name2",
		Value:      1,
		Mtype:      dogstatsd.Gauge,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Interval:   3,
	}

	sampler.addSample(&mSample1, 12346)
	sampler.addSample(&mSample2, 12346)

	orderedSeries := OrderedSeries{sampler.flush(12360)}
	sort.Sort(orderedSeries)

	series := orderedSeries.series

	expectedSerie1 := &Serie{
		Name:     "my.metric.name1",
		Points:   [][]interface{}{{int64(12340), float64(1)}},
		Tags:     &[]string{"bar", "foo"},
		Mtype:    "gauge",
		Interval: 10,
	}
	expectedSerie2 := &Serie{
		Name:     "my.metric.name2",
		Points:   [][]interface{}{{int64(12345), float64(1)}},
		Tags:     &[]string{"bar", "foo"},
		Mtype:    "gauge",
		Interval: 3,
	}

	assert.Equal(t, 2, len(sampler.intervalSamplerByInterval))
	if assert.Equal(t, 2, len(series)) {
		AssertSerieEqual(t, expectedSerie1, series[0])
		AssertSerieEqual(t, expectedSerie2, series[1])
	}
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
