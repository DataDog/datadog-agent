package aggregator

import (
	log "github.com/cihub/seelog"
)

// ContextSketch stores the distributions by context key
type ContextSketch map[string]*Distribution

func makeContextSketch() ContextSketch {
	return ContextSketch(make(map[string]*Distribution))
}

func (c ContextSketch) addSample(contextKey string, sample *MetricSample,
	timestamp int64, interval int64) {

	if _, ok := c[contextKey]; !ok {
		c[contextKey] = NewDistribution()
	}
	c[contextKey].addSample(sample, timestamp)
}

func (c ContextSketch) flush(timestamp int64) []*SketchSerie {
	var sketches []*SketchSerie

	for contextKey, distribution := range c {
		sketchSerie, err := distribution.flush(timestamp)
		if err == nil {
			sketchSerie.contextKey = contextKey
			sketches = append(sketches, sketchSerie)
		} else {
			switch err.(type) {
			case NoSketchError:
			default:
				log.Info("An error occurred while flushing metric summary on context key '%s': %s",
					contextKey, err)
			}
		}
	}
	return sketches
}
