// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ntfsmft

import (
	"encoding/binary"
	"errors"
	"testing"
)

const testRecordSize = 1024

// recordBuilder constructs a synthetic MFT record for parser tests. The
// builder produces a record with a valid "FILE" signature, a 3-entry fixup
// array at offset 0x30 (covering two 512-byte sectors plus signature slot),
// and attributes appended starting at offset 0x38. Callers fill in the flags,
// baseRef, and attribute payload via the helpers below.
type recordBuilder struct {
	buf       [testRecordSize]byte
	attrStart int // grows as Append* is called
}

func newBuilder(flags uint16, baseRef uint64) *recordBuilder {
	rb := &recordBuilder{}
	binary.LittleEndian.PutUint32(rb.buf[0:4], mftSignature)
	binary.LittleEndian.PutUint16(rb.buf[4:6], 0x30)       // fixup array offset
	binary.LittleEndian.PutUint16(rb.buf[6:8], 3)          // fixup count: 1 sig + 2 sectors
	binary.LittleEndian.PutUint16(rb.buf[0x14:0x16], 0x38) // first attribute offset
	binary.LittleEndian.PutUint16(rb.buf[0x16:0x18], flags)
	binary.LittleEndian.PutUint64(rb.buf[0x20:0x28], baseRef)

	rb.attrStart = 0x38
	return rb
}

func (rb *recordBuilder) bytes() []byte { return rb.buf[:] }

func (rb *recordBuilder) appendAttr(attrType uint32, body []byte) {
	off := rb.attrStart
	attrLen := 16 + len(body)
	binary.LittleEndian.PutUint32(rb.buf[off:off+4], attrType)
	binary.LittleEndian.PutUint32(rb.buf[off+4:off+8], uint32(attrLen))
	copy(rb.buf[off+16:off+attrLen], body)
	rb.attrStart += attrLen
	binary.LittleEndian.PutUint32(rb.buf[rb.attrStart:rb.attrStart+4], attrEndMarker)
}

func (rb *recordBuilder) appendFileName(parentRef uint64, alloc, real int64, ns byte) {
	const contentLen = 0x42
	body := make([]byte, contentLen+8)
	binary.LittleEndian.PutUint32(body[0x00:0x04], contentLen)
	binary.LittleEndian.PutUint16(body[0x04:0x06], 0x18)
	c := body[0x08:]
	binary.LittleEndian.PutUint64(c[0x00:0x08], parentRef)
	binary.LittleEndian.PutUint64(c[0x28:0x30], uint64(alloc))
	binary.LittleEndian.PutUint64(c[0x30:0x38], uint64(real))
	c[0x40] = 0
	c[0x41] = ns

	rb.appendAttr(attrFileName, body)
}

func (rb *recordBuilder) appendResidentData(contentLen int) {
	body := make([]byte, 8+contentLen)
	binary.LittleEndian.PutUint32(body[0x00:0x04], uint32(contentLen))
	binary.LittleEndian.PutUint16(body[0x04:0x06], 0x18)
	rb.appendAttr(attrData, body)
}

func (rb *recordBuilder) appendNamedResidentData(contentLen int, name string) {
	nameBytes := make([]byte, len(name)*2)
	for i := range name {
		nameBytes[i*2] = name[i]
	}
	body := make([]byte, 8+len(nameBytes)+contentLen)
	binary.LittleEndian.PutUint32(body[0x00:0x04], uint32(contentLen))
	binary.LittleEndian.PutUint16(body[0x04:0x06], uint16(0x18+len(nameBytes)))
	copy(body[0x08:], nameBytes)
	attrStart := rb.attrStart
	rb.appendAttr(attrData, body)
	rb.buf[attrStart+9] = byte(len(name))
	binary.LittleEndian.PutUint16(rb.buf[attrStart+0x0A:attrStart+0x0C], 0x18)
}

