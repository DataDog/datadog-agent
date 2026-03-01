// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && zstd && test

// Fuzz tests for v2 series payload serialization.
//
// What we're asserting is this: for any maxPayloadSize M and series data S, if
// writeSerie(s) succeeds for all s in S, then len(serialized(S)) <= M.
//
// In other words: if the serializer accepts data, the resulting payload must
// fit within the claimed size limit. This is the contract between the serializer
// and the intake endpoint.
//
// The fuzzer uses arbitrary byte data intentionally. Random data tests
// worst-case compression scenarios where CompressBound predictions are most
// likely to be violated.

package metrics

import (
	"encoding/binary"
	"testing"

	"github.com/richardartoul/molecule"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	noopimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
)

func createPayloadsBuilderWithLimits(compressor compression.Compressor, maxPayloadSize, maxUncompressedSize, maxPoints int) *PayloadsBuilder {
	bufferContext := marshaler.NewBufferContext()
	buf := bufferContext.PrecompressionBuf
	ps := molecule.NewProtoStream(buf)

	return &PayloadsBuilder{
		bufferContext:       bufferContext,
		strategy:            compressor,
		compressor:          nil,
		buf:                 buf,
		ps:                  ps,
		pointsThisPayload:   0,
		seriesThisPayload:   0,
		maxPayloadSize:      maxPayloadSize,
		maxUncompressedSize: maxUncompressedSize,
		maxPointsPerPayload: maxPoints,
		pipelineConfig: PipelineConfig{
			Filter: AllowAllFilter{},
			V3:     false,
		},
		pipelineContext: &PipelineContext{},
	}
}

// byteReader extracts bytes from a data buffer, tracking position.
// Returns nil and sets exhausted=true when buffer is exhausted.
type byteReader struct {
	data      []byte
	pos       int
	exhausted bool
}

func (r *byteReader) read(n int) []byte {
	if r.exhausted || r.pos >= len(r.data) {
		r.exhausted = true
		return nil
	}
	end := r.pos + n
	if end > len(r.data) {
		// Not enough data remaining - mark exhausted and return nil
		r.exhausted = true
		return nil
	}
	result := r.data[r.pos:end]
	r.pos = end
	return result
}

func (r *byteReader) readString(n int) string {
	b := r.read(n)
	if b == nil {
		return ""
	}
	return string(b)
}

