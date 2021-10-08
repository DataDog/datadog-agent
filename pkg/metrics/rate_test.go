// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateSampling(t *testing.T) {
	// Initialize rates
	mRate1 := Rate{}
	mRate2 := Rate{}

	// Add samples
	mRate1.addSample(&MetricSample{Value: 1}, 50)
	mRate1.addSample(&MetricSample{Value: 11}, 52.5)
	mRate2.addSample(&MetricSample{Value: 1}, 60)

	// First rate
	series, err := mRate1.flush(60)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 4., series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 52.5, series[0].Points[0].Ts)

	// Second rate (should return error)
	_, err = mRate2.flush(60)
	assert.NotNil(t, err)
}

func TestRateSamplingMultipleSamplesInSameFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(&MetricSample{Value: 1}, 50)
	mRate.addSample(&MetricSample{Value: 2}, 55)
	mRate.addSample(&MetricSample{Value: 4}, 61)

	// Should compute rate based on the last 2 samples
	series, err := mRate.flush(65)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 2./6., series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 61, series[0].Points[0].Ts)
}

func TestRateSamplingNoSampleForOneFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(&MetricSample{Value: 1}, 50)
	mRate.addSample(&MetricSample{Value: 2}, 55)

	// First flush: no error
	_, err := mRate.flush(60)
	assert.Nil(t, err)

	// Second flush w/o sample: error
	_, err = mRate.flush(60)
	assert.NotNil(t, err)

	// Third flush w/ sample
	mRate.addSample(&MetricSample{Value: 4}, 60)
	// Should compute rate based on the last 2 samples
	series, err := mRate.flush(60)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 2./5., series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 60, series[0].Points[0].Ts)
}

func TestRateSamplingSamplesAtSameTimestamp(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(&MetricSample{Value: 1}, 50)
	mRate.addSample(&MetricSample{Value: 2}, 50)

	series, err := mRate.flush(60)

	assert.NotNil(t, err)
	assert.Len(t, series, 0)
}

func TestRateSamplingNegativeRate(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples, with second value below first one
	mRate.addSample(&MetricSample{Value: 2}, 50)
	mRate.addSample(&MetricSample{Value: 1}, 55)

	// Should return an error
	series, err := mRate.flush(60)
	assert.NotNil(t, err)
	assert.Len(t, series, 0)

	// Add a sample again, this time with positive diff
	mRate.addSample(&MetricSample{Value: 3}, 62)
	// Should compute rate based on the last 2 samples
	series, err = mRate.flush(70)
	assert.Nil(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, (3.-1.)/(62.-55.), series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 62, series[0].Points[0].Ts)
}
