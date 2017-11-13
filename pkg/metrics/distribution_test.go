// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
	"github.com/stretchr/testify/assert"
)

func TestDistributionSampling(t *testing.T) {
	distro := NewDistributionGK()

	// OK to flush an empty distribution
	_, err := distro.flush(10)
	assert.NotNil(t, err)

	// Add metric samples and check that the flushed summary series
	// are correct
	distro.addSample(&MetricSample{Value: 1, Mtype: DistributionType}, 10)
	distro.addSample(&MetricSample{Value: 10, Mtype: DistributionType}, 11)
	distro.addSample(&MetricSample{Value: 5, Mtype: DistributionType}, 12)

	assert.Equal(t, int64(3), distro.count)

	sketchSeries, err := distro.flush(15)
	assert.Nil(t, err)

	expectedSketch := percentile.NewGKArray()
	expectedSketch = expectedSketch.Add(1).(percentile.GKArray)
	expectedSketch = expectedSketch.Add(10).(percentile.GKArray)
	expectedSketch = expectedSketch.Add(5).(percentile.GKArray)
	expectedSeries := &percentile.SketchSeries{
		Sketches:   []percentile.Sketch{{Timestamp: int64(15), Sketch: expectedSketch}},
		SketchType: percentile.SketchGK}

	AssertSketchSeriesEqual(t, expectedSeries, sketchSeries)

	// Distribution is reset after a flush
	assert.Equal(t, int64(0), distro.count)
	_, err = distro.flush(20)
	assert.NotNil(t, err)
}

func TestDistributionWrongSampleType(t *testing.T) {
	distro := NewDistributionKLL()

	// Sample with wrong Mtype does not get added
	distro.addSample(&MetricSample{Value: 1, Mtype: DistributionType}, 10)
	distro.addSample(&MetricSample{Value: 1, Mtype: DistributionKType}, 10)
	assert.Equal(t, int64(1), distro.count)
}
