// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// NOTE: This file contains a feature in development that is NOT supported.

package metrics

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"

	log "github.com/cihub/seelog"
)

// FIXME(Jee): This should be integrated into context_metrics.go as it duplicates
// the logic.

// ContextSketch stores the distributions by context key
type ContextSketch map[string]DistributionMetric

// MakeContextSketch returns a new ContextSketch
func MakeContextSketch() ContextSketch {
	return ContextSketch(make(map[string]DistributionMetric))
}

// AddSample adds a sample to the ContextSketch
func (c ContextSketch) AddSample(contextKey string, sample *MetricSample, timestamp float64, interval int64) {
	if math.IsInf(sample.Value, 0) {
		log.Warn("Ignoring sample with +/-Inf value on context key:", contextKey)
		return
	}
	if _, ok := c[contextKey]; !ok {
		switch sample.Mtype {
		case DistributionType:
			c[contextKey] = NewDistributionGK()
		case DistributionKType:
			c[contextKey] = NewDistributionKLL()
		case DistributionCType:
			c[contextKey] = NewDistributionComplete()
		default:
			log.Error("Unknown distribution metric type:", sample.Mtype)
		}
	}
	c[contextKey].addSample(sample, timestamp)
}

// Flush flushes sketches in the ContextSketch
func (c ContextSketch) Flush(timestamp float64) []*percentile.SketchSeries {
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
