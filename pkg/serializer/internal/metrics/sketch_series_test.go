// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib && zstd

package metrics

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"

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

func TestSketchSeriesMarshalSplitCompressEmpty(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			sl := SketchSeriesList{SketchesSource: metrics.NewSketchesSourceTest()}

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, err := sl.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, compressor, logger)

			assert.Nil(t, err)

			firstPayload := payloads[0]
			assert.Equal(t, 0, firstPayload.GetPointCount())

			decompressed, _ := compressor.Decompress(firstPayload.GetContent())
			// 0b00010 010 - field 2 (metadata) type 2 (bytes), 0 length
			assert.Equal(t, []byte{0x12, 0x00}, decompressed)
		})
	}
}

func TestSketchSeriesMarshalSplitCompressItemTooBigIsDropped(t *testing.T) {
	tests := map[string]struct {
		kind                string
		maxUncompressedSize int
	}{
		"zlib": {kind: compression.ZlibKind, maxUncompressedSize: 100},
		"zstd": {kind: compression.ZstdKind, maxUncompressedSize: 200},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
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

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, compressor, logger)

			assert.Nil(t, err)

			firstPayload := payloads[0]
			require.Equal(t, 0, firstPayload.GetPointCount())

			decompressed, _ := compressor.Decompress(firstPayload.GetContent())

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
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			sl := metrics.NewSketchesSourceTest()

			for i := 0; i < 2; i++ {
				sl.Append(Makeseries(i))
			}

			sl.Reset()
			serializer2 := SketchSeriesList{SketchesSource: sl}

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, err := serializer2.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, compressor, logger)
			require.NoError(t, err)

			firstPayload := payloads[0]
			assert.Equal(t, 11, firstPayload.GetPointCount())

			decompressed, _ := compressor.Decompress(firstPayload.GetContent())

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
		"zlib": {kind: compression.ZlibKind, maxUncompressedSize: 2000},
		"zstd": {kind: compression.ZstdKind, maxUncompressedSize: 2000},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			mockConfig.SetWithoutSource("serializer_max_uncompressed_payload_size", tc.maxUncompressedSize)

			sl := metrics.NewSketchesSourceTest()

			expectedPointCount := 0
			for i := 0; i < 20; i++ {
				sl.Append(Makeseries(i))
				expectedPointCount += i + 5
			}

			serializer := SketchSeriesList{SketchesSource: sl}

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, compressor, logger)
			assert.Nil(t, err)

			recoveredSketches := []gogen.SketchPayload{}
			recoveredCount := 0
			pointCount := 0
			for _, pld := range payloads {
				decompressed, _ := compressor.Decompress(pld.GetContent())

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

func TestSketchSeriesMarshalSplitCompressMultiple(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			sl := metrics.NewSketchesSourceTest()

			for i := 0; i < 2; i++ {
				sl.Append(Makeseries(i))
			}

			sl.Reset()
			serializer2 := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, filteredPayloads, err := serializer2.MarshalSplitCompressMultiple(mockConfig, compressor, func(ss *metrics.SketchSeries) bool {
				return ss.Name == "name.0"
			}, logmock.New(t))
			require.NoError(t, err)

			assert.Equal(t, 1, len(payloads))
			assert.Equal(t, 1, len(filteredPayloads))

			firstPayload := payloads[0]
			assert.Equal(t, 11, firstPayload.GetPointCount())

			firstFilteredPayload := filteredPayloads[0]
			assert.Equal(t, 5, firstFilteredPayload.GetPointCount())
		})
	}
}
