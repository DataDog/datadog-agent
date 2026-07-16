// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// SketchScalarProjection projects a retained sketch point into a scalar point
// for monitor evaluation. The projection is intentionally separate from sketch
// retention and egress serialization: lookback still retains and forwards the
// full sketch, while this interface only bridges the current scalar monitor.
type SketchScalarProjection interface {
	Project(metrics.SketchPoint) (float64, bool)
}

// PlaceholderAverageSketchProjection is a deliberately isolated placeholder for
// distribution-monitor evaluation. It projects each retained sketch to its
// average so DogStatsD distribution metrics can exercise the scalar monitor path
// while the product semantics for sketch-aware monitoring are deferred.
type PlaceholderAverageSketchProjection struct{}

// Project returns the sketch average from BasicStats.
func (PlaceholderAverageSketchProjection) Project(point metrics.SketchPoint) (float64, bool) {
	if point.Sketch == nil {
		return 0, false
	}
	cnt, _, _, _, avg := point.Sketch.BasicStats()
	if cnt <= 0 || math.IsInf(avg, 0) || math.IsNaN(avg) {
		return 0, false
	}
	return avg, true
}
