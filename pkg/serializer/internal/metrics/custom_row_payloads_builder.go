// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/bits"
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	// Size constants
	uint8Size  = 1
	uint16Size = 2
	uint32Size = 4
	uint64Size = 8

	// Preamble size is 1 byte version + 4 bytes series count
	preambleSize = uint8Size + uint32Size

	// String dictionary constants
	stringLengthSize = uint16Size
	stringIDSize     = uint32Size

	// Limits
	TagLengthHardLimit     = 200
	TagsPerTagsetHardLimit = 100
)

// StringDict represents a dictionary of strings used for deduplication
type StringDict struct {
	dict map[string]uint32
	strs []string
}

func (d *StringDict) LookupString(id uint32) string {
	return d.strs[id]
}

func (d *StringDict) LookupID(s string) uint32 {
	return d.dict[s]
}

func (d *StringDict) Len() int {
	return len(d.strs)
}

// StringDictBuilder helps build a StringDict
type StringDictBuilder struct {
	dict map[string]uint32
}

func NewStringDictBuilder() *StringDictBuilder {
	return &StringDictBuilder{make(map[string]uint32)}
}

func (sb *StringDictBuilder) Insert(s string) {
	if len(s) > TagLengthHardLimit {
		s = s[:TagLengthHardLimit]
	}
	sb.dict[s] = 0
}

func (sb *StringDictBuilder) Build() *StringDict {
	strs := make([]string, 0, len(sb.dict))
	for str := range sb.dict {
		strs = append(strs, str)
	}
	slices.Sort(strs)
	for id, str := range strs {
		sb.dict[str] = uint32(id)
	}
	return &StringDict{dict: sb.dict, strs: strs}
}

// seriesData holds the data needed to serialize a series
type seriesData struct {
	serie        *metrics.Serie
	pointsSize   int
	resourceSize int
}

// CustomRowPayloadsBuilder represents an in-progress serialization of a series into potentially multiple custom row payloads.
type CustomRowPayloadsBuilder struct {
	bufferContext *marshaler.BufferContext
	config        config.Component
	strategy      compression.Component

	compressor *stream.Compressor
	buf        *bytes.Buffer
	payloads   []*transaction.BytesPayload

	pointsThisPayload int
	seriesThisPayload int

	maxPayloadSize      int
	maxUncompressedSize int
	maxPointsPerPayload int

	// Data collected during first pass
	stringDictBuilder *StringDictBuilder
	seriesData        []seriesData
	totalSize         int
}

// GetPayloads returns the slice of payloads
func (pb *CustomRowPayloadsBuilder) GetPayloads() []*transaction.BytesPayload {
	return pb.payloads
}

// Prepare to write the next payload
func (pb *CustomRowPayloadsBuilder) startPayload() error {
	pb.pointsThisPayload = 0
	pb.seriesThisPayload = 0
	pb.bufferContext.CompressorInput.Reset()
	pb.bufferContext.CompressorOutput.Reset()
	pb.stringDictBuilder = NewStringDictBuilder()
	pb.seriesData = make([]seriesData, 0)
	pb.totalSize = preambleSize

	compressor, err := stream.NewCompressor(
		pb.bufferContext.CompressorInput, pb.bufferContext.CompressorOutput,
		pb.maxPayloadSize, pb.maxUncompressedSize,
		[]byte{}, []byte{}, []byte{}, pb.strategy)
	if err != nil {
		return err
	}
	pb.compressor = compressor

	return nil
}

func (pb *CustomRowPayloadsBuilder) writeSerie(serie *metrics.Serie) error {
	// First pass: collect strings and calculate sizes
	const seriesSize = 52 // Size of fixed fields per series
	pointsSize := 12 * len(serie.Points)
	resourceSize := 2 + (8 * len(serie.Resources)) // 2 bytes count + 8 bytes per resource (type + name)

	// Add strings to dictionary
	pb.stringDictBuilder.Insert(serie.Name)
	pb.stringDictBuilder.Insert(serie.SourceTypeName)
	pb.stringDictBuilder.Insert(serie.Host)
	if serie.Device != "" {
		pb.stringDictBuilder.Insert(serie.Device)
	}
	serie.Tags.ForEach(func(tag string) {
		pb.stringDictBuilder.Insert(tag)
	})
	for _, resource := range serie.Resources {
		pb.stringDictBuilder.Insert(resource.Type)
		pb.stringDictBuilder.Insert(resource.Name)
	}

	// Calculate tag size
	tagCount := 0
	serie.Tags.ForEach(func(tag string) {
		tagCount++
	})
	if tagCount > TagsPerTagsetHardLimit {
		tagCount = TagsPerTagsetHardLimit
	}
	tagSize := 2 + int(
		bitsPerValue(uint32(len(pb.stringDictBuilder.dict)))*
			uint(tagCount)/8,
	)

	// Calculate total size for this series
	seriesTotalSize := seriesSize + pointsSize + resourceSize + tagSize

	// Check if we need to start a new payload
	if pb.pointsThisPayload+len(serie.Points) > pb.maxPointsPerPayload {
		err := pb.finishPayload()
		if err != nil {
			return err
		}
		err = pb.startPayload()
		if err != nil {
			return err
		}
	}

	// Store series data for second pass
	pb.seriesData = append(pb.seriesData, seriesData{
		serie:        serie,
		pointsSize:   pointsSize,
		resourceSize: resourceSize,
	})

	pb.totalSize += seriesTotalSize
	pb.pointsThisPayload += len(serie.Points)
	pb.seriesThisPayload++
	return nil
}

