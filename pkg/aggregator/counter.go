package aggregator

// Counter track how many times something happened per second. Counters are
// only use by DogStatsD and are very similar to Count: the main diffence is
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

func (c *Counter) addSample(sample *MetricSample, timestamp int64) {
	c.value += sample.Value * (1 / sample.SampleRate)
	c.sampled = true
}

func (c *Counter) flush(timestamp int64) ([]*Serie, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		&Serie{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value / float64(c.interval)}},
			MType:  APIRateType,
		},
	}, nil
}
