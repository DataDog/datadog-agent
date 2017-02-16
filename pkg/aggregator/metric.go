package aggregator

import (
	"fmt"
	"sort"

	log "github.com/cihub/seelog"
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    int64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", p.Ts, p.Value)), nil
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name       string        `json:"metric"`
	Points     []Point       `json:"points"`
	Tags       []string      `json:"tags"`
	Host       string        `json:"host"`
	DeviceName string        `json:"device_name"`
	MType      APIMetricType `json:"type"`
	Interval   int64         `json:"interval"`
	contextKey string
	nameSuffix string
}

// APIMetricType represents an API metric type
type APIMetricType int

// Enumeration of the existing API metric types
const (
	APIGaugeType APIMetricType = iota
	APIRateType
	APICountType
)

// String returns a string representation of APIMetricType
func (a APIMetricType) String() string {
	switch a {
	case APIGaugeType:
		return "gauge"
	case APIRateType:
		return "rate"
	case APICountType:
		return "count"
	default:
		return ""
	}
}

// MarshalText implements the encoding.TextMarshal interface to marshal
// an APIMetricType to a serialized byte slice
func (a APIMetricType) MarshalText() ([]byte, error) {
	str := a.String()
	if str == "" {
		return []byte{}, fmt.Errorf("Can't marshal unknown metric type %d", a)
	}

	return []byte(str), nil
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
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APIGaugeType,
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

	if r.timestamp == r.previousTimestamp {
		return []*Serie{}, fmt.Errorf("Rate was sampled twice at the same timestamp, can't compute a rate")
	}

	value, ts := (r.sample-r.previousSample)/float64(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0, 0

	return []*Serie{
		&Serie{
			Points: []Point{{Ts: ts, Value: value}},
			MType:  APIGaugeType,
		},
	}, nil
}

// Count is used to count the number of events that occur between 2 flushes. Each sample's value is added
// to the value that's flushed
type Count struct {
	value   float64
	sampled bool
}

func (c *Count) addSample(sample float64, timestamp int64) {
	c.value += sample
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

// MonotonicCount tracks a raw counter, based on increasing counter values.
// Samples that have a lower value than the previous sample are ignored (since it usually
// means that the underlying raw counter has been reset).
// Example:
//  submitting samples 2, 3, 6, 7 returns 5 (i.e. 7-2) on flush ;
//  then submitting samples 10, 11 on the same MonotonicCount returns 4 (i.e. 11-7) on flush
type MonotonicCount struct {
	previousSample        float64
	currentSample         float64
	sampledSinceLastFlush bool
	hasPreviousSample     bool
	value                 float64
}

func (mc *MonotonicCount) addSample(sample float64, timestamp int64) {
	if !mc.sampledSinceLastFlush {
		mc.currentSample = sample
		mc.sampledSinceLastFlush = true
	} else {
		mc.previousSample, mc.currentSample = mc.currentSample, sample
		mc.hasPreviousSample = true
	}

	// To handle cases where the samples are not monotonically increasing, we always add the difference
	// between 2 consecutive samples to the value that'll be flushed (if the difference is >0).
	diff := mc.currentSample - mc.previousSample
	if mc.sampledSinceLastFlush && mc.hasPreviousSample && diff > 0. {
		mc.value += diff
	}
}

func (mc *MonotonicCount) flush(timestamp int64) ([]*Serie, error) {
	if !mc.sampledSinceLastFlush || !mc.hasPreviousSample {
		return []*Serie{}, NoSerieError{}
	}

	value := mc.value
	// reset struct fields
	mc.previousSample, mc.currentSample, mc.value = mc.currentSample, 0., 0.
	mc.sampledSinceLastFlush = false

	return []*Serie{
		&Serie{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APICountType,
		},
	}, nil
}

// Histogram tracks the distribution of samples added over one flush period
type Histogram struct {
	aggregates  []string // aggregates configured on this histogram
	percentiles []int    // percentiles configured on this histogram, each in the 1-100 range
	samples     []float64
	configured  bool
}

const (
	maxAgg    = "max"
	minAgg    = "min"
	medianAgg = "median"
	avgAgg    = "avg"
	sumAgg    = "sum"
	countAgg  = "count"
)

func (h *Histogram) configure(aggregates []string, percentiles []int) {
	h.configured = true
	h.aggregates = aggregates
	h.percentiles = percentiles
}

func (h *Histogram) addSample(sample float64, timestamp int64) {
	h.samples = append(h.samples, sample)
}

func (h *Histogram) sum() (sum float64) {
	for _, sample := range h.samples {
		sum += sample
	}
	return sum
}

func (h *Histogram) flush(timestamp int64) ([]*Serie, error) {
	if len(h.samples) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	if !h.configured {
		// Set default aggregates/percentiles if configure() hasn't been called beforehand
		h.configure([]string{maxAgg, medianAgg, avgAgg, countAgg}, []int{95})
	}

	sort.Float64s(h.samples)

	series := make([]*Serie, 0, len(h.aggregates)+len(h.percentiles))

	// Compute aggregates
	sum := h.sum()
	count := len(h.samples)
	for _, aggregate := range h.aggregates {
		var value float64
		mType := APIGaugeType
		switch aggregate {
		case maxAgg:
			value = h.samples[count-1]
		case minAgg:
			value = h.samples[0]
		case medianAgg:
			value = h.samples[(count-1)/2]
		case avgAgg:
			value = sum / float64(count)
		case sumAgg:
			value = sum
		case countAgg:
			value = float64(count)
			mType = APIRateType
		default:
			log.Infof("Configured aggregate '%s' is not implemented, skipping", aggregate)
			continue
		}

		series = append(series, &Serie{
			Points:     []Point{{Ts: timestamp, Value: value}},
			MType:      mType,
			nameSuffix: "." + aggregate,
		})
	}

	// Compute percentiles
	for _, percentile := range h.percentiles {
		value := h.samples[(percentile*len(h.samples)-1)/100]
		series = append(series, &Serie{
			Points:     []Point{{Ts: timestamp, Value: value}},
			MType:      APIGaugeType,
			nameSuffix: fmt.Sprintf(".%dpercentile", percentile),
		})
	}

	h.samples = []float64{}

	return series, nil
}
