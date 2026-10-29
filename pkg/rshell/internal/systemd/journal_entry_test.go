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

func TestJournalEntryObjectParsesRegularAndCompactItems(t *testing.T) {
	for _, compact := range []bool{false, true} {
		name := "regular"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			contents, entryOffset, dataOffset := testJournalEntryContents(compact)
			view, err := newJournalFileView(name+".journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)

			entry, err := view.entryObjectAt(uint64(entryOffset))
			require.NoError(t, err)
			assert.Equal(t, uint64(42), entry.seqnum)
			assert.Equal(t, uint64(1_700_000_000_000_000), entry.realtime)
			assert.Equal(t, uint64(1234), entry.monotonic)
			assert.Equal(t, "11111111111111111111111111111111", entry.bootID.String())
			require.Len(t, entry.items, 1)
			assert.Equal(t, uint64(dataOffset), entry.items[0].dataOffset)
			if compact {
				assert.Zero(t, entry.items[0].hash)
			} else {
				assert.Equal(t, uint64(0x0102030405060708), entry.items[0].hash)
			}
		})
	}
}

func TestJournalEntryOffsetIteratorReadsNewestFirst(t *testing.T) {
	for _, compact := range []bool{false, true} {
		name := "regular"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			contents, dataOffset, entryOffsets := testJournalEntryArrayContents(compact)
			view, err := newJournalFileView(name+"-array.journal", bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)
			data, err := view.dataObjectAt(uint64(dataOffset))
			require.NoError(t, err)
			iterator, err := view.entryOffsetsForData(data)
			require.NoError(t, err)

			var actual []uint64
			for {
				offset, found, err := iterator.previous()
				require.NoError(t, err)
				if !found {
					break
				}
				actual = append(actual, offset)
			}
			expected := make([]uint64, len(entryOffsets))
			for index := range entryOffsets {
				expected[index] = uint64(entryOffsets[len(entryOffsets)-1-index])
			}
			assert.Equal(t, expected, actual)
		})
	}
}

