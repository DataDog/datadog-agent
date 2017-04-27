package aggregator

import (
	"fmt"
	"sort"

	log "github.com/cihub/seelog"
)

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

func (h *Histogram) addSample(sample *MetricSample, timestamp int64) {
	h.samples = append(h.samples, sample.Value)
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
