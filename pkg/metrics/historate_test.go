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

func TestHistorateEmptyFlush(t *testing.T) {
	h := NewHistorate(1)

	// Flush w/o samples: error
	_, err := h.flush(50)
	assert.NotNil(t, err)
}

func TestHistorateAddSampleOnce(t *testing.T) {
	h := NewHistorate(1)
	h.addSample(&MetricSample{Value: 1}, 50)

	// Flush one sample: error
	_, err := h.flush(50)
	assert.NotNil(t, err)
}

func TestHistorateAddSample(t *testing.T) {
	h := NewHistorate(1)

	h.addSample(&MetricSample{Value: 1}, 50)
	h.addSample(&MetricSample{Value: 2}, 51)

	// Flush one sample: error
	series, err := h.flush(52)
	require.Nil(t, err)
	if assert.Len(t, series, 5) {
		for _, serie := range series {
			assert.Len(t, serie.Points, 1)
			assert.EqualValues(t, 52, serie.Points[0].Ts)
		}
		assert.InEpsilon(t, 1, series[0].Points[0].Value, epsilon)  // max
		assert.Equal(t, ".max", series[0].NameSuffix)               // max
		assert.InEpsilon(t, 1, series[1].Points[0].Value, epsilon)  // median
		assert.Equal(t, ".median", series[1].NameSuffix)            // median
		assert.InEpsilon(t, 1., series[2].Points[0].Value, epsilon) // avg
		assert.Equal(t, ".avg", series[2].NameSuffix)               // avg
		assert.InEpsilon(t, 1, series[3].Points[0].Value, epsilon)  // count
		assert.Equal(t, ".count", series[3].NameSuffix)             // count
		assert.InEpsilon(t, 1, series[4].Points[0].Value, epsilon)  // 0.95
		assert.Equal(t, ".95percentile", series[4].NameSuffix)      // 0.95
	}

	assert.Equal(t, false, h.sampled)
	assert.Equal(t, 0.0, h.previousSample)
	assert.EqualValues(t, 0, h.previousTimestamp)
}
