// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const checksSourceTypeName = "System"

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series          []*metrics.Serie
	contextResolver *ContextResolver
	metrics         metrics.ContextMetrics
	sketchMap       sketchMap
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler() *CheckSampler {
	return &CheckSampler{
		series:          make([]*metrics.Serie, 0),
		contextResolver: newContextResolver(),
		metrics:         metrics.MakeContextMetrics(),
		sketchMap:       make(sketchMap),
	}
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample, metricSample.Timestamp)

	if err := cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1); err != nil {
		log.Debug("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (cs *CheckSampler) addBucket(bucket *metrics.HistogramBucket) {
	log.Errorf("Adding bucket %v", bucket)
}

func (cs *CheckSampler) commit(timestamp float64) {
	series, errors := cs.metrics.Flush(timestamp)
	for ckey, err := range errors {
		context, ok := cs.contextResolver.contextsByKey[ckey]
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
			continue
		}
		log.Infof("No value returned for check metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags, err)
	}
	for _, serie := range series {
		// Resolve context and populate new []Serie
		context, ok := cs.contextResolver.contextsByKey[serie.ContextKey]
		if !ok {
			log.Errorf("Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", serie.ContextKey)
			continue
		}
		serie.Name = context.Name + serie.NameSuffix
		serie.Tags = context.Tags
		serie.Host = context.Host
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks

		cs.series = append(cs.series, serie)
	}

	cs.contextResolver.expireContexts(timestamp - defaultExpiry)
}

func (cs *CheckSampler) flush() metrics.Series {
	series := cs.series
	cs.series = make([]*metrics.Serie, 0)
	return series
}
