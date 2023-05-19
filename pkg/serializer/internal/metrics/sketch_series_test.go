// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, in metrics.SketchPoint, pb gogen.SketchPayload_Sketch_Dogsketch) {
	t.Helper()
	s, b := in.Sketch, in.Sketch.Basic
	require.Equal(t, in.Ts, pb.Ts)

	// sketch
	k, n := s.Cols()
	require.Equal(t, k, pb.K)
	require.Equal(t, n, pb.N)

	// summary
	require.Equal(t, b.Cnt, pb.Cnt)
	require.Equal(t, b.Min, pb.Min)
	require.Equal(t, b.Max, pb.Max)
	require.Equal(t, b.Avg, pb.Avg)
	require.Equal(t, b.Sum, pb.Sum)
}

func TestSketchSeriesListMarshal(t *testing.T) {
	sl := metrics.NewSketchesSourceTest()

	for i := 0; i < 2; i++ {
		sl.Append(Makeseries(i))
	}

	serializer := SketchSeriesList{SketchesSource: sl}
	b, err := serializer.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	pl := new(gogen.SketchPayload)
	if err := pl.Unmarshal(b); err != nil {
		t.Fatal(err)
	}

	require.Len(t, pl.Sketches, int(sl.Count()))

	for i, pb := range pl.Sketches {
		in := sl.Get(i)
		require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

		assert.Equal(t, in.Host, pb.Host)
		assert.Equal(t, in.Name, pb.Metric)
		metrics.AssertCompositeTagsEqual(t, in.Tags, tagset.CompositeTagsFromSlice(pb.Tags))
		assert.Len(t, pb.Distributions, 0)

		require.Len(t, pb.Dogsketches, len(in.Points))
		for j, pointPb := range pb.Dogsketches {

			check(t, in.Points[j], pointPb)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)

			// fmt.Printf("%#v %#v\n", pin, s)
		}
	}
}

func TestSketchSeriesSplitEmptyPayload(t *testing.T) {
	sl := SketchSeriesList{SketchesSource: metrics.NewSketchesSourceTest()}
	pieces, err := sl.SplitPayload(10)
	require.Len(t, pieces, 0)
	require.Nil(t, err)
}

func TestSketchSeriesMarshalSplitCompressEmpty(t *testing.T) {

	sl := SketchSeriesList{SketchesSource: metrics.NewSketchesSourceTest()}
	payload, _ := sl.Marshal()
	payloads, err := sl.MarshalSplitCompress(marshaler.NewBufferContext())

	assert.Nil(t, err)

	firstPayload := payloads[0]
	assert.Equal(t, 0, firstPayload.GetPointCount())
	reader := bytes.NewReader(firstPayload.GetContent())
	r, _ := zlib.NewReader(reader)
	decompressed, _ := io.ReadAll(r)
	r.Close()

	// Check that we encoded the protobuf correctly
	assert.Equal(t, decompressed, payload)
}

func TestSketchSeriesMarshalSplitCompressItemTooBigIsDropped(t *testing.T) {

	oldSetting := config.Datadog.Get("serializer_max_uncompressed_payload_size")
	defer config.Datadog.Set("serializer_max_uncompressed_payload_size", oldSetting)
	config.Datadog.Set("serializer_max_uncompressed_payload_size", 100)

	sl := metrics.NewSketchesSourceTest()
	// A big item (to be dropped)
	sl.Append(Makeseries(0))

	// A small item (no dropped)
	sl.Append(&metrics.SketchSeries{
		Name:     "small",
		Tags:     tagset.CompositeTagsFromSlice([]string{}),
		Host:     "",
		Interval: 0,
	})

	serializer := SketchSeriesList{SketchesSource: sl}
	payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext())

	assert.Nil(t, err)

	firstPayload := payloads[0]
	require.Equal(t, 0, firstPayload.GetPointCount())
	reader := bytes.NewReader(firstPayload.GetContent())
	r, _ := zlib.NewReader(reader)
	decompressed, _ := io.ReadAll(r)
	r.Close()

	pl := new(gogen.SketchPayload)
	if err := pl.Unmarshal(decompressed); err != nil {
		t.Fatal(err)
	}

	// Should only have 1 sketch because the the larger one was dropped.
	require.Len(t, pl.Sketches, 1)
}

