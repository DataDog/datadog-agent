package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/percentile"
)

// FIXME(Jee) : This should be integrated with time_sampler.go since it
// duplicates code/logic.

// DistSampler creates sketches of metrics by buckets of 'interval' seconds
type DistSampler struct {
	interval            int64
	contextResolver     *ContextResolver
	sketchesByTimestamp map[int64]ContextSketch
	defaultHostname     string
}

// NewDistSampler returns a newly initialized DistSampler
func NewDistSampler(interval int64, defaultHostname string) *DistSampler {
	return &DistSampler{
		interval:            interval,
		contextResolver:     newContextResolver(),
		sketchesByTimestamp: map[int64]ContextSketch{},
		defaultHostname:     defaultHostname,
	}
}

func (d *DistSampler) calculateBucketStart(timestamp int64) int64 {
	return timestamp - timestamp%d.interval
}

// Add the metricSample to the correct sketch
func (d *DistSampler) addSample(metricSample *MetricSample, timestamp int64) {
	contextKey := d.contextResolver.trackContext(metricSample, timestamp)
	bucketStart := d.calculateBucketStart(timestamp)
	sketch, ok := d.sketchesByTimestamp[bucketStart]
	if !ok {
		sketch = makeContextSketch()
		d.sketchesByTimestamp[bucketStart] = sketch
	}
	sketch.addSample(contextKey, metricSample, timestamp, d.interval)
}

func (d *DistSampler) flush(timestamp int64) []*percentile.SketchSeries {
	var result []*percentile.SketchSeries

	sketchesByContext := make(map[string]*percentile.SketchSeries)

	cutoffTime := d.calculateBucketStart(timestamp)
	for timestamp, ctxSketch := range d.sketchesByTimestamp {
		if cutoffTime <= timestamp {
			continue
		}

		sketches := ctxSketch.flush(timestamp)
		for _, sketchSeries := range sketches {
			contextKey := sketchSeries.ContextKey

			if existingSeries, ok := sketchesByContext[contextKey]; ok {
				existingSeries.Sketches = append(existingSeries.Sketches, sketchSeries.Sketches[0])
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
		delete(d.sketchesByTimestamp, timestamp)
	}
	d.contextResolver.expireContexts(timestamp - int64(defaultExpiry/time.Second))

	return result
}
