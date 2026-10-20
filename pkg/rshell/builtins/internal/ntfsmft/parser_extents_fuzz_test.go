// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Fuzz tests for the $MFT extent-resolution code. Like the record parser
// (parser_fuzz_test.go) these run on every platform — disk I/O is injected via
// the readerAt seam — and must never panic on hostile on-disk bytes: crafted
// data runs, malformed $ATTRIBUTE_LIST entries, and record 0 itself.
package ntfsmft

import "testing"

// fuzzExtentDisk is a valid disk image with record 0 at offset 0, an inline
// $DATA run (LCN 0), a non-resident $ATTRIBUTE_LIST whose content sits at
// LCN 3 and points at record 1, and record 1 holding another $DATA run. It
// seeds the whole getMFTExtents chase path.
func fuzzExtentDisk() []byte {
	disk := make([]byte, 16384)
	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 0)),             // MFT self-location (LCN 0)
		nonResidentAttr(attrAttributeList, 0x18, singleRun(1, 3)), // attr-list content at LCN 3
	)
	rec1 := assembleRecord(nonResidentAttr(attrData, 0, singleRun(5, 50)))
	copy(disk[0:], rec0)
	copy(disk[testExtRecordSize:], rec1)
	copy(disk[3*testBytesPerClus:], attrListEntryBytes(attrData, 1))
	return disk
}

func fuzzExtentVol() *volumeInfo {
	return &volumeInfo{
		recordSize:      testExtRecordSize,
		bytesPerCluster: testBytesPerClus,
		totalClusters:   10000,
		mftStartByte:    0,
	}
}

// FuzzGetMFTExtents drives the whole record-0 walk (inline $DATA, resident and
// non-resident $ATTRIBUTE_LIST, extension chase) over arbitrary disk bytes.
func FuzzGetMFTExtents(f *testing.F) {
	f.Add(fuzzExtentDisk())
	f.Add([]byte{})
	f.Add(make([]byte, testExtRecordSize)) // valid size, no signature
	allFF := make([]byte, testExtRecordSize)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 { // cap at 1 MiB
			return
		}
		// A panic fails the fuzz test; errors are expected and fine.
		_, _, _ = getMFTExtents(memReader(data), fuzzExtentVol())
	})
}

// FuzzDecodeDataRuns verifies the data-run decoder never panics or reads out
// of bounds on hostile run lists (bad length/offset sizes, truncation).
func FuzzDecodeDataRuns(f *testing.F) {
	f.Add(singleRun(2, 1))
	f.Add([]byte{0x11, 0x02, 0x01, 0x11, 0x03, 0x02, 0x00})
	f.Add([]byte{0x01, 0x04, 0x11, 0x01, 0x05, 0x00}) // sparse then normal
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		_, _ = decodeDataRuns(data, testBytesPerClus, 10000)
	})
}

// FuzzParseAttributeList verifies both the resident wrapper and the raw entry
// walk never panic on hostile $ATTRIBUTE_LIST bytes.
func FuzzParseAttributeList(f *testing.F) {
	f.Add(residentAttr(attrAttributeList, attrListEntryBytes(attrData, 3)))
	f.Add(nonResidentAttr(attrAttributeList, 0x18, singleRun(1, 3)))
	f.Add(attrListEntryBytes(attrData, 7))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		_ = parseAttributeList(data)
		_ = parseAttributeListEntries(data)
	})
}
