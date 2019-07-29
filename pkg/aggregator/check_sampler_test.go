// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	checkSampler := newCheckSampler()

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
	s, _ := checkSampler.flush()
	orderedSeries := OrderedSeries{s}
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
	checkSampler := newCheckSampler()

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
	series, _ := checkSampler.flush()

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

func TestHistogramIntervalSampling(t *testing.T) {
	checkSampler := newCheckSampler()

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      10,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.5,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
	series, _ := checkSampler.flush()

	// Check that the `.count` metric returns a raw count of the samples, with no interval normalization
	expectedCountSerie := &metrics.Serie{
		Name:           "my.metric.name.count",
		Tags:           []string{"foo", "bar"},
		Points:         []metrics.Point{{Ts: 12349.0, Value: 3.}},
		MType:          metrics.APIRateType,
		SourceTypeName: checksSourceTypeName,
		NameSuffix:     ".count",
	}

	require.Len(t, series, 5)

	foundCount := false
	for _, serie := range series {
		if serie.Name == expectedCountSerie.Name {
			metrics.AssertSerieEqual(t, expectedCountSerie, serie)
			foundCount = true
		}
	}

	assert.True(t, foundCount)
}
