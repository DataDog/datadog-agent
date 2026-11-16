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

func TestJournalFileQuerySelectsDirectAndManagerUnitEntries(t *testing.T) {
	oldBoot := repeatedJournalID(0x11)
	currentBoot := repeatedJournalID(0x22)
	start := time.Unix(1_700_000_000, 0)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{
			bootID:   oldBoot,
			realtime: uint64(start.UnixMicro()),
			fields:   []string{"_SYSTEMD_UNIT=api.service", "_HOSTNAME=node", "_COMM=api", "_PID=10", "MESSAGE=old boot"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(time.Second).UnixMicro()),
			fields:   []string{"_SYSTEMD_UNIT=worker.service", "MESSAGE=worker"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(2 * time.Second).UnixMicro()),
			fields:   []string{"_SYSTEMD_UNIT=api.service", "_HOSTNAME=node", "SYSLOG_IDENTIFIER=api", "_PID=20", "MESSAGE=ready"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(3 * time.Second).UnixMicro()),
			fields:   []string{"UNIT=api.service", "_PID=1", "SYSLOG_IDENTIFIER=systemd", "MESSAGE=stopped"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(4 * time.Second).UnixMicro()),
			fields:   []string{"UNIT=api.service", "_PID=22", "MESSAGE=untrusted unit field"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(5 * time.Second).UnixMicro()),
			fields:   []string{"_SYSTEMD_UNIT=api.service", "UNIT=api.service", "_PID=1", "_COMM=systemd", "MESSAGE=deduplicated"},
		},
		{
			bootID:   currentBoot,
			realtime: uint64(start.Add(6 * time.Second).UnixMicro()),
			fields:   []string{"_TRANSPORT=kernel", "_HOSTNAME=node", "_COMM=kernel", "MESSAGE=kernel message"},
		},
	})
	view, err := newJournalFileView("query.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:       []string{"api.service"},
		CurrentBoot: true,
		Since:       start.Add(2 * time.Second),
		MaxEntries:  100,
	}, &currentBoot)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 3)
	assert.Equal(t, []string{"deduplicated", "stopped", "ready"}, []string{
		entries[0].selected.Message,
		entries[1].selected.Message,
		entries[2].selected.Message,
	})
	assert.Equal(t, "systemd", entries[0].selected.Identifier)
	assert.Equal(t, "systemd", entries[1].selected.Identifier)
	assert.Equal(t, "api", entries[2].selected.Identifier)
	assert.Equal(t, "node", entries[2].selected.Hostname)
	assert.Equal(t, "20", entries[2].selected.PID)
	assert.Equal(t, start.Add(2*time.Second), entries[2].selected.Timestamp)
}

func TestJournalFileQuerySelectsTrustedUnitRelatedEntries(t *testing.T) {
	bootID := repeatedJournalID(0x33)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=direct"}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{"UNIT=api.service", "_PID=1", "MESSAGE=pid one manager"}},
		{bootID: bootID, realtime: 1_700_000_000_000_002, fields: []string{"UNIT=api.service", "_SYSTEMD_CGROUP=/init.scope", "MESSAGE=init scope manager"}},
		{bootID: bootID, realtime: 1_700_000_000_000_003, fields: []string{"UNIT=api.service", "_PID=22", "MESSAGE=untrusted manager"}},
		{bootID: bootID, realtime: 1_700_000_000_000_004, fields: []string{"OBJECT_SYSTEMD_UNIT=api.service", "_UID=0", "MESSAGE=root object"}},
		{bootID: bootID, realtime: 1_700_000_000_000_005, fields: []string{"OBJECT_SYSTEMD_UNIT=api.service", "_UID=1000", "MESSAGE=untrusted object"}},
		{bootID: bootID, realtime: 1_700_000_000_000_006, fields: []string{"COREDUMP_UNIT=api.service", "_UID=0", "MESSAGE_ID=" + journalCoredumpMessageID, "MESSAGE=root coredump"}},
		{bootID: bootID, realtime: 1_700_000_000_000_007, fields: []string{"COREDUMP_UNIT=api.service", "_UID=1000", "MESSAGE_ID=" + journalCoredumpMessageID, "MESSAGE=untrusted coredump uid"}},
		{bootID: bootID, realtime: 1_700_000_000_000_008, fields: []string{"COREDUMP_UNIT=api.service", "_UID=0", "MESSAGE_ID=00000000000000000000000000000000", "MESSAGE=untrusted coredump id"}},
		{bootID: bootID, realtime: 1_700_000_000_000_009, fields: []string{"OBJECT_SYSTEMD_UNIT=worker.service", "_UID=0", "MESSAGE=other unit"}},
	})
	view, err := newJournalFileView("unit-related.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 10}, nil)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 5)
	assert.Equal(t, []string{"root coredump", "root object", "init scope manager", "pid one manager", "direct"}, []string{
		entries[0].selected.Message,
		entries[1].selected.Message,
		entries[2].selected.Message,
		entries[3].selected.Message,
		entries[4].selected.Message,
	})
}

