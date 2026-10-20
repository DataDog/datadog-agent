// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Fuzz tests for the pure $MFT record parser. These run on every platform
// (the parser has no OS dependencies — see parser.go), so they exercise the
// code that consumes untrusted on-disk binary data — raw MFT records read
// straight off the volume — and must never panic on hostile input.
//
// Seed corpus follows the same A/B/C structure as the ip route parser fuzzer
// (builtins/tests/ip/ip_route_fuzz_linux_test.go):
//
//	A. Implementation edge cases: valid synthetic records (recordBuilder),
//	   empty/zero, signature-only, hostile attribute offsets/lengths.
//	B. CVE / binary class: null bytes, binary magic, truncation, all-0xFF.
//	C. Existing coverage: byte patterns from parser_test.go.
package ntfsmft

import (
	"encoding/binary"
	"testing"
)

// intoRecord copies fuzz bytes into a fixed testRecordSize buffer, mirroring
// how the streaming layer always hands parseInto / applyFixups a full,
// geometry-validated record (recordSize is never attacker-controlled — it comes
// from FSCTL_GET_NTFS_VOLUME_DATA — so fuzzing content within a valid-size
// record is the realistic threat model).
func intoRecord(data []byte) []byte {
	buf := make([]byte, testRecordSize)
	copy(buf, data)
	return buf
}

// parseFuzzModes parses independent copies because applyFixups restores sector
// endings in place. Reusing a record would make the second parse reject a
// valid seed as a torn write before it reaches its mode-specific path.
func parseFuzzModes(data []byte) (allErr, fileBaseOnlyErr error) {
	var entry mftEntry
	_, allErr = parseInto(intoRecord(data), testRecordSize, &entry, modeAll)
	_, fileBaseOnlyErr = parseInto(intoRecord(data), testRecordSize, &entry, modeFileBaseOnly)
	return allErr, fileBaseOnlyErr
}

// hostileOffsets builds a record with a valid signature but an out-of-range
// first-attribute offset and a giant attribute length — the classic malformed
// inputs an attribute walk must reject without reading out of bounds.
func hostileOffsets() []byte {
	buf := make([]byte, testRecordSize)
	binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
	binary.LittleEndian.PutUint16(buf[0x14:0x16], 0xFFFF) // first-attr offset past end
	binary.LittleEndian.PutUint16(buf[0x16:0x18], flagInUse)
	return buf
}

func giantAttrLen() []byte {
	buf := make([]byte, testRecordSize)
	binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
	binary.LittleEndian.PutUint16(buf[0x14:0x16], 0x38)
	binary.LittleEndian.PutUint16(buf[0x16:0x18], flagInUse)
	binary.LittleEndian.PutUint32(buf[0x38:0x3C], attrFileName)
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 0xFFFFFFFF) // attrLen wraps/overruns
	return buf
}

// fileNameAttrAtEnd builds a valid, in-use record whose single $FILE_NAME
// attribute is placed flush against the record end, so the slice the walk
// hands parseFileNameParents has cap == al. When al < 0x18 that cap is below
// the resident-header reads (attr[0x14:0x16], a cap-checked slice expression),
// which panicked before the len(attr) < 0x18 guard was added. These are
// regression seeds for that fix; the fixup descriptor is valid so the record
// survives applyFixups and actually reaches the attribute walk.
func fileNameAttrAtEnd(al int) []byte {
	buf := make([]byte, testRecordSize)
	binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
	binary.LittleEndian.PutUint16(buf[4:6], 0x30) // USA offset
	binary.LittleEndian.PutUint16(buf[6:8], 3)    // USA count: 1 USN + 2 sectors
	// Valid fixups: USN at the sector ends matches the header USN so
	// applyFixups accepts the record (saved originals are left zero).
	binary.LittleEndian.PutUint16(buf[0x30:0x32], 0xCAFE) // USN
	binary.LittleEndian.PutUint16(buf[0x1FE:0x200], 0xCAFE)
	binary.LittleEndian.PutUint16(buf[0x3FE:0x400], 0xCAFE)

	off := testRecordSize - al
	binary.LittleEndian.PutUint16(buf[0x14:0x16], uint16(off)) // first-attr offset
	binary.LittleEndian.PutUint16(buf[0x16:0x18], flagInUse)
	binary.LittleEndian.PutUint32(buf[off:off+4], attrFileName)
	binary.LittleEndian.PutUint32(buf[off+4:off+8], uint32(al))
	return buf
}

func fuzzParserSeeds(f *testing.F) {
	// Source A: valid synthetic records covering each parse path.
	valid := [][]byte{
		newBuilder(0, 0).bytes(), // deleted record
		func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(5, 4096, 1000, nsWin32AndDOS)
			return rb.bytes()
		}(),
		func() []byte {
			rb := newBuilder(flagInUse|flagDirectory, 0)
			rb.appendFileName(5, 0, 0, nsWin32AndDOS)
			return rb.bytes()
		}(),
		newBuilder(flagInUse, 12345).bytes(), // extension record
		func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(100, 0, 0, nsWin32AndDOS)
			rb.appendFileName(200, 0, 0, nsWin32)
			rb.appendNonResidentData(0x8000, 0, 1<<30, 1<<30, 4096)
			rb.appendResidentData(154)
			return rb.bytes()
		}(),
	}
	for _, v := range valid {
		f.Add(v)
	}

	// Source A: hostile offsets / lengths.
	f.Add(hostileOffsets())
	f.Add(giantAttrLen())

	// Source A: $FILE_NAME attributes flush against the record end, whose
	// slice cap lands in and around the resident-header window (< 0x18).
	// Regression seeds for the parseFileNameParents out-of-bounds read.
	for _, al := range []int{16, 17, 21} {
		f.Add(fileNameAttrAtEnd(al))
	}

	// Source B: CVE / binary class.
	f.Add([]byte{})                     // empty
	f.Add(make([]byte, testRecordSize)) // all zero (no signature)
	f.Add([]byte("FILE"))               // signature only, truncated
	f.Add([]byte("\x7fELF\x02\x01\x01\x00"))
	f.Add([]byte("MZ\x90\x00\x03\x00"))
	f.Add([]byte("PK\x03\x04"))
	allFF := make([]byte, testRecordSize)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)
}

// FuzzParseInto verifies parseInto never panics on arbitrary record bytes, in
// both parse modes. parseInto drives parseFileNameParents / parseResidentData /
// parseNonResidentData, so this one fuzzer covers the whole attribute walk.
func FuzzParseInto(f *testing.F) {
	fuzzParserSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 { // cap at 1 MiB
			return
		}
		// A panic here fails the fuzz test. Errors are expected and fine.
		_, _ = parseFuzzModes(data)
	})
}

// TestParseFuzzModesUsesIndependentRecords prevents FuzzParseInto from
// accidentally reusing a record after its in-place fixups have been applied.
func TestParseFuzzModesUsesIndependentRecords(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 4096, 1000, nsWin32AndDOS)
	allErr, fileBaseOnlyErr := parseFuzzModes(rb.bytes())
	if allErr != nil {
		t.Fatalf("modeAll parse: %v", allErr)
	}
	if fileBaseOnlyErr != nil {
		t.Fatalf("modeFileBaseOnly parse: %v", fileBaseOnlyErr)
	}
}

// FuzzApplyFixups verifies the update-sequence-array (fixup) logic never panics
// or reads out of bounds on arbitrary record bytes, including hostile USA
// offsets and counts.
func FuzzApplyFixups(f *testing.F) {
	fuzzParserSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		rec := intoRecord(data)
		_ = applyFixups(rec, testRecordSize)
	})
}
