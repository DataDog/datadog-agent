// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tests for the $MFT extent-resolution code (getMFTExtents and its pure
// helpers: decodeDataRuns, parseAttributeList / parseAttributeListEntries,
// readNonResidentAttrList). This code parses raw on-disk bytes and only needs
// disk I/O, which is injected via the readerAt seam, so it runs on any OS.
//
// The interesting path is a fragmented $MFT whose record 0 carries an
// $ATTRIBUTE_LIST pointing at extension records that hold more $DATA runs. The
// rare-but-real non-resident $ATTRIBUTE_LIST variant (only seen on badly
// fragmented / near-full volumes — exactly this tool's target) cannot be
// exercised on a CI NTFS volume, so these synthetic tests are its only
// automated coverage.
package ntfsmft

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
)

const (
	testExtRecordSize = 1024
	testBytesPerClus  = 4096
)

// memReader returns a readerAt over an in-memory disk image. Out-of-range
// reads error (never panic), matching the real ReadFile wrapper's short-read
// behavior.
func memReader(disk []byte) readerAt {
	return func(buf []byte, off int64) error {
		if off < 0 || off+int64(len(buf)) > int64(len(disk)) {
			return fmt.Errorf("read [%d,%d) out of range (disk %d)", off, off+int64(len(buf)), len(disk))
		}
		copy(buf, disk[off:off+int64(len(buf))])
		return nil
	}
}

// singleRun encodes one NTFS data run with a 1-byte length and 1-byte signed
// LCN delta, followed by the 0x00 terminator.
func singleRun(runLen, lcnDelta byte) []byte {
	return []byte{0x11, runLen, lcnDelta, 0x00}
}

// writeRecordHeader stamps a valid "FILE" header with a 3-word fixup array
// (1 USN + 2 sectors) whose sector-end USNs match, so applyFixups accepts the
// record. firstAttrOff is where the attribute chain begins.
func writeRecordHeader(rec []byte, firstAttrOff int) {
	binary := leWriter{rec}
	binary.u32(0, mftSignature)
	binary.u16(4, 0x30) // fixup (USA) offset
	binary.u16(6, 3)    // fixup count: 1 USN + 2 sectors
	binary.u16(0x14, uint16(firstAttrOff))
	binary.u16(0x16, flagInUse)
	binary.u16(0x30, 0xCAFE)  // USN
	binary.u16(0x1FE, 0xCAFE) // sector 1 end
	binary.u16(0x3FE, 0xCAFE) // sector 2 end
}

// nonResidentAttr builds a non-resident attribute: MappingPairsOffset at 0x20
// points at drOff (0x40, just past the header), real size at 0x30, and the
// data runs copied in at 0x40.
func nonResidentAttr(attrType uint32, dataSize int64, runs []byte) []byte {
	const drOff = 0x40
	attr := make([]byte, drOff+len(runs))
	w := leWriter{attr}
	w.u32(0, attrType)
	w.u32(4, uint32(len(attr)))
	attr[8] = 1 // non-resident
	w.u16(0x20, drOff)
	w.u64(0x30, uint64(dataSize))
	copy(attr[drOff:], runs)
	return attr
}

// residentAttr builds a resident attribute: content length at 0x10, content
// offset at 0x14 (0x18, just past the resident header), content copied in.
func residentAttr(attrType uint32, content []byte) []byte {
	const contentOff = 0x18
	attr := make([]byte, contentOff+len(content))
	w := leWriter{attr}
	w.u32(0, attrType)
	w.u32(4, uint32(len(attr)))
	attr[8] = 0 // resident
	w.u32(0x10, uint32(len(content)))
	w.u16(0x14, contentOff)
	copy(attr[contentOff:], content)
	return attr
}

// attrListEntryBytes builds one $ATTRIBUTE_LIST entry (0x18 bytes): type at
// 0x00, entry length at 0x04, referenced MFT file reference at 0x10.
func attrListEntryBytes(attrType uint32, mftRef uint64) []byte {
	e := make([]byte, 0x18)
	w := leWriter{e}
	w.u32(0, attrType)
	w.u16(4, 0x18)
	w.u64(0x10, mftRef)
	return e
}

// assembleRecord builds a full record: valid header, attributes packed from
// 0x38, terminated with the end marker.
func assembleRecord(attrs ...[]byte) []byte {
	rec := make([]byte, testExtRecordSize)
	writeRecordHeader(rec, 0x38)
	off := 0x38
	for _, a := range attrs {
		copy(rec[off:], a)
		off += len(a)
	}
	leWriter{rec}.u32(off, attrEndMarker)
	return rec
}

// leWriter is a tiny little-endian writer used only by these test builders.
type leWriter struct{ b []byte }

