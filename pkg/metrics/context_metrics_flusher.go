// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

type timestampedContextMetrics struct {
	bucketTimestamp float64
	contextMetrics  ContextMetrics
}

// ContextMetricsFlusher flushes several ContextMetrics
type ContextMetricsFlusher struct {
	metrics []timestampedContextMetrics
}

// NewContextMetricsFlusher creates a new instance of ContextMetricsFlusher
func NewContextMetricsFlusher() *ContextMetricsFlusher {
	return &ContextMetricsFlusher{}
}

// Append appends a new contextMetrics
func (f *ContextMetricsFlusher) Append(bucketTimestamp float64, contextMetrics ContextMetrics) {
	f.metrics = append(f.metrics, timestampedContextMetrics{
		bucketTimestamp: bucketTimestamp,
		contextMetrics:  contextMetrics,
	})
}

// FlushAndClear flushes contextMetrics to series and clear contextMetrics collection.
// For each contextKey, FlushAndClear flushes every metrics (Same as ContextMetrics.Flush) and call
// the callback with all series whose key context is the same.
// If there are 3 context keys, the callback is called 3 times.
// Note: The slice []*Serie in callback is reused.
func (f *ContextMetricsFlusher) FlushAndClear(callback func([]*Serie)) map[ckey.ContextKey][]error {
	errors := make(map[ckey.ContextKey][]error)
	var series []*Serie

	var contextMetricsCollection []ContextMetrics
	for _, m := range f.metrics {
		contextMetricsCollection = append(contextMetricsCollection, m.contextMetrics)
	}
	mergeContextMetrics(
		contextMetricsCollection,
		func(contextKey ckey.ContextKey, m Metric, contextMetricIndex int) {
			flushToSeries(
				contextKey,
				m,
				f.metrics[contextMetricIndex].bucketTimestamp,
				&series,
				errors)
		}, func() {
			callback(series)
			series = series[:0]
		})
	return errors
}
