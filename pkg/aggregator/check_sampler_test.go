package aggregator

import (
	// stdlib
	"sort"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestCheckGaugeSampling(t *testing.T) {
	checkSampler := newCheckSampler("")

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.0,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
	orderedSeries := OrderedSeries{checkSampler.flush()}
	sort.Sort(orderedSeries)
	series := orderedSeries.series

	expectedSerie1 := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           []string{"foo", "bar"},
		Points:         []metrics.Point{{Ts: 12349.0, Value: mSample2.Value}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		ContextKey:     generateContextKey(&mSample2),
		NameSuffix:     "",
	}

	expectedSerie2 := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           []string{"foo", "bar", "baz"},
		Points:         []metrics.Point{{Ts: 12349.0, Value: mSample3.Value}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		ContextKey:     generateContextKey(&mSample3),
		NameSuffix:     "",
	}

	orderedExpectedSeries := OrderedSeries{[]*metrics.Serie{expectedSerie1, expectedSerie2}}
	sort.Sort(orderedExpectedSeries)
	expectedSeries := orderedExpectedSeries.series

	if assert.Equal(t, 2, len(series)) {
		metrics.AssertSerieEqual(t, expectedSeries[0], series[0])
		metrics.AssertSerieEqual(t, expectedSeries[1], series[1])
	}
}

func TestCheckRateSampling(t *testing.T) {
	checkSampler := newCheckSampler("")

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      10,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.5,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
	series := checkSampler.flush()

	expectedSerie := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           []string{"foo", "bar"},
		Points:         []metrics.Point{{Ts: 12347.5, Value: 3.6}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		NameSuffix:     "",
	}

	if assert.Equal(t, 1, len(series)) {
		metrics.AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestCheckSamplerHostname(t *testing.T) {
	checkSampler := newCheckSampler("my.test.hostname")

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
		Timestamp:  12345,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.commit(12346.0)
	series := checkSampler.flush()

	require.Len(t, series, 2)
	actualHostnames := []string{series[0].Host, series[1].Host}
	assert.Contains(t, actualHostnames, "my.test.hostname")
	assert.Contains(t, actualHostnames, "metric-hostname")
}
