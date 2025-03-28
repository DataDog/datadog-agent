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
		kind           string
		maxPayloadSize int
	}{
		"zlib": {kind: compression.ZlibKind, maxPayloadSize: 22},
		"zstd": {kind: compression.ZstdKind, maxPayloadSize: 90},
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
			mockConfig.SetDefault("serializer_max_payload_size", tc.maxPayloadSize)
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
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
		kind           string
		maxPayloadSize int
	}{
		"zlib": {kind: compression.ZlibKind, maxPayloadSize: 22},
		"zstd": {kind: compression.ZstdKind, maxPayloadSize: 70},
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
			mockConfig.SetDefault("serializer_max_payload_size", tc.maxPayloadSize)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			builder := NewJSONPayloadBuilder(true, mockConfig, compressor, logger)
			payloads, err := BuildJSONPayload(builder, m)
			require.NoError(t, err)
			require.Len(t, payloads, 2)

			require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent(), mockConfig))
			require.Equal(t, "{[D,E,F]}", payloadToString(payloads[1].GetContent(), mockConfig))
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
		kind                       string
		maxUncompressedPayloadSize int
	}{
		"zlib": {kind: compression.ZlibKind, maxUncompressedPayloadSize: 40},
		"zstd": {kind: compression.ZstdKind, maxUncompressedPayloadSize: 170},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			mockConfig.SetWithoutSource("serializer_max_uncompressed_payload_size", tc.maxUncompressedPayloadSize)

			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
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
