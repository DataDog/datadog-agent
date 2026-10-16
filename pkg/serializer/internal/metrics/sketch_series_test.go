// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib

package metrics

import (
	"fmt"
	"math"
	"math/bits"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, in metrics.SketchPoint, pb gogen.SketchPayload_Sketch_Dogsketch) {
	t.Helper()
	s := in.Sketch
	bCnt, bMin, bMax, bSum, bAvg := in.Sketch.BasicStats()
	require.Equal(t, in.Ts, pb.Ts)

	// sketch
	k, n := s.Cols()
	require.Equal(t, k, pb.K)
	require.Equal(t, n, pb.N)

	// summary
	require.Equal(t, bCnt, pb.Cnt)
	require.Equal(t, bMin, pb.Min)
	require.Equal(t, bMax, pb.Max)
	require.Equal(t, bAvg, pb.Avg)
	require.Equal(t, bSum, pb.Sum)
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
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			sl := SketchSeriesList{SketchesSource: metrics.NewSketchesSourceTest()}

			pipelines := testPipelines()
			compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp
			err := sl.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
			assert.Nil(t, err)
			payloads := pipelines.GetPayloads()

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
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			mockConfig.SetInTest("serializer_max_uncompressed_payload_size", tc.maxUncompressedSize)

			sl := metrics.NewSketchesSourceTest()
			// A big item (to be dropped)
			sl.Append(Makeseries(0))

			// A small item (no dropped)
			sl.Append(&metrics.SketchSeries{
				DistributionMetadata: metrics.DistributionMetadata{
					Name:     "small",
					Tags:     tagset.CompositeTagsFromSlice([]string{}),
					Host:     "",
					Interval: 0,
				},
			})

			pipelines := testPipelines()
			serializer := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp
			err := serializer.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
			payloads := pipelines.GetPayloads()

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

// TestReproQuantileSerializer records the default serializer's silent-drop boundary
// for a single histogram bucket. It also accepts a bounded-count representation,
// so the same two inputs can be compared before and after the quantile fix.
func TestReproQuantileSerializer(t *testing.T) {
	if os.Getenv("DD_QUANTILE_REPRO") != "1" {
		t.Skip("set DD_QUANTILE_REPRO=1 to run the quantile serializer repro")
	}
	if strconv.IntSize < 64 {
		t.Skip("the default drop boundary requires a 64-bit count")
	}

	cfg := mock.New(t)
	logger := logmock.New(t)
	compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: cfg}).Comp
	maxPayload := cfg.GetInt("serializer_max_payload_size")
	maxUncompressed := cfg.GetInt("serializer_max_uncompressed_payload_size")
	const metricName = "quantile.repro.serializer"
	const timestamp int64 = 10
	const legacyBinCount = uint64(math.MaxUint16)

	varintSize := func(n uint64) int { return (bits.Len64(n|1) + 6) / 7 }
	// All fields involved have one-byte tags. Packed columns and nested
	// messages each add a tag and a varint length to their contents.
	fieldSize := func(n int) int { return 1 + varintSize(uint64(n)) + n }
	metadata := gogen.SketchPayload_Sketch{
		Metric: metricName,
		Metadata: &gogen.Metadata{Origin: &gogen.Origin{
			OriginProduct: uint32(metricSourceToOriginProduct(metrics.MetricSourceUnknown)),
		}},
	}
	itemSize := func(count uint64) int {
		full, remainder := count/legacyBinCount, count%legacyBinCount
		keyBytes, countBytes := int(full), 3*int(full)
		if remainder != 0 {
			keyBytes++ // Every key is zero, encoded as one zigzag-varint byte.
			countBytes += varintSize(remainder)
		}
		// InsertInterpolate(0, 0, count) has zero min/max/sum/avg, which
		// are omitted. Size computes the timestamp and total-count fields.
		point := gogen.SketchPayload_Sketch_Dogsketch{Ts: timestamp, Cnt: int64(count)}
		pointBytes := point.Size() + fieldSize(keyBytes) + fieldSize(countBytes)
		return fieldSize(metadata.Size() + fieldSize(pointBytes))
	}
	const footerBytes = 2 // Empty SketchPayload.Metadata, written by startPayload.
	wouldDrop := func(count uint64) bool {
		n := itemSize(count)
		bound := compressor.CompressBound(n)
		// Mirror stream.Compressor.checkItemSize, including its strict limits.
		return n >= maxPayload-footerBytes ||
			bound >= maxPayload-footerBytes ||
			bound >= maxUncompressed-compressor.CompressBound(footerBytes)
	}

	// Search sizes only: no sketches, columns, or large protobuf buffers are
	// allocated during the search. DD_QUANTILE_COUNT optionally sets its upper
	// count bound; it must be large enough to include a dropped legacy item.
	high := uint64(maxPayload) * legacyBinCount
	if raw := os.Getenv("DD_QUANTILE_COUNT"); raw != "" {
		var err error
		high, err = strconv.ParseUint(raw, 10, 53)
		require.NoError(t, err, "DD_QUANTILE_COUNT must be a positive count below 2^53")
	}
	require.Positive(t, high)
	require.True(t, wouldDrop(high), "DD_QUANTILE_COUNT must include the legacy drop boundary")
	low := uint64(1)
	for low < high {
		mid := low + (high-low)/2
		if wouldDrop(mid) {
			high = mid
		} else {
			low = mid + 1
		}
	}
	boundary := low
	require.Greater(t, boundary, uint64(1))
	require.False(t, wouldDrop(boundary-1))
	require.True(t, wouldDrop(boundary))
	t.Logf("compressor=%s max_payload=%d max_uncompressed=%d first_legacy_drop_count=%d",
		cfg.GetString("serializer_compressor_kind"), maxPayload, maxUncompressed, boundary)

	// Derive bin width from MemSize; do not assume that the fixed layout still
	// uses uint16 counts, and do not allocate Cols outside serialization.
	empty := &quantile.Sketch{}
	emptyUsed, _ := empty.MemSize()
	small := &quantile.Sketch{}
	small.Insert(quantile.Default(), 0)
	smallUsed, _ := small.MemSize()
	binBytes := smallUsed - emptyUsed
	require.Positive(t, binBytes)

	for _, count := range []uint64{boundary - 1, boundary} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			var agent quantile.Agent
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			insertErr := agent.InsertInterpolate(0, 0, uint(count))
			runtime.ReadMemStats(&after)
			insertAlloc := after.TotalAlloc - before.TotalAlloc
			require.NoError(t, insertErr)
			// InsertInterpolate flushes CountBuf itself. Finish would add a
			// deep copy that is not part of the insertion cost being measured.
			sketch := &agent.Sketch
			require.Equal(t, int64(count), sketch.Basic.Cnt)
			used, allocated := sketch.MemSize()
			bins := (used - emptyUsed) / binBytes
			legacyBins := int((count + legacyBinCount - 1) / legacyBinCount)
			bounded := bins < legacyBins
			if bounded {
				require.LessOrEqual(t, bins, 4096, "the fixed representation must bound its bins")
			} else {
				require.Equal(t, legacyBins, bins)
			}
			require.Positive(t, bins)

			source := metrics.NewSketchesSourceTest()
			source.Append(&metrics.SketchSeries{
				DistributionMetadata: metrics.DistributionMetadata{Name: metricName},
				Points:               []metrics.SketchPoint{{Ts: timestamp, Sketch: sketch}},
			})
			serializer := SketchSeriesList{SketchesSource: source}
			pipelines := testPipelines()
			dropsBefore := expvarsItemTooBig.Value()
			runtime.GC()
			runtime.ReadMemStats(&before)
			err := serializer.MarshalSplitCompressPipelines(cfg, compressor, pipelines, logger)
			runtime.ReadMemStats(&after)
			serializeAlloc := after.TotalAlloc - before.TotalAlloc
			dropDelta := expvarsItemTooBig.Value() - dropsBefore
			require.NoError(t, err, "an oversized sketch is silently dropped, not returned as an error")
			t.Logf("count=%d bins=%d bin_bytes=%d MemSize_used_bytes=%d MemSize_allocated_bytes=%d bounded=%t legacy_item_bytes=%d legacy_compress_bound=%d TotalAlloc_insert_bytes=%d TotalAlloc_serialize_bytes=%d ItemTooBig_delta=%d",
				count, bins, binBytes, used, allocated, bounded, itemSize(count), compressor.CompressBound(itemSize(count)), insertAlloc, serializeAlloc, dropDelta)

			payloads := pipelines.GetPayloads()
			require.Len(t, payloads, 1)
			decompressed, err := compressor.Decompress(payloads[0].GetContent())
			require.NoError(t, err)
			var decoded gogen.SketchPayload
			require.NoError(t, decoded.Unmarshal(decompressed))
			if !bounded && wouldDrop(count) {
				require.Equal(t, int64(1), dropDelta)
				require.Zero(t, payloads[0].GetPointCount())
				require.Empty(t, decoded.Sketches, "the dropped metric must be absent from the decoded payload")
				return
			}
			require.Zero(t, dropDelta)
			require.Equal(t, 1, payloads[0].GetPointCount())
			require.Len(t, decoded.Sketches, 1)
			recovered := decoded.Sketches[0]
			require.Equal(t, metricName, recovered.Metric)
			require.Len(t, recovered.Dogsketches, 1)
			point := recovered.Dogsketches[0]
			require.Equal(t, timestamp, point.Ts)
			require.Equal(t, int64(count), point.Cnt)
			require.Len(t, point.K, len(point.N))
			var recoveredCount uint64
			allKeysZero := true
			for i, n := range point.N {
				allKeysZero = allKeysZero && point.K[i] == 0
				recoveredCount += uint64(n)
			}
			require.True(t, allKeysZero, "all counts must remain in the zero key")
			require.Equal(t, count, recoveredCount)
			if !bounded {
				// Validate the analytical model against the actual protobuf
				// serializer on the accepted predecessor.
				require.Equal(t, itemSize(count), len(decompressed)-footerBytes)
				require.Len(t, point.K, legacyBins)
			}
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
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			sl := metrics.NewSketchesSourceTest()

			for i := 0; i < 2; i++ {
				sl.Append(Makeseries(i))
			}
			sl.Reset()

			pipelines := testPipelines()
			serializer2 := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp
			err := serializer2.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
			require.NoError(t, err)
			payloads := pipelines.GetPayloads()

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
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			mockConfig.SetInTest("serializer_max_uncompressed_payload_size", tc.maxUncompressedSize)

			sl := metrics.NewSketchesSourceTest()

			expectedPointCount := 0
			for i := 0; i < 20; i++ {
				sl.Append(Makeseries(i))
				expectedPointCount += i + 5
			}

			pipelines := testPipelines()
			serializer := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp
			err := serializer.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
			assert.Nil(t, err)
			payloads := pipelines.GetPayloads()

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
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			sl := metrics.NewSketchesSourceTest()

			for i := 0; i < 2; i++ {
				sl.Append(Makeseries(i))
			}

			sl.Reset()
			serializer2 := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp

			primaryConf := PipelineConfig{
				Filter: AllowAllFilter{},
			}
			secondaryConf := PipelineConfig{
				Filter: NewMapFilter(map[string]struct{}{"name.0": {}}),
			}
			pipelines := PipelineSet{
				primaryConf:   {},
				secondaryConf: {},
			}

			err := serializer2.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logmock.New(t))
			require.NoError(t, err)

			payloads := pipelines[primaryConf].payloads
			filteredPayloads := pipelines[secondaryConf].payloads

			assert.Equal(t, 1, len(payloads))
			assert.Equal(t, 1, len(filteredPayloads))

			firstPayload := payloads[0]
			assert.Equal(t, 11, firstPayload.GetPointCount())

			firstFilteredPayload := filteredPayloads[0]
			assert.Equal(t, 5, firstFilteredPayload.GetPointCount())
		})
	}
}

func BenchmarkSketchSerialization(b *testing.B) {
	src := metrics.NewSketchesSourceTest()
	for i := 0; i < 10000; i++ {
		src.Append(Makeseries(0))
	}
	serializer := SketchSeriesList{SketchesSource: src}

	mockConfig := mock.New(b)
	compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp
	logger := logmock.New(b)

	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		pipelines := testPipelines()
		err := serializer.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
		require.NoError(b, err)
		// Reset the source iterator so the next iteration serializes the same data.
		src.Reset()
	}
}
