package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/percentile"
	"github.com/stretchr/testify/assert"
)

func TestDistributionSampling(t *testing.T) {
	distro := NewDistribution()

	// OK to flush an empty global histogram
	_, err := distro.flush(10)
	assert.NotNil(t, err)

	// Add metric samples and check that the flushed summary series
	// are correct
	distro.addSample(&MetricSample{Value: 1}, 10)
	distro.addSample(&MetricSample{Value: 10}, 11)
	distro.addSample(&MetricSample{Value: 5}, 12)

	assert.Equal(t, int64(3), distro.count)

	sketchSeries, err := distro.flush(15)
	assert.Nil(t, err)

	expectedSketch := percentile.NewQSketch()
	expectedSketch.Add(1)
	expectedSketch.Add(10)
	expectedSketch.Add(5)
	expectedSketch.Compress()
	expectedSeries := &percentile.SketchSeries{
		Sketches: []percentile.Sketch{{Timestamp: int64(15), Sketch: expectedSketch}}}
	AssertSketchSeriesEqual(t, expectedSeries, sketchSeries)

	// Global histogram is reset after a flush
	assert.Equal(t, int64(0), distro.count)

	_, err = distro.flush(20)
	assert.NotNil(t, err)
}
