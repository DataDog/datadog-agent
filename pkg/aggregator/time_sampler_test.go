package aggregator

import (
	// stdlib
	"fmt"
	"sort"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper(s)
func AssertSerieEqual(t *testing.T, expected, actual *Serie) {
	assert.Equal(t, expected.Name, actual.Name)
	if expected.Tags != nil {
		assert.NotNil(t, actual.Tags)
		AssertTagsEqual(t, expected.Tags, actual.Tags)
	}
	assert.Equal(t, expected.Host, actual.Host)
	assert.Equal(t, expected.MType, actual.MType)
	assert.Equal(t, expected.Interval, actual.Interval)
	assert.Equal(t, expected.SourceTypeName, actual.SourceTypeName)
	if expected.contextKey != "" {
		// Only test the contextKey if it's set in the expected Serie
		assert.Equal(t, expected.contextKey, actual.contextKey)
	}
	assert.Equal(t, expected.nameSuffix, actual.nameSuffix)
	AssertPointsEqual(t, expected.Points, actual.Points)
}

func AssertTagsEqual(t *testing.T, expected, actual []string) {
	if assert.Equal(t, len(expected), len(actual), fmt.Sprintf("Unexpected number of tags: expected %s, actual: %s", expected, actual)) {
		for _, tag := range expected {
			assert.Contains(t, actual, tag)
		}
	}
}

func AssertPointsEqual(t *testing.T, expected, actual []Point) {
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

// TimeSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := NewTimeSampler(10, "")

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460))

}

func TestBucketSampling(t *testing.T) {
	sampler := NewTimeSampler(10, "")

	mSample := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample, 12345)
	sampler.addSample(&mSample, 12355)
	sampler.addSample(&mSample, 12365)

	series := sampler.flush(12360)

	expectedSerie := &Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     []Point{{int64(12340), mSample.Value}, {int64(12350), mSample.Value}},
		MType:      APIGaugeType,
		Interval:   10,
		nameSuffix: "",
	}

	assert.Equal(t, 1, len(sampler.metricsByTimestamp))
	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextSampling(t *testing.T) {
	sampler := NewTimeSampler(10, "default-hostname")

	mSample1 := MetricSample{
		Name:       "my.metric.name1",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := MetricSample{
		Name:       "my.metric.name2",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample3 := MetricSample{
		Name:       "my.metric.name3",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	sampler.addSample(&mSample1, 12346)
	sampler.addSample(&mSample2, 12346)
	sampler.addSample(&mSample3, 12346)

	orderedSeries := OrderedSeries{sampler.flush(12360)}
	sort.Sort(orderedSeries)

	series := orderedSeries.series

	expectedSerie1 := &Serie{
		Name:     "my.metric.name1",
		Points:   []Point{{int64(12340), float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    APIGaugeType,
		Interval: 10,
	}
	expectedSerie2 := &Serie{
		Name:     "my.metric.name2",
		Points:   []Point{{int64(12340), float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "default-hostname",
		MType:    APIGaugeType,
		Interval: 10,
	}
	expectedSerie3 := &Serie{
		Name:     "my.metric.name3",
		Points:   []Point{{int64(12340), float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "metric-hostname",
		MType:    APIGaugeType,
		Interval: 10,
	}

	require.Equal(t, 3, len(series))
	AssertSerieEqual(t, expectedSerie1, series[0])
	AssertSerieEqual(t, expectedSerie2, series[1])
	AssertSerieEqual(t, expectedSerie3, series[2])
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
