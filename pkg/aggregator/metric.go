package aggregator

import (
	"errors"
	"sort"
)

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample float64, timestamp int64)
}

// Gauge tracks the value of a metric
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

// Rate tracks the rate of a metric over 2 successive flushes
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

// Histogram tracks the distribution of samples added over one flush period
type Histogram struct {
	samples   []float64
	timestamp int64
}

func (h *Histogram) addSample(sample float64, timestamp int64) {
	h.samples = append(h.samples, sample)
	h.timestamp = timestamp
}

func (h *Histogram) flush() ([]float64, int64) {
	count := len(h.samples)
	if count == 0 {
		return []float64{}, 0
	}

	sort.Float64s(h.samples)
	max := h.samples[count-1]
	med := h.samples[(count-1)/2]
	var avg float64
	for _, sample := range h.samples {
		avg += sample
	}
	avg /= float64(len(h.samples))

	timestamp := h.timestamp

	h.samples = []float64{}
	h.timestamp = 0

	return []float64{max, med, avg, float64(count)}, timestamp
}

// Counter stores and aggregates a counter values
type Counter struct {
	count     int
	timestamp int64
}

func (c *Counter) addSample(sample float64, timestamp int64) {
	// TODO
}
