// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

func TestDistributionSampling(t *testing.T) {
	distro := NewDistribution()

	// OK to flush an empty global histogram
	_, err := distro.flush(10)
	assert.NotNil(t, err)

	// Add metric samples and check that the flushed summary series
	// are correct
	distro.addSample(&metrics.MetricSample{Value: 1}, 10)
	distro.addSample(&metrics.MetricSample{Value: 10}, 11)
	distro.addSample(&metrics.MetricSample{Value: 5}, 12)

	assert.Equal(t, int64(3), distro.count)

	sketchSeries, err := distro.flush(15)
	assert.Nil(t, err)

	expectedSketch := percentile.NewQSketch()
	expectedSketch = expectedSketch.Add(1)
	expectedSketch = expectedSketch.Add(10)
	expectedSketch = expectedSketch.Add(5)
	expectedSeries := &percentile.SketchSeries{
		Sketches: []percentile.Sketch{{Timestamp: int64(15), Sketch: expectedSketch}}}

	AssertSketchSeriesEqual(t, expectedSeries, sketchSeries)

	// Global histogram is reset after a flush
	assert.Equal(t, int64(0), distro.count)

	_, err = distro.flush(20)
	assert.NotNil(t, err)
}