func (rb *recordBuilder) appendNonResidentData(flags uint16, lowestVcn uint64, allocSize, dataSize, totalAlloc int64) {
	bodyLen := 0x38
	body := make([]byte, bodyLen)
	binary.LittleEndian.PutUint64(body[0x00:0x08], lowestVcn)
	binary.LittleEndian.PutUint64(body[0x18:0x20], uint64(allocSize))
	binary.LittleEndian.PutUint64(body[0x20:0x28], uint64(dataSize))
	binary.LittleEndian.PutUint64(body[0x30:0x38], uint64(totalAlloc))

	attrStart := rb.attrStart
	rb.appendAttr(attrData, body)
	rb.buf[attrStart+8] = 1 // nonResident
	binary.LittleEndian.PutUint16(rb.buf[attrStart+0x0C:attrStart+0x0E], flags)
}

// -------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------

func TestParse_BadSignature(t *testing.T) {
	buf := make([]byte, testRecordSize)
	var entry mftEntry
	if _, err := parseInto(buf, testRecordSize, &entry, modeAll); err == nil {
		t.Fatal("expected error on missing signature")
	}
}

func TestParse_DeletedRecord(t *testing.T) {
	rb := newBuilder(0, 0)
	var entry mftEntry
	baseRef, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if baseRef != 0 {
		t.Errorf("baseRef = %d, want 0", baseRef)
	}
	if entry.isInUse {
		t.Error("isInUse = true, want false")
	}
}

func TestParse_DirectoryFlag(t *testing.T) {
	rb := newBuilder(flagInUse|flagDirectory, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !entry.isDir {
		t.Error("isDir = false, want true")
	}
	if entry.primaryParent != 5 {
		t.Errorf("primaryParent = %d, want 5", entry.primaryParent)
	}
}

func TestParse_ExtensionRecord(t *testing.T) {
	rb := newBuilder(flagInUse, 12345)
	var entry mftEntry
	baseRef, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if baseRef != 12345 {
		t.Errorf("baseRef = %d, want 12345", baseRef)
	}
}

func TestParse_DOSNamespaceSkipped(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsDOS)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(entry.hardlinkParents); got != 1 {
		t.Errorf("hardlinkParents len = %d, want 1 (DOS-only skipped)", got)
	}
}

func TestParse_MultipleHardlinks(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(100, 0, 0, nsWin32AndDOS)
	rb.appendFileName(200, 0, 0, nsWin32)
	rb.appendFileName(300, 0, 0, nsPosix)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(entry.hardlinkParents); got != 3 {
		t.Fatalf("hardlinkParents = %v, want 3 entries", entry.hardlinkParents)
	}
	for i, want := range []uint64{100, 200, 300} {
		if entry.hardlinkParents[i] != want {
			t.Errorf("hardlinkParents[%d] = %d, want %d", i, entry.hardlinkParents[i], want)
		}
	}
	if entry.primaryParent != 100 {
		t.Errorf("primaryParent = %d, want 100", entry.primaryParent)
	}
}

func TestParse_NamespacePriority(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(10, 0, 0, nsPosix)
	rb.appendFileName(20, 0, 0, nsWin32)
	rb.appendFileName(30, 0, 0, nsWin32AndDOS)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.primaryParent != 30 {
		t.Errorf("primaryParent = %d, want 30 (Win32+DOS wins)", entry.primaryParent)
	}
}

func TestParse_ResidentData(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendResidentData(123)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.data.size(true, false) != 123 {
		t.Errorf("dataSize = %d, want 123", entry.data.size(true, false))
	}
	if entry.data.size(false, false) != 123 {
		t.Errorf("allocatedSize = %d, want 123 (resident)", entry.data.size(false, false))
	}
}

func TestParse_NonResidentData_Normal(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0, 0, 8192, 5000, 0)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.data.size(true, false) != 5000 {
		t.Errorf("dataSize = %d, want 5000", entry.data.size(true, false))
	}
	if entry.data.size(false, false) != 8192 {
		t.Errorf("allocatedSize = %d, want 8192", entry.data.size(false, false))
	}
	if entry.isSparse || entry.isCompressed {
		t.Error("normal data marked sparse/compressed")
	}
}

