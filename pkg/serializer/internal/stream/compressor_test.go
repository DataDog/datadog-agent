// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build zlib && test

package stream

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	maxPayloadSizeDefault = config.Datadog.GetInt("serializer_max_payload_size")
)

func resetDefaults() {
	config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSizeDefault)
}

func decompressPayload(payload []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func payloadToString(payload []byte) string {
	p, err := decompressPayload(payload)
	if err != nil {
		return err.Error()
	}
	return string(p)
}

func TestCompressorSimple(t *testing.T) {
	maxPayloadSize := config.Datadog.GetInt("serializer_max_payload_size")
	maxUncompressedSize := config.Datadog.GetInt("serializer_max_uncompressed_payload_size")
	c, err := NewCompressor(
		&bytes.Buffer{}, &bytes.Buffer{},
		maxPayloadSize, maxUncompressedSize,
		[]byte("{["), []byte("]}"), []byte(","))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		c.AddItem([]byte("A"))
	}

	p, err := c.Close()
	require.NoError(t, err)
	require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p))
}

// With an empty payload, AddItem should never return "ErrPayloadFull"
// ErrItemTooBig is a more appropriate error code if the item cannot
// be added to an empty compressor
func TestCompressorAddItemErrCodeWithEmptyCompressor(t *testing.T) {
	checkAddItemErrCode := func(maxPayloadSize, maxUncompressedSize, dataLen int) {
		c, err := NewCompressor(
			&bytes.Buffer{}, &bytes.Buffer{},
			maxPayloadSize, maxUncompressedSize,
			[]byte("{["), []byte("]}"), []byte(","))
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
	t.Run("Edge Case from real world", func(t *testing.T) {
		checkAddItemErrCode(2_621_440, 4_194_304, 2_620_896)
	})

	t.Run("Other values from iterative testing", func(t *testing.T) {
		checkAddItemErrCode(17, 32, 1)
		checkAddItemErrCode(19, 35, 4)
		checkAddItemErrCode(23, 43, 12)
		checkAddItemErrCode(44, 71, 39)
		checkAddItemErrCode(1110, 1129, 1098)
		checkAddItemErrCode(11100, 11119, 11085)
	})
}

func TestOnePayloadSimple(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C"},
		Header: "{[",
		Footer: "]}",
	}

	builder := NewJSONPayloadBuilder(true)
	payloads, err := BuildJSONPayload(builder, m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent()))
}

func TestMaxCompressedSizePayload(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C"},
		Header: "{[",
		Footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := BuildJSONPayload(builder, m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent()))
}

func TestTwoPayload(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C", "D", "E", "F"},
		Header: "{[",
		Footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := BuildJSONPayload(builder, m)
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	require.Equal(t, "{[A,B,C]}", payloadToString(payloads[0].GetContent()))
	require.Equal(t, "{[D,E,F]}", payloadToString(payloads[1].GetContent()))
}

func TestLockedCompressorProducesSamePayloads(t *testing.T) {
	m := &marshaler.DummyMarshaller{
		Items:  []string{"A", "B", "C", "D", "E", "F"},
		Header: "{[",
		Footer: "]}",
	}
	defer resetDefaults()

	builderLocked := NewJSONPayloadBuilder(true)
	builderUnLocked := NewJSONPayloadBuilder(false)
	payloads1, err := BuildJSONPayload(builderLocked, m)
	require.NoError(t, err)
	payloads2, err := BuildJSONPayload(builderUnLocked, m)
	require.NoError(t, err)

	require.Equal(t, payloadToString(payloads1[0].GetContent()), payloadToString(payloads2[0].GetContent()))
}

func TestBuildWithOnErrItemTooBigPolicyMetadata(t *testing.T) {
	config.Datadog.Set("serializer_max_uncompressed_payload_size", 40)
	defer config.Datadog.Set("serializer_max_uncompressed_payload_size", nil)
	marshaler := &IterableStreamJSONMarshalerMock{index: 0, maxIndex: 100}
	builder := NewJSONPayloadBuilder(false)
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
}

type IterableStreamJSONMarshalerMock struct {
	index    int
	maxIndex int
}

func (i *IterableStreamJSONMarshalerMock) WriteHeader(stream *jsoniter.Stream) error { return nil }
func (i *IterableStreamJSONMarshalerMock) WriteFooter(stream *jsoniter.Stream) error { return nil }
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
