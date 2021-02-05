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
)

func TestMonotonicCountSampling(t *testing.T) {
	// Initialize monotonic count
	monotonicCount := MonotonicCount{}

	// Flush w/o samples: error
	_, err := monotonicCount.flush(40)
	assert.NotNil(t, err)

	// Flush with one sample only and no prior samples: error
	monotonicCount.addSample(&MetricSample{Value: 2}, 45)
	_, err = monotonicCount.flush(40)
	assert.NotNil(t, err)

	// Add another sample with lower value: flush 0.
	monotonicCount.addSample(&MetricSample{Value: 1}, 48)
	series, err := monotonicCount.flush(50)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.Equal(t, 0., series[0].Points[0].Value)
		assert.EqualValues(t, 50, series[0].Points[0].Ts)
	}

	// Add samples
	monotonicCount.addSample(&MetricSample{Value: 2}, 50)
	monotonicCount.addSample(&MetricSample{Value: 3}, 55)
	monotonicCount.addSample(&MetricSample{Value: 6}, 55)
	monotonicCount.addSample(&MetricSample{Value: 7}, 58)
	series, err = monotonicCount.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 6, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 60, series[0].Points[0].Ts)
	}

	// Flush w/o samples: error
	_, err = monotonicCount.flush(70)
	assert.NotNil(t, err)

	// Add a single sample
	monotonicCount.addSample(&MetricSample{Value: 11}, 75)
	series, err = monotonicCount.flush(80)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.InEpsilon(t, 4, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 80, series[0].Points[0].Ts)
	}

	// Add another sample with same value: flush 0.
	monotonicCount.addSample(&MetricSample{Value: 11}, 81)
	series, err = monotonicCount.flush(82)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.Equal(t, 0., series[0].Points[0].Value)
		assert.EqualValues(t, 82, series[0].Points[0].Ts)
	}

	// Add another sample with lower value: flush 0.
	monotonicCount.addSample(&MetricSample{Value: 9}, 83)
	series, err = monotonicCount.flush(84)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		assert.Equal(t, 0., series[0].Points[0].Value)
		assert.EqualValues(t, 84, series[0].Points[0].Ts)
	}

	// Add sequence of non-monotonic samples
	monotonicCount.addSample(&MetricSample{Value: 12}, 85)
	monotonicCount.addSample(&MetricSample{Value: 10}, 85)
	monotonicCount.addSample(&MetricSample{Value: 20}, 85)
	monotonicCount.addSample(&MetricSample{Value: 13}, 85)
	monotonicCount.addSample(&MetricSample{Value: 17}, 85)
	series, err = monotonicCount.flush(90)
	assert.Nil(t, err)
	if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
		// should skip when counter is reset, i.e. between 12 and 9, and btw 20 and 13
		// 17 = (12 - 9) + (20 - 10) + (17 - 13)
		assert.InEpsilon(t, 17, series[0].Points[0].Value, epsilon)
		assert.EqualValues(t, 90, series[0].Points[0].Ts)
	}
}

func TestMonotonicCount_FlushFirstValue(t *testing.T) {
	// Initialize monotonic counts
	monotonicCount1 := MonotonicCount{}
	monotonicCount2 := MonotonicCount{}

	// use a constant timestamp for all submissions and flushes. It's not relevant for these tests.
	timestamp := 1.

	tests := []struct {
		desc                 string
		monotonicCount       *MonotonicCount
		sampleValue          float64
		flushFirstValue      bool
		expectsError         bool
		expectedFlushedValue float64
	}{
		{
			"1: Flush after first sample and FlushFirstValue enabled: flush value as-is",
			&monotonicCount1,
			10.,
			true,
			false,
			10.,
		},
		{
			"1: Flush after another sample with a lower value and FlushFirstValue enabled: flush the lower value as-is",
			&monotonicCount1,
			8.,
			true,
			false,
			8.,
		},
		{
			"1: Flush after another sample with a higher value and FlushFirstValue enabled: flush diff",
			&monotonicCount1,
			10.,
			true,
			false,
			2.,
		},
		{
			"1: Flush after another sample with the same value and FlushFirstValue enabled: flush 0",
			&monotonicCount1,
			10.,
			true,
			false,
			0.,
		},
		{
			"1: Flush after another sample with a lower value and FlushFirstValue disabled: flush 0",
			&monotonicCount1,
			6.,
			false,
			false,
			0.,
		},
		{
			"1: Flush after another sample with a higher value and FlushFirstValue disabled: flush diff",
			&monotonicCount1,
			9.,
			false,
			false,
			3.,
		},
		{
			"2: Flush after first sample and FlushFirstValue disabled: error, flush nothing",
			&monotonicCount2,
			10.,
			false,
			true,
			0.,
		},
		{
			"2: Flush after another sample with a higher value and FlushFirstValue enabled: flush diff",
			&monotonicCount2,
			12.,
			true,
			false,
			2.,
		},
		{
			"2: Flush after another sample with a lower value and FlushFirstValue enabled: flush value as-is",
			&monotonicCount2,
			10.,
			true,
			false,
			10.,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tt.monotonicCount.addSample(&MetricSample{Value: tt.sampleValue, FlushFirstValue: tt.flushFirstValue}, timestamp)
			series, err := tt.monotonicCount.flush(timestamp)
			if tt.expectsError {
				assert.NotNil(t, err)
				assert.Len(t, series, 0)
			} else {
				assert.Nil(t, err)
				if assert.Len(t, series, 1) && assert.Len(t, series[0].Points, 1) {
					if tt.expectedFlushedValue == 0. {
						assert.Equal(t, 0., series[0].Points[0].Value, epsilon)
					} else {
						assert.InEpsilon(t, tt.expectedFlushedValue, series[0].Points[0].Value, epsilon)
					}
					assert.EqualValues(t, timestamp, series[0].Points[0].Ts)
				}
			}
		})
	}

	// at the end of all the tests, both monotonic counters should flush no value if no further samples are submitted
	series, err := monotonicCount1.flush(timestamp)
	assert.NotNil(t, err)
	assert.Len(t, series, 0)
	series, err = monotonicCount2.flush(timestamp)
	assert.NotNil(t, err)
	assert.Len(t, series, 0)
}