// TestParse_RejectsOverflowingSizeField covers a size field whose high bit is
// set (raw uint64 > math.MaxInt64). No real NTFS volume can hold a file that
// large, so it only appears on a crafted/corrupt image; casting it to int64
// would wrap negative and corrupt scan totals. parseInto must reject the whole
// record with errBadSize. The overflow value is injected as a negative int64
// (uint64 high bit set) through the builder helpers.
func TestParse_RejectsOverflowingSizeField(t *testing.T) {
	cases := []struct {
		name  string
		build func() []byte
	}{
		{"$FILE_NAME realSize", func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(5, 0, -1, nsWin32AndDOS)
			return rb.bytes()
		}},
		{"$FILE_NAME allocSize", func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(5, -1, 0, nsWin32AndDOS)
			return rb.bytes()
		}},
		{"non-resident $DATA dataSize", func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(5, 0, 0, nsWin32AndDOS)
			rb.appendNonResidentData(0, 0, 8192, -1, 0)
			return rb.bytes()
		}},
		{"non-resident $DATA allocSize", func() []byte {
			rb := newBuilder(flagInUse, 0)
			rb.appendFileName(5, 0, 0, nsWin32AndDOS)
			rb.appendNonResidentData(0, 0, -1, 5000, 0)
			return rb.bytes()
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var entry mftEntry
			if _, err := parseInto(c.build(), testRecordSize, &entry, modeAll); !errors.Is(err, errBadSize) {
				t.Errorf("got err=%v, want errBadSize", err)
			}
		})
	}
}

func TestParse_NonResidentData_Sparse(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	const virtualAlloc = 1 << 30
	const physicalAlloc = 4096
	rb.appendNonResidentData(0x8000, 0, virtualAlloc, virtualAlloc, physicalAlloc)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !entry.isSparse {
		t.Error("isSparse = false, want true")
	}
	if entry.data.size(false, false) != physicalAlloc {
		t.Errorf("allocatedSize = %d, want %d (physical from offset 0x40)", entry.data.size(false, false), physicalAlloc)
	}
	if entry.data.size(true, false) != virtualAlloc {
		t.Errorf("dataSize = %d, want %d", entry.data.size(true, false), virtualAlloc)
	}
}

func TestParse_NonResidentData_Compressed(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0x0001, 0, 65536, 60000, 32768)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !entry.isCompressed {
		t.Error("isCompressed = false, want true")
	}
	if entry.data.size(false, false) != 32768 {
		t.Errorf("allocatedSize = %d, want 32768 (compressed physical)", entry.data.size(false, false))
	}
}

func TestParse_NonResidentData_Continuation(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0, 100, 999999, 999999, 999999)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.data.size(true, false) != 0 || entry.data.size(false, false) != 0 {
		t.Errorf("continuation fragment overwrote sizes: data=%d alloc=%d",
			entry.data.size(true, false), entry.data.size(false, false))
	}
}

