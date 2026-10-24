// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalFileViewParsesCurrentHeader(t *testing.T) {
	contents := testJournalContents(journalHeaderCurrentSize, 0, 0)
	copy(contents[24:40], bytes.Repeat([]byte{0x11}, 16))
	copy(contents[40:56], bytes.Repeat([]byte{0x22}, 16))
	copy(contents[56:72], bytes.Repeat([]byte{0x33}, 16))
	copy(contents[72:88], bytes.Repeat([]byte{0x44}, 16))
	binary.LittleEndian.PutUint64(contents[152:160], 7)
	binary.LittleEndian.PutUint64(contents[160:168], 11)
	binary.LittleEndian.PutUint64(contents[168:176], 5)
	binary.LittleEndian.PutUint64(contents[184:192], 1_700_000_000_000_000)
	binary.LittleEndian.PutUint64(contents[192:200], 1_700_000_000_000_500)

	view, err := newJournalFileView("fixture.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)

	assert.Equal(t, journalStateArchived, int(view.header.state))
	assert.Equal(t, "11111111111111111111111111111111", view.header.fileID.String())
	assert.Equal(t, "22222222222222222222222222222222", view.header.machineID.String())
	assert.Equal(t, "33333333333333333333333333333333", view.header.tailEntryBootID.String())
	assert.Equal(t, "44444444444444444444444444444444", view.header.seqnumID.String())
	assert.Equal(t, uint64(7), view.header.nEntries)
	assert.Equal(t, uint64(11), view.header.tailEntrySeqnum)
	assert.Equal(t, uint64(5), view.header.headEntrySeqnum)
	assert.Equal(t, uint64(1_700_000_000_000_000), view.header.headEntryRealtime)
	assert.Equal(t, uint64(1_700_000_000_000_500), view.header.tailEntryRealtime)
	assert.True(t, view.header.hasTailEntryArray)
	assert.True(t, view.header.hasTailEntryOffset)
	assert.False(t, view.header.compact())
	assert.False(t, view.header.keyedHash())
}

func TestJournalFileViewAcceptsMinimumAndFutureHeaders(t *testing.T) {
	minimum := testJournalContents(journalHeaderMinSize, 0, 0)
	minimumView, err := newJournalFileView("old.journal", bytes.NewReader(minimum), uint64(len(minimum)))
	require.NoError(t, err)
	assert.False(t, minimumView.header.hasTailEntryArray)
	assert.False(t, minimumView.header.hasTailEntryOffset)

	future := testJournalContentsWithHeader(280, 280, 0, journalHeaderIncompatibleCompact|journalHeaderIncompatibleKeyedHash)
	futureView, err := newJournalFileView("future.journal", bytes.NewReader(future), uint64(len(future)))
	require.NoError(t, err)
	assert.True(t, futureView.header.compact())
	assert.True(t, futureView.header.keyedHash())
}

