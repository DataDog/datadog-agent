// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package percentile

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func TestUnmarshalJSONSketchSeries(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"buf\":[],\"min\":1,\"max\":1,\"cnt\":1,\"sum\":1,\"avg\":1}}]}]}\n")

	sketch := QSketch{
		GKArray{Entries: []Entry{{1, 1, 0}}, Incoming: []float64{},
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

func TestUnmarshalJSONSketchSeriesWithBuf(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[],\"buf\":[1],\"min\":1,\"max\":1,\"cnt\":1,\"sum\":1,\"avg\":1}}]}]}\n")

	sketch := QSketch{
		GKArray{Entries: []Entry{}, Incoming: []float64{1},
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

func createSketchSeries() []*SketchSeries {
	sketch1 := QSketch{
		GKArray: GKArray{
			Entries:  []Entry{},
			Incoming: []float64{1},
			Min:      1, Count: 1, Sum: 1, Avg: 1, Max: 1}}
	sketch2 := QSketch{
		GKArray: GKArray{
			Entries:  []Entry{{V: 10, G: 1, Delta: 0}, {V: 14, G: 3, Delta: 0}, {V: 21, G: 2, Delta: 0}},
			Incoming: []float64{},
			Min:      10, Count: 6, Sum: 96, Avg: 16, Max: 21}}
	series := []*SketchSeries{{
		ContextKey: "test_context",
		Sketches:   []Sketch{{Timestamp: int64(12345), Sketch: sketch1}, {Timestamp: int64(67890), Sketch: sketch2}},
		Name:       "test.metrics",
		Host:       "localHost",
		Tags:       []string{"tag1", "tag2:yes"},
	}}

	return series
}

func TestMarshal(t *testing.T) {
	series := createSketchSeries()
	payload, err := SketchSeriesList(series).Marshal()
	assert.Nil(t, err)

	decodedPayload := &agentpayload.SketchPayload{}
	err = proto.Unmarshal(payload, decodedPayload)
	assert.Nil(t, err)

	require.Len(t, decodedPayload.Sketches, 1)
	assert.Equal(t, agentpayload.CommonMetadata{}, decodedPayload.Metadata)
	assert.Equal(t, "test.metrics", decodedPayload.Sketches[0].Metric)
	assert.Equal(t, "tag1", decodedPayload.Sketches[0].Tags[0])
	assert.Equal(t, "tag2:yes", decodedPayload.Sketches[0].Tags[1])
	assert.Equal(t, "localHost", decodedPayload.Sketches[0].Host)

	require.Len(t, decodedPayload.Sketches[0].Distributions, 2)

	// first sketch
	assert.Equal(t, int64(12345), decodedPayload.Sketches[0].Distributions[0].Ts)
	assert.Equal(t, int64(1), decodedPayload.Sketches[0].Distributions[0].Cnt)
	assert.Equal(t, float64(1), decodedPayload.Sketches[0].Distributions[0].Min)
	assert.Equal(t, float64(1), decodedPayload.Sketches[0].Distributions[0].Max)
	assert.Equal(t, float64(1), decodedPayload.Sketches[0].Distributions[0].Avg)
	assert.Equal(t, float64(1), decodedPayload.Sketches[0].Distributions[0].Sum)

	require.Len(t, decodedPayload.Sketches[0].Distributions[0].V, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].G, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].Delta, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].Buf, 1)
	assert.Equal(t, float64(1), decodedPayload.Sketches[0].Distributions[0].Buf[0])

	// second sketch
	assert.Equal(t, int64(67890), decodedPayload.Sketches[0].Distributions[1].Ts)
	assert.Equal(t, int64(6), decodedPayload.Sketches[0].Distributions[1].Cnt)
	assert.Equal(t, float64(10), decodedPayload.Sketches[0].Distributions[1].Min)
	assert.Equal(t, float64(21), decodedPayload.Sketches[0].Distributions[1].Max)
	assert.Equal(t, float64(16), decodedPayload.Sketches[0].Distributions[1].Avg)
	assert.Equal(t, float64(96), decodedPayload.Sketches[0].Distributions[1].Sum)

	require.Len(t, decodedPayload.Sketches[0].Distributions[1].V, 3)
	require.Len(t, decodedPayload.Sketches[0].Distributions[1].G, 3)
	require.Len(t, decodedPayload.Sketches[0].Distributions[1].Delta, 3)
	assert.Equal(t, float64(10), decodedPayload.Sketches[0].Distributions[1].V[0])
	assert.Equal(t, uint32(1), decodedPayload.Sketches[0].Distributions[1].G[0])
	assert.Equal(t, uint32(0), decodedPayload.Sketches[0].Distributions[1].Delta[0])
	assert.Equal(t, float64(14), decodedPayload.Sketches[0].Distributions[1].V[1])
	assert.Equal(t, uint32(3), decodedPayload.Sketches[0].Distributions[1].G[1])
	assert.Equal(t, uint32(0), decodedPayload.Sketches[0].Distributions[1].Delta[1])
	assert.Equal(t, float64(21), decodedPayload.Sketches[0].Distributions[1].V[2])
	assert.Equal(t, uint32(2), decodedPayload.Sketches[0].Distributions[1].G[2])
	assert.Equal(t, uint32(0), decodedPayload.Sketches[0].Distributions[1].Delta[2])
	require.Len(t, decodedPayload.Sketches[0].Distributions[1].Buf, 0)
}

func TestMarshalJSON(t *testing.T) {
	series := createSketchSeries()

	payload, err := SketchSeriesList(series).MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[],\"buf\":[1],\"min\":1,\"max\":1,\"cnt\":1,\"sum\":1,\"avg\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"buf\":[],\"min\":10,\"max\":21,\"cnt\":6,\"sum\":96,\"avg\":16}}]}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}
