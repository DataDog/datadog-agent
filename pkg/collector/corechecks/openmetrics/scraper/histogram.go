// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// HistogramOptions controls how histograms are transformed.
type HistogramOptions struct {
	CollectHistogramBuckets          bool
	HistogramBucketsAsDistributions  bool
	NonCumulativeHistogramBuckets    bool
	CollectCountersWithDistributions bool
}

// newHistogramTransformer returns a TransformerFunc that handles histogram-type
// metrics with multiple code paths depending on the HistogramOptions.
func newHistogramTransformer(opts HistogramOptions) TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, flushFirstValue bool) {
		if !opts.CollectHistogramBuckets {
			// Path 5: only submit _sum and _count, skip all buckets.
			submitHistogramCounters(metricName, samples, sndr)
			return
		}

		if opts.HistogramBucketsAsDistributions {
			// Paths 1 and 2: decumulate and submit as OpenmetricsBucket.
			decumulated := decumulateHistogramBuckets(samples)

			if opts.CollectCountersWithDistributions {
				// Path 1: submit _sum, _count, and buckets.
				submitHistogramCounters(metricName, decumulated, sndr)
			}
			// Both Path 1 and Path 2 submit buckets.
			submitDistributionBuckets(metricName, decumulated, sndr, flushFirstValue)
			return
		}

		if opts.NonCumulativeHistogramBuckets {
			// Path 3: decumulate and submit as MonotonicCount.
			decumulated := decumulateHistogramBuckets(samples)
			submitHistogramCounters(metricName, decumulated, sndr)
			submitMonotonicBuckets(metricName, decumulated, sndr)
			return
		}

		// Path 4 (DEFAULT): cumulative buckets as MonotonicCount.
		submitHistogramCounters(metricName, samples, sndr)
		submitMonotonicBuckets(metricName, samples, sndr)
	}
}

// submitHistogramCounters submits _sum and _count samples as MonotonicCount.
func submitHistogramCounters(metricName string, samples []SampleData, sndr sender.Sender) {
	for i := range samples {
		sd := &samples[i]
		if shouldSkip(sd.Sample.Value) {
			continue
		}
		name := sd.Sample.Metric["__name__"]
		switch {
		case strings.HasSuffix(name, "_sum"):
			sndr.MonotonicCount(metricName+".sum", sd.Sample.Value, sd.Hostname, sd.Tags)
		case strings.HasSuffix(name, "_count"):
			sndr.MonotonicCount(metricName+".count", sd.Sample.Value, sd.Hostname, sd.Tags)
		}
	}
}

// submitDistributionBuckets submits decumulated bucket samples via OpenmetricsBucket.
func submitDistributionBuckets(metricName string, samples []SampleData, sndr sender.Sender, flushFirstValue bool) {
	for i := range samples {
		sd := &samples[i]
		name := sd.Sample.Metric["__name__"]
		if !strings.HasSuffix(name, "_bucket") {
			continue
		}
		if shouldSkip(sd.Sample.Value) {
			continue
		}

		lowerBound := parseBound(sd.Sample.Metric["lower_bound"])
		upperBound := parseBound(getUpperBound(sd.Sample.Metric))

		// Skip degenerate buckets where lower == upper (e.g., -Inf/-Inf).
		if lowerBound == upperBound {
			continue
		}

		sndr.OpenmetricsBucket(metricName, int64(sd.Sample.Value), lowerBound, upperBound, true, sd.Hostname, sd.Tags, flushFirstValue)
	}
}

// submitMonotonicBuckets submits bucket samples as MonotonicCount, skipping +Inf buckets.
func submitMonotonicBuckets(metricName string, samples []SampleData, sndr sender.Sender) {
	for i := range samples {
		sd := &samples[i]
		name := sd.Sample.Metric["__name__"]
		if !strings.HasSuffix(name, "_bucket") {
			continue
		}
		if shouldSkip(sd.Sample.Value) {
			continue
		}

		ub := getUpperBound(sd.Sample.Metric)
		if ub == "+Inf" || ub == "Inf" {
			continue
		}

		sndr.MonotonicCount(metricName+".bucket", sd.Sample.Value, sd.Hostname, sd.Tags)
	}
}

