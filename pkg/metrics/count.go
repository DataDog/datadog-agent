// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// Count is used to count the number of events that occur between 2 flushes. Each sample's value is added
// to the value that's flushed
type Count struct {
	value   float64
	sampled bool
}

//nolint:revive // TODO(AML) Fix revive linter
func (c *Count) addSample(sample *MetricSample, _ float64) {
	c.value += sample.Value
	c.sampled = true
}

func (c *Count) flush(timestamp float64) ([]*Serie, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APICountType,
		},
	}, nil
}

func (c *Count) isStateful() bool {
	return false
}
