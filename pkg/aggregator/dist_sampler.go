// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// NOTE: This file contains a feature in development that is NOT supported.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"

	log "github.com/cihub/seelog"
)

// FIXME(Jee) : This should be integrated with time_sampler.go since it
// duplicates code/logic.

// DistSampler creates sketches of metrics by buckets of 'interval' seconds
type DistSampler struct {
	interval            int64
	contextResolver     *ContextResolver
	sketchesByTimestamp map[int64]metrics.ContextSketch
	defaultHostname     string
}

// NewDistSampler returns a newly initialized DistSampler
func NewDistSampler(interval int64, defaultHostname string) *DistSampler {
	return &DistSampler{
		interval:            interval,
		contextResolver:     newContextResolver(),
		sketchesByTimestamp: map[int64]metrics.ContextSketch{},
		defaultHostname:     defaultHostname,
	}
}

func (d *DistSampler) calculateBucketStart(timestamp float64) int64 {
	return int64(timestamp) - int64(timestamp)%d.interval
}

// Add the metricSample to the correct sketch
func (d *DistSampler) addSample(metricSample *metrics.MetricSample, timestamp float64) {
	contextKey := d.contextResolver.trackContext(metricSample, timestamp)
	bucketStart := d.calculateBucketStart(timestamp)
	sketch, ok := d.sketchesByTimestamp[bucketStart]
	if !ok {
		sketch = metrics.MakeContextSketch()
		d.sketchesByTimestamp[bucketStart] = sketch
	}
	sketch.AddSample(contextKey, metricSample, timestamp, d.interval)
}

// flush returns a list of SketchSeries, each of which contains sketches
// of the same SketchType. Only sketches of the first occurring SketchType
// will be flushed.
func (d *DistSampler) flush(timestamp float64) percentile.SketchSeriesList {
	var result []*percentile.SketchSeries

	sketchesByContext := make(map[ckey.ContextKey]*percentile.SketchSeries)

	cutoffTime := d.calculateBucketStart(timestamp)
	for bucketTimestamp, ctxSketch := range d.sketchesByTimestamp {
		if cutoffTime <= bucketTimestamp {
			continue
		}

		sketches := ctxSketch.Flush(float64(bucketTimestamp))
		for _, sketchSeries := range sketches {
			contextKey := sketchSeries.ContextKey

			if existingSeries, ok := sketchesByContext[contextKey]; ok {
				// Make sure that all sketches in a SketchSeries are of the same type
				if existingSeries.SketchType == sketchSeries.SketchType {
					existingSeries.Sketches = append(existingSeries.Sketches, sketchSeries.Sketches[0])
				} else {
					log.Errorf("ContextKey %s has two SketchTypes %s and %s. Dropping %s",
						contextKey, existingSeries.SketchType,
						sketchSeries.SketchType, sketchSeries.SketchType)
				}
			} else {
				context := d.contextResolver.contextsByKey[contextKey]
				sketchSeries.Name = context.Name
				sketchSeries.Tags = context.Tags
				if context.Host != "" {
					sketchSeries.Host = context.Host
				} else {
					sketchSeries.Host = d.defaultHostname
				}
				sketchSeries.Interval = d.interval

				sketchesByContext[contextKey] = sketchSeries
				result = append(result, sketchSeries)
			}
		}
		delete(d.sketchesByTimestamp, bucketTimestamp)
	}
	d.contextResolver.expireContexts(timestamp - defaultExpiry)

	return result
}
