package aggregator

import (
	// stdlib
	"sort"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func TestCheckGaugeSampling(t *testing.T) {
	checkSampler := newCheckSampler("")

	mSample1 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345,
	}
	mSample2 := MetricSample{
		Name:       "my.metric.name",
		Value:      2,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347,
	}
	mSample3 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349)
	orderedSeries := OrderedSeries{checkSampler.flush()}
	sort.Sort(orderedSeries)
	series := orderedSeries.series

	expectedSerie1 := &Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     [][]interface{}{{int64(12349), mSample2.Value}},
		Mtype:      "gauge",
		contextKey: generateContextKey(&mSample2),
		nameSuffix: "",
	}

	expectedSerie2 := &Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar", "baz"},
		Points:     [][]interface{}{{int64(12349), mSample3.Value}},
		Mtype:      "gauge",
		contextKey: generateContextKey(&mSample3),
		nameSuffix: "",
	}

	orderedExpectedSeries := OrderedSeries{[]*Serie{expectedSerie1, expectedSerie2}}
	sort.Sort(orderedExpectedSeries)
	expectedSeries := orderedExpectedSeries.series

	if assert.Equal(t, 2, len(series)) {
		AssertSerieEqual(t, expectedSeries[0], series[0])
		AssertSerieEqual(t, expectedSeries[1], series[1])
	}
}

func TestCheckRateSampling(t *testing.T) {
	checkSampler := newCheckSampler("")

	mSample1 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      RateType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345,
	}
	mSample2 := MetricSample{
		Name:       "my.metric.name",
		Value:      2,
		Mtype:      RateType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347,
	}
	mSample3 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      RateType,
		Tags:       &[]string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349)
	series := checkSampler.flush()

	expectedSerie := &Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     [][]interface{}{{int64(12347), 0.5}},
		Mtype:      "gauge",
		nameSuffix: "",
	}

	if assert.Equal(t, 1, len(series)) {
		AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestCheckSamplerHostname(t *testing.T) {
	checkSampler := newCheckSampler("my.test.hostname")

	mSample1 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.commit(12346)
	series := checkSampler.flush()

	if assert.Len(t, series, 1) {
		assert.Equal(t, "my.test.hostname", series[0].Host)
	}
}
