package percentile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func TestMarshalSketches(t *testing.T) {
	sketch1 := QSketch{
		GKArray{
			Entries: []Entry{{1, 1, 0}},
			Min:     1, Count: 1, Sum: 1, Avg: 1, Max: 1,
			incoming: make([]float64, 0, int(1/EPSILON))}}
	sketch2 := QSketch{
		GKArray{
			Entries: []Entry{{10, 1, 0}, {14, 3, 0}, {21, 2, 0}},
			Min:     10, Count: 6, Sum: 96, Avg: 16, Max: 21,
			incoming: make([]float64, 0, int(1/EPSILON))}}
	series := []*SketchSeries{{
		ContextKey: "test_context",
		Sketches:   []Sketch{{int64(12345), sketch1}, {int64(67890), sketch2}},
		Name:       "test.metrics",
		Host:       "localHost",
		Tags:       []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalSketchSeries(series)
	assert.Nil(t, err)

	decodedPayload, metadata, err := UnmarshalSketchSeries(payload)
	assert.Nil(t, err)

	assert.Equal(t, 1, len(decodedPayload))
	assert.Equal(t, agentpayload.CommonMetadata{}, metadata)
	assert.Equal(t, "test.metrics", (*decodedPayload[0]).Name)
	assert.Equal(t, "tag1", (*decodedPayload[0]).Tags[0])
	assert.Equal(t, "tag2:yes", (*decodedPayload[0]).Tags[1])
	assert.Equal(t, "localHost", (*decodedPayload[0]).Host)
	assert.Equal(t, Sketch{Timestamp: 12345, Sketch: sketch1}, (*decodedPayload[0]).Sketches[0])
	assert.Equal(t, Sketch{Timestamp: 67890, Sketch: sketch2}, (*decodedPayload[0]).Sketches[1])

}

func TestMarshalJSONSketchSeries(t *testing.T) {
	sketch1 := QSketch{
		GKArray{
			Entries: []Entry{{1, 1, 0}},
			Min:     1, Count: 1, Sum: 1, Avg: 1, Max: 1}}
	sketch2 := QSketch{
		GKArray{
			Entries: []Entry{{10, 1, 0}, {14, 3, 0}, {21, 2, 0}},
			Min:     10, Count: 6, Sum: 96, Avg: 16, Max: 21}}
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

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"max\":1,\"n\":1,\"sum\":1,\"avg\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"min\":10,\"max\":21,\"n\":6,\"sum\":96,\"avg\":16}}]}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}

func TestUnmarshalJSONSketchSeries(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"max\":1,\"n\":1,\"sum\":1,\"avg\":1}}]}]}\n")

	sketch := QSketch{
		GKArray{Entries: []Entry{{1, 1, 0}},
			Min: 1, Count: 1, Sum: 1, Avg: 1, Max: 1},
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
