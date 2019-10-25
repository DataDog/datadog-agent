// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func TestMonotonicCountSampling(t *testing.T) {
	// Initialize monotonic count
	monotonicCount := MonotonicCount{}

	// Flush w/o samples: error
	_, err := monotonicCount.flush(40)
	assert.NotNil(t, err)

	// Flush with one sample only and no prior samples: error
	monotonicCount.addSample(MetricSampleValue{Value: 1}, 45)
	_, err = monotonicCount.flush(40)
	assert.NotNil(t, err)

	// Add samples
	monotonicCount.addSample(MetricSampleValue{Value: 2}, 50)
	monotonicCount.addSample(MetricSampleValue{Value: 3}, 55)
	monotonicCount.addSample(MetricSampleValue{Value: 6}, 55)
	monotonicCount.addSample(MetricSampleValue{Value: 7}, 58)
	series, err := monotonicCount.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 6, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 60, series[0].Points[0].Ts)
	}

	// Flush w/o samples: error
	_, err = monotonicCount.flush(70)
	assert.NotNil(t, err)

	// Add a single sample
	monotonicCount.addSample(MetricSampleValue{Value: 11}, 75)
	series, err = monotonicCount.flush(80)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 4, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 80, series[0].Points[0].Ts)
	}

	// Add sequence of non-monotonic samples
	monotonicCount.addSample(MetricSampleValue{Value: 12}, 85)
	monotonicCount.addSample(MetricSampleValue{Value: 10}, 85)
	monotonicCount.addSample(MetricSampleValue{Value: 20}, 85)
	monotonicCount.addSample(MetricSampleValue{Value: 13}, 85)
	monotonicCount.addSample(MetricSampleValue{Value: 17}, 85)
	series, err = monotonicCount.flush(90)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		// should skip when counter is reset, i.e. between 12 and 10, and btw 20 and 13
		// 15 = (12 - 11) + (20 - 10) + (17 - 13)
		assert.InEpsilon(t, 15, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 90, series[0].Points[0].Ts)
	}
}
