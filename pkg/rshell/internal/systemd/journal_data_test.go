// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

func TestJournalDataObjectParsesRegularAndCompactLayouts(t *testing.T) {
	for _, compact := range []bool{false, true} {
		name := "regular"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			contents, offset := testJournalDataContents(t, []byte("MESSAGE=hello"), 0, compact)
			binary.LittleEndian.PutUint64(contents[offset+16:offset+24], 0x0102030405060708)
			binary.LittleEndian.PutUint64(contents[offset+56:offset+64], 3)

			view, err := newJournalFileView("data.journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)
			data, err := view.dataObjectAt(uint64(offset))
			require.NoError(t, err)

			assert.Equal(t, uint64(0x0102030405060708), data.hash)
			assert.Equal(t, uint64(3), data.nEntries)
			assert.Equal(t, uint64(len("MESSAGE=hello")), data.payloadSize)
			headerSize := uint64(journalDataRegularHeaderSize)
			if compact {
				headerSize = journalDataCompactHeaderSize
			}
			assert.Equal(t, uint64(offset)+headerSize, data.payloadOffset)
			assert.Equal(t, compact, data.hasTailEntryArrayReference)
		})
	}
}

func TestJournalDataPayloadDecodesSupportedCompression(t *testing.T) {
	payload := []byte("MESSAGE=" + strings.Repeat("journal payload ", 32))
	tests := []struct {
		name       string
		objectFlag uint8
		encode     func(*testing.T, []byte) []byte
	}{
		{name: "uncompressed", encode: func(_ *testing.T, data []byte) []byte { return data }},
		{name: "xz", objectFlag: journalObjectCompressedXZ, encode: encodeJournalXZ},
		{name: "lz4", objectFlag: journalObjectCompressedLZ4, encode: encodeJournalLZ4},
		{name: "zstd", objectFlag: journalObjectCompressedZSTD, encode: encodeJournalZSTD},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded := test.encode(t, payload)
			contents, offset := testJournalDataContents(t, encoded, test.objectFlag, false)
			view, err := newJournalFileView(test.name+".journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)
			data, err := view.dataObjectAt(uint64(offset))
			require.NoError(t, err)

			decoded, truncated, err := view.readDataPayload(data, len(payload)+1)
			require.NoError(t, err)
			assert.False(t, truncated)
			assert.Equal(t, payload, decoded)

			decoded, truncated, err = view.readDataPayload(data, 17)
			require.NoError(t, err)
			assert.True(t, truncated)
			assert.Equal(t, payload[:17], decoded)
		})
	}
}

func TestJournalDataPayloadRejectsMalformedCompression(t *testing.T) {
	for _, objectFlag := range []uint8{
		journalObjectCompressedXZ,
		journalObjectCompressedLZ4,
		journalObjectCompressedZSTD,
	} {
		malformed := bytes.Repeat([]byte{0xff}, 32)
		if objectFlag == journalObjectCompressedLZ4 {
			binary.LittleEndian.PutUint64(malformed[:8], 64)
		}
		contents, offset := testJournalDataContents(t, malformed, objectFlag, false)
		view, err := newJournalFileView("malformed.journal", bytes.NewReader(contents), uint64(len(contents)))
		require.NoError(t, err)
		data, err := view.dataObjectAt(uint64(offset))
		require.NoError(t, err)

		_, _, err = view.readDataPayload(data, 64)
		require.Error(t, err)
		assert.ErrorIs(t, err, errJournalCorrupt)
		assert.Contains(t, err.Error(), "decode")
	}
}

func TestJournalDataPayloadBoundsLZ4Expansion(t *testing.T) {
	encoded := make([]byte, 9)
	binary.LittleEndian.PutUint64(encoded[:8], maxJournalLZ4DataSize+1)
	contents, offset := testJournalDataContents(t, encoded, journalObjectCompressedLZ4, false)
	view, err := newJournalFileView("large-lz4.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(offset))
	require.NoError(t, err)

	_, _, err = view.readDataPayload(data, 64)
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalLimit)
	assert.Contains(t, err.Error(), "expands to 8388609 bytes")
}

func TestJournalDataPayloadBoundsLZ4PrefixDecode(t *testing.T) {
	payload := append([]byte("MESSAGE="), bytes.Repeat([]byte{'x'}, maxJournalLZ4DataSize-len("MESSAGE="))...)
	encoded := encodeJournalLZ4(t, payload)
	require.Greater(t, len(encoded), 2*journalLZ4ReadBufferSize)
	contents, offset := testJournalDataContents(t, encoded, journalObjectCompressedLZ4, false)
	reader := &journalCountingReaderAt{ReaderAt: bytes.NewReader(contents)}
	view, err := newJournalFileView("bounded-lz4.journal", reader, uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(offset))
	require.NoError(t, err)
	reader.bytesRead = 0

	decoded, truncated, err := view.readDataPayload(data, 64)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Equal(t, payload[:64], decoded)
	assert.LessOrEqual(t, reader.bytesRead, 8+journalLZ4ReadBufferSize)
}