// decumulateHistogramBuckets separates bucket samples from _sum/_count samples,
// groups buckets by tag identity (excluding upper_bound/le), sorts by upper_bound,
// computes deltas, and adds lower_bound tags.
func decumulateHistogramBuckets(samples []SampleData) []SampleData {
	var nonBucket []SampleData
	var buckets []SampleData

	for i := range samples {
		name := samples[i].Sample.Metric["__name__"]
		if strings.HasSuffix(name, "_bucket") {
			buckets = append(buckets, samples[i])
		} else {
			nonBucket = append(nonBucket, samples[i])
		}
	}

	if len(buckets) == 0 {
		return nonBucket
	}

	// Group buckets by tag identity (excluding upper_bound and le).
	type bucketGroup struct {
		key     string
		indices []int
	}
	groups := make(map[string]*bucketGroup)
	var groupOrder []string

	for i := range buckets {
		key := bucketTagKey(buckets[i])
		g, ok := groups[key]
		if !ok {
			g = &bucketGroup{key: key}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}
		g.indices = append(g.indices, i)
	}

	var decumulated []SampleData

	for _, key := range groupOrder {
		g := groups[key]
		// Sort by upper_bound within each group.
		sort.Slice(g.indices, func(a, b int) bool {
			ubA := parseBound(getUpperBound(buckets[g.indices[a]].Sample.Metric))
			ubB := parseBound(getUpperBound(buckets[g.indices[b]].Sample.Metric))
			return ubA < ubB
		})

		// Save original cumulative values before computing deltas,
		// since Sample is a pointer and we must not mutate the originals.
		origValues := make([]float64, len(g.indices))
		for j, idx := range g.indices {
			origValues[j] = buckets[idx].Sample.Value
		}

		// Compute deltas and assign lower_bound.
		for j, idx := range g.indices {
			orig := buckets[idx]

			// Create a new Sample copy so we don't mutate the original.
			newMetric := make(map[string]string, len(orig.Sample.Metric)+1)
			for k, v := range orig.Sample.Metric {
				newMetric[k] = v
			}
			newSample := &prometheus.Sample{
				Metric:    prometheus.Metric(newMetric),
				Value:     origValues[j],
				Timestamp: orig.Sample.Timestamp,
			}

			// Determine lower_bound.
			var lb string
			if j == 0 {
				lb = strconv.FormatFloat(math.Inf(-1), 'g', -1, 64)
			} else {
				lb = getUpperBound(buckets[g.indices[j-1]].Sample.Metric)
			}

			// Decumulate: subtract previous bucket's original cumulative value.
			if j > 0 {
				newSample.Value -= origValues[j-1]
			}

			// Add lower_bound to the new sample metric map.
			newSample.Metric["lower_bound"] = lb

			decumulated = append(decumulated, SampleData{
				Sample:   newSample,
				Tags:     orig.Tags,
				Hostname: orig.Hostname,
			})
		}
	}

	result := make([]SampleData, 0, len(nonBucket)+len(decumulated))
	result = append(result, nonBucket...)
	result = append(result, decumulated...)
	return result
}

// bucketTagKey computes a string key from a sample's tags excluding upper_bound
// and le labels, so that buckets belonging to the same histogram instance are
// grouped together.
func bucketTagKey(sd SampleData) string {
	pairs := make([]string, 0, len(sd.Sample.Metric))
	for k, v := range sd.Sample.Metric {
		if k == "le" || k == "upper_bound" {
			continue
		}
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// getUpperBound returns the upper bound value from a sample's labels,
// checking both "le" and "upper_bound" keys.
func getUpperBound(metric map[string]string) string {
	if v, ok := metric["le"]; ok {
		return v
	}
	if v, ok := metric["upper_bound"]; ok {
		return v
	}
	return ""
}

// parseBound parses a bound string into a float64, handling Inf values.
func parseBound(s string) float64 {
	switch s {
	case "+Inf", "Inf":
		return math.Inf(1)
	case "-Inf":
		return math.Inf(-1)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
