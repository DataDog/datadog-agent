// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build zlib && test && zstd

package stream

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func payloadToString(payload []byte, cfg config.Component) string {
	compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: cfg}).Comp
	p, err := compressor.Decompress(payload)
	if err != nil {
		return err.Error()
	}
	return string(p)
}

func TestCompressorSimple(t *testing.T) {
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
			maxPayloadSize := mockConfig.GetInt("serializer_max_payload_size")
			maxUncompressedSize := mockConfig.GetInt("serializer_max_uncompressed_payload_size")
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			c, err := NewCompressor(
				&bytes.Buffer{}, &bytes.Buffer{},
				maxPayloadSize, maxUncompressedSize,
				[]byte("{["), []byte("]}"), []byte(","), compressor)
			require.NoError(t, err)

			for i := 0; i < 5; i++ {
				c.AddItem([]byte("A"))
			}

			p, err := c.Close()
			require.NoError(t, err)
			require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p, mockConfig))
		})
	}
}

func TestCompressorLimits(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", "zstd")
	maxPayloadSize := mockConfig.GetInt("serializer_max_payload_size")
	maxUncompressedSize := mockConfig.GetInt("serializer_max_uncompressed_payload_size")

	compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
	c, err := NewCompressor(
		&bytes.Buffer{}, &bytes.Buffer{},
		maxPayloadSize, maxUncompressedSize,
		[]byte("headerheader"), []byte("footerfooter"), []byte(","), compressor)

	require.NoError(t, err)

	//nolint:revive // https://github.com/mgechev/revive/issues/386
	for c.AddItem([]byte("contentontent")) == nil {
	}

	p, err := c.Close()
	require.NoError(t, err)
	require.Less(t, len(p), maxPayloadSize)
	d, err := compressor.Decompress(p)
	require.NoError(t, err)
	require.Less(t, len(d), maxUncompressedSize)
}

// With an empty payload, AddItem should never return "ErrPayloadFull"
// ErrItemTooBig is a more appropriate error code if the item cannot
// be added to an empty compressor
func TestCompressorAddItemErrCodeWithEmptyCompressor(t *testing.T) {

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

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			checkAddItemErrCode := func(maxPayloadSize, maxUncompressedSize, dataLen int) {
				c, err := NewCompressor(
					&bytes.Buffer{}, &bytes.Buffer{},
					maxPayloadSize, maxUncompressedSize,
					[]byte("{["), []byte("]}"), []byte(","), compressor)
				require.NoError(t, err)

				payload := strings.Repeat("A", dataLen)
				err = c.AddItem([]byte(payload))
				if err != nil {
					// Only possible values for AddItem error codes are ErrPayloadFull and ErrItemTooBig
					// But with an empty compressor, only ErrItemTooBig should be returned
					require.ErrorIs(t, err, ErrItemTooBig)
				}
			}

			// While some of these values may look like they should fit, they currently don't due to a combination
			// of due to overhead in the payload (header and footer) and the CompressBound calculation
			t.Run("Edge Case from real world", func(_ *testing.T) {
				checkAddItemErrCode(2_621_440, 4_194_304, 2_620_896)
			})

			t.Run("Other values from iterative testing", func(_ *testing.T) {
				checkAddItemErrCode(17, 32, 1)
				checkAddItemErrCode(19, 35, 4)
				checkAddItemErrCode(23, 43, 12)
				checkAddItemErrCode(44, 71, 39)
				checkAddItemErrCode(1110, 1129, 1098)
				checkAddItemErrCode(11100, 11119, 11085)
			})
		})
	}
}

func TestOnePayloadSimple(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := &marshaler.DummyMarshaller{
				Items:  []string{"A", "B", "C"},
				Header: "{[",
				Footer: "]}",
			}

			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			builder := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			payloads, err := BuildJSONPayload(builder, m)
			require.NoError(t, err)
			require.Len(t, payloads, 1)

			require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent(), mockConfig))
		})
	}
}

func TestMaxCompressedSizePayload(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := &marshaler.DummyMarshaller{
				Items:  []string{"A", "B", "C"},
				Header: "{[",
				Footer: "]}",
			}
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			// Calculate maxPayloadSize dynamically based on CompressBound
			// The uncompressed payload "{[A,B,C]}" is 9 bytes
			maxPayloadSize := compressor.CompressBound(9)
			mockConfig.SetDefault("serializer_max_payload_size", maxPayloadSize)
			builder := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			payloads, err := BuildJSONPayload(builder, m)
			require.NoError(t, err)
			require.Len(t, payloads, 1)

			require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent(), mockConfig))
		})
	}
}

func TestZstdCompressionLevel(t *testing.T) {
	tests := []int{1, 5, 9}

	logger := logmock.New(t)
	for _, level := range tests {
		t.Run(fmt.Sprintf("zstd %d", level), func(t *testing.T) {
			m := &marshaler.DummyMarshaller{
				Items:  []string{"A", "B", "C"},
				Header: "{[",
				Footer: "]}",
			}
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", "zstd")
			mockConfig.SetDefault("serializer_zstd_compressor_level", level)

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			builder := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			payloads, err := BuildJSONPayload(builder, m)
			require.NoError(t, err)
			require.Len(t, payloads, 1)

			require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent(), mockConfig))
		})
	}
}

