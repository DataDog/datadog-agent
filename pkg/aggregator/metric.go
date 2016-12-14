package aggregator

import "errors"

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample float64, timestamp int64)
}

// Gauge stores and aggregates a gauge value
type Gauge struct {
	gauge     float64
	timestamp int64
}

func (g *Gauge) addSample(sample float64, timestamp int64) {
	g.gauge = sample
	g.timestamp = timestamp
}

func (g *Gauge) flush() (value float64, timestamp int64) {
	value, timestamp = g.gauge, g.timestamp
	g.gauge, g.timestamp = 0, 0

	return
}

// Rate stores and aggregates a rate value
type Rate struct {
	previousSample    float64
	previousTimestamp int64
	sample            float64
	timestamp         int64
}

func (r *Rate) addSample(sample float64, timestamp int64) {
	if r.timestamp != 0 {
		r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	}
	r.sample, r.timestamp = sample, timestamp
}

func (r *Rate) flush() (value float64, timestamp int64, err error) {
	value, timestamp, err = 0, 0, errors.New("No value sampled at this check run or at previous check run, no rate value can be computed")
	if r.previousTimestamp != 0 && r.timestamp != 0 {
		value, timestamp, err = (r.sample-r.previousSample)/float64(r.timestamp-r.previousTimestamp), r.timestamp, nil
		r.previousSample, r.previousTimestamp = r.sample, r.timestamp
		r.sample, r.timestamp = 0, 0
	}
	return
}

// Counter stores and aggregates a counter values
type Counter struct {
	count     int
	timestamp int64
}

func (c *Counter) addSample(sample float64, timestamp int64) {
	// TODO
}
