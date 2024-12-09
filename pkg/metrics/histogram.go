// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"sort"
	"strconv"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// weightSample represent a sample with its weight in the histogram (deduce from SampleRate)
type weightSample struct {
	value  float64
	weight int64
}

type weightSamples []weightSample

func (w weightSamples) Len() int           { return len(w) }
func (w weightSamples) Less(i, j int) bool { return w[i].value < w[j].value }
func (w weightSamples) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }

// Histogram tracks the distribution of samples added over one flush period
type Histogram struct {
	aggregates  []string // aggregates configured on this histogram
	percentiles []int    // percentiles configured on this histogram, each in the 1-100 range
	interval    int64    // interval over which the `count` value is normalized (bucket interval for Dogstatsd, 1 otherwise)
	samples     weightSamples
	sum         float64
	count       int64
}

const (
	maxAgg    = "max"
	minAgg    = "min"
	medianAgg = "median"
	avgAgg    = "avg"
	sumAgg    = "sum"
	countAgg  = "count"
)

var (
	defaultAggregates  = []string(nil)
	defaultPercentiles = []int(nil)
)

func parsePercentiles(percentiles []string) []int {
	res := []int{}
	for _, p := range percentiles {
		i, err := strconv.ParseFloat(p, 64)
		if err != nil {
			log.Errorf("Could not parse '%s' from 'histogram_percentiles' (skipping): %s", p, err)
			continue
		}
		if i < 0 || i > 1 {
			log.Errorf("histogram_percentiles must be between 0 and 1: skipping %f", i)
			continue
		}
		// in some cases the '*100' will lower the number resulting in
		// an int lower by 1 from what is expected (ex: 0.29 would
		// become 28). As a workaround we add 0.5 before casting.
		res = append(res, int(i*100+0.5))
	}
	return res
}

// NewHistogram returns a newly initialized histogram
func NewHistogram(interval int64, config pkgconfigmodel.Config) *Histogram {
	// we initialize default value on the first histogram creation
	if defaultAggregates == nil {
		defaultAggregates = config.GetStringSlice("histogram_aggregates")
	}
	if defaultPercentiles == nil {
		c := []string{}
		err := structure.UnmarshalKey(config, "histogram_percentiles", &c)
		if err != nil {
			log.Errorf("Could not Unmarshal histogram configuration: %s", err)
		} else {
			defaultPercentiles = parsePercentiles(c)
			sort.Ints(defaultPercentiles)
		}
	}

	return &Histogram{
		interval:    interval,
		aggregates:  defaultAggregates,
		percentiles: defaultPercentiles,
	}
}

func (h *Histogram) configure(aggregates []string, percentiles []int) {
	h.aggregates = aggregates
	sort.Ints(percentiles)
	h.percentiles = percentiles
}

//nolint:revive // TODO(AML) Fix revive linter
func (h *Histogram) addSample(sample *MetricSample, _ float64) {
	rate := sample.SampleRate
	if rate == 0 {
		rate = 1
	}

	h.samples = append(h.samples, weightSample{sample.Value, int64(1 / rate)}) // add value and its weight
	h.sum += sample.Value * (1 / rate)
	h.count += int64(1 / rate)
}

func (h *Histogram) flush(timestamp float64) ([]*Serie, error) {
	if len(h.samples) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	sort.Sort(h.samples)

	series := make([]*Serie, 0, len(h.aggregates)+len(h.percentiles))

	// Compute aggregates
	for _, aggregate := range h.aggregates {
		var value float64
		mType := APIGaugeType
		switch aggregate {
		case maxAgg:
			value = h.samples[len(h.samples)-1].value
		case minAgg:
			value = h.samples[0].value
		case medianAgg:
			weight := int64(0)
			target := (h.count - 1) / 2
			for _, s := range h.samples {
				weight += s.weight
				if weight > target {
					value = s.value
					break
				}
			}
		case avgAgg:
			value = h.sum / float64(h.count)
		case sumAgg:
			value = h.sum
		case countAgg:
			value = float64(h.count) / float64(h.interval)
			mType = APIRateType
		default:
			log.Infof("Configured aggregate '%s' is not implemented, skipping", aggregate)
			continue
		}

		series = append(series, &Serie{
			Points:     []Point{{Ts: timestamp, Value: value}},
			MType:      mType,
			NameSuffix: "." + aggregate,
		})
	}

	// Compute percentiles
	target := make([]int64, 0, len(h.percentiles))
	for _, percentile := range h.percentiles {
		target = append(target, (int64(percentile)*h.count-1)/100)
	}

	if len(target) > 0 {
		weight := int64(0)
		idx := 0
		for _, s := range h.samples {
			weight += s.weight
			for idx < len(target) && weight > target[idx] {
				series = append(series, &Serie{
					Points:     []Point{{Ts: timestamp, Value: s.value}},
					MType:      APIGaugeType,
					NameSuffix: fmt.Sprintf(".%dpercentile", h.percentiles[idx]),
				})
				idx++
			}
			if idx >= len(h.percentiles) {
				break
			}
		}
	}

	// reset histogram
	h.samples = weightSamples{}
	h.sum = 0
	h.count = 0

	return series, nil
}

func (h *Histogram) isStateful() bool {
	return false
}
