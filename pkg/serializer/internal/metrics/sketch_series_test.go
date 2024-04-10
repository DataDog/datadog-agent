// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib && zstd

package metrics

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			sl := SketchSeriesList{SketchesSource: metrics.NewSketchesSourceTest()}
			payload, _ := sl.Marshal()
			strategy := compressionimpl.NewCompressor(mockConfig)
			payloads, err := sl.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, strategy)

			assert.Nil(t, err)

			firstPayload := payloads[0]
			assert.Equal(t, 0, firstPayload.GetPointCount())

			decompressed, _ := strategy.Decompress(firstPayload.GetContent())
			// Check that we encoded the protobuf correctly
			assert.Equal(t, decompressed, payload)
		})
	}
}

func TestSketchSeriesMarshalSplitCompressItemTooBigIsDropped(t *testing.T) {
	tests := map[string]struct {
		kind                string
		maxUncompressedSize int
	}{
		"zlib": {kind: compressionimpl.ZlibKind, maxUncompressedSize: 100},
		"zstd": {kind: compressionimpl.ZstdKind, maxUncompressedSize: 200},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			mockConfig.SetWithoutSource("serializer_max_uncompressed_payload_size", tc.maxUncompressedSize)

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
			strategy := compressionimpl.NewCompressor(mockConfig)
			payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, strategy)

			assert.Nil(t, err)

			firstPayload := payloads[0]
			require.Equal(t, 0, firstPayload.GetPointCount())

			decompressed, _ := strategy.Decompress(firstPayload.GetContent())

			pl := new(gogen.SketchPayload)
			if err := pl.Unmarshal(decompressed); err != nil {
				t.Fatal(err)
			}

			// Should only have 1 sketch because the the larger one was dropped.
			require.Len(t, pl.Sketches, 1)
		})
	}

}

func TestSketchSeriesMarshalSplitCompress(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			sl := metrics.NewSketchesSourceTest()

			for i := 0; i < 2; i++ {
				sl.Append(Makeseries(i))
			}

			sl.Reset()
			serializer2 := SketchSeriesList{SketchesSource: sl}
			strategy := compressionimpl.NewCompressor(mockConfig)
			payloads, err := serializer2.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, strategy)
			require.NoError(t, err)

			firstPayload := payloads[0]
			assert.Equal(t, 11, firstPayload.GetPointCount())

			decompressed, _ := strategy.Decompress(firstPayload.GetContent())

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
		})
	}

}

func TestSketchSeriesMarshalSplitCompressSplit(t *testing.T) {

	tests := map[string]struct {
		kind                string
		maxUncompressedSize int
	}{
		"zlib": {kind: compressionimpl.ZlibKind, maxUncompressedSize: 2000},
		"zstd": {kind: compressionimpl.ZstdKind, maxUncompressedSize: 2000},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			mockConfig.SetWithoutSource("serializer_max_uncompressed_payload_size", tc.maxUncompressedSize)

			sl := metrics.NewSketchesSourceTest()

			expectedPointCount := 0
			for i := 0; i < 20; i++ {
				sl.Append(Makeseries(i))
				expectedPointCount += i + 5
			}

			serializer := SketchSeriesList{SketchesSource: sl}
			strategy := compressionimpl.NewCompressor(mockConfig)
			payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, strategy)
			assert.Nil(t, err)

			recoveredSketches := []gogen.SketchPayload{}
			recoveredCount := 0
			pointCount := 0
			for _, pld := range payloads {
				decompressed, _ := strategy.Decompress(pld.GetContent())

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
		})
	}
}