func TestJournalFileQuerySelectsSliceEntries(t *testing.T) {
	bootID := repeatedJournalID(0x34)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_SLICE=workload.slice", "MESSAGE=slice member"}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{"_SYSTEMD_SLICE=other.slice", "MESSAGE=other slice"}},
	})
	view, err := newJournalFileView("slice.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{Units: []string{"workload.slice"}, MaxEntries: 10}, nil)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 1)
	assert.Equal(t, "slice member", entries[0].selected.Message)
}

// TestJournalFileQuerySliceGrantReturnsEveryMemberUnit pins the documented
// slice amplification: a read grant on a `.slice` unit returns the entries of
// every unit inside that slice, because journald stamps `_SYSTEMD_SLICE` on
// each member's entries. The match is exact rather than transitive, so a
// nested child slice's members are not reached by the parent grant. See the
// trusted systemd target exception in docs/RULES.md.
func TestJournalFileQuerySliceGrantReturnsEveryMemberUnit(t *testing.T) {
	bootID := repeatedJournalID(0x35)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{
			"_SYSTEMD_UNIT=api.service", "_SYSTEMD_SLICE=workload.slice", "MESSAGE=api member",
		}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{
			"_SYSTEMD_UNIT=db.service", "_SYSTEMD_SLICE=workload.slice", "MESSAGE=db member",
		}},
		{bootID: bootID, realtime: 1_700_000_000_000_002, fields: []string{
			"_SYSTEMD_UNIT=nested.service", "_SYSTEMD_SLICE=workload-nested.slice", "MESSAGE=nested member",
		}},
		{bootID: bootID, realtime: 1_700_000_000_000_003, fields: []string{
			"_SYSTEMD_UNIT=other.service", "_SYSTEMD_SLICE=other.slice", "MESSAGE=other member",
		}},
	})
	view, err := newJournalFileView("slice-membership.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)

	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:      []string{"workload.slice"},
		MaxEntries: 10,
	}, nil)
	require.NoError(t, err)
	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"db member", "api member"}, []string{entries[0].selected.Message, entries[1].selected.Message})

	nestedIterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:      []string{"workload-nested.slice"},
		MaxEntries: 10,
	}, nil)
	require.NoError(t, err)
	nestedEntries := collectJournalQueryEntries(t, nestedIterator)
	require.Len(t, nestedEntries, 1)
	assert.Equal(t, "nested member", nestedEntries[0].selected.Message)

	unitIterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:      []string{"api.service"},
		MaxEntries: 10,
	}, nil)
	require.NoError(t, err)
	unitEntries := collectJournalQueryEntries(t, unitIterator)
	require.Len(t, unitEntries, 1)
	assert.Equal(t, "api member", unitEntries[0].selected.Message)
}

func TestJournalFileQuerySelectsKernelEntries(t *testing.T) {
	bootID := repeatedJournalID(0x33)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=service"}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{"_TRANSPORT=kernel", "_COMM=kernel", "MESSAGE=kernel"}},
	})
	view, err := newJournalFileView("kernel.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Kernel:      true,
		CurrentBoot: true,
		MaxEntries:  10,
	}, &bootID)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 1)
	assert.Equal(t, "kernel", entries[0].selected.Message)
	assert.Equal(t, "kernel", entries[0].selected.Identifier)
}

