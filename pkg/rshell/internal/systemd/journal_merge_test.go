// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestQueryJournalEntriesMergesRotatedFilesAndFindsCurrentBoot(t *testing.T) {
	client, journalDir, machineID := newJournalSnapshotTestClient(t)
	sequenceID := repeatedJournalID(0x44)
	oldBoot := repeatedJournalID(0x11)
	currentBoot := repeatedJournalID(0x22)
	start := time.Unix(1_700_000_000, 0)

	writeJournalSnapshotFixture(t, journalDir, "system@old.journal", buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{seqnum: 1, bootID: oldBoot, realtime: uint64(start.UnixMicro()), monotonic: 900, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=old boot"}},
		{seqnum: 2, bootID: currentBoot, realtime: uint64(start.Add(time.Second).UnixMicro()), monotonic: 100, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=started"}},
	}), machineID, repeatedJournalID(0x31), sequenceID)
	writeJournalSnapshotFixture(t, journalDir, "system.journal", buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{seqnum: 3, bootID: currentBoot, realtime: uint64(start.Add(2 * time.Second).UnixMicro()), monotonic: 200, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=ready"}},
		{seqnum: 4, bootID: currentBoot, realtime: uint64(start.Add(3 * time.Second).UnixMicro()), monotonic: 300, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=healthy"}},
	}), machineID, repeatedJournalID(0x32), sequenceID)

	entries, err := client.queryJournalEntries(context.Background(), builtins.JournalQuery{
		Units:       []string{"api.service"},
		CurrentBoot: true,
		MaxEntries:  10,
	})
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, []string{"healthy", "ready", "started"}, journalMessages(entries))
}

