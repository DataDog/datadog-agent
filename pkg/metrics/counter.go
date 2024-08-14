// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// Counter tracks how many times something happened per second. Counters are
// only used by DogStatsD and are very similar to Count: the main diffence is
// that they are sent as Rate.
type Counter struct {
	value    float64
	sampled  bool
	interval int64
}

// NewCounter return a new initialized Counter
func NewCounter(interval int64) *Counter {
	return &Counter{
		sampled:  false,
		interval: interval,
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func (c *Counter) addSample(sample *MetricSample, _ float64) {
	c.value += sample.Value * (1 / sample.SampleRate)
	c.sampled = true
}

func (c *Counter) flush(timestamp float64) ([]*Serie, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value / float64(c.interval)}},
			MType:  APIRateType,
		},
	}, nil
}

func (c *Counter) isStateful() bool {
	return false
}
