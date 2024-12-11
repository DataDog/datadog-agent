// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricWithTimestampGauge(t *testing.T) {
	// Initialize a new Gauge
	mGauge := NewMetricWithTimestamp(APIGaugeType)

	// Add samples
	mGauge.addSample(&MetricSample{Value: 1}, 50)
	mGauge.addSample(&MetricSample{Value: 2}, 55)

	series, _ := mGauge.flush(60)

	// all samples are flushed
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 2)

	// First point
	assert.InEpsilon(t, 1, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 50, series[0].Points[0].Ts)

	// Second point
	assert.InEpsilon(t, 2, series[0].Points[1].Value, epsilon)
	assert.EqualValues(t, 55, series[0].Points[1].Ts)

	// Add another sample after flush
	mGauge.addSample(&MetricSample{Value: 3}, 40)

	series, _ = mGauge.flush(100)

	// all samples are flushed
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.EqualValues(t, APIGaugeType, series[0].MType)

	// First point
	assert.InEpsilon(t, 3, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 40, series[0].Points[0].Ts)
}

func TestMetricWithTimestampCount(t *testing.T) {
	// Initialize a new Count
	mGauge := NewMetricWithTimestamp(APICountType)

	// Add samples
	mGauge.addSample(&MetricSample{Value: 1}, 50)
	mGauge.addSample(&MetricSample{Value: 2}, 55)

	series, _ := mGauge.flush(60)

	// all samples are flushed
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 2)

	// First point
	assert.InEpsilon(t, 1, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 50, series[0].Points[0].Ts)

	// Second point
	assert.InEpsilon(t, 2, series[0].Points[1].Value, epsilon)
	assert.EqualValues(t, 55, series[0].Points[1].Ts)

	// Add another sample after flush
	mGauge.addSample(&MetricSample{Value: 3}, 40)

	series, _ = mGauge.flush(100)

	// all samples are flushed
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.EqualValues(t, APICountType, series[0].MType)

	// First point
	assert.InEpsilon(t, 3, series[0].Points[0].Value, epsilon)
	assert.EqualValues(t, 40, series[0].Points[0].Ts)
}
