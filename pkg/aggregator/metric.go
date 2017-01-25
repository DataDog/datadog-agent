package aggregator

import (
	// "errors"
	"sort"
)

type points [][]interface{}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name       string   `json:"metric"`
	Points     points   `json:"points"`
	Tags       []string `json:"tags"`
	Host       string   `json:"host"`
	DeviceName string   `json:"device_name"`
	Mtype      string   `json:"type"`
	Interval   int64    `json:"interval"`
	contextKey string
	nameSuffix string
}

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample float64, timestamp int64)
	flush(timestamp int64) ([]*Serie, error)
}

// NoSerieError is the error returned by a metric when not enough samples have been
// submitted to generate a serie
type NoSerieError struct{}

func (e NoSerieError) Error() string {
	return "Not enough samples to generate points"
}

// Gauge tracks the value of a metric
type Gauge struct {
	gauge   float64
	sampled bool
}

func (g *Gauge) addSample(sample float64, timestamp int64) {
	g.gauge = sample
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
			Points: points{{timestamp, value}},
			Mtype:  "gauge",
		},
	}, nil
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

func (r *Rate) flush(timestamp int64) ([]*Serie, error) {
	if r.previousTimestamp == 0 || r.timestamp == 0 {
		return []*Serie{}, NoSerieError{}
	}

	value, ts := (r.sample-r.previousSample)/float64(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0, 0

	return []*Serie{
		&Serie{
			Points: points{{ts, value}},
			Mtype:  "gauge",
		},
	}, nil
}

// Histogram tracks the distribution of samples added over one flush period
type Histogram struct {
	samples []float64
}

func (h *Histogram) addSample(sample float64, timestamp int64) {
	h.samples = append(h.samples, sample)
}

func (h *Histogram) flush(timestamp int64) ([]*Serie, error) {
	count := len(h.samples)
	if count == 0 {
		return []*Serie{}, NoSerieError{}
	}

	sort.Float64s(h.samples)
	max := h.samples[count-1]
	med := h.samples[(count-1)/2]
	var avg float64
	for _, sample := range h.samples {
		avg += sample
	}
	avg /= float64(len(h.samples))

	h.samples = []float64{}

	return []*Serie{
		&Serie{
			Points:     points{{timestamp, max}},
			Mtype:      "gauge",
			nameSuffix: ".max",
		},
		&Serie{
			Points:     points{{timestamp, med}},
			Mtype:      "gauge",
			nameSuffix: ".median",
		},
		&Serie{
			Points:     points{{timestamp, avg}},
			Mtype:      "gauge",
			nameSuffix: ".avg",
		},
		&Serie{
			Points:     points{{timestamp, float64(count)}},
			Mtype:      "rate",
			nameSuffix: ".count",
		},
	}, nil
}
