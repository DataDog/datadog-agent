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

// ContextMetricsFlusher sorts Metrics by context key, in a streaming fashion.
// It accepts a collection of timestamped ContextMetrics instances,
// each of which contains Metric instances organized by context key.  Its FlushAndClear
// method then flushes those Metric instances one context key at a time, without
// requiring the space to sort Metrics by context key.
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

// FlushAndClear flushes Metrics appended to this instance, and clears the instance.
// For each context key present in any of the ContextMetrics instances, it constructs
// a slice containing all Serie instances with that context key, and passes that slice to
// `callback`. Any errors encountered flushing the Metric instances are returned,
// but such errors do not interrupt the flushing operation.
func (f *ContextMetricsFlusher) FlushAndClear(callback func([]*Serie)) map[ckey.ContextKey][]error {
	errors := make(map[ckey.ContextKey][]error)
	var series []*Serie

	contextMetricsCollection := make([]ContextMetrics, 0, len(f.metrics))
	for _, m := range f.metrics {
		contextMetricsCollection = append(contextMetricsCollection, m.contextMetrics)
	}

	errorsByContextKey := make(map[ckey.ContextKey]error)

	aggregateContextMetricsByContextKey(
		contextMetricsCollection,
		func(contextKey ckey.ContextKey, m Metric, contextMetricIndex int) {
			series = flushToSeries(
				contextKey,
				m,
				f.metrics[contextMetricIndex].bucketTimestamp,
				series,
				errorsByContextKey)
			for k, err := range errorsByContextKey {
				errors[k] = append(errors[k], err)
				delete(errorsByContextKey, k)
			}
		}, func() {
			callback(series)
			series = series[:0]
		})
	return errors
}
