// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package percentile

import (
	"testing"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

func createSketchSeries() []*SketchSeries {
	sketch1 := GKArray{
		Entries:  []Entry{},
		Incoming: []float64{1},
		Min:      1, Count: 1, Sum: 1, Avg: 1, Max: 1}
	sketch2 := GKArray{
		Entries:  []Entry{{V: 10, G: 1, Delta: 0}, {V: 14, G: 3, Delta: 0}, {V: 21, G: 2, Delta: 0}},
		Incoming: []float64{},
		Min:      10, Count: 6, Sum: 96, Avg: 16, Max: 21}
	contextKey, _ := ckey.Parse("ffffffffffffffffffffffffffffffff")
	series := []*SketchSeries{{
		ContextKey: contextKey,
		Sketches:   []Sketch{{Timestamp: int64(12345), Sketch: sketch1}, {Timestamp: int64(67890), Sketch: sketch2}},
		Name:       "test.metrics",
		Host:       "localhost",
		Tags:       []string{"tag1", "tag2:yes"},
		SketchType: SketchGK,
	}}

	return series
}

func createMultiSketchSeries() []*SketchSeries {
	sketch1 := GKArray{
		Entries:  []Entry{},
		Incoming: []float64{1},
		Min:      1, Count: 1, Sum: 1, Avg: 1, Max: 1}
	sketch2 := KLL{
		Compactors: [][]float64{{2, 9, 13}, {6, 7}},
		Min:        2, Count: 8, Sum: 56, Avg: 7, Max: 13,
		Length: 5, Capacity: 167, H: 2}
	sketch3 := CompleteDS{
		Values: []float64{2, 8},
		Min:    2, Count: 2, Sum: 10, Avg: 5, Max: 8,
	}
	contextKey1, _ := ckey.Parse("aaffffffffffffffffffffffffffffff")
	contextKey2, _ := ckey.Parse("bbffffffffffffffffffffffffffffff")
	contextKey3, _ := ckey.Parse("ccffffffffffffffffffffffffffffff")

	series := []*SketchSeries{{
		ContextKey: contextKey1,
		Sketches:   []Sketch{{Timestamp: int64(12345), Sketch: sketch1}},
		Name:       "test.metrics1",
		Host:       "localhost",
		Tags:       []string{"tag1", "tag2:yes"},
		SketchType: SketchGK,
	}, {
		ContextKey: contextKey2,
		Sketches:   []Sketch{{Timestamp: int64(12345), Sketch: sketch2}},
		Name:       "test.metrics2",
		Host:       "localhost",
		Tags:       []string{"tag2:yes"},
		SketchType: SketchKLL,
	}, {
		ContextKey: contextKey3,
		Sketches:   []Sketch{{Timestamp: int64(12345), Sketch: sketch3}},
		Name:       "test.metrics3",
		Host:       "localhost",
		Tags:       []string{"tag3"},
		SketchType: SketchComplete,
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
	assert.Equal(t, "localhost", decodedPayload.Sketches[0].Host)

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

func TestMarshalMultiSketch(t *testing.T) {
	assert := assert.New(t)

	series := createMultiSketchSeries()
	payload, err := SketchSeriesList(series).Marshal()
	assert.Nil(err)

	decodedPayload := &agentpayload.SketchPayload{}
	err = proto.Unmarshal(payload, decodedPayload)
	assert.Nil(err)

	require.Len(t, decodedPayload.Sketches, 3)
	assert.Equal(agentpayload.CommonMetadata{}, decodedPayload.Metadata)

	// first sketch
	assert.Equal("test.metrics1", decodedPayload.Sketches[0].Metric)
	assert.Equal("tag1", decodedPayload.Sketches[0].Tags[0])
	assert.Equal("tag2:yes", decodedPayload.Sketches[0].Tags[1])
	assert.Equal("localhost", decodedPayload.Sketches[0].Host)

	require.Len(t, decodedPayload.Sketches[0].Distributions, 1)
	assert.Equal(int64(12345), decodedPayload.Sketches[0].Distributions[0].Ts)
	assert.Equal(int64(1), decodedPayload.Sketches[0].Distributions[0].Cnt)
	assert.Equal(float64(1), decodedPayload.Sketches[0].Distributions[0].Min)
	assert.Equal(float64(1), decodedPayload.Sketches[0].Distributions[0].Max)
	assert.Equal(float64(1), decodedPayload.Sketches[0].Distributions[0].Avg)
	assert.Equal(float64(1), decodedPayload.Sketches[0].Distributions[0].Sum)

	require.Len(t, decodedPayload.Sketches[0].Distributions[0].V, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].G, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].Delta, 0)
	require.Len(t, decodedPayload.Sketches[0].Distributions[0].Buf, 1)
	assert.Equal(float64(1), decodedPayload.Sketches[0].Distributions[0].Buf[0])

	// second sketch
	assert.Equal("test.metrics2", decodedPayload.Sketches[1].Metric)
	assert.Equal("tag2:yes", decodedPayload.Sketches[1].Tags[0])
	assert.Equal("localhost", decodedPayload.Sketches[1].Host)

	require.Len(t, decodedPayload.Sketches[1].DistributionsK, 1)
	assert.Equal(int64(12345), decodedPayload.Sketches[1].DistributionsK[0].Ts)
	assert.Equal(int64(8), decodedPayload.Sketches[1].DistributionsK[0].Cnt)
	assert.Equal(float64(2), decodedPayload.Sketches[1].DistributionsK[0].Min)
	assert.Equal(float64(13), decodedPayload.Sketches[1].DistributionsK[0].Max)
	assert.Equal(float64(7), decodedPayload.Sketches[1].DistributionsK[0].Avg)
	assert.Equal(float64(56), decodedPayload.Sketches[1].DistributionsK[0].Sum)
	assert.Equal(uint32(5), decodedPayload.Sketches[1].DistributionsK[0].Length)
	assert.Equal(uint32(167), decodedPayload.Sketches[1].DistributionsK[0].Capacity)
	assert.Equal(uint32(2), decodedPayload.Sketches[1].DistributionsK[0].H)

	require.Len(t, decodedPayload.Sketches[1].DistributionsK[0].Compactors, 2)
	require.Len(t, decodedPayload.Sketches[1].DistributionsK[0].Compactors[0].V, 3)
	assert.Equal(float64(2), decodedPayload.Sketches[1].DistributionsK[0].Compactors[0].V[0])
	assert.Equal(float64(9), decodedPayload.Sketches[1].DistributionsK[0].Compactors[0].V[1])
	assert.Equal(float64(13), decodedPayload.Sketches[1].DistributionsK[0].Compactors[0].V[2])
	require.Len(t, decodedPayload.Sketches[1].DistributionsK[0].Compactors[1].V, 2)
	assert.Equal(float64(6), decodedPayload.Sketches[1].DistributionsK[0].Compactors[1].V[0])
	assert.Equal(float64(7), decodedPayload.Sketches[1].DistributionsK[0].Compactors[1].V[1])

	// third sketch
	assert.Equal("test.metrics3", decodedPayload.Sketches[2].Metric)
	assert.Equal("tag3", decodedPayload.Sketches[2].Tags[0])
	assert.Equal("localhost", decodedPayload.Sketches[2].Host)

	require.Len(t, decodedPayload.Sketches[2].DistributionsC, 1)
	assert.Equal(int64(12345), decodedPayload.Sketches[2].DistributionsC[0].Ts)
	assert.Equal(int64(2), decodedPayload.Sketches[2].DistributionsC[0].Cnt)
	assert.Equal(float64(2), decodedPayload.Sketches[2].DistributionsC[0].Min)
	assert.Equal(float64(8), decodedPayload.Sketches[2].DistributionsC[0].Max)
	assert.Equal(float64(5), decodedPayload.Sketches[2].DistributionsC[0].Avg)
	assert.Equal(float64(10), decodedPayload.Sketches[2].DistributionsC[0].Sum)

	require.Len(t, decodedPayload.Sketches[2].DistributionsC[0].Vals, 2)
	assert.Equal(float64(2), decodedPayload.Sketches[2].DistributionsC[0].Vals[0])
	assert.Equal(float64(8), decodedPayload.Sketches[2].DistributionsC[0].Vals[1])
}

func TestMarshalJSON(t *testing.T) {
	series := createSketchSeries()

	payload, err := SketchSeriesList(series).MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localhost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[],\"buf\":[1],\"min\":1,\"max\":1,\"cnt\":1,\"sum\":1,\"avg\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"buf\":[],\"min\":10,\"max\":21,\"cnt\":6,\"sum\":96,\"avg\":16}}],\"sketch_type\":0}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}