func (w leWriter) u16(off int, v uint16) {
	w.b[off] = byte(v)
	w.b[off+1] = byte(v >> 8)
}
func (w leWriter) u32(off int, v uint32) {
	for i := 0; i < 4; i++ {
		w.b[off+i] = byte(v >> (uint(i) * 8))
	}
}
func (w leWriter) u64(off int, v uint64) {
	for i := 0; i < 8; i++ {
		w.b[off+i] = byte(v >> (uint(i) * 8))
	}
}

func testVol() *volumeInfo {
	return &volumeInfo{
		recordSize:      testExtRecordSize,
		bytesPerCluster: testBytesPerClus,
		totalClusters:   10000,
		mftStartByte:    testBytesPerClus, // record 0 at LCN 1
		// Matches the 2-cluster $DATA run most fixtures below give record 0. The
		// valid length is what mergeMFTSegments clamps to, so a mismatch would pad
		// or truncate the list instead of testing what the fixture intends; use
		// testVolValid for fixtures whose segments span more.
		mftValidBytes: testBytesPerClus * 2,
	}
}

// testVolValid is testVol with the valid $MFT length set to cover clusters, for
// fixtures whose segments span more than the default. A real volume's valid length
// is never below what its extents cover, so a fixture claiming otherwise would be
// exercising the clamp rather than whatever it means to test.
func testVolValid(clusters int64) *volumeInfo {
	v := testVol()
	v.mftValidBytes = clusters * testBytesPerClus
	return v
}

// withLowestVcn stamps LowestVcn onto a non-resident attribute, marking it as the
// segment beginning at that cluster of the stream. Extension-record segments
// always start past the base record's range; leaving this at 0 would claim the
// same range twice.
func withLowestVcn(attr []byte, lowestVcn uint64) []byte {
	leWriter{attr}.u64(attrOffLowestVcn, lowestVcn)
	return attr
}

func extentsEqual(a, b []extent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// -------------------------------------------------------------------------
// getMFTExtents
// -------------------------------------------------------------------------

func TestGetMFTExtents_InlineOnly(t *testing.T) {
	// Record 0 with a single non-resident $DATA run and no $ATTRIBUTE_LIST:
	// the inline extents are returned as-is.
	disk := make([]byte, 8192)
	copy(disk[testBytesPerClus:], assembleRecord(nonResidentAttr(attrData, 0, singleRun(2, 1))))

	got, _, err := getMFTExtents(memReader(disk), testVol())
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	want := []extent{{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus}}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
}

func TestGetMFTExtents_ResidentAttrListChasesExtension(t *testing.T) {
	// Record 0's inline $DATA covers records 0-7 (2 clusters at LCN 1). Its
	// resident $ATTRIBUTE_LIST points at record 1 (a $DATA extension), whose
	// own run must be appended to the result.
	disk := make([]byte, 8192)
	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),
		residentAttr(attrAttributeList, attrListEntryBytes(attrData, 1)),
	)
	// The extension segment continues the stream at VCN 2, immediately after
	// record 0's 2-cluster run.
	rec1 := assembleRecord(withLowestVcn(nonResidentAttr(attrData, 0, singleRun(5, 50)), 2))
	copy(disk[testBytesPerClus:], rec0)                   // record 0 at 4096
	copy(disk[testBytesPerClus+testExtRecordSize:], rec1) // record 1 at 5120
	got, _, err := getMFTExtents(memReader(disk), testVolValid(7))
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	want := []extent{
		{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
		{byteOffset: 50 * testBytesPerClus, byteLength: 5 * testBytesPerClus},
	}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
}

func TestGetMFTExtents_NonResidentAttrListChasesExtension(t *testing.T) {
	// The rare case: record 0's $ATTRIBUTE_LIST is non-resident. Its content
	// (one entry pointing at record 1) lives in a disk cluster located by the
	// attr-list's own inline runs — read directly by LCN, no MFT indirection.
	disk := make([]byte, 16384)
	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),             // MFT self-location
		nonResidentAttr(attrAttributeList, 0x18, singleRun(1, 3)), // content at LCN 3
	)
	// The extension segment continues the stream at VCN 2, immediately after
	// record 0's 2-cluster run.
	rec1 := assembleRecord(withLowestVcn(nonResidentAttr(attrData, 0, singleRun(5, 50)), 2))
	copy(disk[testBytesPerClus:], rec0)                              // record 0 at 4096
	copy(disk[testBytesPerClus+testExtRecordSize:], rec1)            // record 1 at 5120
	copy(disk[3*testBytesPerClus:], attrListEntryBytes(attrData, 1)) // content at 12288

	got, _, err := getMFTExtents(memReader(disk), testVolValid(7))
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	want := []extent{
		{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
		{byteOffset: 50 * testBytesPerClus, byteLength: 5 * testBytesPerClus},
	}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
}

