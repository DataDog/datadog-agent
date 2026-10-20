// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalDataIndexFindsExactPayload(t *testing.T) {
	for _, keyed := range []bool{false, true} {
		name := "jenkins"
		if keyed {
			name = "siphash"
		}
		t.Run(name, func(t *testing.T) {
			contents, offsets := testIndexedJournal(t, [][]byte{
				[]byte("_SYSTEMD_UNIT=other.service"),
				[]byte("_SYSTEMD_UNIT=api.service"),
			}, keyed)
			view, err := newJournalFileView(name+".journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)

			data, found, err := view.findDataObject([]byte("_SYSTEMD_UNIT=api.service"))
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, uint64(offsets[1]), data.offset)

			_, found, err = view.findDataObject([]byte("_SYSTEMD_UNIT=missing.service"))
			require.NoError(t, err)
			assert.False(t, found)
		})
	}
}

func TestJournalDataIndexVerifiesPayloadAfterHashMatch(t *testing.T) {
	target := []byte("_SYSTEMD_UNIT=api.service")
	contents, offsets := testIndexedJournal(t, [][]byte{
		[]byte("_SYSTEMD_UNIT=not-api.service"),
		target,
	}, false)
	targetHash := journalJenkinsHash64(target)
	binary.LittleEndian.PutUint64(contents[offsets[0]+16:offsets[0]+24], targetHash)

	view, err := newJournalFileView("collision.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, found, err := view.findDataObject(target)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, uint64(offsets[1]), data.offset)
}

func TestJournalDataIndexRejectsCycle(t *testing.T) {
	contents, offsets := testIndexedJournal(t, [][]byte{
		[]byte("_SYSTEMD_UNIT=one.service"),
		[]byte("_SYSTEMD_UNIT=two.service"),
		[]byte("_SYSTEMD_UNIT=three.service"),
	}, false)
	binary.LittleEndian.PutUint64(contents[offsets[1]+24:offsets[1]+32], uint64(offsets[0]))

	view, err := newJournalFileView("cycle.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	_, _, err = view.findDataObject([]byte("_SYSTEMD_UNIT=missing.service"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
	assert.Contains(t, err.Error(), "contains a cycle")
}

func TestJournalDataIndexRejectsWrongTableObject(t *testing.T) {
	contents, _ := testIndexedJournal(t, [][]byte{[]byte("MESSAGE=hello")}, false)
	contents[journalHeaderCurrentSize] = journalObjectFieldHashTable
	view, err := newJournalFileView("table.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)

	_, _, err = view.findDataObject([]byte("MESSAGE=hello"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
	assert.Contains(t, err.Error(), "expected object type 4, found 5")
}

func TestJournalDataIndexSkipsOversizedLZ4Collision(t *testing.T) {
	target := []byte("MESSAGE=hello")
	contents, offsets := testIndexedJournal(t, [][]byte{[]byte("MESSAGE=other"), target}, false)
	targetHash := journalJenkinsHash64(target)
	first := offsets[0]
	binary.LittleEndian.PutUint64(contents[first+16:first+24], targetHash)
	contents[first+1] = journalObjectCompressedLZ4
	binary.LittleEndian.PutUint32(contents[12:16], journalHeaderIncompatibleCompressedLZ4)
	binary.LittleEndian.PutUint64(contents[first+64:first+72], maxJournalLZ4DataSize+1)

	view, err := newJournalFileView("large-collision.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, found, err := view.findDataObject(target)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, uint64(offsets[1]), data.offset)
}

func TestJournalDataPayloadEqualDecodesCompressedValue(t *testing.T) {
	payload := []byte("_SYSTEMD_UNIT=api.service")
	encoded := encodeJournalZSTD(t, payload)
	contents, offset := testJournalDataContents(t, encoded, journalObjectCompressedZSTD, false)
	view, err := newJournalFileView("compressed-match.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(offset))
	require.NoError(t, err)

	equal, err := view.dataPayloadEqual(data, payload)
	require.NoError(t, err)
	assert.True(t, equal)
	equal, err = view.dataPayloadEqual(data, []byte("_SYSTEMD_UNIT=other.service"))
	require.NoError(t, err)
	assert.False(t, equal)
}

func testIndexedJournal(t *testing.T, payloads [][]byte, keyed bool) ([]byte, []int) {
	t.Helper()
	require.NotEmpty(t, payloads)

	const bucketCount = 1
	tableOffset := journalHeaderCurrentSize
	tableSize := journalObjectHeaderSize + bucketCount*journalHashItemSize
	nextOffset := alignJournalTestSize(tableOffset + tableSize)
	offsets := make([]int, len(payloads))
	for index, payload := range payloads {
		offsets[index] = nextOffset
		nextOffset = alignJournalTestSize(nextOffset + journalDataRegularHeaderSize + len(payload))
	}
	contents := testJournalContents(nextOffset, 0, 0)
	if keyed {
		binary.LittleEndian.PutUint32(contents[12:16], journalHeaderIncompatibleKeyedHash)
		for index := 0; index < 16; index++ {
			contents[24+index] = byte(index)
		}
	}
	binary.LittleEndian.PutUint64(contents[104:112], uint64(tableOffset+journalObjectHeaderSize))
	binary.LittleEndian.PutUint64(contents[112:120], bucketCount*journalHashItemSize)
	binary.LittleEndian.PutUint64(contents[136:144], uint64(offsets[len(offsets)-1]))
	binary.LittleEndian.PutUint64(contents[144:152], uint64(len(payloads)+1))

	table := contents[tableOffset:]
	table[0] = journalObjectDataHashTable
	binary.LittleEndian.PutUint64(table[8:16], uint64(tableSize))
	binary.LittleEndian.PutUint64(table[16:24], uint64(offsets[0]))
	binary.LittleEndian.PutUint64(table[24:32], uint64(offsets[len(offsets)-1]))

	var fileID journalID
	copy(fileID[:], contents[24:40])
	for index, payload := range payloads {
		offset := offsets[index]
		object := contents[offset:]
		object[0] = journalObjectData
		binary.LittleEndian.PutUint64(object[8:16], uint64(journalDataRegularHeaderSize+len(payload)))
		hash := journalJenkinsHash64(payload)
		if keyed {
			hash = journalSipHash24(payload, fileID)
		}
		binary.LittleEndian.PutUint64(object[16:24], hash)
		if index+1 < len(offsets) {
			binary.LittleEndian.PutUint64(object[24:32], uint64(offsets[index+1]))
		}
		copy(object[journalDataRegularHeaderSize:], payload)
	}
	return contents, offsets
}