func TestTwoPayload(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// For zstd compatibility, item_size must satisfy:
			// CompressBound(item_size) < maxZippedItemSize
			// => item_size + 63 < maxUncompressed - CompressBound(header+footer)
			// => item_size < maxUncompressed - 130 (approximately)
			//
			// With 50-char items (52 bytes quoted in JSON):
			// - 3 items = header(2) + 3*quoted(52) + 2*sep(1) + footer(2) = 162 bytes
			// - 4 items = header(2) + 4*quoted(52) + 3*sep(1) + footer(2) = 215 bytes
			// - 6 items = header(2) + 6*quoted(52) + 5*sep(1) + footer(2) = 321 bytes
			// Set maxUncompressed = 200 to force split after 3 items (162 < 200 < 215)
			// Item check: 50 < 200 - 130 = 70 ✓
			itemSize := 50
			item1 := strings.Repeat("A", itemSize)
			item2 := strings.Repeat("B", itemSize)
			item3 := strings.Repeat("C", itemSize)
			item4 := strings.Repeat("D", itemSize)
			item5 := strings.Repeat("E", itemSize)
			item6 := strings.Repeat("F", itemSize)

			m := &marshaler.DummyMarshaller{
				Items:  []string{item1, item2, item3, item4, item5, item6},
				Header: "{[",
				Footer: "]}",
			}
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp

			maxUncompressed := 200
			maxCompressed := compressor.CompressBound(maxUncompressed)
			mockConfig.SetDefault("serializer_max_uncompressed_payload_size", maxUncompressed)
			mockConfig.SetDefault("serializer_max_payload_size", maxCompressed)

			builder := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			payloads, err := BuildJSONPayload(builder, m)
			require.NoError(t, err)
			require.Len(t, payloads, 2)

			expected1 := "{[" + item1 + "," + item2 + "," + item3 + "]}"
			expected2 := "{[" + item4 + "," + item5 + "," + item6 + "]}"
			require.Equal(t, expected1, payloadToString(payloads[0].GetContent(), mockConfig))
			require.Equal(t, expected2, payloadToString(payloads[1].GetContent(), mockConfig))
		})
	}
}

func TestLockedCompressorProducesSamePayloads(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := &marshaler.DummyMarshaller{
				Items:  []string{"A", "B", "C", "D", "E", "F"},
				Header: "{[",
				Footer: "]}",
			}
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			builderLocked := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			builderUnLocked := NewJSONPayloadBuilder(false, mockConfig, compressor, logger)
			payloads1, err := BuildJSONPayload(builderLocked, m)
			require.NoError(t, err)
			payloads2, err := BuildJSONPayload(builderUnLocked, m)
			require.NoError(t, err)

			require.Equal(t, payloadToString(payloads1[0].GetContent(), mockConfig), payloadToString(payloads2[0].GetContent(), mockConfig))
		})
	}
}

func TestBuildWithOnErrItemTooBigPolicyMetadata(t *testing.T) {
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
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp

			// Use limits that:
			// 1. Allow individual items to fit (each item is ~5-7 bytes, max "Item99")
			// 2. Force multiple payloads from 100 items
			// Set uncompressed limit to 200 bytes (~25-30 items per payload, giving 4+ payloads)
			// Set compressed limit high enough to fit any single item
			maxUncompressed := 200
			maxCompressed := compressor.CompressBound(maxUncompressed)
			mockConfig.SetWithoutSource("serializer_max_uncompressed_payload_size", maxUncompressed)
			mockConfig.SetWithoutSource("serializer_max_payload_size", maxCompressed)

			marshaler := &IterableStreamJSONMarshalerMock{index: 0, maxIndex: 100}
			builder := NewJSONPayloadBuilder(false, mockConfig, compressor, logger)
			payloads, err := builder.BuildWithOnErrItemTooBigPolicy(
				marshaler,
				DropItemOnErrItemTooBig)
			r := require.New(t)
			r.NoError(err)

			// Make sure there are at least few payloads
			r.Greater(len(payloads), 3)

			pointCount := 0
			for _, payload := range payloads {
				pointCount += payload.GetPointCount()
			}
			maxValue := marshaler.maxIndex - 1
			r.Equal((maxValue*(maxValue+1))/2, pointCount)
		})
	}
}

type IterableStreamJSONMarshalerMock struct {
	index    int
	maxIndex int
}

func (i *IterableStreamJSONMarshalerMock) WriteHeader(*jsoniter.Stream) error { return nil }
func (i *IterableStreamJSONMarshalerMock) WriteFooter(*jsoniter.Stream) error { return nil }
func (i *IterableStreamJSONMarshalerMock) WriteCurrentItem(stream *jsoniter.Stream) error {
	stream.WriteString(fmt.Sprintf("Item%v", i.index))
	return nil
}
func (i *IterableStreamJSONMarshalerMock) DescribeCurrentItem() string   { return "" }
func (i *IterableStreamJSONMarshalerMock) GetCurrentItemPointCount() int { return i.index }
func (i *IterableStreamJSONMarshalerMock) MoveNext() bool {
	i.index++
	return i.index < i.maxIndex
}