func (pb *CustomRowPayloadsBuilder) finishPayload() error {
	if pb.seriesThisPayload == 0 {
		return nil
	}

	// Build the final string dictionary
	stringDict := pb.stringDictBuilder.Build()

	// Write preamble
	pb.buf.Reset()
	pb.buf.Grow(pb.totalSize)
	binary.Write(pb.buf, binary.LittleEndian, uint8(1)) // Version
	binary.Write(pb.buf, binary.LittleEndian, uint32(pb.seriesThisPayload))

	// Write string dictionary
	binary.Write(pb.buf, binary.LittleEndian, uint32(len(stringDict.strs)))
	for _, str := range stringDict.strs {
		binary.Write(pb.buf, binary.LittleEndian, uint16(len(str)))
		pb.buf.WriteString(str)
	}

	// Second pass: write series data with correct string indices
	for _, data := range pb.seriesData {
		serie := data.serie

		// Write series data
		binary.Write(pb.buf, binary.LittleEndian, stringDict.LookupID(serie.Name))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.MType.SeriesAPIV2Enum()))
		binary.Write(pb.buf, binary.LittleEndian, stringDict.LookupID("")) // Empty unit
		binary.Write(pb.buf, binary.LittleEndian, stringDict.LookupID(serie.SourceTypeName))
		binary.Write(pb.buf, binary.LittleEndian, uint64(serie.Interval))

		// Write origin metadata
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))
		binary.Write(pb.buf, binary.LittleEndian, uint32(serie.Source))

		// Write points
		binary.Write(pb.buf, binary.LittleEndian, uint32(len(serie.Points)))
		for _, point := range serie.Points {
			binary.Write(pb.buf, binary.LittleEndian, uint32(point.Ts))
			binary.Write(pb.buf, binary.LittleEndian, math.Float64bits(point.Value))
		}

		// Write resources
		binary.Write(pb.buf, binary.LittleEndian, uint16(len(serie.Resources)))
		for _, resource := range serie.Resources {
			binary.Write(pb.buf, binary.LittleEndian, stringDict.LookupID(resource.Type))
			binary.Write(pb.buf, binary.LittleEndian, stringDict.LookupID(resource.Name))
		}

		// Write tags using packed encoding
		var encodedTags []uint32
		serie.Tags.ForEach(func(tag string) {
			encodedTags = append(encodedTags, stringDict.LookupID(tag))
		})
		slices.Sort(encodedTags)
		encodedTags = slices.Compact(encodedTags)
		if len(encodedTags) > TagsPerTagsetHardLimit {
			encodedTags = encodedTags[:TagsPerTagsetHardLimit]
		}
		binary.Write(pb.buf, binary.LittleEndian, uint16(len(encodedTags)))
		writePackedUint32s(pb.buf, uint32(stringDict.Len()), encodedTags)
	}

	// Compress and store the payload
	payload, err := pb.compressor.Close()
	if err != nil {
		return err
	}

	pb.payloads = append(pb.payloads, transaction.NewBytesPayload(payload, pb.pointsThisPayload))
	return nil
}

// Helper function to write packed uint32s
func writePackedUint32s(buf *bytes.Buffer, dictSize uint32, values []uint32) {
	if len(values) == 0 {
		return
	}

	// Calculate number of bits needed per value
	bitsPerValue := uint(32 - bits.LeadingZeros32(dictSize-1))
	if bitsPerValue == 0 {
		bitsPerValue = 1
	}

	// Write bits per value
	buf.WriteByte(byte(bitsPerValue))

	// Pack values into bytes
	var currentByte byte
	var bitsUsed uint
	for _, value := range values {
		remainingBits := bitsPerValue
		for remainingBits > 0 {
			if bitsUsed == 8 {
				buf.WriteByte(currentByte)
				currentByte = 0
				bitsUsed = 0
			}
			bitsToWrite := min(remainingBits, 8-bitsUsed)
			mask := byte((1 << bitsToWrite) - 1)
			currentByte |= byte(value&uint32(mask)) << bitsUsed
			value >>= bitsToWrite
			bitsUsed += bitsToWrite
			remainingBits -= bitsToWrite
		}
	}
	if bitsUsed > 0 {
		buf.WriteByte(currentByte)
	}
}

// Helper function to calculate bits per value for packed encoding
func bitsPerValue(maxValue uint32) uint {
	bits := uint(32 - bits.LeadingZeros32(maxValue))
	if bits == 0 {
		return 1
	}
	return bits
}