func TestGetMFTExtents_UnresolvableEntrySkipped(t *testing.T) {
	// An attr-list entry whose MFT index falls outside the known inline extents
	// cannot be read; getMFTExtents skips it and still returns the inline runs
	// rather than failing the whole scan.
	disk := make([]byte, 8192)
	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),
		residentAttr(attrAttributeList, attrListEntryBytes(attrData, 100000)),
	)
	copy(disk[testBytesPerClus:], rec0)

	got, _, err := getMFTExtents(memReader(disk), testVol())
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	want := []extent{{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus}}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
}

func TestGetMFTExtents_BadSignature(t *testing.T) {
	disk := make([]byte, 8192) // record 0 region is all zero: no signature
	if _, _, err := getMFTExtents(memReader(disk), testVol()); err == nil {
		t.Fatal("expected error on missing record 0 signature")
	}
}

func TestGetMFTExtents_NoData(t *testing.T) {
	// Record 0 with a valid header but no $DATA and no $ATTRIBUTE_LIST.
	disk := make([]byte, 8192)
	copy(disk[testBytesPerClus:], assembleRecord())
	_, _, err := getMFTExtents(memReader(disk), testVol())
	if err == nil || !strings.Contains(err.Error(), "no $DATA") {
		t.Fatalf("err = %v, want no-$DATA error", err)
	}
}

func TestGetMFTExtents_MalformedBootstrapRunlistFails(t *testing.T) {
	// Record 0's $DATA determines where every later MFT record lives. A valid
	// prefix is not enough: accepting it would silently turn an invalid map into
	// an undercount or, after a later segment, a misindexed scan.
	disk := make([]byte, 8192)
	copy(disk[testBytesPerClus:], assembleRecord(nonResidentAttr(attrData, 0, []byte{0x19, 1, 0})))
	if _, _, err := getMFTExtents(memReader(disk), testVol()); err == nil || !strings.Contains(err.Error(), "field sizes") {
		t.Fatalf("getMFTExtents error = %v, want malformed bootstrap runlist error", err)
	}
}

func TestGetMFTExtents_ReadError(t *testing.T) {
	// mftStartByte points past the end of the disk: the record 0 read fails.
	vol := testVol()
	if _, _, err := getMFTExtents(memReader(make([]byte, 1024)), vol); err == nil {
		t.Fatal("expected read error when record 0 is out of range")
	}
}

func TestGetMFTExtents_NonResidentAttrListTooLargeRejected(t *testing.T) {
	// A non-resident $ATTRIBUTE_LIST claiming a content size above the memory
	// bound must fail loudly rather than allocate unbounded memory.
	disk := make([]byte, 16384)
	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),
		nonResidentAttr(attrAttributeList, maxAttrListBytes+1, singleRun(1, 3)),
	)
	copy(disk[testBytesPerClus:], rec0)
	if _, _, err := getMFTExtents(memReader(disk), testVol()); err == nil {
		t.Fatal("expected error on oversized non-resident $ATTRIBUTE_LIST")
	}
}

// -------------------------------------------------------------------------
// readNonResidentAttrList
// -------------------------------------------------------------------------

func TestReadNonResidentAttrList_Valid(t *testing.T) {
	disk := make([]byte, 16384)
	content := append(attrListEntryBytes(attrData, 7), attrListEntryBytes(attrData, 9)...)
	copy(disk[3*testBytesPerClus:], content) // at LCN 3
	attr := nonResidentAttr(attrAttributeList, int64(len(content)), singleRun(1, 3))

	got, err := readNonResidentAttrList(memReader(disk), attr, testBytesPerClus, 10000)
	if err != nil {
		t.Fatalf("readNonResidentAttrList: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch:\n got %x\nwant %x", got, content)
	}
}

