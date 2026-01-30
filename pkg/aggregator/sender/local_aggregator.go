// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// localAggregator accumulates metrics over a time window before flushing.
// Aggregation rules:
// - Gauges: keep latest value
// - Counts: sum values
// - Rates: average values
// - Monotonic counts: sum values
// - Histograms: accumulate all values
type localAggregator struct {
	metrics          map[string]*aggregatedMetric
	histogramBuckets map[string]*aggregatedHistogramBucket
}

type aggregatedMetric struct {
	name            string
	mtype           metrics.MetricType
	hostname        string
	tags            []string
	value           float64
	count           int
	noIndex         bool
	flushFirstValue bool
}

type aggregatedHistogramBucket struct {
	name            string
	value           int64
	lowerBound      float64
	upperBound      float64
	monotonic       bool
	hostname        string
	tags            []string
	flushFirstValue bool
}

func newLocalAggregator() *localAggregator {
	return &localAggregator{
		metrics:          make(map[string]*aggregatedMetric),
		histogramBuckets: make(map[string]*aggregatedHistogramBucket),
	}
}

// addGauge adds or updates a gauge (keeps latest value)
func (a *localAggregator) addGauge(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	a.metrics[key] = &aggregatedMetric{
		name:     name,
		mtype:    metrics.GaugeType,
		hostname: hostname,
		tags:     tags,
		value:    value, // Latest value for gauges
		count:    1,
		noIndex:  false,
	}
}

// addGaugeNoIndex adds or updates a gauge with noIndex flag
func (a *localAggregator) addGaugeNoIndex(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	a.metrics[key] = &aggregatedMetric{
		name:     name,
		mtype:    metrics.GaugeType,
		hostname: hostname,
		tags:     tags,
		value:    value,
		count:    1,
		noIndex:  true,
	}
}

// addCount adds to a count (sums values)
func (a *localAggregator) addCount(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:     name,
			mtype:    metrics.CountType,
			hostname: hostname,
			tags:     tags,
			value:    value,
			count:    1,
		}
	}
}

// addRate adds to a rate (averages values)
func (a *localAggregator) addRate(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:     name,
			mtype:    metrics.RateType,
			hostname: hostname,
			tags:     tags,
			value:    value,
			count:    1,
		}
	}
}

// addMonotonicCount adds to a monotonic count (sums values)
func (a *localAggregator) addMonotonicCount(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:     name,
			mtype:    metrics.MonotonicCountType,
			hostname: hostname,
			tags:     tags,
			value:    value,
			count:    1,
		}
	}
}

// addMonotonicCountWithFlushFirstValue adds to a monotonic count with flush first value flag
func (a *localAggregator) addMonotonicCountWithFlushFirstValue(name string, value float64, hostname string, tags []string, flushFirstValue bool) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:            name,
			mtype:           metrics.MonotonicCountType,
			hostname:        hostname,
			tags:            tags,
			value:           value,
			count:           1,
			flushFirstValue: flushFirstValue,
		}
	}
}

// addHistogram adds to a histogram (averages values for simplicity)
func (a *localAggregator) addHistogram(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:     name,
			mtype:    metrics.HistogramType,
			hostname: hostname,
			tags:     tags,
			value:    value,
			count:    1,
		}
	}
}

// addHistorate adds to a historate (averages values)
func (a *localAggregator) addHistorate(name string, value float64, hostname string, tags []string) {
	key := metricKey(name, tags, hostname)
	if existing, found := a.metrics[key]; found {
		existing.value += value
		existing.count++
	} else {
		a.metrics[key] = &aggregatedMetric{
			name:     name,
			mtype:    metrics.HistorateType,
			hostname: hostname,
			tags:     tags,
			value:    value,
			count:    1,
		}
	}
}

// addHistogramBucket adds a histogram bucket
func (a *localAggregator) addHistogramBucket(name string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	key := metricKey(name, tags, hostname)
	// For histogram buckets, we sum the values
	if existing, found := a.histogramBuckets[key]; found {
		existing.value += value
	} else {
		a.histogramBuckets[key] = &aggregatedHistogramBucket{
			name:            name,
			value:           value,
			lowerBound:      lowerBound,
			upperBound:      upperBound,
			monotonic:       monotonic,
			hostname:        hostname,
			tags:            tags,
			flushFirstValue: flushFirstValue,
		}
	}
}

// flush returns all aggregated metrics and resets the aggregator
func (a *localAggregator) flush() []aggregatedMetric {
	result := make([]aggregatedMetric, 0, len(a.metrics))
	for _, metric := range a.metrics {
		// For rates, histograms, and historates, compute average
		if (metric.mtype == metrics.RateType || metric.mtype == metrics.HistogramType || metric.mtype == metrics.HistorateType) && metric.count > 0 {
			metric.value = metric.value / float64(metric.count)
		}
		result = append(result, *metric)
	}

	// Reset for next window
	a.metrics = make(map[string]*aggregatedMetric)

	return result
}

// flushHistogramBuckets returns all aggregated histogram buckets and resets them
func (a *localAggregator) flushHistogramBuckets() []aggregatedHistogramBucket {
	result := make([]aggregatedHistogramBucket, 0, len(a.histogramBuckets))
	for _, bucket := range a.histogramBuckets {
		result = append(result, *bucket)
	}

	// Reset for next window
	a.histogramBuckets = make(map[string]*aggregatedHistogramBucket)

	return result
}

// metricKey creates a unique key for metric name + tags + hostname
func metricKey(name string, tags []string, hostname string) string {
	// Sort tags for consistent key
	sortedTags := make([]string, len(tags))
	copy(sortedTags, tags)
	sort.Strings(sortedTags)

	return name + "|" + hostname + "|" + strings.Join(sortedTags, ",")
}
