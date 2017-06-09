package aggregator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextSketchSampling(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey := "context_key"

	ctxSketch.addSample(contextKey, &MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey, &MetricSample{Value: 5}, 3, 10)
	resultSeries := ctxSketch.flush(12345)

	expectedSketch := SliceSummary{}
	expectedSketch.Insert(1)
	expectedSketch.Insert(5)
	expectedSerie := &SketchSerie{
		contextKey: contextKey,
		Sketches:   []Sketch{{int64(12345), expectedSketch}}}

	assert.Equal(t, 1, len(resultSeries))
	AssertSketchSerieEqual(t, expectedSerie, resultSeries[0])

	// No series should be flushed when there's no new sample since
	// last flush
	resultSeries = ctxSketch.flush(12355)
	assert.Equal(t, 0, len(resultSeries))
}

// The summaries can handle sample values of +Inf/-Inf
func TestContextSketchSamplingInfinity(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey := "context_key"

	ctxSketch.addSample(contextKey, &MetricSample{Value: math.Inf(1)}, 1, 10)
	ctxSketch.addSample(contextKey, &MetricSample{Value: math.Inf(-1)}, 2, 10)
	resultSeries := ctxSketch.flush(12345)

	expectedSketch := SliceSummary{}
	expectedSketch.Insert(math.Inf(1))
	expectedSketch.Insert(math.Inf(-1))
	expectedSerie := &SketchSerie{
		contextKey: contextKey,
		Sketches:   []Sketch{{int64(12345), expectedSketch}}}

	assert.Equal(t, 1, len(resultSeries))
	AssertSketchSerieEqual(t, expectedSerie, resultSeries[0])
}

func TestContextSketchSamplingMultiContexts(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey1 := "context_key1"
	contextKey2 := "context_key2"
	ctxSketch.addSample(contextKey1, &MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey2, &MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey1, &MetricSample{Value: 3}, 5, 10)
	orderedSketchSeries := OrderedSketchSeries{ctxSketch.flush(12345)}

	expectedSketch1 := SliceSummary{}
	expectedSketch1.Insert(1)
	expectedSketch1.Insert(3)
	expectedSerie1 := &SketchSerie{
		contextKey: contextKey1,
		Sketches:   []Sketch{{int64(12345), expectedSketch1}}}
	expectedSketch2 := SliceSummary{}
	expectedSketch2.Insert(1)
	expectedSerie2 := &SketchSerie{
		contextKey: contextKey2,
		Sketches:   []Sketch{{int64(12345), expectedSketch2}}}

	assert.Equal(t, 2, orderedSketchSeries.Len())

	AssertSketchSerieEqual(t, expectedSerie1, orderedSketchSeries.sketchSeries[0])
	AssertSketchSerieEqual(t, expectedSerie2, orderedSketchSeries.sketchSeries[1])
}