func TestResolveDataSize(t *testing.T) {
	tests := []struct {
		name string
		base dataSummary
		ext  dataSummary
		fn   streamBytes
		want int64
		ok   bool
	}{
		{
			name: "extension unnamed stream suppresses file name fallback",
			ext:  dataSummary{unnamed: streamBytes{apparent: 100}, unnamedStarts: 1},
			fn:   streamBytes{apparent: 100}, want: 100, ok: true,
		},
		{
			name: "extension unnamed stream plus base ADS",
			base: dataSummary{named: streamBytes{apparent: 10}},
			ext:  dataSummary{unnamed: streamBytes{apparent: 100}, unnamedStarts: 1},
			fn:   streamBytes{apparent: 100}, want: 110, ok: true,
		},
		{
			name: "conflicting unnamed starts are rejected",
			base: dataSummary{unnamed: streamBytes{apparent: 100}, unnamedStarts: 1},
			ext:  dataSummary{unnamed: streamBytes{apparent: 100}, unnamedStarts: 1},
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveDataSize(tt.base, tt.ext, tt.fn, true)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("resolveDataSize() = (%d, %t), want (%d, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseAndResolve_ExtensionUnnamedStreamWithBaseADS(t *testing.T) {
	base := newBuilder(flagInUse, 0)
	base.appendFileName(5, 100, 100, nsWin32AndDOS)
	base.appendNamedResidentData(10, "ads")
	var baseEntry mftEntry
	if _, err := parseInto(base.bytes(), testRecordSize, &baseEntry, modeAll); err != nil {
		t.Fatalf("parse base: %v", err)
	}

	extension := newBuilder(flagInUse, 42)
	extension.appendNonResidentData(0, 0, 128, 100, 0)
	var extensionEntry mftEntry
	if _, err := parseInto(extension.bytes(), testRecordSize, &extensionEntry, modeAll); err != nil {
		t.Fatalf("parse extension: %v", err)
	}

	if baseEntry.data.unnamedStarts != 0 || baseEntry.data.named.apparent != 10 {
		t.Fatalf("base data = %+v, want only a 10-byte named stream", baseEntry.data)
	}
	if extensionEntry.data.unnamedStarts != 1 || extensionEntry.data.unnamed.apparent != 100 {
		t.Fatalf("extension data = %+v, want a 100-byte unnamed stream", extensionEntry.data)
	}
	got, ok := resolveDataSize(baseEntry.data, extensionEntry.data,
		streamBytes{apparent: baseEntry.fnDataSize, allocated: baseEntry.fnAllocSize}, true)
	if !ok || got != 110 {
		t.Fatalf("resolveDataSize() = (%d, %t), want (110, true)", got, ok)
	}
}

func TestParse_MultipleDataStreams_Resident(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendResidentData(100)
	rb.appendResidentData(200)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.data.size(true, false) != 300 {
		t.Errorf("dataSize = %d, want 300 (sum of two streams)", entry.data.size(true, false))
	}
	if entry.data.size(false, false) != 300 {
		t.Errorf("allocatedSize = %d, want 300 (sum of two streams)", entry.data.size(false, false))
	}
}

func TestParse_MultipleDataStreams_NonResident(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0, 0, 4096, 4000, 0)
	rb.appendNonResidentData(0, 0, 8192, 7000, 0)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.data.size(false, false) != 12288 {
		t.Errorf("allocatedSize = %d, want 12288 (4096+8192)", entry.data.size(false, false))
	}
	if entry.data.size(true, false) != 11000 {
		t.Errorf("dataSize = %d, want 11000 (4000+7000)", entry.data.size(true, false))
	}
}

func TestParse_MultipleDataStreams_MixedResidence(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0, 0, 1<<30, 1<<30, 0)
	rb.appendResidentData(154)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := int64(1<<30) + 154
	if entry.data.size(false, false) != want {
		t.Errorf("allocatedSize = %d, want %d (1 GiB main + 154 B ADS)",
			entry.data.size(false, false), want)
	}
}

func TestParse_MultipleDataStreams_SparseFlagSticky(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	rb.appendNonResidentData(0x8000, 0, 1<<30, 1<<30, 4096)
	rb.appendNonResidentData(0, 0, 4096, 4000, 0)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !entry.isSparse {
		t.Error("isSparse = false, want true (one stream is sparse)")
	}
	if entry.data.size(false, false) != 8192 {
		t.Errorf("allocatedSize = %d, want 8192 (4096 sparse-physical + 4096 normal)",
			entry.data.size(false, false))
	}
}

func TestParse_FileNameSizesAsFallback(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(5, 4096, 1234, nsWin32AndDOS)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.fnAllocSize != 4096 {
		t.Errorf("fnAllocSize = %d, want 4096", entry.fnAllocSize)
	}
	if entry.fnDataSize != 1234 {
		t.Errorf("fnDataSize = %d, want 1234", entry.fnDataSize)
	}
}

func TestParse_ModeFileBaseOnly_SkipsExtension(t *testing.T) {
	rb := newBuilder(flagInUse, 555)
	rb.appendFileName(1, 0, 0, nsWin32AndDOS)
	var entry mftEntry
	baseRef, err := parseInto(rb.bytes(), testRecordSize, &entry, modeFileBaseOnly)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if baseRef != 555 {
		t.Errorf("baseRef = %d, want 555", baseRef)
	}
	if len(entry.hardlinkParents) != 0 {
		t.Error("extension record was parsed under modeFileBaseOnly")
	}
}

func TestParse_ModeFileBaseOnly_SkipsDirectory(t *testing.T) {
	rb := newBuilder(flagInUse|flagDirectory, 0)
	rb.appendFileName(5, 0, 0, nsWin32AndDOS)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeFileBaseOnly); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !entry.isDir {
		t.Error("isDir = false, want true")
	}
	if len(entry.hardlinkParents) != 0 || entry.primaryParent != 0 {
		t.Error("directory was parsed under modeFileBaseOnly")
	}
}

