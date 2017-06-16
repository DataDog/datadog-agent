package aggregator

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func AssertSketchSerieEqual(t *testing.T, expected, actual *SketchSerie) {
	assert.Equal(t, expected.Name, actual.Name)
	if expected.Tags != nil {
		assert.NotNil(t, actual.Tags)
		AssertTagsEqual(t, expected.Tags, actual.Tags)
	}
	assert.Equal(t, expected.Host, actual.Host)
	assert.Equal(t, expected.DeviceName, actual.DeviceName)
	assert.Equal(t, expected.Interval, actual.Interval)
	if expected.contextKey != "" {
		assert.Equal(t, expected.contextKey, actual.contextKey)
	}
	if expected.Sketches != nil {
		assert.NotNil(t, actual.Sketches)
		AssertSketchesEqual(t, expected.Sketches, actual.Sketches)
	}
}

func AssertSketchesEqual(t *testing.T, expected, actual []Sketch) {
	if assert.Equal(t, len(expected), len(actual)) {
		actualOrdered := OrderedSketches{actual}
		sort.Sort(actualOrdered)
		for i, sketch := range expected {
			assert.Equal(t, sketch, actualOrdered.sketches[i])
		}
	}
}

// OrderedSketches used to sort []Sketch
type OrderedSketches struct {
	sketches []Sketch
}

func (os OrderedSketches) Len() int {
	return len(os.sketches)
}

func (os OrderedSketches) Less(i, j int) bool {
	return os.sketches[i].timestamp < os.sketches[j].timestamp
}

func (os OrderedSketches) Swap(i, j int) {
	os.sketches[i], os.sketches[j] = os.sketches[j], os.sketches[i]
}

// OrderedSketchSeries used to sort []*SketchSerie
type OrderedSketchSeries struct {
	sketchSeries []*SketchSerie
}

func (oss OrderedSketchSeries) Len() int {
	return len(oss.sketchSeries)
}

func (oss OrderedSketchSeries) Less(i, j int) bool {
	return oss.sketchSeries[i].contextKey < oss.sketchSeries[j].contextKey
}

func (oss OrderedSketchSeries) Swap(i, j int) {
	oss.sketchSeries[i], oss.sketchSeries[j] = oss.sketchSeries[j], oss.sketchSeries[i]
}

func TestDistSamplerBucketSampling(t *testing.T) {
	distSampler := NewDistSampler(10, "")

	mSample1 := MetricSample{
		Name:       "test.metric.name",
		Value:      1,
		Mtype:      DistributionType,
		Tags:       &[]string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := MetricSample{
		Name:       "test.metric.name",
		Value:      2,
		Mtype:      DistributionType,
		Tags:       &[]string{"a", "b"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10001)
	distSampler.addSample(&mSample2, 10002)
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10012)
	distSampler.addSample(&mSample1, 10021)

	sketchSeries := distSampler.flush(10020)

	expectedSketch := QSketch{}
	expectedSketch.Insert(1)
	expectedSketch.Insert(2)
	expectedSerie := &SketchSerie{
		Name:     "test.metric.name",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []Sketch{
			Sketch{int64(10000), expectedSketch},
			Sketch{int64(10010), expectedSketch},
		},
		contextKey: "test.metric.name,a,b",
	}
	assert.Equal(t, 1, len(sketchSeries))
	AssertSketchSerieEqual(t, expectedSerie, sketchSeries[0])

	// The samples added after the flush time remains in the dist sampler
	assert.Equal(t, 1, len(distSampler.sketchesByTimestamp))
}

func TestDistSamplerContextSampling(t *testing.T) {
	distSampler := NewDistSampler(10, "")

	mSample1 := MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      DistributionType,
		Tags:       &[]string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := MetricSample{
		Name:       "test.metric.name2",
		Value:      1,
		Mtype:      DistributionType,
		Tags:       &[]string{"a", "c"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10011)

	orderedSketchSeries := OrderedSketchSeries{distSampler.flush(10020)}
	sort.Sort(orderedSketchSeries)
	sketchSeries := orderedSketchSeries.sketchSeries

	expectedSketch := QSketch{}
	expectedSketch.Insert(1)
	expectedSerie1 := &SketchSerie{
		Name:       "test.metric.name1",
		Tags:       []string{"a", "b"},
		Interval:   10,
		Sketches:   []Sketch{Sketch{int64(10010), expectedSketch}},
		contextKey: "test.metric.name1,a,b",
	}
	expectedSerie2 := &SketchSerie{
		Name:       "test.metric.name2",
		Tags:       []string{"a", "c"},
		Interval:   10,
		Sketches:   []Sketch{Sketch{int64(10010), expectedSketch}},
		contextKey: "test.metric.name2,a,c",
	}

	assert.Equal(t, 2, len(sketchSeries))
	AssertSketchSerieEqual(t, expectedSerie1, sketchSeries[0])
	AssertSketchSerieEqual(t, expectedSerie2, sketchSeries[1])
}
