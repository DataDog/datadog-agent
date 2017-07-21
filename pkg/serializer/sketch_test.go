package serializer

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

func createSketchSeries() []*percentile.SketchSeries {
	sketch1 := percentile.QSketch{
		GKArray: percentile.GKArray{
			Entries: []percentile.Entry{{V: 1, G: 1, Delta: 0}},
			Min:     1, Count: 1, Sum: 1, Avg: 1, Max: 1}}
	sketch2 := percentile.QSketch{
		GKArray: percentile.GKArray{
			Entries: []percentile.Entry{{V: 10, G: 1, Delta: 0}, {V: 14, G: 3, Delta: 0}, {V: 21, G: 2, Delta: 0}},
			Min:     10, Count: 6, Sum: 96, Avg: 16, Max: 21}}
	series := []*percentile.SketchSeries{{
		ContextKey: "test_context",
		Sketches:   []percentile.Sketch{{Timestamp: int64(12345), Sketch: sketch1}, {Timestamp: int64(67890), Sketch: sketch2}},
		Name:       "test.metrics",
		Host:       "localHost",
		Tags:       []string{"tag1", "tag2:yes"},
	}}

	return series
}

func TestMarshalSketches(t *testing.T) {
	series := createSketchSeries()

	payload, contentType, err := MarshalSketchSeries(series)
	assert.Nil(t, err)
	assert.Equal(t, contentType, "application/x-protobuf")

	decodedPayload := &agentpayload.SketchPayload{}
	err = proto.Unmarshal(payload, decodedPayload)
	assert.Nil(t, err)

	require.Len(t, decodedPayload.Summaries, 1)
	assert.Equal(t, agentpayload.CommonMetadata{}, decodedPayload.Metadata)
	assert.Equal(t, "test.metrics", decodedPayload.Summaries[0].Metric)
	assert.Equal(t, "tag1", decodedPayload.Summaries[0].Tags[0])
	assert.Equal(t, "tag2:yes", decodedPayload.Summaries[0].Tags[1])
	assert.Equal(t, "localHost", decodedPayload.Summaries[0].Host)

	require.Len(t, decodedPayload.Summaries[0].Sketches, 2)

	// first sketch
	assert.Equal(t, int64(12345), decodedPayload.Summaries[0].Sketches[0].Ts)
	assert.Equal(t, int64(1), decodedPayload.Summaries[0].Sketches[0].N)
	assert.Equal(t, float64(1), decodedPayload.Summaries[0].Sketches[0].Min)
	assert.Equal(t, float64(1), decodedPayload.Summaries[0].Sketches[0].Max)
	assert.Equal(t, float64(1), decodedPayload.Summaries[0].Sketches[0].Avg)
	assert.Equal(t, float64(1), decodedPayload.Summaries[0].Sketches[0].Sum)

	require.Len(t, decodedPayload.Summaries[0].Sketches[0].Entries, 1)
	assert.Equal(t, float64(1), decodedPayload.Summaries[0].Sketches[0].Entries[0].V)
	assert.Equal(t, int64(1), decodedPayload.Summaries[0].Sketches[0].Entries[0].G)
	assert.Equal(t, int64(0), decodedPayload.Summaries[0].Sketches[0].Entries[0].Delta)

	// second sketch
	assert.Equal(t, int64(67890), decodedPayload.Summaries[0].Sketches[1].Ts)
	assert.Equal(t, int64(6), decodedPayload.Summaries[0].Sketches[1].N)
	assert.Equal(t, float64(10), decodedPayload.Summaries[0].Sketches[1].Min)
	assert.Equal(t, float64(21), decodedPayload.Summaries[0].Sketches[1].Max)
	assert.Equal(t, float64(16), decodedPayload.Summaries[0].Sketches[1].Avg)
	assert.Equal(t, float64(96), decodedPayload.Summaries[0].Sketches[1].Sum)

	require.Len(t, decodedPayload.Summaries[0].Sketches[1].Entries, 3)
	assert.Equal(t, float64(10), decodedPayload.Summaries[0].Sketches[1].Entries[0].V)
	assert.Equal(t, int64(1), decodedPayload.Summaries[0].Sketches[1].Entries[0].G)
	assert.Equal(t, int64(0), decodedPayload.Summaries[0].Sketches[1].Entries[0].Delta)

	assert.Equal(t, float64(14), decodedPayload.Summaries[0].Sketches[1].Entries[1].V)
	assert.Equal(t, int64(3), decodedPayload.Summaries[0].Sketches[1].Entries[1].G)
	assert.Equal(t, int64(0), decodedPayload.Summaries[0].Sketches[1].Entries[1].Delta)

	assert.Equal(t, float64(21), decodedPayload.Summaries[0].Sketches[1].Entries[2].V)
	assert.Equal(t, int64(2), decodedPayload.Summaries[0].Sketches[1].Entries[2].G)
	assert.Equal(t, int64(0), decodedPayload.Summaries[0].Sketches[1].Entries[2].Delta)
}

func TestMarshalJSONSketchSeries(t *testing.T) {
	series := createSketchSeries()

	payload, contentType, err := MarshalJSONSketchSeries(series)
	assert.Nil(t, err)
	assert.Equal(t, contentType, "application/json")
	assert.NotNil(t, payload)

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"max\":1,\"n\":1,\"sum\":1,\"avg\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"min\":10,\"max\":21,\"n\":6,\"sum\":96,\"avg\":16}}]}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}