func TestJournalEntryOffsetIteratorHandlesUnreferencedData(t *testing.T) {
	contents, dataOffset := testJournalDataContents(t, []byte("MESSAGE=unused"), 0, false)
	view, err := newJournalFileView("unused.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(dataOffset))
	require.NoError(t, err)
	iterator, err := view.entryOffsetsForData(data)
	require.NoError(t, err)

	_, found, err := iterator.previous()
	require.NoError(t, err)
	assert.False(t, found)
}

func TestJournalEntryOffsetIteratorRejectsUnsortedItems(t *testing.T) {
	contents, dataOffset, _ := testJournalEntryArrayContents(false)
	firstArrayOffset := binary.LittleEndian.Uint64(contents[dataOffset+48 : dataOffset+56])
	firstItem := firstArrayOffset + journalEntryArrayHeaderSize
	first := binary.LittleEndian.Uint64(contents[firstItem : firstItem+8])
	binary.LittleEndian.PutUint64(contents[firstItem+8:firstItem+16], first-8)

	view, err := newJournalFileView("unsorted.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, err := view.dataObjectAt(uint64(dataOffset))
	require.NoError(t, err)
	iterator, err := view.entryOffsetsForData(data)
	require.NoError(t, err)

	for {
		_, found, err := iterator.previous()
		if err != nil {
			assert.ErrorIs(t, err, errJournalCorrupt)
			assert.Contains(t, err.Error(), "not strictly increasing")
			break
		}
		require.True(t, found)
	}
}

func testJournalEntryContents(compact bool) ([]byte, int, int) {
	headerFlags := uint32(0)
	dataHeaderSize := journalDataRegularHeaderSize
	itemSize := journalEntryRegularItemSize
	if compact {
		headerFlags = journalHeaderIncompatibleCompact
		dataHeaderSize = journalDataCompactHeaderSize
		itemSize = journalEntryCompactItemSize
	}
	dataOffset := journalHeaderCurrentSize
	entryOffset := alignJournalTestSize(dataOffset + dataHeaderSize)
	fileSize := alignJournalTestSize(entryOffset + journalEntryHeaderSize + itemSize)
	contents := testJournalContents(fileSize, 0, headerFlags)
	binary.LittleEndian.PutUint64(contents[136:144], uint64(entryOffset))
	binary.LittleEndian.PutUint64(contents[152:160], 1)

	data := contents[dataOffset:]
	data[0] = journalObjectData
	binary.LittleEndian.PutUint64(data[8:16], uint64(dataHeaderSize))

	entry := contents[entryOffset:]
	entry[0] = journalObjectEntry
	binary.LittleEndian.PutUint64(entry[8:16], uint64(journalEntryHeaderSize+itemSize))
	binary.LittleEndian.PutUint64(entry[16:24], 42)
	binary.LittleEndian.PutUint64(entry[24:32], 1_700_000_000_000_000)
	binary.LittleEndian.PutUint64(entry[32:40], 1234)
	copy(entry[40:56], bytes.Repeat([]byte{0x11}, 16))
	if compact {
		binary.LittleEndian.PutUint32(entry[64:68], uint32(dataOffset))
	} else {
		binary.LittleEndian.PutUint64(entry[64:72], uint64(dataOffset))
		binary.LittleEndian.PutUint64(entry[72:80], 0x0102030405060708)
	}
	return contents, entryOffset, dataOffset
}

func testJournalEntryArrayContents(compact bool) ([]byte, int, []int) {
	headerFlags := uint32(0)
	dataHeaderSize := journalDataRegularHeaderSize
	arrayItemSize := journalEntryArrayRegularItemSize
	if compact {
		headerFlags = journalHeaderIncompatibleCompact
		dataHeaderSize = journalDataCompactHeaderSize
		arrayItemSize = journalEntryCompactItemSize
	}
	dataOffset := journalHeaderCurrentSize
	nextOffset := alignJournalTestSize(dataOffset + dataHeaderSize)
	entryOffsets := make([]int, 6)
	for index := range entryOffsets {
		entryOffsets[index] = nextOffset
		nextOffset = alignJournalTestSize(nextOffset + journalEntryHeaderSize)
	}
	firstArrayOffset := nextOffset
	firstArrayCapacity := 4
	nextOffset = alignJournalTestSize(nextOffset + journalEntryArrayHeaderSize + firstArrayCapacity*arrayItemSize)
	secondArrayOffset := nextOffset
	secondArrayCapacity := 4
	nextOffset = alignJournalTestSize(nextOffset + journalEntryArrayHeaderSize + secondArrayCapacity*arrayItemSize)

	contents := testJournalContents(nextOffset, 0, headerFlags)
	binary.LittleEndian.PutUint64(contents[136:144], uint64(secondArrayOffset))
	binary.LittleEndian.PutUint64(contents[144:152], 9)
	binary.LittleEndian.PutUint64(contents[152:160], uint64(len(entryOffsets)))
	data := contents[dataOffset:]
	data[0] = journalObjectData
	binary.LittleEndian.PutUint64(data[8:16], uint64(dataHeaderSize))
	binary.LittleEndian.PutUint64(data[40:48], uint64(entryOffsets[0]))
	binary.LittleEndian.PutUint64(data[48:56], uint64(firstArrayOffset))
	binary.LittleEndian.PutUint64(data[56:64], uint64(len(entryOffsets)))
	if compact {
		binary.LittleEndian.PutUint32(data[64:68], uint32(secondArrayOffset))
		binary.LittleEndian.PutUint32(data[68:72], 1)
	}
	for _, offset := range entryOffsets {
		entry := contents[offset:]
		entry[0] = journalObjectEntry
		binary.LittleEndian.PutUint64(entry[8:16], journalEntryHeaderSize)
	}

	firstArray := contents[firstArrayOffset:]
	firstArray[0] = journalObjectEntryArray
	binary.LittleEndian.PutUint64(firstArray[8:16], uint64(journalEntryArrayHeaderSize+firstArrayCapacity*arrayItemSize))
	binary.LittleEndian.PutUint64(firstArray[16:24], uint64(secondArrayOffset))
	secondArray := contents[secondArrayOffset:]
	secondArray[0] = journalObjectEntryArray
	binary.LittleEndian.PutUint64(secondArray[8:16], uint64(journalEntryArrayHeaderSize+secondArrayCapacity*arrayItemSize))
	for index, offset := range entryOffsets[1:] {
		array := firstArray
		itemIndex := index
		if index >= firstArrayCapacity {
			array = secondArray
			itemIndex -= firstArrayCapacity
		}
		position := journalEntryArrayHeaderSize + itemIndex*arrayItemSize
		if compact {
			binary.LittleEndian.PutUint32(array[position:position+4], uint32(offset))
		} else {
			binary.LittleEndian.PutUint64(array[position:position+8], uint64(offset))
		}
	}
	return contents, dataOffset, entryOffsets
}