func (r *byteReader) readUint16() uint16 {
	b := r.read(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *byteReader) readUint64() uint64 {
	b := r.read(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// seriesFromReader creates a Serie by reading from a byte stream.
// Each series consumes different bytes, creating variation.
func seriesFromReader(r *byteReader, index int) *metrics.Serie {
	if r.exhausted {
		return nil
	}

	// Caps based on regression detector experiments

	nameLen := int(r.readUint16() % 200)
	hostLen := int(r.readUint16() % 64)
	deviceLen := int(r.readUint16() % 32)
	sourceTypeLen := int(r.readUint16() % 32)
	tagCount := int(r.readUint16() % 50)
	pointCount := int(r.readUint16() % 20)
	resourceCount := int(r.readUint16() % 3)

	if r.exhausted {
		return nil
	}

	name := r.readString(nameLen)
	host := r.readString(hostLen)
	device := r.readString(deviceLen)
	sourceTypeName := r.readString(sourceTypeLen)

	var tags []string
	for i := 0; i < tagCount && !r.exhausted; i++ {
		tagLen := int(r.readUint16() % 150)
		tag := r.readString(tagLen)
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	var points []metrics.Point
	// Each series gets a unique time range to avoid timestamp collisions,
	// worse case behavior.
	baseTs := float64(1700000000 + int64(index)*1000)
	interval := int64((index % 60) + 1)
	for i := 0; i < pointCount && !r.exhausted; i++ {
		valueBits := r.readUint64()
		points = append(points, metrics.Point{
			Ts:    baseTs + float64(int64(i)*interval),
			Value: float64(valueBits),
		})
	}

	var resources []metrics.Resource
	for i := 0; i < resourceCount && !r.exhausted; i++ {
		resNameLen := int(r.readUint16() % 64)
		resTypeLen := int(r.readUint16() % 32)
		resName := r.readString(resNameLen)
		resType := r.readString(resTypeLen)
		if resName != "" || resType != "" {
			resources = append(resources, metrics.Resource{Name: resName, Type: resType})
		}
	}

	mtype := metrics.APIMetricType(index % 3)

	return &metrics.Serie{
		Name:           name,
		Host:           host,
		Device:         device,
		SourceTypeName: sourceTypeName,
		Tags:           tagset.CompositeTagsFromSlice(tags),
		Points:         points,
		Resources:      resources,
		MType:          mtype,
		Interval:       int64((index % 60) + 1),
		Source:         metrics.MetricSource(index % 3),
	}
}

func testSeriesV2PayloadInvariant(
	t *testing.T,
	compressor compression.Compressor,
	maxPayloadSize, maxUncompressedSize uint32,
	maxPoints uint16,
	numSeries uint8,
	data []byte,
) {
	const (
		productionMaxPayloadSize      = 512000  // 512KB compressed
		productionMaxUncompressedSize = 5242880 // 5MB uncompressed
		productionMaxPoints           = 10000
	)

	effectiveMaxPayload := int(maxPayloadSize)
	if effectiveMaxPayload > productionMaxPayloadSize {
		effectiveMaxPayload = productionMaxPayloadSize
	}

	effectiveMaxUncompressed := int(maxUncompressedSize)
	if effectiveMaxUncompressed > productionMaxUncompressedSize {
		effectiveMaxUncompressed = productionMaxUncompressedSize
	}

	effectiveMaxPoints := int(maxPoints)
	if effectiveMaxPoints > productionMaxPoints {
		effectiveMaxPoints = productionMaxPoints
	}

	effectiveNumSeries := int(numSeries)

	pb := createPayloadsBuilderWithLimits(compressor, effectiveMaxPayload, effectiveMaxUncompressed, effectiveMaxPoints)
	if err := pb.startPayload(); err != nil {
		// startPayload failing means limits too small for headers. It's
		// a valid rejection.
		return
	}

	reader := &byteReader{data: data}

	for i := 0; i < effectiveNumSeries; i++ {
		series := seriesFromReader(reader, i)
		if series == nil {
			break
		}
		_ = pb.writeSerie(series)
	}

	_ = pb.finishPayload()

	for i, payload := range pb.pipelineContext.payloads {
		content := payload.GetContent()
		if len(content) > effectiveMaxPayload {
			t.Errorf("INVARIANT VIOLATION: payload %d size %d exceeds limit %d (compressor: %s)",
				i, len(content), effectiveMaxPayload, compressor.ContentEncoding())
		}
	}

}

func addSeeds(f *testing.F, withLevel bool) {
	highEntropy := make([]byte, 2048)
	for i := range highEntropy {
		highEntropy[i] = byte(i * 17)
	}

	repetitive := make([]byte, 2048)
	for i := range repetitive {
		repetitive[i] = byte('A' + (i % 3)) // AAA BBB CCC pattern
	}

	zeros := make([]byte, 1024)

	mixed := make([]byte, 2048)
	for i := range mixed {
		if i%100 < 50 {
			mixed[i] = byte(i%26) + 'a' // lowercase letters
		} else {
			mixed[i] = byte(i * 7) // pseudo-random
		}
	}

	if withLevel {
		// (maxPayload, maxUncompressed, maxPoints, numSeries, level, data)
		// Seeds use moderate limits - fuzzer will explore larger values
		f.Add(uint32(8192), uint32(32768), uint16(500), uint8(10), int8(1), highEntropy)
		f.Add(uint32(2048), uint32(8192), uint16(100), uint8(8), int8(3), repetitive)
		f.Add(uint32(512), uint32(2048), uint16(50), uint8(5), int8(1), zeros)
		f.Add(uint32(256), uint32(1024), uint16(20), uint8(3), int8(5), mixed)
	} else {
		f.Add(uint32(8192), uint32(32768), uint16(500), uint8(10), highEntropy)
		f.Add(uint32(2048), uint32(8192), uint16(100), uint8(8), repetitive)
		f.Add(uint32(512), uint32(2048), uint16(50), uint8(5), zeros)
		f.Add(uint32(256), uint32(1024), uint16(20), uint8(3), mixed)
	}
}

func FuzzSeriesV2PayloadOverflowZstd(f *testing.F) {
	addSeeds(f, true)
	f.Fuzz(func(t *testing.T, maxPayloadSize, maxUncompressedSize uint32, maxPoints uint16, numSeries uint8, level int8, data []byte) {
		// Production default is level 1. Cap at 5 to avoid slow fuzzer runs.
		effectiveLevel := int(level)
		if effectiveLevel > 5 {
			effectiveLevel = 5
		}
		testSeriesV2PayloadInvariant(t,
			zstdimpl.New(zstdimpl.Requires{Level: compression.ZstdCompressionLevel(effectiveLevel)}),
			maxPayloadSize, maxUncompressedSize, maxPoints, numSeries, data)
	})
}

func FuzzSeriesV2PayloadOverflowGzip(f *testing.F) {
	addSeeds(f, true)
	f.Fuzz(func(t *testing.T, maxPayloadSize, maxUncompressedSize uint32, maxPoints uint16, numSeries uint8, level int8, data []byte) {
		effectiveLevel := int(level)
		if effectiveLevel < 1 {
			effectiveLevel = 1
		}
		if effectiveLevel > 9 {
			effectiveLevel = 9
		}
		testSeriesV2PayloadInvariant(t,
			gzipimpl.New(gzipimpl.Requires{Level: effectiveLevel}),
			maxPayloadSize, maxUncompressedSize, maxPoints, numSeries, data)
	})
}

func FuzzSeriesV2PayloadOverflowZlib(f *testing.F) {
	addSeeds(f, false)
	f.Fuzz(func(t *testing.T, maxPayloadSize, maxUncompressedSize uint32, maxPoints uint16, numSeries uint8, data []byte) {
		testSeriesV2PayloadInvariant(t, zlibimpl.New(),
			maxPayloadSize, maxUncompressedSize, maxPoints, numSeries, data)
	})
}

func FuzzSeriesV2PayloadOverflowNone(f *testing.F) {
	addSeeds(f, false)
	f.Fuzz(func(t *testing.T, maxPayloadSize, maxUncompressedSize uint32, maxPoints uint16, numSeries uint8, data []byte) {
		testSeriesV2PayloadInvariant(t, noopimpl.New(),
			maxPayloadSize, maxUncompressedSize, maxPoints, numSeries, data)
	})
}