func TestJournalFileQuerySupportsCompactKeyedLayout(t *testing.T) {
	bootID := repeatedJournalID(0x77)
	contents := buildJournalQueryFixtureWithLayout(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=first"}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=second"}},
	}, true)
	view, err := newJournalFileView("compact.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 10}, nil)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 2)
	assert.Equal(t, "second", entries[0].selected.Message)
	assert.Equal(t, "first", entries[1].selected.Message)
}

func TestJournalFileQueryStopsAtCurrentBootBoundaryBeforeScanLimit(t *testing.T) {
	oldBoot := repeatedJournalID(0x11)
	currentBoot := repeatedJournalID(0x22)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: oldBoot, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=oldest"}},
		{bootID: oldBoot, realtime: 1_700_000_000_000_001, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=old"}},
		{bootID: currentBoot, realtime: 1_700_000_000_000_002, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=current"}},
	})
	view, err := newJournalFileView("boot-boundary.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:       []string{"api.service"},
		CurrentBoot: true,
		MaxEntries:  10,
	}, &currentBoot)
	require.NoError(t, err)
	iterator.scanned = maxJournalCandidatesScanned - 2

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 1)
	assert.Equal(t, "current", entries[0].selected.Message)
	assert.Equal(t, maxJournalCandidatesScanned, iterator.scanned)
}

func TestJournalFileQueryContinuesPastRealtimeClockStep(t *testing.T) {
	bootID := repeatedJournalID(0x33)
	start := time.Unix(1_700_000_000, 0)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: uint64(start.Add(10 * time.Second).UnixMicro()), fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=before clock step"}},
		{bootID: bootID, realtime: uint64(start.Add(5 * time.Second).UnixMicro()), fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=before since"}},
		{bootID: bootID, realtime: uint64(start.Add(12 * time.Second).UnixMicro()), fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=latest"}},
	})
	view, err := newJournalFileView("realtime-clock-step.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:      []string{"api.service"},
		Since:      start.Add(8 * time.Second),
		MaxEntries: 10,
	}, nil)
	require.NoError(t, err)

	entries := collectJournalQueryEntries(t, iterator)
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"latest", "before clock step"}, []string{entries[0].selected.Message, entries[1].selected.Message})
}

