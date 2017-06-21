package percentile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalJSONSketchSeries(t *testing.T) {
	sketch1 := QSketch{
		GKArray{
			Entries: []Entry{{1, 1, 0}},
			Min:     1, ValCount: 1}}
	sketch2 := QSketch{
		GKArray{
			Entries: []Entry{{10, 1, 0}, {14, 3, 0}, {21, 2, 0}},
			Min:     10, ValCount: 6}}
	series := []*SketchSeries{{
		ContextKey: "test_context",
		Sketches:   []Sketch{{int64(12345), sketch1}, {int64(67890), sketch2}},
		Name:       "test.metrics",
		Host:       "localHost",
		Tags:       []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalJSONSketchSeries(series)
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"n\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"min\":10,\"n\":6}}]}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}

func TestUnmarshalJSONSketchSeries(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"n\":1}}]}]}\n")

	sketch := QSketch{
		GKArray{Entries: []Entry{{1, 1, 0}},
			Min: 1, ValCount: 1},
	}

	data, err := UnmarshalJSONSketchSeries(payload)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(data))

	assert.Equal(t, "test.metrics", data[0].Name)
	assert.Equal(t, "tag:yes", data[0].Tags[0])
	assert.Equal(t, "localHost", data[0].Host)
	assert.Equal(t, int64(0), data[0].Interval)
	assert.Equal(t, Sketch{Timestamp: 12345, Sketch: sketch}, data[0].Sketches[0])
}
