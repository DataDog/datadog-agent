// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const checksSourceTypeName = "System"

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series          []*metrics.Serie
	contextResolver *ContextResolver
	metrics         metrics.ContextMetrics
	defaultHostname string
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler(hostname string) *CheckSampler {
	return &CheckSampler{
		series:          make([]*metrics.Serie, 0),
		contextResolver: newContextResolver(),
		metrics:         metrics.MakeContextMetrics(),
		defaultHostname: hostname,
	}
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample, metricSample.Timestamp)

	cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1)
}

func (cs *CheckSampler) commit(timestamp float64) {
	for _, serie := range cs.metrics.Flush(timestamp) {
		// Resolve context and populate new []Serie
		context := cs.contextResolver.contextsByKey[serie.ContextKey]
		serie.Name = context.Name + serie.NameSuffix
		serie.Tags = context.Tags
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks
		if context.Host != "" {
			serie.Host = context.Host
		} else {
			serie.Host = cs.defaultHostname
		}

		cs.series = append(cs.series, serie)
	}

	cs.contextResolver.expireContexts(timestamp - defaultExpiry)
}

func (cs *CheckSampler) flush() metrics.Series {
	series := cs.series
	cs.series = make([]*metrics.Serie, 0)
	return series
}
