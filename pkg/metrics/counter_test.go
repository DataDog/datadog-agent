// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCounterEmptyFlush(t *testing.T) {
	// Initialize counter
	counter := NewCounter(10)

	// Flush w/o samples: error
	_, err := counter.flush(50)
	assert.NotNil(t, err)
}

func TestCounterAddSample(t *testing.T) {
	// Initialize counter
	counter := NewCounter(10)

	assert.False(t, counter.sampled)
	assert.EqualValues(t, 10, counter.interval)
	assert.EqualValues(t, 0, counter.value)

	// Add sample with SampleRate
	sample := MetricSample{
		Value:      2,
		SampleRate: 0.5,
	}

	counter.addSample(&sample, 10)
	assert.EqualValues(t, 4, counter.value, "SampleRate should have modify the counter value")
	assert.True(t, counter.sampled)

	// Add more samples
	sampleValues := []float64{1, 2, 5, 0, 8, 3}
	for _, sampleValue := range sampleValues {
		sample := MetricSample{Value: sampleValue, SampleRate: 1}
		counter.addSample(&sample, 55)
	}

	series, err := counter.flush(60)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	// value is divided by interval (aka 10)
	assert.InEpsilon(t, (4+1+2+5+0+8+3)/10.0, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 60, series[0].Points[0].Ts)
	assert.Equal(t, APIRateType, series[0].MType)

	// counter should have been reset
	assert.False(t, counter.sampled)
	assert.EqualValues(t, 10, counter.interval)
	assert.EqualValues(t, 0, counter.value)

	// Add a few new samples and flush: the counter should've been reset after the previous flush
	sampleValues = []float64{5, 3}
	for _, sampleValue := range sampleValues {
		sample := MetricSample{Value: sampleValue, SampleRate: 0.5}
		counter.addSample(&sample, 65)
	}
	series, err = counter.flush(70)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, (5+3)*2/10.0, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 70, series[0].Points[0].Ts)

	// Flush w/o samples: error
	_, err = counter.flush(80)
	assert.NotNil(t, err)
}
