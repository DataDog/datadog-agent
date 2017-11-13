// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// NOTE: This file contains a feature in development that is NOT supported.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"

	log "github.com/cihub/seelog"
)

// Distribution tracks the distribution of samples added over one flush
// period. Designed to be globally accurate for percentiles.
type Distribution struct {
	sketch     percentile.QSketch
	sketchType percentile.SketchType
	count      int64
}

// NewDistributionGK creates a new Distribution containing GKArray
func NewDistributionGK() *Distribution {
	return &Distribution{
		sketch:     percentile.QSketch(percentile.NewGKArray()),
		sketchType: percentile.SketchGK,
	}
}

// NewDistributionKLL creates a new Distribution containing GKArray
func NewDistributionKLL() *Distribution {
	return &Distribution{
		sketch:     percentile.NewKLL(),
		sketchType: percentile.SketchKLL,
	}
}

// NewDistributionComplete creates a new Distribution containing CompleteDS
func NewDistributionComplete() *Distribution {
	return &Distribution{
		sketch:     percentile.NewCompleteDS(),
		sketchType: percentile.SketchComplete,
	}
}

func (d *Distribution) addSample(sample *MetricSample, timestamp float64) {
	// Insert sample value into the sketch
	switch sample.Mtype {
	case DistributionType:
		if d.sketchType != percentile.SketchGK {
			log.Errorf("Sample metric type %s does not match sketch type %s",
				sample.Mtype, d.sketchType)
			return
		}
	case DistributionKType:
		if d.sketchType != percentile.SketchKLL {
			log.Errorf("Sample metric type %s does not match sketch type %s",
				sample.Mtype, d.sketchType)
			return
		}
	case DistributionCType:
		if d.sketchType != percentile.SketchComplete {
			log.Errorf("Sample metric type %s does not match sketch type %s",
				sample.Mtype, d.sketchType)
			return
		}
	default:
		log.Errorf("Sample metric type %d is not a distribution metric type",
			sample.Mtype)
		return
	}
	d.sketch = d.sketch.Add(sample.Value)
	d.count++
}

func (d *Distribution) flush(timestamp float64) (*percentile.SketchSeries, error) {
	if d.count == 0 {
		return &percentile.SketchSeries{}, percentile.NoSketchError{}
	}
	sketch := &percentile.SketchSeries{
		Sketches: []percentile.Sketch{{Timestamp: int64(timestamp),
			Sketch: d.sketch}},
		SketchType: d.sketchType,
	}
	// Reset the distribution after flush. The sketch type remains the same.
	d.count = 0
	switch d.sketchType {
	case percentile.SketchGK:
		d.sketch = percentile.QSketch(percentile.NewGKArray())
	case percentile.SketchKLL:
		d.sketch = percentile.QSketch(percentile.NewKLL())
	case percentile.SketchComplete:
		d.sketch = percentile.QSketch(percentile.NewCompleteDS())
	default:
		log.Error("Unknown sketch type:", d.sketchType)
	}
	return sketch, nil
}

// DistributionMetric is an interface for distributions
type DistributionMetric interface {
	addSample(sample *MetricSample, timestamp float64)
	flush(timestamp float64) (*percentile.SketchSeries, error)
}
