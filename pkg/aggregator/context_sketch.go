package aggregator

import (
	"math"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator/percentile"
)

// FIXME(Jee): This should be integrated into context_metrics.go as it duplicates
// the logic.

// ContextSketch stores the distributions by context key
type ContextSketch map[string]*Distribution

func makeContextSketch() ContextSketch {
	return ContextSketch(make(map[string]*Distribution))
}

func (c ContextSketch) addSample(contextKey string, sample *MetricSample, timestamp int64, interval int64) {
	if math.IsInf(sample.Value, 0) {
		log.Warn("Ignoring sample with +/-Inf value on context key:", contextKey)
		return
	}
	if _, ok := c[contextKey]; !ok {
		c[contextKey] = NewDistribution()
	}
	c[contextKey].addSample(sample, timestamp)
}

func (c ContextSketch) flush(timestamp int64) []*percentile.SketchSeries {
	var sketches []*percentile.SketchSeries

	for contextKey, distribution := range c {
		sketchSeries, err := distribution.flush(timestamp)
		if err == nil {
			sketchSeries.ContextKey = contextKey
			sketches = append(sketches, sketchSeries)
		} else {
			switch err.(type) {
			case percentile.NoSketchError:
			default:
				log.Info("An error occurred while flushing metric summary on context key '%s': %s",
					contextKey, err)
			}
		}
	}
	return sketches
}
