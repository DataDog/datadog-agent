package aggregator

// Gauge tracks the value of a metric
type Gauge struct {
	gauge   float64
	sampled bool
}

func (g *Gauge) addSample(sample *MetricSample, timestamp int64) {
	g.gauge = sample.Value
	g.sampled = true
}

func (g *Gauge) flush(timestamp int64) ([]*Serie, error) {
	value, sampled := g.gauge, g.sampled
	g.gauge, g.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		&Serie{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APIGaugeType,
		},
	}, nil
}
