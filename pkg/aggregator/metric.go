package aggregator

import (
	"fmt"
	"sort"

	log "github.com/cihub/seelog"
)

type points [][]interface{}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name       string        `json:"metric"`
	Points     points        `json:"points"`
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
)

// MarshalText implements the encoding.TextMarshal interface to marshal
// an APIMetricType to a serialized byte slice
func (a APIMetricType) MarshalText() ([]byte, error) {
	switch a {
	case APIGaugeType:
		return []byte("gauge"), nil
	case APIRateType:
		return []byte("rate"), nil
	default:
		return []byte{}, fmt.Errorf("Can't marshal unknown metric type %d", a)
	}
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

	if r.timestamp-r.previousTimestamp == 0 {
		return []*Serie{}, fmt.Errorf("Rate was sampled twice at the same timestamp, can't compute a rate")
	}

	value, ts := (r.sample-r.previousSample)/float64(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0, 0

	return []*Serie{
		&Serie{
			Points: points{{ts, value}},
			MType:  APIGaugeType,
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
			Points:     points{{timestamp, value}},
			MType:      mType,
			nameSuffix: "." + aggregate,
		})
	}

	// Compute percentiles
	for _, percentile := range h.percentiles {
		value := h.samples[(percentile*len(h.samples)-1)/100]
		series = append(series, &Serie{
			Points:     points{{timestamp, value}},
			MType:      APIGaugeType,
			nameSuffix: fmt.Sprintf(".%dpercentile", percentile),
		})
	}

	h.samples = []float64{}

	return series, nil
}