func TestParse_ModeFileBaseOnly_ParsesFileBase(t *testing.T) {
	rb := newBuilder(flagInUse, 0)
	rb.appendFileName(99, 4096, 1000, nsWin32AndDOS)
	rb.appendNonResidentData(0, 0, 4096, 1000, 0)
	var entry mftEntry
	if _, err := parseInto(rb.bytes(), testRecordSize, &entry, modeFileBaseOnly); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entry.primaryParent != 99 {
		t.Errorf("primaryParent = %d, want 99", entry.primaryParent)
	}
	if entry.data.size(false, false) != 4096 {
		t.Errorf("allocatedSize = %d, want 4096", entry.data.size(false, false))
	}
}

func TestParse_HardlinkBackingArrayReused(t *testing.T) {
	rb1 := newBuilder(flagInUse, 0)
	rb1.appendFileName(1, 0, 0, nsWin32AndDOS)
	rb1.appendFileName(2, 0, 0, nsWin32)
	rb1.appendFileName(3, 0, 0, nsPosix)

	var entry mftEntry
	if _, err := parseInto(rb1.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse 1: %v", err)
	}
	if cap(entry.hardlinkParents) < 3 {
		t.Fatalf("expected cap >= 3 after first parse, got %d", cap(entry.hardlinkParents))
	}
	firstAddr := &entry.hardlinkParents[:cap(entry.hardlinkParents)][0]

	rb2 := newBuilder(flagInUse, 0)
	rb2.appendFileName(99, 0, 0, nsWin32AndDOS)
	if _, err := parseInto(rb2.bytes(), testRecordSize, &entry, modeAll); err != nil {
		t.Fatalf("parse 2: %v", err)
	}
	if len(entry.hardlinkParents) != 1 || entry.hardlinkParents[0] != 99 {
		t.Errorf("hardlinkParents = %v, want [99]", entry.hardlinkParents)
	}
	secondAddr := &entry.hardlinkParents[:cap(entry.hardlinkParents)][0]
	if firstAddr != secondAddr {
		t.Error("hardlinkParents backing array was reallocated; reuse broke")
	}
}

func TestApplyFixups_RestoresSectorEnds(t *testing.T) {
	buf := make([]byte, testRecordSize)
	binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
	binary.LittleEndian.PutUint16(buf[4:6], 0x30)
	binary.LittleEndian.PutUint16(buf[6:8], 3)

	binary.LittleEndian.PutUint16(buf[0x30:0x32], 0xCAFE)
	binary.LittleEndian.PutUint16(buf[0x32:0x34], 0xDEAD)
	binary.LittleEndian.PutUint16(buf[0x34:0x36], 0xBEEF)

	binary.LittleEndian.PutUint16(buf[0x1FE:0x200], 0xCAFE)
	binary.LittleEndian.PutUint16(buf[0x3FE:0x400], 0xCAFE)

	if err := applyFixups(buf, testRecordSize); err != nil {
		t.Fatalf("applyFixups: %v", err)
	}

	if got := binary.LittleEndian.Uint16(buf[0x1FE:0x200]); got != 0xDEAD {
		t.Errorf("sector 1 end = 0x%X, want 0xDEAD", got)
	}
	if got := binary.LittleEndian.Uint16(buf[0x3FE:0x400]); got != 0xBEEF {
		t.Errorf("sector 2 end = 0x%X, want 0xBEEF", got)
	}
}