func TestReadNonResidentAttrList_Errors(t *testing.T) {
	disk := make([]byte, 16384)
	cases := []struct {
		name            string
		attr            []byte
		bytesPerCluster int64
		totalClusters   int64
		want            string
	}{
		{name: "too short", attr: make([]byte, 0x20), want: "too short"},
		{name: "bad drOff", attr: func() []byte {
			a := nonResidentAttr(attrAttributeList, 0x18, singleRun(1, 3))
			leWriter{a}.u16(0x20, 0x10) // drOff below the 0x40 header floor
			return a
		}(), want: "data-run offset"},
		{name: "zero size", attr: nonResidentAttr(attrAttributeList, 0, singleRun(1, 3)), want: "out of range"},
		{name: "oversize", attr: nonResidentAttr(attrAttributeList, maxAttrListBytes+1, singleRun(1, 3)), want: "out of range"},
		{name: "no runs", attr: nonResidentAttr(attrAttributeList, 0x18, []byte{0x00}), want: "no data runs"},
		{name: "runs too small", attr: nonResidentAttr(attrAttributeList, 2*testBytesPerClus, singleRun(1, 3)), want: "content size"},
		{name: "run allocation overflow", attr: nonResidentAttr(attrAttributeList, 1, []byte{
			0x11, 0x01, 0x01, // one cluster at LCN 1
			0x18, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, // MaxInt64 clusters at LCN 0
			0x00,
		}), bytesPerCluster: 1, totalClusters: math.MaxInt64, want: "allocated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bytesPerCluster := c.bytesPerCluster
			if bytesPerCluster == 0 {
				bytesPerCluster = testBytesPerClus
			}
			totalClusters := c.totalClusters
			if totalClusters == 0 {
				totalClusters = 10000
			}
			_, err := readNonResidentAttrList(memReader(disk), c.attr, bytesPerCluster, totalClusters)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

// -------------------------------------------------------------------------
// decodeDataRuns
// -------------------------------------------------------------------------

func TestDecodeDataRuns(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		want    []extent
		wantErr string
	}{
		{"empty", nil, nil, "unterminated"},
		{"terminator only", []byte{0x00}, nil, ""},
		{"single run", singleRun(2, 1), []extent{{byteOffset: 4096, byteLength: 8192}}, ""},
		{
			"two runs, second relative",
			// run1: len 2, lcn +1 → {4096, 8192}; run2: len 3, lcn +2 (=3) → {12288, 12288}
			[]byte{0x11, 0x02, 0x01, 0x11, 0x03, 0x02, 0x00},
			[]extent{{byteOffset: 4096, byteLength: 8192}, {byteOffset: 12288, byteLength: 12288}},
			"",
		},
		{
			"sparse run skipped",
			// sparse run (offSz 0): len 4, no offset → skipped; then len 1 at lcn +5
			[]byte{0x01, 0x04, 0x11, 0x01, 0x05, 0x00},
			[]extent{{byteOffset: 5 * 4096, byteLength: 4096}},
			"",
		},
		{
			"negative lcn delta (sign extension)",
			// run1: len 1 at lcn +10 → {40960,4096}; run2: len 1 at lcn -1 (0xFF) → lcn 9 → {36864,4096}
			[]byte{0x11, 0x01, 0x0A, 0x11, 0x01, 0xFF, 0x00},
			[]extent{{byteOffset: 10 * 4096, byteLength: 4096}, {byteOffset: 9 * 4096, byteLength: 4096}},
			"",
		},
		{"truncated run bytes", []byte{0x11, 0x02}, nil, "truncated"},
		{"zero length size", []byte{0x10, 0x05}, nil, "field sizes"},
		{"oversized length field", []byte{0x19, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}, nil, "field sizes"},
		{"oversized offset field", []byte{0x91, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}, nil, "field sizes"},
		{"negative 8-byte length", []byte{0x18, 0, 0, 0, 0, 0, 0, 0, 0x80, 1, 0}, nil, "invalid data-run length"},
		{"negative resulting LCN", []byte{0x11, 1, 0xFF, 0}, nil, "negative data-run LCN"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeDataRuns(c.data, testBytesPerClus, 10000)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("decodeDataRuns(%x) error = %v, want containing %q", c.data, err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeDataRuns(%x): %v", c.data, err)
			}
			if !extentsEqual(got, c.want) {
				t.Errorf("decodeDataRuns(%x) = %+v, want %+v", c.data, got, c.want)
			}
		})
	}
	t.Run("LCN addition overflows", func(t *testing.T) {
		data := []byte{0x81, 1, 0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0x11, 1, 2, 0}
		if _, err := decodeDataRuns(data, 1, math.MaxInt64); err == nil || !strings.Contains(err.Error(), "addition overflows") {
			t.Fatalf("decodeDataRuns(%x) error = %v, want addition overflow", data, err)
		}
	})
	t.Run("run exceeds volume", func(t *testing.T) {
		data := []byte{0x11, 2, 9, 0}
		if _, err := decodeDataRuns(data, testBytesPerClus, 10); err == nil || !strings.Contains(err.Error(), "exceeds volume size") {
			t.Fatalf("decodeDataRuns(%x) error = %v, want volume bound", data, err)
		}
	})
	t.Run("byte length overflows", func(t *testing.T) {
		data := []byte{0x18, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0, 0}
		if _, err := decodeDataRuns(data, testBytesPerClus, math.MaxInt64); err == nil || !strings.Contains(err.Error(), "byte offset or length overflows") {
			t.Fatalf("decodeDataRuns(%x) error = %v, want byte multiplication overflow", data, err)
		}
	})
}

// -------------------------------------------------------------------------
// parseAttributeList / parseAttributeListEntries
// -------------------------------------------------------------------------

func TestParseAttributeList_Resident(t *testing.T) {
	content := append(attrListEntryBytes(attrData, 3), attrListEntryBytes(attrFileName, 4)...)
	attr := residentAttr(attrAttributeList, content)
	got := parseAttributeList(attr)
	want := []attrListEntry{{attrType: attrData, mftRef: 3}, {attrType: attrFileName, mftRef: 4}}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseAttributeList_RejectsNonResident(t *testing.T) {
	attr := nonResidentAttr(attrAttributeList, 0x18, singleRun(1, 3))
	if got := parseAttributeList(attr); got != nil {
		t.Errorf("parseAttributeList on non-resident attr = %+v, want nil", got)
	}
}

func TestParseAttributeList_TooShort(t *testing.T) {
	if got := parseAttributeList(make([]byte, 16)); got != nil {
		t.Errorf("parseAttributeList(short) = %+v, want nil", got)
	}
}

func TestParseAttributeList_ContentOutOfBounds(t *testing.T) {
	attr := residentAttr(attrAttributeList, attrListEntryBytes(attrData, 1))
	// Claim a content length far beyond the attribute buffer.
	leWriter{attr}.u32(0x10, 0xFFFF)
	if got := parseAttributeList(attr); got != nil {
		t.Errorf("parseAttributeList(oob content) = %+v, want nil", got)
	}
}

func TestParseAttributeListEntries(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := parseAttributeListEntries(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("mftRef masked to 48 bits", func(t *testing.T) {
		e := attrListEntryBytes(attrData, 0x0007_0000_0000_0005)
		got := parseAttributeListEntries(e)
		if len(got) != 1 || got[0].mftRef != 5 {
			t.Errorf("got %+v, want one entry with mftRef 5", got)
		}
	})
	t.Run("undersized entry length stops walk", func(t *testing.T) {
		e := attrListEntryBytes(attrData, 1)
		leWriter{e}.u16(4, 0x10) // entryLen < 0x18
		if got := parseAttributeListEntries(e); got != nil {
			t.Errorf("got %+v, want nil (walk should stop)", got)
		}
	})
	t.Run("entry length past content stops walk", func(t *testing.T) {
		e := attrListEntryBytes(attrData, 1)
		leWriter{e}.u16(4, 0xFF) // entryLen beyond the buffer
		if got := parseAttributeListEntries(e); got != nil {
			t.Errorf("got %+v, want nil (walk should stop)", got)
		}
	})
}

// TestMFTDataRuns pins the gate that decides whether a $DATA attribute really
// describes $MFT's own clusters. Everything it accepts is fed straight to
// decodeDataRuns and then read off the raw volume as MFT records, so a wrong
// accept silently redefines where the scan believes $MFT lives.
func TestMFTDataRuns(t *testing.T) {
	// One run: 8 clusters at LCN 0x20, then the 0x00 terminator.
	runs := []byte{0x11, 0x08, 0x20, 0x00}

	t.Run("unnamed non-resident $DATA is accepted", func(t *testing.T) {
		got, _, ok := mftDataRuns(nonResidentAttr(attrData, 4096, runs))
		if !ok {
			t.Fatal("mftDataRuns rejected a valid unnamed non-resident $DATA")
		}
		if ext, err := decodeDataRuns(got, 4096, 10000); err != nil || len(ext) != 1 {
			t.Fatalf("decoded %d extents, err %v, want 1", len(ext), err)
		}
	})

	// A NAMED $DATA is an alternate data stream with its own unrelated extent
	// chain. Accepting it would splice foreign clusters into the $MFT extent
	// list, so the scan would parse unrelated bytes as MFT records.
	t.Run("named $DATA (ADS) is rejected", func(t *testing.T) {
		attr := nonResidentAttr(attrData, 4096, runs)
		attr[attrOffNameLength] = 4 // a 4-character stream name
		if _, _, ok := mftDataRuns(attr); ok {
			t.Error("mftDataRuns accepted a NAMED $DATA attribute")
		}
	})

	t.Run("resident $DATA is rejected", func(t *testing.T) {
		attr := nonResidentAttr(attrData, 4096, runs)
		attr[attrOffFormCode] = 0 // RESIDENT_FORM describes no clusters
		if _, _, ok := mftDataRuns(attr); ok {
			t.Error("mftDataRuns accepted a resident $DATA attribute")
		}
	})

	// Compressed/sparse/encrypted runs do not map onto plain byte extents.
	for name, flags := range map[string]uint16{
		"compressed": 0x0001,
		"sparse":     0x8000,
		"encrypted":  0x4000,
	} {
		t.Run(name+" $DATA is rejected", func(t *testing.T) {
			attr := nonResidentAttr(attrData, 4096, runs)
			leWriter{attr}.u16(attrOffFlags, flags)
			if _, _, ok := mftDataRuns(attr); ok {
				t.Errorf("mftDataRuns accepted a %s $DATA attribute", name)
			}
		})
	}

	// MappingPairsOffset is attacker-controlled. Inside the header we would
	// decode LowestVcn/HighestVcn bytes as run data; past the record we would
	// read beyond the attribute.
	t.Run("mapping pairs offset inside the header is rejected", func(t *testing.T) {
		attr := nonResidentAttr(attrData, 4096, runs)
		leWriter{attr}.u16(attrOffMappingPairs, 0x10)
		if _, _, ok := mftDataRuns(attr); ok {
			t.Error("mftDataRuns accepted MappingPairsOffset inside the header")
		}
	})
	t.Run("mapping pairs offset past the record is rejected", func(t *testing.T) {
		attr := nonResidentAttr(attrData, 4096, runs)
		leWriter{attr}.u16(attrOffMappingPairs, 0xFFFF)
		if _, _, ok := mftDataRuns(attr); ok {
			t.Error("mftDataRuns accepted MappingPairsOffset past the record")
		}
	})

	t.Run("attribute shorter than the non-resident header is rejected", func(t *testing.T) {
		attr := nonResidentAttr(attrData, 4096, runs)
		if _, _, ok := mftDataRuns(attr[:attrNonresidentHeaderLen-1]); ok {
			t.Error("mftDataRuns accepted a truncated attribute header")
		}
	})
}

// TestMergeMFTSegments covers the invariant that licenses the scan's cheap
// positional record indexing: the flattened extent list must cover the $MFT
// stream contiguously from VCN 0. Segments are merged in VCN order, holes are
// reported rather than spliced out, and overlaps are refused.
func TestMergeMFTSegments(t *testing.T) {
	const cluster = 4096
	// Each segment covers 2 clusters; recordSize is irrelevant here.
	vol := func(validBytes int64) *volumeInfo {
		return &volumeInfo{recordSize: 1024, bytesPerCluster: cluster, mftValidBytes: validBytes}
	}
	seg := func(lowestVcn uint64, diskOff int64) mftSegment {
		return mftSegment{lowestVcn: lowestVcn, runs: []extent{{byteOffset: diskOff, byteLength: 2 * cluster}}}
	}

	t.Run("single segment passes through untouched", func(t *testing.T) {
		got, unmapped, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000)}, vol(2*cluster))
		if unmapped != 0 {
			t.Errorf("unmappedBytes = %d, want 0 (nothing missing)", unmapped)
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []extent{{byteOffset: 0x10000, byteLength: 2 * cluster}}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("two in-order segments concatenate", func(t *testing.T) {
		got, _, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000), seg(2, 0x90000)}, vol(4*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].byteOffset != 0x10000 || got[1].byteOffset != 0x90000 {
			t.Errorf("got %+v, want the two runs in VCN order", got)
		}
	})

	// Arrival order is not guaranteed, so the merge must sort rather than trust it.
	t.Run("out-of-order segments are sorted by VCN", func(t *testing.T) {
		got, _, err := mergeMFTSegments([]mftSegment{seg(2, 0x90000), seg(0, 0x10000)}, vol(4*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].byteOffset != 0x10000 || got[1].byteOffset != 0x90000 {
			t.Errorf("got %+v, want VCN 0's run first", got)
		}
	})

	// A hole must become a counted placeholder, not a silent splice: dropping it
	// would renumber every record after the gap.
	t.Run("gap becomes an unmapped extent of exactly the missing length", func(t *testing.T) {
		got, unmapped, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000), seg(4, 0x90000)}, vol(6*cluster))
		if unmapped != 2*cluster {
			t.Errorf("unmappedBytes = %d, want %d (VCN 2..3)", unmapped, 2*cluster)
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d extents, want 3 (run, gap, run): %+v", len(got), got)
		}
		if got[1].byteOffset != unmappedExtent {
			t.Errorf("got[1].byteOffset = %d, want unmappedExtent", got[1].byteOffset)
		}
		if got[1].byteLength != 2*cluster {
			t.Errorf("gap length = %d, want %d (VCN 2..3)", got[1].byteLength, 2*cluster)
		}
	})

	// A leading gap matters just as much: without it record 0 would be attributed
	// to whatever the first mapped extent happens to hold.
	t.Run("missing VCN 0 becomes a leading unmapped extent", func(t *testing.T) {
		got, _, err := mergeMFTSegments([]mftSegment{seg(2, 0x90000)}, vol(4*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].byteOffset != unmappedExtent || got[0].byteLength != 2*cluster {
			t.Errorf("got %+v, want a leading 2-cluster unmapped extent", got)
		}
	})

	t.Run("overlapping segments are rejected", func(t *testing.T) {
		_, _, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000), seg(1, 0x90000)}, vol(4*cluster))
		if err == nil {
			t.Fatal("merged overlapping segments; want an error")
		}
		if !strings.Contains(err.Error(), "overlapping") {
			t.Errorf("error = %v, want it to name the overlap", err)
		}
	})

	// The volume's reported valid $MFT length is an independent authority, so a
	// shortfall means segments are missing and the tail must be reported.
	t.Run("coverage short of mftValidBytes gets an unmapped tail", func(t *testing.T) {
		got, unmapped, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000)}, vol(6*cluster))
		if unmapped != 4*cluster {
			t.Errorf("unmappedBytes = %d, want %d", unmapped, 4*cluster)
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d extents, want 2 (run + unmapped tail): %+v", len(got), got)
		}
		if got[1].byteOffset != unmappedExtent || got[1].byteLength != 4*cluster {
			t.Errorf("tail = %+v, want a 4-cluster unmapped extent", got[1])
		}
	})

	t.Run("no segments is an error", func(t *testing.T) {
		if _, _, err := mergeMFTSegments(nil, vol(cluster)); err == nil {
			t.Error("merged an empty segment list; want an error")
		}
	})

	// The merge must not reorder the caller's slice.
	t.Run("caller's segment order is preserved", func(t *testing.T) {
		segs := []mftSegment{seg(2, 0x90000), seg(0, 0x10000)}
		if _, _, err := mergeMFTSegments(segs, vol(4*cluster)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if segs[0].lowestVcn != 2 {
			t.Errorf("mergeMFTSegments mutated the caller's slice order")
		}
	})

	// Coverage past the valid length is $MFT slack. Read off the raw volume it is
	// not zeros, and a stale record there can pass every parser check, so it must
	// be cut rather than scanned.
	t.Run("coverage past mftValidBytes is truncated mid-extent", func(t *testing.T) {
		// One 2-cluster segment, but only 1 cluster is valid.
		got, unmapped, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000)}, vol(1*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []extent{{byteOffset: 0x10000, byteLength: 1 * cluster}}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %+v, want %+v (shortened to the valid length)", got, want)
		}
		if unmapped != 0 {
			t.Errorf("unmappedBytes = %d, want 0", unmapped)
		}
	})

	t.Run("extents entirely past mftValidBytes are dropped", func(t *testing.T) {
		// Three 2-cluster segments (6 clusters) with only 3 clusters valid: the
		// second is halved and the third disappears.
		got, _, err := mergeMFTSegments(
			[]mftSegment{seg(0, 0x10000), seg(2, 0x90000), seg(4, 0x99000)}, vol(3*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []extent{
			{byteOffset: 0x10000, byteLength: 2 * cluster},
			{byteOffset: 0x90000, byteLength: 1 * cluster},
		}
		if !extentsEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	// A boundary landing exactly on an extent start must drop it, not keep a
	// zero-length extent.
	t.Run("boundary at an extent start drops it cleanly", func(t *testing.T) {
		got, _, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000), seg(2, 0x90000)}, vol(2*cluster))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []extent{{byteOffset: 0x10000, byteLength: 2 * cluster}}
		if !extentsEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	// Truncation shortens an extent, and the fast path hands its caller's runs
	// straight in, so it must copy rather than write through.
	t.Run("truncation does not mutate the caller's runs", func(t *testing.T) {
		s := seg(0, 0x10000)
		if _, _, err := mergeMFTSegments([]mftSegment{s}, vol(1*cluster)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.runs[0].byteLength != 2*cluster {
			t.Errorf("caller's run was shortened to %d; mergeMFTSegments wrote through the slice",
				s.runs[0].byteLength)
		}
	})

	// A zero valid length cannot be clamped against; trimming to nothing would
	// make the scan silently find no files at all.
	t.Run("zero mftValidBytes leaves the list alone", func(t *testing.T) {
		got, _, err := mergeMFTSegments([]mftSegment{seg(0, 0x10000)}, vol(0))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []extent{{byteOffset: 0x10000, byteLength: 2 * cluster}}
		if !extentsEqual(got, want) {
			t.Errorf("got %+v, want %+v (unclamped)", got, want)
		}
	})
}

// TestMFTStreamToDisk covers the VCN-aware mapping the extension-record chase
// relies on while the segment list is still being assembled.
func TestMFTStreamToDisk(t *testing.T) {
	const cluster = 4096
	segs := []mftSegment{
		{lowestVcn: 0, runs: []extent{{byteOffset: 0x10000, byteLength: cluster}}},
		{lowestVcn: 4, runs: []extent{{byteOffset: 0x90000, byteLength: cluster}}},
	}

	// Offset 0 is the first byte of the segment at VCN 0.
	if got, ok := mftStreamToDisk(segs, cluster, 0); !ok || got != 0x10000 {
		t.Errorf("streamOff 0 -> (%#x, %v), want (0x10000, true)", got, ok)
	}
	// Mid-run offsets are translated, not rounded to the run start.
	if got, ok := mftStreamToDisk(segs, cluster, 512); !ok || got != 0x10200 {
		t.Errorf("streamOff 512 -> (%#x, %v), want (0x10200, true)", got, ok)
	}
	// A stream offset inside the VCN 4 segment must map through that segment's
	// own lowestVcn, not through cumulative position in the list.
	if got, ok := mftStreamToDisk(segs, cluster, 4*cluster); !ok || got != 0x90000 {
		t.Errorf("streamOff 4*cluster -> (%#x, %v), want (0x90000, true)", got, ok)
	}
	// The hole at VCN 1..3 is unmapped: this is how the extension chase learns it
	// cannot reach a record yet.
	if _, ok := mftStreamToDisk(segs, cluster, 2*cluster); ok {
		t.Error("streamOff in the VCN 1..3 hole reported mapped; want unmapped")
	}
	// Past the end is unmapped too.
	if _, ok := mftStreamToDisk(segs, cluster, 99*cluster); ok {
		t.Error("streamOff past the last segment reported mapped; want unmapped")
	}
}

// TestGetMFTExtents_DeferredExtensionResolvedOnRetry covers the order dependence
// in the extension chase. Locating an extension record needs whichever part of
// the $MFT map other segments supply, so an entry listed before the segment that
// maps it is unreachable on the first pass and must be retried.
//
// Layout (recordSize 1024, cluster 4096), with the attribute list naming the
// far extension FIRST so a single forward pass would lose it:
//
//	record 0   $DATA  VCN 0, 2 clusters @ LCN 1  -> maps MFT records 0..7
//	           $ATTRIBUTE_LIST -> [ext A @ idx 9, ext B @ idx 1]
//	ext B      MFT idx 1  (inside record 0's range: reachable immediately)
//	           $DATA  VCN 2, 2 clusters @ LCN 5  -> maps MFT records 8..15
//	ext A      MFT idx 9  (only inside ext B's range: reachable on pass 2)
//	           $DATA  VCN 4, 1 cluster  @ LCN 9
func TestGetMFTExtents_DeferredExtensionResolvedOnRetry(t *testing.T) {
	disk := make([]byte, 40960)

	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),
		residentAttr(attrAttributeList, append(
			attrListEntryBytes(attrData, 9),    // unreachable on pass 1
			attrListEntryBytes(attrData, 1)..., // maps idx 9 once resolved
		)),
	)
	extB := assembleRecord(withLowestVcn(nonResidentAttr(attrData, 0, singleRun(2, 5)), 2))
	extA := assembleRecord(withLowestVcn(nonResidentAttr(attrData, 0, singleRun(1, 9)), 4))

	copy(disk[1*testBytesPerClus:], rec0) // record 0 at disk 4096
	// MFT idx 1 -> stream offset 1024, inside record 0's VCN 0 run at disk 4096.
	copy(disk[1*testBytesPerClus+1*testExtRecordSize:], extB) // disk 5120
	// MFT idx 9 -> stream offset 9216, inside ext B's VCN 2 run (stream 8192 ->
	// disk 20480), so disk 20480 + (9216-8192) = 21504.
	copy(disk[5*testBytesPerClus+1*testExtRecordSize:], extA) // disk 21504

	got, _, err := getMFTExtents(memReader(disk), testVolValid(5))
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	want := []extent{
		{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
		{byteOffset: 5 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
		{byteOffset: 9 * testBytesPerClus, byteLength: 1 * testBytesPerClus},
	}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v\n(the VCN 4 run is only reachable after the VCN 2 run resolves)", got, want)
	}
}

// TestGetMFTExtents_PermanentlyUnreachableExtensionTerminates pins that the retry
// loop stops instead of spinning when an entry can never be resolved, and that
// the records it would have covered are reported as an unmapped range rather than
// dropped (which would renumber every later record).
func TestGetMFTExtents_PermanentlyUnreachableExtensionTerminates(t *testing.T) {
	disk := make([]byte, 40960)

	rec0 := assembleRecord(
		nonResidentAttr(attrData, 0, singleRun(2, 1)),
		residentAttr(attrAttributeList, append(
			attrListEntryBytes(attrData, 900000), // never mappable
			attrListEntryBytes(attrData, 1)...,   // resolvable, so a pass makes progress
		)),
	)
	extB := assembleRecord(withLowestVcn(nonResidentAttr(attrData, 0, singleRun(2, 5)), 2))
	copy(disk[1*testBytesPerClus:], rec0)
	copy(disk[1*testBytesPerClus+1*testExtRecordSize:], extB)

	got, gaps, err := getMFTExtents(memReader(disk), testVolValid(4))
	if err != nil {
		t.Fatalf("getMFTExtents: %v", err)
	}
	if gaps.unreachableExtensions != 1 {
		t.Errorf("unreachableExtensions = %d, want 1", gaps.unreachableExtensions)
	}
	// Nothing was actually lost: the resolved segments already cover mftValidBytes,
	// so the unreachable entry costs no records and must not be warned about.
	if gaps.unmappedBytes != 0 {
		t.Errorf("unmappedBytes = %d, want 0", gaps.unmappedBytes)
	}
	// The reachable segments still merge; the unreachable entry contributes nothing
	// and must not stall or duplicate the walk.
	want := []extent{
		{byteOffset: 1 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
		{byteOffset: 5 * testBytesPerClus, byteLength: 2 * testBytesPerClus},
	}
	if !extentsEqual(got, want) {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
}
