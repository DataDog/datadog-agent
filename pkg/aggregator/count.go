package aggregator

// Count is used to count the number of events that occur between 2 flushes. Each sample's value is added
// to the value that's flushed
type Count struct {
	value   float64
	sampled bool
}

func (c *Count) addSample(sample *MetricSample, timestamp int64) {
	c.value += sample.Value
	c.sampled = true
}

func (c *Count) flush(timestamp int64) ([]*Serie, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		&Serie{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APICountType,
		},
	}, nil
}