func TestJournalFileViewRejectsInvalidHeaders(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte)
		match  string
		target error
	}{
		{
			name: "signature",
			mutate: func(contents []byte) {
				contents[0] = 0
			},
			match:  "invalid journal signature",
			target: errJournalCorrupt,
		},
		{
			name: "unknown incompatible flag",
			mutate: func(contents []byte) {
				binary.LittleEndian.PutUint32(contents[12:16], 1<<31)
			},
			match:  "unknown incompatible feature flags 0x80000000",
			target: errJournalUnsupported,
		},
		{
			name: "invalid state",
			mutate: func(contents []byte) {
				contents[16] = 3
			},
			match:  "invalid journal state 3",
			target: errJournalCorrupt,
		},
		{
			name: "short header size",
			mutate: func(contents []byte) {
				binary.LittleEndian.PutUint64(contents[88:96], journalHeaderMinSize-8)
			},
			match:  "header size 200 is smaller than 208",
			target: errJournalCorrupt,
		},
		{
			name: "unaligned header size",
			mutate: func(contents []byte) {
				binary.LittleEndian.PutUint64(contents[88:96], journalHeaderCurrentSize+1)
			},
			match:  "header size 273 is not 8-byte aligned",
			target: errJournalCorrupt,
		},
		{
			name: "arena beyond file",
			mutate: func(contents []byte) {
				binary.LittleEndian.PutUint64(contents[96:104], 8)
			},
			match:  "journal arena ends at 280 beyond the 272-byte file",
			target: errJournalCorrupt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contents := testJournalContents(journalHeaderCurrentSize, 0, 0)
			test.mutate(contents)
			_, err := newJournalFileView("bad.journal", bytes.NewReader(contents), uint64(len(contents)))
			require.Error(t, err)
			assert.ErrorIs(t, err, test.target)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}

func TestJournalFileViewValidatesHeaderOffsets(t *testing.T) {
	contents := testJournalContents(journalHeaderCurrentSize+64, 0, 0)
	binary.LittleEndian.PutUint64(contents[136:144], journalHeaderCurrentSize)
	binary.LittleEndian.PutUint64(contents[176:184], journalHeaderCurrentSize+1)

	_, err := newJournalFileView("bad-offset.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
	assert.Contains(t, err.Error(), "entry array offset is not 8-byte aligned")
}

func TestJournalFileViewReadsBoundedObjectHeader(t *testing.T) {
	contents := testJournalContents(journalHeaderCurrentSize+64, 0, 0)
	binary.LittleEndian.PutUint64(contents[136:144], journalHeaderCurrentSize)
	contents[journalHeaderCurrentSize] = journalObjectEntry
	binary.LittleEndian.PutUint64(contents[journalHeaderCurrentSize+8:journalHeaderCurrentSize+16], 64)

	view, err := newJournalFileView("entry.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	object, err := view.objectAt(journalHeaderCurrentSize, journalObjectEntry)
	require.NoError(t, err)
	assert.Equal(t, uint8(journalObjectEntry), object.objectType)
	assert.Equal(t, uint64(64), object.size)

	_, err = view.objectAt(journalHeaderCurrentSize, journalObjectData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected object type 1, found 3")
}

func TestJournalFileViewRejectsInvalidObjectHeaders(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte)
		match  string
		target error
	}{
		{
			name: "unknown flags",
			mutate: func(object []byte) {
				object[1] = 1 << 7
			},
			match:  "unknown flags 0x80",
			target: errJournalUnsupported,
		},
		{
			name: "compressed non-data",
			mutate: func(object []byte) {
				object[1] = journalObjectCompressedZSTD
			},
			match:  "compression flags are set on non-DATA object",
			target: errJournalCorrupt,
		},
		{
			name: "undeclared compression",
			mutate: func(object []byte) {
				object[0] = journalObjectData
				object[1] = journalObjectCompressedZSTD
			},
			match:  "ZSTD-compressed DATA object is not declared",
			target: errJournalCorrupt,
		},
		{
			name: "multiple compression algorithms",
			mutate: func(object []byte) {
				object[0] = journalObjectData
				object[1] = journalObjectCompressedLZ4 | journalObjectCompressedZSTD
			},
			match:  "multiple compression flags",
			target: errJournalCorrupt,
		},
		{
			name: "short object size",
			mutate: func(object []byte) {
				binary.LittleEndian.PutUint64(object[8:16], journalObjectHeaderSize-1)
			},
			match:  "object size 15 is smaller than its header",
			target: errJournalCorrupt,
		},
		{
			name: "object beyond arena",
			mutate: func(object []byte) {
				binary.LittleEndian.PutUint64(object[8:16], 65)
			},
			match:  "object size 65 extends beyond the journal arena",
			target: errJournalCorrupt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contents := testJournalContents(journalHeaderCurrentSize+64, 0, 0)
			binary.LittleEndian.PutUint64(contents[136:144], journalHeaderCurrentSize)
			object := contents[journalHeaderCurrentSize:]
			object[0] = journalObjectEntry
			binary.LittleEndian.PutUint64(object[8:16], 64)
			test.mutate(object)

			view, err := newJournalFileView("bad-object.journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)
			_, err = view.objectAt(journalHeaderCurrentSize, 0)
			require.Error(t, err)
			assert.True(t, errors.Is(err, test.target))
			assert.Contains(t, err.Error(), test.match)
		})
	}
}

func testJournalContents(size int, compatibleFlags, incompatibleFlags uint32) []byte {
	headerSize := size
	if headerSize > journalHeaderCurrentSize {
		headerSize = journalHeaderCurrentSize
	}
	return testJournalContentsWithHeader(size, headerSize, compatibleFlags, incompatibleFlags)
}

func testJournalContentsWithHeader(size, headerSize int, compatibleFlags, incompatibleFlags uint32) []byte {
	contents := make([]byte, size)
	copy(contents[:8], journalSignature[:])
	binary.LittleEndian.PutUint32(contents[8:12], compatibleFlags)
	binary.LittleEndian.PutUint32(contents[12:16], incompatibleFlags)
	contents[16] = journalStateArchived
	binary.LittleEndian.PutUint64(contents[88:96], uint64(headerSize))
	binary.LittleEndian.PutUint64(contents[96:104], uint64(size-headerSize))
	return contents
}