func TestJournalFileQueryHonorsCancellation(t *testing.T) {
	bootID := repeatedJournalID(0x44)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=service"}},
	})
	view, err := newJournalFileView("cancel.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 1}, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = iterator.previous(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestJournalFileQueryRejectsCandidateMissingIndexedData(t *testing.T) {
	bootID := repeatedJournalID(0x66)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=service"}},
	})
	initial, err := newJournalFileView("broken-index.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	selector, found, err := initial.findDataObject([]byte("_SYSTEMD_UNIT=api.service"))
	require.NoError(t, err)
	require.True(t, found)
	message, found, err := initial.findDataObject([]byte("MESSAGE=service"))
	require.NoError(t, err)
	require.True(t, found)
	entry, err := initial.entryObjectAt(selector.entryOffset)
	require.NoError(t, err)
	for index, item := range entry.items {
		if item.dataOffset != selector.offset {
			continue
		}
		position := int(entry.offset) + journalEntryHeaderSize + index*journalEntryRegularItemSize
		binary.LittleEndian.PutUint64(contents[position:position+8], message.offset)
		binary.LittleEndian.PutUint64(contents[position+8:position+16], message.hash)
	}

	view, err := newJournalFileView("broken-index.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 1}, nil)
	require.NoError(t, err)
	_, _, err = iterator.previous(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
	assert.Contains(t, err.Error(), "does not reference the DATA object")
}

func collectJournalQueryEntries(t *testing.T, iterator *journalFileQueryIterator) []journalQueryEntry {
	t.Helper()
	var entries []journalQueryEntry
	for {
		entry, found, err := iterator.previous(context.Background())
		require.NoError(t, err)
		if !found {
			return entries
		}
		entries = append(entries, entry)
	}
}

type journalQueryFixtureEntry struct {
	seqnum    uint64
	bootID    journalID
	realtime  uint64
	monotonic uint64
	fields    []string
}

type journalQueryFixtureData struct {
	payload     []byte
	hash        uint64
	offset      int
	references  []int
	arrayOffset int
}

func buildJournalQueryFixture(t testing.TB, entries []journalQueryFixtureEntry) []byte {
	t.Helper()
	return buildJournalQueryFixtureWithLayout(t, entries, false)
}

func buildJournalQueryFixtureWithLayout(t testing.TB, entries []journalQueryFixtureEntry, compact bool) []byte {
	t.Helper()
	require.NotEmpty(t, entries)
	headerFlags := uint32(0)
	dataHeaderSize := journalDataRegularHeaderSize
	entryItemSize := journalEntryRegularItemSize
	arrayItemSize := journalEntryArrayRegularItemSize
	var fileID journalID
	for index := range fileID {
		fileID[index] = byte(index)
	}
	if compact {
		headerFlags = journalHeaderIncompatibleCompact | journalHeaderIncompatibleKeyedHash
		dataHeaderSize = journalDataCompactHeaderSize
		entryItemSize = journalEntryCompactItemSize
		arrayItemSize = journalEntryCompactItemSize
	}

	dataByValue := make(map[string]*journalQueryFixtureData)
	var dataObjects []*journalQueryFixtureData
	for entryIndex, entry := range entries {
		require.NotEmpty(t, entry.fields)
		for _, field := range entry.fields {
			data := dataByValue[field]
			if data == nil {
				payload := []byte(field)
				hash := journalJenkinsHash64(payload)
				if compact {
					hash = journalSipHash24(payload, fileID)
				}
				data = &journalQueryFixtureData{payload: payload, hash: hash}
				dataByValue[field] = data
				dataObjects = append(dataObjects, data)
			}
			data.references = append(data.references, entryIndex)
		}
	}

	const bucketCount = 64
	tableOffset := journalHeaderCurrentSize
	tableObjectSize := journalObjectHeaderSize + bucketCount*journalHashItemSize
	nextOffset := alignJournalTestSize(tableOffset + tableObjectSize)
	for _, data := range dataObjects {
		data.offset = nextOffset
		nextOffset = alignJournalTestSize(nextOffset + dataHeaderSize + len(data.payload))
	}
	entryOffsets := make([]int, len(entries))
	for index, entry := range entries {
		entryOffsets[index] = nextOffset
		nextOffset = alignJournalTestSize(nextOffset + journalEntryHeaderSize + len(entry.fields)*entryItemSize)
	}
	arrayCount := 0
	for _, data := range dataObjects {
		if len(data.references) <= 1 {
			continue
		}
		data.arrayOffset = nextOffset
		capacity := len(data.references) - 1
		if capacity < 4 {
			capacity = 4
		}
		nextOffset = alignJournalTestSize(nextOffset + journalEntryArrayHeaderSize + capacity*arrayItemSize)
		arrayCount++
	}

	contents := testJournalContents(nextOffset, 0, headerFlags)
	copy(contents[24:40], fileID[:])
	copy(contents[56:72], entries[len(entries)-1].bootID[:])
	copy(contents[72:88], bytes.Repeat([]byte{0x55}, 16))
	binary.LittleEndian.PutUint64(contents[104:112], uint64(tableOffset+journalObjectHeaderSize))
	binary.LittleEndian.PutUint64(contents[112:120], bucketCount*journalHashItemSize)
	binary.LittleEndian.PutUint64(contents[136:144], uint64(lastJournalFixtureObjectOffset(dataObjects, entryOffsets)))
	binary.LittleEndian.PutUint64(contents[144:152], uint64(1+len(dataObjects)+len(entries)+arrayCount))
	binary.LittleEndian.PutUint64(contents[152:160], uint64(len(entries)))
	binary.LittleEndian.PutUint64(contents[160:168], journalQueryFixtureSeqnum(entries[len(entries)-1], len(entries)-1))
	binary.LittleEndian.PutUint64(contents[168:176], journalQueryFixtureSeqnum(entries[0], 0))
	binary.LittleEndian.PutUint64(contents[184:192], entries[0].realtime)
	binary.LittleEndian.PutUint64(contents[192:200], entries[len(entries)-1].realtime)
	binary.LittleEndian.PutUint64(contents[264:272], uint64(entryOffsets[len(entryOffsets)-1]))

	table := contents[tableOffset:]
	table[0] = journalObjectDataHashTable
	binary.LittleEndian.PutUint64(table[8:16], uint64(tableObjectSize))
	buckets := make([][]*journalQueryFixtureData, bucketCount)
	for _, data := range dataObjects {
		bucket := data.hash % bucketCount
		buckets[bucket] = append(buckets[bucket], data)
	}
	for bucket, chain := range buckets {
		if len(chain) == 0 {
			continue
		}
		position := journalObjectHeaderSize + bucket*journalHashItemSize
		binary.LittleEndian.PutUint64(table[position:position+8], uint64(chain[0].offset))
		binary.LittleEndian.PutUint64(table[position+8:position+16], uint64(chain[len(chain)-1].offset))
		for index := 0; index+1 < len(chain); index++ {
			next := chain[index+1].offset
			binary.LittleEndian.PutUint64(contents[chain[index].offset+24:chain[index].offset+32], uint64(next))
		}
	}

	for _, data := range dataObjects {
		object := contents[data.offset:]
		object[0] = journalObjectData
		binary.LittleEndian.PutUint64(object[8:16], uint64(dataHeaderSize+len(data.payload)))
		binary.LittleEndian.PutUint64(object[16:24], data.hash)
		binary.LittleEndian.PutUint64(object[40:48], uint64(entryOffsets[data.references[0]]))
		binary.LittleEndian.PutUint64(object[48:56], uint64(data.arrayOffset))
		binary.LittleEndian.PutUint64(object[56:64], uint64(len(data.references)))
		copy(object[dataHeaderSize:], data.payload)

		if data.arrayOffset != 0 {
			capacity := len(data.references) - 1
			if capacity < 4 {
				capacity = 4
			}
			array := contents[data.arrayOffset:]
			array[0] = journalObjectEntryArray
			binary.LittleEndian.PutUint64(array[8:16], uint64(journalEntryArrayHeaderSize+capacity*arrayItemSize))
			if compact {
				binary.LittleEndian.PutUint32(object[64:68], uint32(data.arrayOffset))
				binary.LittleEndian.PutUint32(object[68:72], uint32(len(data.references)-1))
			}
			for index, entryIndex := range data.references[1:] {
				position := journalEntryArrayHeaderSize + index*arrayItemSize
				if compact {
					binary.LittleEndian.PutUint32(array[position:position+4], uint32(entryOffsets[entryIndex]))
				} else {
					binary.LittleEndian.PutUint64(array[position:position+8], uint64(entryOffsets[entryIndex]))
				}
			}
		}
	}

	for index, entrySpec := range entries {
		entry := contents[entryOffsets[index]:]
		entry[0] = journalObjectEntry
		binary.LittleEndian.PutUint64(entry[8:16], uint64(journalEntryHeaderSize+len(entrySpec.fields)*entryItemSize))
		seqnum := journalQueryFixtureSeqnum(entrySpec, index)
		monotonic := entrySpec.monotonic
		if monotonic == 0 {
			monotonic = seqnum * 100
		}
		binary.LittleEndian.PutUint64(entry[16:24], seqnum)
		binary.LittleEndian.PutUint64(entry[24:32], entrySpec.realtime)
		binary.LittleEndian.PutUint64(entry[32:40], monotonic)
		copy(entry[40:56], entrySpec.bootID[:])
		var xorHash uint64
		for itemIndex, field := range entrySpec.fields {
			data := dataByValue[field]
			position := journalEntryHeaderSize + itemIndex*entryItemSize
			if compact {
				binary.LittleEndian.PutUint32(entry[position:position+4], uint32(data.offset))
			} else {
				binary.LittleEndian.PutUint64(entry[position:position+8], uint64(data.offset))
				binary.LittleEndian.PutUint64(entry[position+8:position+16], data.hash)
			}
			xorHash ^= journalJenkinsHash64(data.payload)
		}
		binary.LittleEndian.PutUint64(entry[56:64], xorHash)
	}
	return contents
}

func journalQueryFixtureSeqnum(entry journalQueryFixtureEntry, index int) uint64 {
	if entry.seqnum != 0 {
		return entry.seqnum
	}
	return uint64(index + 1)
}

func lastJournalFixtureObjectOffset(data []*journalQueryFixtureData, entryOffsets []int) int {
	last := entryOffsets[len(entryOffsets)-1]
	for _, object := range data {
		if object.arrayOffset > last {
			last = object.arrayOffset
		}
	}
	return last
}

func repeatedJournalID(value byte) journalID {
	var id journalID
	for index := range id {
		id[index] = value
	}
	return id
}
