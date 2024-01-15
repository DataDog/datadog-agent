// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountSampling(t *testing.T) {
	// Initialize count
	count := Count{}

	// Flush w/o samples: error
	_, err := count.flush(50)
	assert.NotNil(t, err)

	// Add samples
	sampleValues := []float64{1, 2, 5, 0, 8, 3}
	for _, sampleValue := range sampleValues {
		sample := MetricSample{Value: sampleValue}
		count.addSample(&sample, 55)
	}
	series, err := count.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 1+2+5+0+8+3, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 60, series[0].Points[0].Ts)
	}

	// Add a few new samples and flush: the count should've been reset after the previous flush
	sampleValues = []float64{5, 3}
	for _, sampleValue := range sampleValues {
		sample := MetricSample{Value: sampleValue}
		count.addSample(&sample, 65)
	}
	series, err = count.flush(70)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 5+3, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 70, series[0].Points[0].Ts)
	}

	// Flush w/o samples: error
	_, err = count.flush(80)
	assert.NotNil(t, err)
}
