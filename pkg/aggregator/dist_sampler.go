// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// NOTE: This file contains a feature in development that is NOT supported.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	defaultDistInterval = 10
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
	if interval == 0 {
		interval = defaultDistInterval
	}

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

	byCtx, ok := d.sketchesByTimestamp[bucketStart]
	if !ok {
		byCtx = make(metrics.ContextSketch)
		d.sketchesByTimestamp[bucketStart] = byCtx
	}

	byCtx.AddSample(contextKey, metricSample, timestamp, d.interval)
}

func (d *DistSampler) each(flushTs float64, f func(ckey.ContextKey, metrics.SketchPoint)) {
	tsLimit := d.calculateBucketStart(flushTs)

	for bucketTs, bucket := range d.sketchesByTimestamp {
		if tsLimit <= bucketTs {
			continue
		}

		for ctxkey, as := range bucket {
			f(ctxkey, metrics.SketchPoint{
				Sketch: as.Finish(),
				Ts:     bucketTs,
			})
		}

		delete(d.sketchesByTimestamp, bucketTs)
	}
}
func (d *DistSampler) flush(flushTs float64) metrics.SketchSeriesList {
	// points are inserted by `ts` -> `ctx`, we want to invert this mapping.
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)
	d.each(flushTs, func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}

		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})

	out := make(metrics.SketchSeriesList, 0, len(pointsByCtx))
	for ck, points := range pointsByCtx {
		out = append(out, d.newSeries(ck, points))
	}

	d.contextResolver.expireContexts(flushTs - defaultExpiry)
	return out
}

func (d *DistSampler) newSeries(ck ckey.ContextKey, points []metrics.SketchPoint) metrics.SketchSeries {
	ctx := d.contextResolver.contextsByKey[ck]
	ss := metrics.SketchSeries{
		Name:     ctx.Name,
		Tags:     ctx.Tags,
		Host:     ctx.Host,
		Interval: d.interval,
		Points:   points,
	}

	if ss.Host == "" {
		ss.Host = d.defaultHostname
	}

	return ss
}
