package aggregator

import (
	"time"
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

func (d *DistSampler) flush(timestamp int64) []*SketchSerie {
	var result []*SketchSerie

	sketchesByContext := make(map[string]*SketchSerie)

	cutoffTime := d.calculateBucketStart(timestamp)
	for timestamp, ctxSketch := range d.sketchesByTimestamp {
		if cutoffTime <= timestamp {
			continue
		}

		sketches := ctxSketch.flush(timestamp)
		for _, sketchSerie := range sketches {
			contextKey := sketchSerie.contextKey

			if existingSerie, ok := sketchesByContext[contextKey]; ok {
				existingSerie.Sketches = append(existingSerie.Sketches, sketchSerie.Sketches[0])
			} else {
				context := d.contextResolver.contextsByKey[contextKey]
				sketchSerie.Name = context.Name
				sketchSerie.Tags = context.Tags
				if context.Host != "" {
					sketchSerie.Host = context.Host
				} else {
					sketchSerie.Host = d.defaultHostname
				}
				sketchSerie.DeviceName = context.DeviceName
				sketchSerie.Interval = d.interval

				sketchesByContext[contextKey] = sketchSerie
				result = append(result, sketchSerie)
			}
		}
		delete(d.sketchesByTimestamp, timestamp)
	}
	d.contextResolver.expireContexts(timestamp - int64(defaultExpiry/time.Second))

	return result
}