func TestApplyFixups_DetectsTornWrite(t *testing.T) {
	build := func() []byte {
		buf := make([]byte, testRecordSize)
		binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
		binary.LittleEndian.PutUint16(buf[4:6], 0x30) // USA offset
		binary.LittleEndian.PutUint16(buf[6:8], 3)    // USA size: 1 USN + 2 sectors
		// USN + saved-original bytes for each sector.
		binary.LittleEndian.PutUint16(buf[0x30:0x32], 0xCAFE) // USN
		binary.LittleEndian.PutUint16(buf[0x32:0x34], 0xDEAD) // sector 1 original
		binary.LittleEndian.PutUint16(buf[0x34:0x36], 0xBEEF) // sector 2 original
		// Both sector ends carry the USN (intact write).
		binary.LittleEndian.PutUint16(buf[0x1FE:0x200], 0xCAFE)
		binary.LittleEndian.PutUint16(buf[0x3FE:0x400], 0xCAFE)
		return buf
	}

	// Sector 2 was torn (USN replaced with stale/random bytes).
	torn := build()
	binary.LittleEndian.PutUint16(torn[0x3FE:0x400], 0xBEEF)
	if err := applyFixups(torn, testRecordSize); err == nil {
		t.Error("torn sector 2 was accepted; expected errTornWrite")
	}

	// Sector 1 was torn.
	torn1 := build()
	binary.LittleEndian.PutUint16(torn1[0x1FE:0x200], 0xDEAD)
	if err := applyFixups(torn1, testRecordSize); err == nil {
		t.Error("torn sector 1 was accepted; expected errTornWrite")
	}

	// On detected torn-write, the record buffer must NOT be modified.
	// Otherwise a caller that ignores the error would still see partially-
	// restored data.
	torn2 := build()
	binary.LittleEndian.PutUint16(torn2[0x3FE:0x400], 0xBEEF)
	before := make([]byte, len(torn2))
	copy(before, torn2)
	_ = applyFixups(torn2, testRecordSize)
	for i := range before {
		if before[i] != torn2[i] {
			t.Fatalf("buffer mutated at offset 0x%X on torn-write detection", i)
		}
	}
}

// TestApplyFixups_RejectsMalformedDescriptor covers the fixup-array *descriptor*
// itself being out of range — as opposed to a well-formed descriptor with a
// torn USN (TestApplyFixups_DetectsTornWrite). applyFixups must reject these
// with errBadFixup rather than failing open (returning nil), which would let an
// unvalidated record flow into the attribute walk.
func TestApplyFixups_RejectsMalformedDescriptor(t *testing.T) {
	base := func() []byte {
		buf := make([]byte, testRecordSize)
		binary.LittleEndian.PutUint32(buf[0:4], mftSignature)
		return buf
	}

	cases := []struct {
		name       string
		usaOffset  uint16
		usaCount   uint16
		recordSize int
	}{
		// fixupCount < 2 means there is not even one sector to protect; the
		// descriptor is nonsensical and must be rejected.
		{"count zero", 0x30, 0, testRecordSize},
		{"count one", 0x30, 1, testRecordSize},
		// The update-sequence array (offset + count*2 bytes) runs past the end
		// of the record, so reading the saved originals would be out of bounds.
		{"array past record end", 0x30, 0xFFFF, testRecordSize},
		{"offset past record end", uint16(testRecordSize - 2), 3, testRecordSize},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := base()
			binary.LittleEndian.PutUint16(buf[4:6], c.usaOffset)
			binary.LittleEndian.PutUint16(buf[6:8], c.usaCount)
			if err := applyFixups(buf, c.recordSize); !errors.Is(err, errBadFixup) {
				t.Errorf("applyFixups accepted malformed descriptor (%s); got err=%v, want errBadFixup", c.name, err)
			}
		})
	}
}

func TestMFTIndex_MasksUpperBits(t *testing.T) {
	const seqStamped = 0x0007_0000_DEAD_BEEF
	const wantIdx = 0x0000_0000_DEAD_BEEF
	if got := MFTIndex(seqStamped); got != wantIdx {
		t.Errorf("MFTIndex(0x%X) = 0x%X, want 0x%X", uint64(seqStamped), got, uint64(wantIdx))
	}
}
