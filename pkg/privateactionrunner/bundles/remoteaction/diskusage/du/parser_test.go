//go:build windows

package du

import (
	"encoding/binary"
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
	if entry.dataSize != 123 {
		t.Errorf("dataSize = %d, want 123", entry.dataSize)
	}
	if entry.allocatedSize != 123 {
		t.Errorf("allocatedSize = %d, want 123 (resident)", entry.allocatedSize)
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
	if entry.dataSize != 5000 {
		t.Errorf("dataSize = %d, want 5000", entry.dataSize)
	}
	if entry.allocatedSize != 8192 {
		t.Errorf("allocatedSize = %d, want 8192", entry.allocatedSize)
	}
	if entry.isSparse || entry.isCompressed {
		t.Error("normal data marked sparse/compressed")
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
	if entry.allocatedSize != physicalAlloc {
		t.Errorf("allocatedSize = %d, want %d (physical from offset 0x40)", entry.allocatedSize, physicalAlloc)
	}
	if entry.dataSize != virtualAlloc {
		t.Errorf("dataSize = %d, want %d", entry.dataSize, virtualAlloc)
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
	if entry.allocatedSize != 32768 {
		t.Errorf("allocatedSize = %d, want 32768 (compressed physical)", entry.allocatedSize)
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
	if entry.dataSize != 0 || entry.allocatedSize != 0 {
		t.Errorf("continuation fragment overwrote sizes: data=%d alloc=%d",
			entry.dataSize, entry.allocatedSize)
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
	if entry.dataSize != 300 {
		t.Errorf("dataSize = %d, want 300 (sum of two streams)", entry.dataSize)
	}
	if entry.allocatedSize != 300 {
		t.Errorf("allocatedSize = %d, want 300 (sum of two streams)", entry.allocatedSize)
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
	if entry.allocatedSize != 12288 {
		t.Errorf("allocatedSize = %d, want 12288 (4096+8192)", entry.allocatedSize)
	}
	if entry.dataSize != 11000 {
		t.Errorf("dataSize = %d, want 11000 (4000+7000)", entry.dataSize)
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
	if entry.allocatedSize != want {
		t.Errorf("allocatedSize = %d, want %d (1 GiB main + 154 B ADS)",
			entry.allocatedSize, want)
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
	if entry.allocatedSize != 8192 {
		t.Errorf("allocatedSize = %d, want 8192 (4096 sparse-physical + 4096 normal)",
			entry.allocatedSize)
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
	if entry.allocatedSize != 4096 {
		t.Errorf("allocatedSize = %d, want 4096", entry.allocatedSize)
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

func TestMFTIndex_MasksUpperBits(t *testing.T) {
	const seqStamped = 0x0007_0000_DEAD_BEEF
	const wantIdx = 0x0000_0000_DEAD_BEEF
	if got := MFTIndex(seqStamped); got != wantIdx {
		t.Errorf("MFTIndex(0x%X) = 0x%X, want 0x%X", uint64(seqStamped), got, uint64(wantIdx))
	}
}
