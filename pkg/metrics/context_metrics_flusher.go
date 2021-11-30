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

	f.merge(func(contextKey ckey.ContextKey, m Metric, bucketTimestamp float64) {
		flushToSeries(
			contextKey,
			m,
			bucketTimestamp,
			&series,
			errors)
	}, func() {
		callback(series)
		series = series[:0]
	})
	return errors
}

// For each context key, calls several times `callback``.
// Call `contextKeyChanged` when handling another context key
func (f *ContextMetricsFlusher) merge(
	callback func(ckey.ContextKey, Metric, float64),
	contextKeyChanged func()) {
	for i := 0; i < len(f.metrics); i++ {
		for contextKey, metrics := range f.metrics[i].contextMetrics {
			callback(contextKey, metrics, f.metrics[i].bucketTimestamp)

			// Find `contextKey` in the remaining contextMetrics
			for j := i + 1; j < len(f.metrics); j++ {
				contextMetrics := f.metrics[j].contextMetrics
				if m, found := contextMetrics[contextKey]; found {
					callback(contextKey, m, f.metrics[j].bucketTimestamp)
					delete(contextMetrics, contextKey)
				}
			}
			contextKeyChanged()
		}
	}
}
