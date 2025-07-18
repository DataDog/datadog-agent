// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math"
)

// Counter tracks how many times something happened per second. Counters are
// only used by DogStatsD and are very similar to Count: the main diffence is
// that they are sent as Rate.
type Counter struct {
	value float64
}

// NewCounter return a new initialized Counter
func NewCounter() *Counter {
	return &Counter{
		value: math.NaN(),
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func (c *Counter) addSample(sample *MetricSample, _ float64) {
	if math.IsNaN(c.value) {
		c.value = 0.0
	}
	c.value += sample.Value * (1 / sample.SampleRate)
}

func (c *Counter) flush(timestamp float64) ([]*Serie, error) {
	value := c.value
	c.value = math.NaN()

	if math.IsNaN(value) {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value / 10}},
			MType:  APIRateType,
		},
	}, nil
}

func (c *Counter) isStateful() bool {
	return false
}