func TestSketchSeriesMarshalSplitCompress(t *testing.T) {
	sl := metrics.NewSketchesSourceTest()

	for i := 0; i < 2; i++ {
		sl.Append(Makeseries(i))
	}

	serializer1 := SketchSeriesList{SketchesSource: sl}
	payload, _ := serializer1.Marshal()
	sl.Reset()
	serializer2 := SketchSeriesList{SketchesSource: sl}
	payloads, err := serializer2.MarshalSplitCompress(marshaler.NewBufferContext())
	require.NoError(t, err)

	firstPayload := payloads[0]
	assert.Equal(t, 11, firstPayload.GetPointCount())
	reader := bytes.NewReader(firstPayload.GetContent())
	r, _ := zlib.NewReader(reader)
	decompressed, _ := io.ReadAll(r)
	r.Close()

	// Check that we encoded the protobuf correctly
	assert.Equal(t, decompressed, payload)

	pl := new(gogen.SketchPayload)
	err = pl.Unmarshal(decompressed)
	require.NoError(t, err)

	require.Len(t, pl.Sketches, int(sl.Count()))

	for i, pb := range pl.Sketches {
		in := sl.Get(i)
		require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

		assert.Equal(t, in.Host, pb.Host)
		assert.Equal(t, in.Name, pb.Metric)
		metrics.AssertCompositeTagsEqual(t, in.Tags, tagset.CompositeTagsFromSlice(pb.Tags))
		assert.Len(t, pb.Distributions, 0)

		require.Len(t, pb.Dogsketches, len(in.Points))
		for j, pointPb := range pb.Dogsketches {

			check(t, in.Points[j], pointPb)
		}
	}
}

func TestSketchSeriesMarshalSplitCompressSplit(t *testing.T) {
	oldSetting := config.Datadog.Get("serializer_max_uncompressed_payload_size")
	defer config.Datadog.Set("serializer_max_uncompressed_payload_size", oldSetting)
	config.Datadog.Set("serializer_max_uncompressed_payload_size", 2000)

	sl := metrics.NewSketchesSourceTest()

	expectedPointCount := 0
	for i := 0; i < 20; i++ {
		sl.Append(Makeseries(i))
		expectedPointCount += i + 5
	}

	serializer := SketchSeriesList{SketchesSource: sl}
	payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext())
	assert.Nil(t, err)

	recoveredSketches := []gogen.SketchPayload{}
	recoveredCount := 0
	pointCount := 0
	for _, pld := range payloads {
		reader := bytes.NewReader(pld.GetContent())
		r, _ := zlib.NewReader(reader)
		decompressed, _ := io.ReadAll(r)
		r.Close()

		pl := new(gogen.SketchPayload)
		if err := pl.Unmarshal(decompressed); err != nil {
			t.Fatal(err)
		}
		recoveredSketches = append(recoveredSketches, *pl)
		recoveredCount += len(pl.Sketches)
		pointCount += pld.GetPointCount()
	}
	assert.Equal(t, expectedPointCount, pointCount)
	assert.Equal(t, recoveredCount, int(sl.Count()))
	assert.Greater(t, len(recoveredSketches), 1)

	i := 0
	for _, pl := range recoveredSketches {
		for _, pb := range pl.Sketches {
			in := sl.Get(i)
			require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

			assert.Equal(t, in.Host, pb.Host)
			assert.Equal(t, in.Name, pb.Metric)
			metrics.AssertCompositeTagsEqual(t, in.Tags, tagset.CompositeTagsFromSlice(pb.Tags))
			assert.Len(t, pb.Distributions, 0)

			require.Len(t, pb.Dogsketches, len(in.Points))
			for j, pointPb := range pb.Dogsketches {

				check(t, in.Points[j], pointPb)
			}
			i++
		}
	}
}