func TestQueryJournalEntriesHandlesDuplicateSequenceNumbers(t *testing.T) {
	bootID := repeatedJournalID(0x22)
	sequenceID := repeatedJournalID(0x44)
	start := time.Unix(1_700_000_000, 0)

	t.Run("different locations remain visible", func(t *testing.T) {
		client, journalDir, machineID := newJournalSnapshotTestClient(t)
		writeJournalSnapshotFixture(t, journalDir, "one.journal", buildJournalQueryFixture(t, []journalQueryFixtureEntry{
			{seqnum: 5, bootID: bootID, realtime: uint64(start.UnixMicro()), monotonic: 100, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=first"}},
		}), machineID, repeatedJournalID(0x31), sequenceID)
		writeJournalSnapshotFixture(t, journalDir, "two.journal", buildJournalQueryFixture(t, []journalQueryFixtureEntry{
			{seqnum: 5, bootID: bootID, realtime: uint64(start.Add(time.Second).UnixMicro()), monotonic: 200, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=second"}},
		}), machineID, repeatedJournalID(0x32), sequenceID)

		entries, err := client.queryJournalEntries(context.Background(), builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 10})
		require.NoError(t, err)
		assert.Equal(t, []string{"second", "first"}, journalMessages(entries))
	})

	t.Run("same location is deduplicated", func(t *testing.T) {
		client, journalDir, machineID := newJournalSnapshotTestClient(t)
		spec := []journalQueryFixtureEntry{
			{seqnum: 5, bootID: bootID, realtime: uint64(start.UnixMicro()), monotonic: 100, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=duplicate"}},
		}
		writeJournalSnapshotFixture(t, journalDir, "one.journal", buildJournalQueryFixture(t, spec), machineID, repeatedJournalID(0x31), sequenceID)
		writeJournalSnapshotFixture(t, journalDir, "two.journal", buildJournalQueryFixture(t, spec), machineID, repeatedJournalID(0x32), sequenceID)

		entries, err := client.queryJournalEntries(context.Background(), builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 10})
		require.NoError(t, err)
		assert.Equal(t, []string{"duplicate"}, journalMessages(entries))
	})
}

func TestNewestJournalEntrySupportsLegacyHeader(t *testing.T) {
	bootID := repeatedJournalID(0x22)
	contents := buildLegacyJournalTailFixture(bootID)
	view, err := newJournalFileView("legacy.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	require.False(t, view.header.hasTailEntryOffset)

	entry, found, err := newestJournalEntry(view)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, uint64(3), entry.seqnum)
	assert.Equal(t, bootID, entry.bootID)
}

func journalMessages(entries []journalQueryEntry) []string {
	messages := make([]string, len(entries))
	for index, entry := range entries {
		messages[index] = entry.selected.Message
	}
	return messages
}

func buildLegacyJournalTailFixture(bootID journalID) []byte {
	const entryCount = 3
	payload := []byte("MESSAGE=x")
	hash := journalJenkinsHash64(payload)
	dataOffset := journalHeaderMinSize
	dataSize := journalDataRegularHeaderSize + len(payload)
	arrayOffset := alignJournalTestSize(dataOffset + dataSize)
	arraySize := journalEntryArrayHeaderSize + entryCount*journalEntryArrayRegularItemSize
	nextOffset := alignJournalTestSize(arrayOffset + arraySize)
	entryOffsets := make([]int, entryCount)
	for index := range entryOffsets {
		entryOffsets[index] = nextOffset
		nextOffset = alignJournalTestSize(nextOffset + journalEntryHeaderSize + journalEntryRegularItemSize)
	}

	contents := testJournalContentsWithHeader(nextOffset, journalHeaderMinSize, 0, 0)
	fileID := repeatedJournalID(0x31)
	sequenceID := repeatedJournalID(0x44)
	copy(contents[24:40], fileID[:])
	copy(contents[56:72], bootID[:])
	copy(contents[72:88], sequenceID[:])
	binary.LittleEndian.PutUint64(contents[136:144], uint64(entryOffsets[len(entryOffsets)-1]))
	binary.LittleEndian.PutUint64(contents[144:152], 2+entryCount)
	binary.LittleEndian.PutUint64(contents[152:160], entryCount)
	binary.LittleEndian.PutUint64(contents[160:168], entryCount)
	binary.LittleEndian.PutUint64(contents[168:176], 1)
	binary.LittleEndian.PutUint64(contents[176:184], uint64(arrayOffset))
	binary.LittleEndian.PutUint64(contents[184:192], 1_700_000_000_000_001)
	binary.LittleEndian.PutUint64(contents[192:200], 1_700_000_000_000_003)

	data := contents[dataOffset:]
	data[0] = journalObjectData
	binary.LittleEndian.PutUint64(data[8:16], uint64(dataSize))
	binary.LittleEndian.PutUint64(data[16:24], hash)
	copy(data[journalDataRegularHeaderSize:], payload)

	array := contents[arrayOffset:]
	array[0] = journalObjectEntryArray
	binary.LittleEndian.PutUint64(array[8:16], uint64(arraySize))
	for index, offset := range entryOffsets {
		position := journalEntryArrayHeaderSize + index*journalEntryArrayRegularItemSize
		binary.LittleEndian.PutUint64(array[position:position+8], uint64(offset))
	}

	for index, offset := range entryOffsets {
		entry := contents[offset:]
		entry[0] = journalObjectEntry
		binary.LittleEndian.PutUint64(entry[8:16], journalEntryHeaderSize+journalEntryRegularItemSize)
		binary.LittleEndian.PutUint64(entry[16:24], uint64(index+1))
		binary.LittleEndian.PutUint64(entry[24:32], uint64(1_700_000_000_000_001+index))
		binary.LittleEndian.PutUint64(entry[32:40], uint64(100+index))
		copy(entry[40:56], bootID[:])
		binary.LittleEndian.PutUint64(entry[56:64], hash)
		binary.LittleEndian.PutUint64(entry[64:72], uint64(dataOffset))
		binary.LittleEndian.PutUint64(entry[72:80], hash)
	}
	return contents
}