func TestJournalDataPayloadLZ4PrefixDecodeMatchesPayload(t *testing.T) {
	literalPrefix := make([]byte, 512)
	for index := range literalPrefix {
		literalPrefix[index] = byte(index)
	}
	payloads := []struct {
		name    string
		payload []byte
	}{
		{name: "single-byte matches", payload: bytes.Repeat([]byte{'x'}, 4*1024)},
		{name: "phrase matches", payload: bytes.Repeat([]byte("journal payload "), 8*1024)},
		{name: "literal prefix", payload: bytes.Repeat(literalPrefix, 32)},
	}

	for _, test := range payloads {
		encoded := encodeJournalLZ4(t, test.payload)
		contents, offset := testJournalDataContents(t, encoded, journalObjectCompressedLZ4, false)
		view, err := newJournalFileView(test.name+".journal", bytes.NewReader(contents), uint64(len(contents)))
		require.NoError(t, err)
		data, err := view.dataObjectAt(uint64(offset))
		require.NoError(t, err)

		limits := []int{1, 17, 64, 1024, maxJournalPayloadRead, len(test.payload) - 1, len(test.payload), len(test.payload) + 1}
		for _, limit := range limits {
			if limit <= 0 || limit > maxJournalPayloadRead {
				continue
			}
			decoded, truncated, err := view.readDataPayload(data, limit)
			require.NoErrorf(t, err, "%s with limit %d", test.name, limit)
			expectedLength := min(limit, len(test.payload))
			assert.Equalf(t, test.payload[:expectedLength], decoded, "%s with limit %d", test.name, limit)
			assert.Equalf(t, len(test.payload) > limit, truncated, "%s with limit %d", test.name, limit)
		}
	}
}

func TestJournalDataPayloadRejectsUnboundedReadLimit(t *testing.T) {
	contents, offset := testJournalDataContents(t, []byte("MESSAGE=hello"), 0, false)
	view, err := newJournalFileView("limit.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(offset))
	require.NoError(t, err)

	_, _, err = view.readDataPayload(data, maxJournalPayloadRead+1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestJournalDataObjectRejectsInvalidReferences(t *testing.T) {
	contents, offset := testJournalDataContents(t, []byte("MESSAGE=hello"), 0, false)
	binary.LittleEndian.PutUint64(contents[offset+24:offset+32], uint64(offset+1))
	view, err := newJournalFileView("reference.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)

	_, err = view.dataObjectAt(uint64(offset))
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
	assert.Contains(t, err.Error(), "next DATA hash offset is not 8-byte aligned")
}

func testJournalDataContents(t *testing.T, encoded []byte, objectFlag uint8, compact bool) ([]byte, int) {
	t.Helper()
	headerFlags := uint32(0)
	switch objectFlag {
	case journalObjectCompressedXZ:
		headerFlags |= journalHeaderIncompatibleCompressedXZ
	case journalObjectCompressedLZ4:
		headerFlags |= journalHeaderIncompatibleCompressedLZ4
	case journalObjectCompressedZSTD:
		headerFlags |= journalHeaderIncompatibleCompressedZSTD
	}
	headerSize := journalHeaderCurrentSize
	dataHeaderSize := journalDataRegularHeaderSize
	if compact {
		headerFlags |= journalHeaderIncompatibleCompact
		dataHeaderSize = journalDataCompactHeaderSize
	}
	objectSize := dataHeaderSize + len(encoded)
	fileSize := alignJournalTestSize(headerSize + objectSize)
	contents := testJournalContents(fileSize, 0, headerFlags)
	binary.LittleEndian.PutUint64(contents[136:144], uint64(headerSize))
	object := contents[headerSize:]
	object[0] = journalObjectData
	object[1] = objectFlag
	binary.LittleEndian.PutUint64(object[8:16], uint64(objectSize))
	copy(object[dataHeaderSize:], encoded)
	return contents, headerSize
}

func alignJournalTestSize(size int) int {
	return (size + 7) &^ 7
}

func encodeJournalXZ(t *testing.T, payload []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer, err := xz.NewWriter(&encoded)
	require.NoError(t, err)
	_, err = writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

func encodeJournalLZ4(t *testing.T, payload []byte) []byte {
	t.Helper()
	compressed := make([]byte, lz4.CompressBlockBound(len(payload)))
	n, err := lz4.CompressBlock(payload, compressed, nil)
	require.NoError(t, err)
	require.Positive(t, n)
	encoded := make([]byte, 8+n)
	binary.LittleEndian.PutUint64(encoded[:8], uint64(len(payload)))
	copy(encoded[8:], compressed[:n])
	return encoded
}

func encodeJournalZSTD(t *testing.T, payload []byte) []byte {
	t.Helper()
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	defer encoder.Close()
	return encoder.EncodeAll(payload, nil)
}

type journalCountingReaderAt struct {
	io.ReaderAt
	bytesRead int
}

func (r *journalCountingReaderAt) ReadAt(destination []byte, offset int64) (int, error) {
	n, err := r.ReaderAt.ReadAt(destination, offset)
	r.bytesRead += n
	return n, err
}
