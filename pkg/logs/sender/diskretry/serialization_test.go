// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskretry

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func makeTestPayload(encoded []byte, encoding string, unencodedSize int, msgCount int) *message.Payload {
	metas := make([]*message.MessageMetadata, msgCount)
	for i := range metas {
		metas[i] = &message.MessageMetadata{}
	}
	return &message.Payload{
		MessageMetas:  metas,
		Encoded:       encoded,
		Encoding:      encoding,
		UnencodedSize: unencodedSize,
	}
}

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	encoded := []byte("compressed-log-data-here")
	payload := makeTestPayload(encoded, "gzip", 1024, 5)

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	assert.Equal(t, payload.Encoded, result.Encoded)
	assert.Equal(t, payload.Encoding, result.Encoding)
	assert.Equal(t, payload.UnencodedSize, result.UnencodedSize)
	assert.Equal(t, payload.Count(), result.Count())
}

func TestSerializeDeserializeEmptyEncoding(t *testing.T) {
	payload := makeTestPayload([]byte("data"), "", 100, 1)

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	assert.Equal(t, "", result.Encoding)
	assert.Equal(t, []byte("data"), result.Encoded)
}

func TestSerializeDeserializeEmptyPayload(t *testing.T) {
	payload := makeTestPayload([]byte{}, "deflate", 0, 0)

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	assert.Equal(t, []byte{}, result.Encoded)
	assert.Equal(t, int64(0), result.Count())
}

func TestSerializeDeserializeLargePayload(t *testing.T) {
	// 1 MB payload
	encoded := make([]byte, 1024*1024)
	for i := range encoded {
		encoded[i] = byte(i % 256)
	}
	payload := makeTestPayload(encoded, "zstd", 2*1024*1024, 100)

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	assert.Equal(t, encoded, result.Encoded)
	assert.Equal(t, int64(100), result.Count())
	assert.Equal(t, 2*1024*1024, result.UnencodedSize)
}

func TestDeserializeInvalidMagic(t *testing.T) {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[0:], 0xDEADBEEF)
	_, err := DeserializePayload(data)
	assert.ErrorContains(t, err, "invalid magic number")
}

func TestDeserializeUnsupportedVersion(t *testing.T) {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[0:], fileMagic)
	binary.LittleEndian.PutUint32(data[4:], 99)
	_, err := DeserializePayload(data)
	assert.ErrorContains(t, err, "unsupported format version")
}

func TestDeserializeTruncatedFile(t *testing.T) {
	// Too small
	_, err := DeserializePayload([]byte{1, 2, 3})
	assert.ErrorContains(t, err, "file too small")
}

func TestDeserializeTruncatedEncodedPayload(t *testing.T) {
	payload := makeTestPayload([]byte("data"), "gzip", 100, 1)
	data, err := SerializePayload(payload)
	require.NoError(t, err)

	// Truncate the file mid-payload
	_, err = DeserializePayload(data[:len(data)-5])
	assert.Error(t, err)
}

func TestSerializeDeserializeMRFRoundTrip(t *testing.T) {
	encoded := []byte("mrf-payload-data")
	metas := make([]*message.MessageMetadata, 3)
	for i := range metas {
		metas[i] = &message.MessageMetadata{
			ParsingExtra: message.ParsingExtra{IsMRFAllow: true},
		}
	}
	payload := &message.Payload{
		MessageMetas:  metas,
		Encoded:       encoded,
		Encoding:      "gzip",
		UnencodedSize: 200,
	}

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	assert.True(t, result.IsMRF(), "MRF flag should survive round-trip")
	assert.Equal(t, encoded, result.Encoded)
	assert.Equal(t, int64(3), result.Count())
}

func TestDeserializeOldFormatWithoutMRFByte(t *testing.T) {
	// Build a v1 binary blob WITHOUT the trailing isMRF byte (old format).
	encodingBytes := []byte("gzip")
	encodedData := []byte("old-payload")
	messageCount := uint32(2)

	oldSize := headerSize +
		4 + len(encodingBytes) +
		4 +
		4 + len(encodedData) +
		4 // no isMRF byte

	buf := make([]byte, oldSize)
	offset := 0
	binary.LittleEndian.PutUint32(buf[offset:], fileMagic)
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], formatVersion)
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(encodingBytes)))
	offset += 4
	copy(buf[offset:], encodingBytes)
	offset += len(encodingBytes)
	binary.LittleEndian.PutUint32(buf[offset:], 100) // unencodedSize
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(encodedData)))
	offset += 4
	copy(buf[offset:], encodedData)
	offset += len(encodedData)
	binary.LittleEndian.PutUint32(buf[offset:], messageCount)

	result, err := DeserializePayload(buf)
	require.NoError(t, err)

	assert.False(t, result.IsMRF(), "old format without isMRF byte should default to false")
	assert.Equal(t, encodedData, result.Encoded)
	assert.Equal(t, int64(2), result.Count())
}

func TestDeserializedPayloadHasValidOrigin(t *testing.T) {
	payload := makeTestPayload([]byte("data"), "gzip", 100, 3)

	data, err := SerializePayload(payload)
	require.NoError(t, err)

	result, err := DeserializePayload(data)
	require.NoError(t, err)

	// Verify each meta has a non-nil Origin with empty Identifier
	// (so the auditor skips registry updates without panicking)
	for _, meta := range result.MessageMetas {
		require.NotNil(t, meta.Origin)
		assert.Empty(t, meta.Origin.Identifier)
		require.NotNil(t, meta.Origin.LogSource)
		require.NotNil(t, meta.Origin.LogSource.Config)
	}
}
