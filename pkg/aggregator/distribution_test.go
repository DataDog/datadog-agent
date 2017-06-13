package aggregator

import (
	"testing"

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

	sketchSerie, err := distro.flush(15)
	assert.Nil(t, err)

	expectedSketch := QSketch{}
	expectedSketch.Insert(1)
	expectedSketch.Insert(10)
	expectedSketch.Insert(5)
	expectedSerie := &SketchSerie{
		Sketches: []Sketch{{int64(15), expectedSketch}}}
	AssertSketchSerieEqual(t, expectedSerie, sketchSerie)

	// Global histogram is reset after a flush
	assert.Equal(t, int64(0), distro.count)

	_, err = distro.flush(20)
	assert.NotNil(t, err)
}
