// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ntfsmft computes disk usage for a target directory on an NTFS volume
// by reading the raw $MFT.
//
// The volume I/O and scan orchestration are Windows-only (see du_windows.go);
// this file holds the pure $MFT record/attribute parser, which has no
// platform dependencies (stdlib only) so it can be unit-tested and fuzzed on
// any OS.
//
// Scan pipeline (see Scan in du_windows.go for section markers):
//
//   - Setup: open \\.\<drive>:, resolve the target and its immediate children
//     (resolveScopeIndices) to MFT indices via the Windows API (CreateFile,
//     GetFileInformationByHandle, FindFirstFile). Exclusion paths are resolved
//     to indices before the MFT walks so out-of-scope subtrees short-circuit
//     cheaply.
//
//   - Pass 1 (modeAll, one full MFT stream): build dirParent (directory →
//     parent idx), plus extSize and extParents per file base. Extension records
//     are folded into this pass so their $DATA sizes and spillover $FILE_NAME
//     parents are not rescanned. The bulk walk does not decode UTF-16 names.
//
//   - Map dirs to size accumulators (mapDirsToSizeAccumulators): assign each
//     directory to the running total pass 2 accumulates its subtree bytes into.
//     TreeDepth <= 1 (fast path) precomputes dirBucket via walkUp from target
//     and its immediate children so pass 2 attributes a file in O(1); TreeDepth
//     >= 2 (general path) retains dirParent and the in-tree anchor totals for
//     per-file chain walks in pass 2.
//
//   - Pass 2 (modeFileBaseOnly, or modeAll when TreeDepth >= 2): tally in-use
//     file base records into per-child / subtree totals; optional top-N files,
//     extension aggregation, and find predicates run inline in this callback.
//     The general path opportunistically decodes names only for dirs at
//     depth ≤ TreeDepth.
//
//   - Post-scan: assemble the optional Result.Tree; resolve top-file paths via
//     OpenFileByID (bounded, not part of the MFT stream).
//
//   - Pipelined ReadFile (double-buffered) overlaps disk I/O with parsing.
//
//   - parseMode header-only early exit skips the attribute walk on records a
//     pass cannot use (see modeAll / modeFileBaseOnly below).
//
//   - No per-file info map: pass 2 unions base + extension parents and adds
//     directly into totals. No per-file slice allocation on the hot path.
//
// Requires Administrator privileges (\\.\C: open).
package ntfsmft

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
)

// -------------------------------------------------------------------------
// MFT record / attribute constants
// -------------------------------------------------------------------------

const (
	mftSignature = 0x454C4946 // "FILE" little-endian

	attrStandardInfo  = 0x10
	attrAttributeList = 0x20
	attrFileName      = 0x30
	attrData          = 0x80
	attrEndMarker     = 0xFFFFFFFF

	// ATTRIBUTE_RECORD_HEADER field offsets, relative to the start of the
	// attribute record. Verified field-by-field against the documented struct:
	// https://learn.microsoft.com/en-us/windows/win32/devnotes/attribute-record-header
	attrOffFormCode     = 0x08 // UCHAR  FormCode
	attrOffNameLength   = 0x09 // UCHAR  NameLength (UTF-16 chars; 0 = unnamed)
	attrOffFlags        = 0x0C // USHORT Flags
	attrOffLowestVcn    = 0x10 // VCN    LowestVcn (non-resident form)
	attrOffMappingPairs = 0x20 // USHORT MappingPairsOffset (non-resident form)

	attrFormNonresident = 0x01 // NONRESIDENT_FORM (RESIDENT_FORM is 0x00)

	// attrNonresidentHeaderLen is the smallest valid non-resident attribute
	// header: through ValidDataLength at 0x38. TotalAllocated at 0x40 follows
	// only for compressed/sparse attributes. The mapping pairs array therefore
	// can never begin before this offset.
	attrNonresidentHeaderLen = 0x40

	// attrFlagsUnsupportedForMFT are the documented Flags bits that invalidate a
	// plain runlist-to-extent reading: ATTRIBUTE_FLAG_COMPRESSION_MASK (0x00FF),
	// ATTRIBUTE_FLAG_ENCRYPTED (0x4000) and ATTRIBUTE_FLAG_SPARSE (0x8000). A
	// compressed attribute's runs describe compression units and a sparse one has
	// holes, so neither maps directly onto "read these bytes as MFT records".
	attrFlagsUnsupportedForMFT = 0x00FF | 0x4000 | 0x8000

	flagInUse     = 0x01
	flagDirectory = 0x02

	nsPosix       = 0x00
	nsWin32       = 0x01
	nsDOS         = 0x02
	nsWin32AndDOS = 0x03

	// Records 0–15 are NTFS metafiles ($MFT, $MFTMirr, $LogFile, $Volume,
	// $AttrDef, root, $Bitmap, $Boot, $BadClus, $Secure, $UpCase, $Extend,
	// reserved 12–15). Real user-actionable system files (pagefile.sys,
	// hiberfil.sys, swapfile.sys) have idx >= 16.
	maxMetafileMFTIndex = 15

	// Root directory MFT index (always 5 on NTFS).
	rootDirMFTIndex = 5
)

// errBadSignature indicates the record does not start with "FILE".
var errBadSignature = errors.New("bad MFT signature")

// MFTIndex masks the lower 48 bits of an MFT file reference. The upper 16 are
// the sequence number; we don't need them for disk-usage tally because we
// always cross-reference by record index, not sequence-stamped reference.
func MFTIndex(ref uint64) uint64 {
	return ref & 0x0000FFFFFFFFFFFF
}

// -------------------------------------------------------------------------
// Parsed MFT entry — caller-buffer reuse via parseInto
// -------------------------------------------------------------------------

// hardlinkParents are the parent MFT indices from the $FILE_NAME attributes of
// a single record. A file with N hardlinks contributes N entries (one per
// $FILE_NAME, except DOS-only 8.3 aliases which we drop). For a directory or
// a single-link file this typically has 1 entry.
//
// The slice's backing array is reused across records via parseInto.

// mftEntry is the result of parsing a single MFT record. parseInto resets the
// fields on entry but preserves hardlinkParents' backing array, so the only
// allocations across many records are when hardlinkParents grows.
type mftEntry struct {
	// hardlinkParents is the list of parent MFT indices from non-DOS
	// $FILE_NAME attributes on this record. Used by the scan to attribute
	// hard-linked files to multiple buckets.
	hardlinkParents []uint64

	// primaryParent is the parent MFT index of the highest-namespace-priority
	// $FILE_NAME on this record. Falls back to hardlinkParents[0] if needed.
	primaryParent uint64

	// nameBytes is a slice into the record buffer of the raw UTF-16 little-
	// endian name from the highest-priority $FILE_NAME. Valid ONLY for the
	// duration of the streamPipelined callback that produced this entry; the
	// underlying buffer is reused on the next chunk. Callers needing the name
	// past the callback must copy it out.
	nameBytes []byte

	// sequence is the MFT record sequence number (header offset 0x10).
	// Combined with the record idx it forms the 64-bit NTFS file reference
	// used by OpenFileByID for post-scan path resolution.
	sequence uint16

	// fnAllocSize / fnDataSize are the cached sizes from the highest-priority
	// $FILE_NAME. They are only a fallback when no unnamed $DATA start is
	// recoverable from the base record or its extensions.
	fnAllocSize int64
	fnDataSize  int64

	data dataSummary

	isInUse      bool
	isDir        bool
	isSparse     bool
	isCompressed bool
}

// streamBytes is a stream's logical and allocated size.
type streamBytes struct {
	apparent  int64
	allocated int64
}

// dataSummary separates the ordinary unnamed stream from all named ADS
// streams. Attribute names are never decoded or retained: the header's
// name-length byte is enough to make this split.
type dataSummary struct {
	unnamed       streamBytes
	named         streamBytes
	unnamedStarts uint8
}

// selectedDataSummary is the one-mode representation retained for extension
// records during a scan. A scan never needs apparent and allocated values at
// the same time, keeping the potentially large extension map compact.
type selectedDataSummary struct {
	unnamed       int64
	named         int64
	unnamedStarts uint8
}

func (d *dataSummary) add(named bool, apparent, allocated int64) {
	if named {
		d.named.apparent = saturatingAdd(d.named.apparent, apparent)
		d.named.allocated = saturatingAdd(d.named.allocated, allocated)
		return
	}
	if d.unnamedStarts < ^uint8(0) {
		d.unnamedStarts++
	}
	d.unnamed.apparent = saturatingAdd(d.unnamed.apparent, apparent)
	d.unnamed.allocated = saturatingAdd(d.unnamed.allocated, allocated)
}

func (d dataSummary) empty() bool {
	return d.unnamedStarts == 0 && d.named.apparent == 0 && d.named.allocated == 0
}

func (d dataSummary) size(apparent bool, named bool) int64 {
	b := d.unnamed
	if named {
		b = d.named
	}
	if apparent {
		return b.apparent
	}
	return b.allocated
}

func (d dataSummary) selectSize(apparent bool) selectedDataSummary {
	return selectedDataSummary{
		unnamed:       d.size(apparent, false),
		named:         d.size(apparent, true),
		unnamedStarts: d.unnamedStarts,
	}
}

// resolveDataSize merges base and extension $DATA summaries. It returns false
// when metadata names more than one unnamed stream start, which is invalid.
func resolveDataSize(base, ext dataSummary, fn streamBytes, apparent bool) (int64, bool) {
	return resolveSelectedDataSize(base.selectSize(apparent), ext.selectSize(apparent), func() int64 {
		if apparent {
			return fn.apparent
		}
		return fn.allocated
	}())
}

func resolveSelectedDataSize(base, ext selectedDataSummary, fallback int64) (int64, bool) {
	if int(base.unnamedStarts)+int(ext.unnamedStarts) > 1 {
		return 0, false
	}
	unnamed := base.unnamed
	if base.unnamedStarts == 0 {
		if ext.unnamedStarts != 0 {
			unnamed = ext.unnamed
		} else {
			unnamed = fallback
		}
	}
	return saturatingAdd(unnamed, saturatingAdd(base.named, ext.named)), true
}

// -------------------------------------------------------------------------
// Parser modes
// -------------------------------------------------------------------------

// parseMode lets each pass skip records it cannot use after a 0x28-byte
// header read, before the attribute walk. Header-only early exit saves
// ~25–30% wall on the file-tally pass.
type parseMode uint8

const (
	// modeAll: parse every in-use record fully. The map-building pass uses
	// this so it can see directory base records AND extension records (for
	// dir-name spillover reconciliation, $DATA size accumulation per base,
	// and cross-bucket hardlink parents from extension $FILE_NAMEs).
	modeAll parseMode = iota

	// modeFileBaseOnly: skip records with baseRef != 0 OR isDir. The
	// file-tally pass uses this. The parent-only $FILE_NAME walk still runs
	// on bases so callers can attribute hardlinks across buckets.
	modeFileBaseOnly
)

// -------------------------------------------------------------------------
// Top-level parse
// -------------------------------------------------------------------------

// parseInto parses one MFT record into the caller-provided *mftEntry,
// preserving hardlinkParents' backing array.
//
// Returns (baseRef, error). baseRef is 0 for base records and non-zero (the
// MFT index of the file the extension belongs to) for extension records.
//
// For mode != modeAll, the function may return early after the header read
// without populating any attribute-derived fields. Callers must check the
// pass-specific predicates again before use; e.g. pass 2's callback re-checks
// isDir even though modeFileBaseOnly already filtered it, because dir flag
// is a header bit set early. (In practice it's a single-bit re-check.)
func parseInto(record []byte, recordSize int, entry *mftEntry, mode parseMode) (uint64, error) {
	// Reset fields, preserve hardlinks backing array.
	hl := entry.hardlinkParents[:0]
	*entry = mftEntry{hardlinkParents: hl}

	if len(record) < recordSize {
		return 0, errBadSignature
	}
	if binary.LittleEndian.Uint32(record[0:4]) != mftSignature {
		return 0, errBadSignature
	}
	if err := applyFixups(record, recordSize); err != nil {
		return 0, err
	}

	flags := binary.LittleEndian.Uint16(record[0x16:0x18])
	firstAttrOffset := binary.LittleEndian.Uint16(record[0x14:0x16])
	entry.sequence = binary.LittleEndian.Uint16(record[0x10:0x12])

	// base_record_file_reference at offset 0x20. Non-zero = extension record.
	var baseRef uint64
	if recordSize >= 0x28 {
		baseRef = MFTIndex(binary.LittleEndian.Uint64(record[0x20:0x28]))
	}

	entry.isInUse = flags&flagInUse != 0
	entry.isDir = flags&flagDirectory != 0
	if !entry.isInUse {
		return baseRef, nil
	}

	// Pass-mode early-exit: skip the attribute walk for records the file-
	// tally pass cannot use. Most records are extensions or directories and
	// get skipped here.
	if mode == modeFileBaseOnly {
		if baseRef != 0 || entry.isDir {
			return baseRef, nil
		}
	}

	// Walk the attribute chain.
	bestNS := -1
	offset := int(firstAttrOffset)
	for offset+8 <= recordSize {
		attrType := binary.LittleEndian.Uint32(record[offset : offset+4])
		if attrType == attrEndMarker || attrType == 0 {
			break
		}
		attrLen := int(binary.LittleEndian.Uint32(record[offset+4 : offset+8]))
		if attrLen < 16 || offset+attrLen > recordSize {
			break
		}

		switch attrType {
		case attrFileName:
			if err := parseFileNameParents(record[offset:offset+attrLen], entry, &bestNS); err != nil {
				return 0, err
			}
		case attrData:
			nonResident := record[offset+8]
			if nonResident == 1 {
				if err := parseNonResidentData(record[offset:offset+attrLen], entry); err != nil {
					return 0, err
				}
			} else {
				parseResidentData(record[offset:offset+attrLen], entry)
			}
		}

		offset += attrLen
	}

	return baseRef, nil
}

// -------------------------------------------------------------------------
// Attribute parsers
// -------------------------------------------------------------------------

// parseFileNameParents extracts only what the scan needs from $FILE_NAME:
// parent MFT idx (always), and sizes from the highest-priority namespace
// (used as fallback when $DATA is missing). The UTF-16 name is NEVER
// decoded — see package-level doc.
//
// DOS-only ($FILE_NAME with namespace == nsDOS) is the 8.3 alias of an
// existing Win32 entry; we drop it to avoid double-counting parents.
func parseFileNameParents(attr []byte, entry *mftEntry, bestNS *int) error {
	// parseInto only guarantees attrLen >= 16 before dispatching here, but the
	// resident-header reads below reach attr[0x14:0x16]. When a crafted
	// attribute sits within 22 bytes of the record end its capacity drops to
	// attrLen and the read panics. Guard like parseResidentData/NonResidentData.
	if len(attr) < 0x18 {
		return nil
	}
	contentOffset := int(binary.LittleEndian.Uint16(attr[0x14:0x16]))
	contentLen := int(binary.LittleEndian.Uint32(attr[0x10:0x14]))
	if contentOffset+contentLen > len(attr) || contentLen < 0x42 {
		return nil
	}
	c := attr[contentOffset : contentOffset+contentLen]

	parentRef := MFTIndex(binary.LittleEndian.Uint64(c[0x00:0x08]))
	allocSize, ok := safeSize(binary.LittleEndian.Uint64(c[0x28:0x30]))
	if !ok {
		return errBadSize
	}
	realSize, ok := safeSize(binary.LittleEndian.Uint64(c[0x30:0x38]))
	if !ok {
		return errBadSize
	}
	namespace := int(c[0x41])

	if namespace == nsDOS {
		return nil
	}

	entry.hardlinkParents = append(entry.hardlinkParents, parentRef)

	pri := nsPriority(namespace)
	if pri > *bestNS {
		*bestNS = pri
		entry.primaryParent = parentRef
		entry.fnAllocSize = allocSize
		entry.fnDataSize = realSize
		// Capture the raw UTF-16 name bytes for callers that need basename
		// or extension. Slice points into the record buffer — valid for the
		// callback duration only.
		nameLen := int(c[0x40])
		if 0x42+nameLen*2 <= len(c) {
			entry.nameBytes = c[0x42 : 0x42+nameLen*2]
		}
	}
	return nil
}

func nsPriority(ns int) int {
	switch ns {
	case nsWin32AndDOS:
		return 4
	case nsWin32:
		return 3
	case nsPosix:
		return 2
	default:
		return 0
	}
}

// parseResidentData: $DATA is small enough to live inside the MFT record.
//
// A resident stream allocates zero clusters, so Windows' "size on disk"
// (GetCompressedFileSizeW / Explorer) reports 0 for a tiny resident file — the
// classic ~700-byte crossover where $DATA spills out to clusters. RESIDENT_FORM
// means "the value is contained in the file record" per the attribute record
// header docs:
// https://learn.microsoft.com/en-us/windows/win32/devnotes/attribute-record-header
//
// We nonetheless add the content length to allocatedSize (not just dataSize):
// the bytes physically occupy the $MFT's own on-disk allocation, and the scan
// skips the $MFT metafile itself, so attributing each resident stream to its
// owning file is what keeps whole-volume totals from dropping those bytes on
// the floor. Counting them per-file also matches what a raw GetCompressedFileSizeW
// probe returns for resident data on a real volume (content length, not 0).
//
// A single MFT record can hold multiple $DATA attributes when the file has
// alternate data streams (e.g. the unnamed main stream + a Zone.Identifier
// ADS on a downloaded file). Each is its own $DATA attribute. We accumulate
// across them so the reported size matches the file's true on-disk usage.
func parseResidentData(attr []byte, entry *mftEntry) {
	if len(attr) < 0x18 {
		return
	}
	contentLen := int64(binary.LittleEndian.Uint32(attr[0x10:0x14]))
	addDataSize(entry, attr[9] != 0, contentLen, contentLen)
}

// parseNonResidentData: $DATA is in cluster runs on disk. AllocatedLength /
// FileSize are valid only when LowestVcn == 0 (per MS spec). Continuation
// fragments must be ignored; the base $DATA's sizes are authoritative.
//
// For sparse or compressed files, offset 0x40 ("Total allocated size") gives
// the actual on-disk allocation accounting for sparse holes / compression.
// Offset 0x28 is the VIRTUAL allocation including holes — useful for apparent
// size reporting but wrong for "size on disk" mode.
//
// Multiple $DATA attributes (alternate data streams) on the same record each
// contribute their own first-fragment sizes. We accumulate; sparse/compressed
// flags are sticky (any-stream).
func parseNonResidentData(attr []byte, entry *mftEntry) error {
	if len(attr) < 0x40 {
		return nil
	}
	dataFlags := binary.LittleEndian.Uint16(attr[0x0C:0x0E])
	isCompressed := dataFlags&0x0001 != 0
	isSparse := dataFlags&0x8000 != 0

	lowestVcn := binary.LittleEndian.Uint64(attr[0x10:0x18])
	if lowestVcn != 0 {
		return nil // continuation run — sizes are invalid
	}

	dataSize, ok := safeSize(binary.LittleEndian.Uint64(attr[0x30:0x38]))
	if !ok {
		return errBadSize
	}
	var allocSize int64
	if (isSparse || isCompressed) && len(attr) >= 0x48 {
		allocSize, ok = safeSize(binary.LittleEndian.Uint64(attr[0x40:0x48]))
	} else {
		allocSize, ok = safeSize(binary.LittleEndian.Uint64(attr[0x28:0x30]))
	}
	if !ok {
		return errBadSize
	}

	addDataSize(entry, attr[9] != 0, dataSize, allocSize)
	if isSparse {
		entry.isSparse = true
	}
	if isCompressed {
		entry.isCompressed = true
	}
	return nil
}

// addDataSize records one authoritative $DATA stream start. named is derived
// directly from the attribute header; retaining only that distinction avoids
// allocating or decoding stream names during the bulk MFT walk.
func addDataSize(entry *mftEntry, named bool, dataSize, allocSize int64) {
	entry.data.add(named, dataSize, allocSize)
}

// -------------------------------------------------------------------------
// Multi-sector transfer protection (fixups)
// -------------------------------------------------------------------------

// errTornWrite is returned by applyFixups when a sector-end USN does not
// match the header USN, indicating the write that produced this record
// did not complete atomically. The record's content is in an
// indeterminate state and must not be parsed.
var errTornWrite = errors.New("torn write detected (USN mismatch)")

// errBadFixup is returned by applyFixups when the fixup (update sequence
// array) descriptor is malformed: fewer than 2 words, or an offset/count
// that places the array outside the record. Such a record cannot be
// validated or restored, so it must be rejected rather than parsed with
// unrestored (USN-corrupted) sector ends.
var errBadFixup = errors.New("malformed fixup descriptor")

// errBadSize is returned when a raw NTFS size field has its high bit set
// (value > math.MaxInt64). No real NTFS volume can hold a file or allocation
// that large (the maximum file size is far below 8 EiB), so such a value only
// occurs on a corrupted or crafted image. Casting it to int64 would wrap to a
// negative number and corrupt the scan totals, so the record is rejected.
var errBadSize = errors.New("size field exceeds int64 range")

// safeSize converts a raw uint64 NTFS size field to int64, reporting ok=false
// when the value would wrap negative (high bit set). See errBadSize.
func safeSize(raw uint64) (int64, bool) {
	if raw > math.MaxInt64 {
		return 0, false
	}
	return int64(raw), true
}

// saturatingAdd adds two non-negative int64 size values, clamping to MaxInt64
// instead of wrapping negative. safeSize rejects any single field with its high
// bit set, but summing several near-MaxInt64 fields (e.g. multiple $DATA streams
// on one record, or many records on a crafted image) would still overflow the
// accumulators and corrupt the totals. Both operands are always >= 0 here.
func saturatingAdd(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

// applyFixups validates the multi-sector transfer protection on an MFT
// record and restores the original sector-end bytes in place.
//
// NTFS writes records sector-by-sector. At write time it places a USN at
// the last 2 bytes of every 512-byte sector (overwriting the real
// content) and stashes the original bytes in the update sequence array
// at the start of the record. On read we must:
//
//  1. Validate that every sector-end still equals the USN. A mismatch
//     means the write was torn (process / power interrupted mid-write)
//     and the sector contents are unreliable.
//  2. Restore the original bytes from the USA back to the sector ends,
//     so the parser sees the record's real content rather than the USN.
//
// Without step 2 the parser reads USN garbage at every 512-byte boundary;
// without step 1 we silently parse a partially-written record. Matches
// what the in-kernel NTFS driver does at the file API layer.
func applyFixups(record []byte, recordSize int) error {
	fixupOffset := binary.LittleEndian.Uint16(record[4:6])
	fixupCount := binary.LittleEndian.Uint16(record[6:8])
	// The update sequence array holds one USN plus one saved word per 512-byte
	// sector, so a valid record declares exactly recordSize/512 + 1 entries.
	// recordSize is a validated multiple of 512 (see validateNTFSLayout). A
	// smaller count (e.g. 2 on a 1024-byte record) would leave later sectors'
	// USN-corrupted end bytes unvalidated and unrestored, feeding torn-write
	// garbage to the attribute parser — so reject anything but the exact count.
	expected := recordSize/512 + 1
	if int(fixupCount) != expected || int(fixupOffset)+int(fixupCount)*2 > recordSize {
		return errBadFixup
	}

	// First word of the USA is the USN; the remaining words are the saved
	// original bytes for each sector.
	usn0 := record[int(fixupOffset)]
	usn1 := record[int(fixupOffset)+1]

	// Pass 1: USN validation. Walk every sector and confirm its trailing
	// 2 bytes still equal the header USN. If any sector fails, the write
	// did not complete atomically and we must reject the record before
	// touching its content.
	for i := uint16(1); i < fixupCount; i++ {
		sectorEnd := int(i)*512 - 2
		if sectorEnd+2 > recordSize {
			break
		}
		if record[sectorEnd] != usn0 || record[sectorEnd+1] != usn1 {
			return errTornWrite
		}
	}

	// Pass 2: restore. Copy the saved bytes from the USA back to each
	// sector end. Cannot be folded into pass 1 because step 1 must
	// complete (verify all sectors are intact) before we mutate anything.
	for i := uint16(1); i < fixupCount; i++ {
		sectorEnd := int(i)*512 - 2
		if sectorEnd+2 > recordSize {
			break
		}
		fvOff := int(fixupOffset) + int(i)*2
		if fvOff+2 > recordSize {
			break
		}
		record[sectorEnd] = record[fvOff]
		record[sectorEnd+1] = record[fvOff+1]
	}
	return nil
}

// -------------------------------------------------------------------------
// MFT extents (record 0 + $ATTRIBUTE_LIST chasing)
// -------------------------------------------------------------------------
//
// This block resolves the on-disk byte ranges of the $MFT itself. The only
// platform coupling is disk I/O, injected via the readerAt seam, so the byte
// parsing here is pure and cross-platform (unit-tested and fuzzed on any OS).

// volumeInfo carries the volume geometry needed to locate and parse the $MFT.
type volumeInfo struct {
	recordSize      int
	bytesPerCluster int64
	totalClusters   int64
	mftStartByte    int64
	mftValidBytes   int64
}

// extent is one contiguous on-disk byte range of the $MFT.
//
// A byteOffset of unmappedExtent marks a range whose disk location is unknown. It
// stays in the list because record indices come from position in the stream, so
// dropping it would renumber every later record.
type extent struct {
	byteOffset int64
	byteLength int64
}

// unmappedExtent is negative so it can never collide with a real offset.
const unmappedExtent = -1

// mftSegment is one $DATA attribute segment of $MFT: the stream VCN it starts at
// plus the extents its runlist maps. $MFT's $DATA may span several segments,
// encountered in any order, so they are merged by mftSegment ordering rather than
// by arrival.
type mftSegment struct {
	lowestVcn uint64
	runs      []extent
}

// clusters returns the number of clusters the segment's runs cover.
func (s mftSegment) clusters(bytesPerCluster int64) uint64 {
	var total int64
	for _, ex := range s.runs {
		total += ex.byteLength
	}
	if bytesPerCluster <= 0 {
		return 0
	}
	return uint64(total / bytesPerCluster)
}

// mftStreamOffset returns the byte offset within the $MFT stream at which the
// segment begins.
func (s mftSegment) mftStreamOffset(bytesPerCluster int64) int64 {
	return int64(s.lowestVcn) * bytesPerCluster
}

// mergeMFTSegments flattens the segments into the positional extent list the scan
// consumes, in VCN order.
//
// Record indices come from position in that list, which only matches the true
// index while it covers the stream contiguously from VCN 0. Establishing that
// invariant is what keeps streamPipelined's positional walk correct:
//
//   - a hole becomes an unmappedExtent of exactly the missing length, so later
//     records keep their indices;
//   - an overlap is rejected; guessing which segment is wrong would corrupt every
//     index after it.
func mergeMFTSegments(segments []mftSegment, vol *volumeInfo) ([]extent, int64, error) {
	if len(segments) == 0 {
		return nil, 0, errors.New("no $DATA in record 0")
	}
	// A healthy volume has one segment covering all of $MFT.
	if len(segments) == 1 && segments[0].lowestVcn == 0 {
		covered := int64(segments[0].clusters(vol.bytesPerCluster)) * vol.bytesPerCluster
		exts, unmapped := clampToValidLength(segments[0].runs, covered, 0, vol)
		return exts, unmapped, nil
	}

	sorted := slices.Clone(segments)
	sortSegmentsByVcn(sorted)

	var out []extent
	var nextVcn uint64
	var unmapped int64
	for _, seg := range sorted {
		switch {
		case seg.lowestVcn < nextVcn:
			return nil, 0, fmt.Errorf("overlapping $MFT $DATA segments: segment at VCN %d re-covers VCN %d",
				seg.lowestVcn, nextVcn-1)
		case seg.lowestVcn > nextVcn:
			gap := int64(seg.lowestVcn-nextVcn) * vol.bytesPerCluster
			out = append(out, extent{byteOffset: unmappedExtent, byteLength: gap})
			unmapped += gap
		}
		out = append(out, seg.runs...)
		nextVcn = seg.lowestVcn + seg.clusters(vol.bytesPerCluster)
	}
	// Runs are always whole clusters (decodeDataRuns multiplies by bytesPerCluster),
	// and gaps advance nextVcn too, so this is the list's exact total length.
	covered := int64(nextVcn) * vol.bytesPerCluster
	exts, unmapped := clampToValidLength(out, covered, unmapped, vol)
	return exts, unmapped, nil
}

// clampToValidLength makes the extent list describe exactly the $MFT's valid data
// length, which is the authority for how many records exist. covered is the list's
// total length and unmapped the part of it with no known location; both are already
// known to the caller, so neither is recomputed here.
//
//   - Short of it: append an unmapped range. Segments are missing, and the records
//     they covered must be reported rather than silently dropped.
//   - Past it: truncate. $MFT's allocation can legitimately exceed its valid length
//     (NTFS grows the allocation in chunks without advancing that length), and a
//     corrupt runlist can claim more still. Those bytes are $MFT slack, and reading
//     the raw volume does not return zeros for them the way reading through the
//     filesystem would: they can hold a stale MFT region that still carries a valid
//     signature, the in-use flag and intact fixups, which would be tallied as
//     phantom files. Truncating also keeps the streamed record count consistent with
//     the TotalMFTRecords the scan reports.
func clampToValidLength(exts []extent, covered, unmapped int64, vol *volumeInfo) ([]extent, int64) {
	valid := vol.mftValidBytes
	switch {
	case valid <= 0:
		// Nothing trustworthy to clamp against; keep the map as parsed rather than
		// trimming it to nothing.
		return exts, unmapped
	case covered == valid:
		return exts, unmapped
	case covered < valid:
		tail := valid - covered
		return append(exts, extent{byteOffset: unmappedExtent, byteLength: tail}), unmapped + tail
	}

	// Past the valid length: locate the extent the boundary falls in. The list is
	// ordered by stream position, so this is a prefix cut.
	var pos int64
	cut, shorten := len(exts), int64(0)
	for i, ex := range exts {
		if pos+ex.byteLength > valid {
			cut, shorten = i, valid-pos
			break
		}
		pos += ex.byteLength
	}

	// Clone rather than reslice-and-assign: exts can alias a caller's segment runs,
	// and shortening an extent in place would corrupt them.
	out := slices.Clone(exts[:cut])
	if shorten > 0 {
		out = append(out, extent{byteOffset: exts[cut].byteOffset, byteLength: shorten})
	}
	// Recompute over the kept prefix; adjusting the running total for a shortened
	// extent plus every dropped one is easy to get subtly wrong for no gain.
	var kept int64
	for _, ex := range out {
		if ex.byteOffset == unmappedExtent {
			kept += ex.byteLength
		}
	}
	return out, kept
}

// mftStreamToDisk maps a byte offset within the $MFT stream to an absolute disk
// offset, using the segments discovered so far (which must be VCN-sorted). ok is
// false when no segment maps the offset, which is how the extension chase learns
// it cannot reach a record yet.
func mftStreamToDisk(segments []mftSegment, bytesPerCluster int64, streamOff int64) (int64, bool) {
	for _, seg := range segments {
		cum := seg.mftStreamOffset(bytesPerCluster)
		for _, ex := range seg.runs {
			if streamOff >= cum && streamOff < cum+ex.byteLength {
				return ex.byteOffset + (streamOff - cum), true
			}
			cum += ex.byteLength
		}
	}
	return 0, false
}

// readerAt reads len(buf) bytes from the volume at an absolute byte offset.
// On Windows it wraps ReadFile (see du_windows.go); tests inject an in-memory
// reader so getMFTExtents and its helpers run on any platform.
type readerAt func(buf []byte, offset int64) error

// maxAttrListBytes caps how much of a non-resident $ATTRIBUTE_LIST we will
// read into memory (RULES.md memory-safety: never allocate from untrusted
// on-disk sizes without a bound). Real MFT attribute lists are a few KiB even
// on heavily fragmented volumes; 4 MiB is far above any legitimate size.
const maxAttrListBytes = 4 << 20

// getMFTExtents builds the map of where $MFT physically lives, which every later
// pass reads through.
//
// Assembling it is self-referential: the records describing $MFT's location are
// themselves stored in $MFT. The boot sector gives record 0 outright, and on a
// healthy volume its own $DATA covers the whole stream. A fragmented $MFT needs
// extension records, named by record 0's $ATTRIBUTE_LIST.
//
// That record 0 reaches those extensions is an observation about healthy volumes,
// not a format guarantee, so an unreachable extension yields a reported hole
// rather than a silent truncation.
func getMFTExtents(read readerAt, vol *volumeInfo) ([]extent, mftMapGaps, error) {
	var gaps mftMapGaps

	rec0, err := readMFTRecord(read, vol, vol.mftStartByte)
	if err != nil {
		return nil, gaps, fmt.Errorf("record 0: %w", err)
	}

	segments, err := mftDataSegments(rec0, vol)
	if err != nil {
		return nil, gaps, fmt.Errorf("record 0 $DATA: %w", err)
	}

	attrList, err := mftAttributeList(read, rec0, vol)
	if err != nil {
		return nil, gaps, err
	}
	if len(attrList) > 0 {
		segments, gaps.unreachableExtensions = resolveMFTExtensions(read, vol, segments, attrList)
	}

	exts, unmappedBytes, err := mergeMFTSegments(segments, vol)
	if err != nil {
		return nil, gaps, err
	}
	gaps.unmappedBytes = unmappedBytes
	return exts, gaps, nil
}

// mftMapGaps records what the assembled $MFT map is missing:
//
//   - unmappedBytes: how much of the $MFT we could not locate, so those records
//     were never read. This is how much was lost.
//   - unreachableExtensions: how many extension records we could not read. This is
//     why it was lost.
//
// They can disagree. Failing to read an extension record costs nothing if the
// segments we did resolve already cover the range it would have described, so
// unreachableExtensions can be nonzero while unmappedBytes is 0.
type mftMapGaps struct {
	unreachableExtensions int
	unmappedBytes         int64
}

// readMFTRecord reads one MFT record and validates its signature and fixups.
func readMFTRecord(read readerAt, vol *volumeInfo, diskOffset int64) ([]byte, error) {
	rec := make([]byte, vol.recordSize)
	if err := read(rec, diskOffset); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(rec[0:4]) != mftSignature {
		return nil, errBadSignature
	}
	if err := applyFixups(rec, vol.recordSize); err != nil {
		return nil, err
	}
	return rec, nil
}

// mftDataSegments returns every $DATA segment in a record that describes $MFT's
// own clusters. A record may hold more than one, and not necessarily the VCN-0
// one, so each keeps its LowestVcn instead of being collapsed.
func mftDataSegments(rec []byte, vol *volumeInfo) ([]mftSegment, error) {
	var segs []mftSegment
	var decodeErr error
	forEachAttribute(rec, vol.recordSize, func(attrType uint32, attr []byte) {
		if attrType != attrData || decodeErr != nil {
			return
		}
		if runs, lowestVcn, ok := mftDataRuns(attr); ok {
			decoded, err := decodeDataRuns(runs, vol.bytesPerCluster, vol.totalClusters)
			if err != nil {
				decodeErr = err
				return
			}
			if len(decoded) == 0 {
				decodeErr = errors.New("no data runs")
				return
			}
			segs = append(segs, mftSegment{
				lowestVcn: lowestVcn,
				runs:      decoded,
			})
		}
	})
	return segs, decodeErr
}

// mftAttributeList returns record 0's $ATTRIBUTE_LIST entries, or nil when $MFT's
// $DATA fits in the base record and there is nothing to chase.
func mftAttributeList(read readerAt, rec0 []byte, vol *volumeInfo) ([]attrListEntry, error) {
	var entries []attrListEntry
	var readErr error
	forEachAttribute(rec0, vol.recordSize, func(attrType uint32, attr []byte) {
		if attrType != attrAttributeList || entries != nil || readErr != nil {
			return
		}
		if attr[attrOffFormCode] != attrFormNonresident {
			entries = parseAttributeList(attr)
			return
		}
		// The entries live in disk clusters, but the runlist locating them is inline
		// here as raw LCNs, readable without the extent map still being assembled.
		content, err := readNonResidentAttrList(read, attr, vol.bytesPerCluster, vol.totalClusters)
		if err != nil {
			readErr = fmt.Errorf("record 0 $ATTRIBUTE_LIST: %w", err)
			return
		}
		entries = parseAttributeListEntries(content)
	})
	return entries, readErr
}

// forEachAttribute walks a record's attributes in order. It stops at the end
// marker or an impossible length, so a malformed record truncates the walk rather
// than reading past the buffer.
func forEachAttribute(rec []byte, recordSize int, fn func(attrType uint32, attr []byte)) {
	firstAttrOff := int(binary.LittleEndian.Uint16(rec[0x14:0x16]))
	for off := firstAttrOff; off >= 0 && off+8 <= recordSize; {
		attrType := binary.LittleEndian.Uint32(rec[off : off+4])
		if attrType == attrEndMarker || attrType == 0 {
			return
		}
		length := int(binary.LittleEndian.Uint32(rec[off+4 : off+8]))
		if length < 16 || off+length > recordSize {
			return
		}
		fn(attrType, rec[off:off+length])
		off += length
	}
}

// resolveMFTExtensions chases the $DATA extension records named by $MFT's
// $ATTRIBUTE_LIST.
//
// Locating one needs whichever part of the map other segments supply, so an entry
// unreachable on one pass can become reachable after another resolves; the loop
// repeats until a pass resolves nothing new. Entries are known up front and
// resolving one never reveals more, so this is a fixpoint filter, not a traversal
// — rounds over a shrinking list rather than a requeueing queue, which makes
// termination evident: each round either shortens the list or breaks.
//
// Entries that stay unreachable are dropped deliberately; their own location is
// the missing piece. mergeMFTSegments reports the resulting hole.
// It returns the segments plus the number of entries that stayed unreachable.
func resolveMFTExtensions(read readerAt, vol *volumeInfo, segments []mftSegment, attrList []attrListEntry) ([]mftSegment, int) {
	// Record 0 is already accounted for by the caller.
	seen := map[uint64]bool{0: true}
	pending := make([]uint64, 0, len(attrList))
	for _, e := range attrList {
		if e.attrType != attrData || seen[e.mftRef] {
			continue
		}
		seen[e.mftRef] = true
		pending = append(pending, e.mftRef)
	}

	for len(pending) > 0 {
		var unreachable []uint64
		for _, ref := range pending {
			segs, located := readExtensionSegments(read, vol, segments, ref)
			if !located {
				unreachable = append(unreachable, ref)
				continue
			}
			if len(segs) > 0 {
				segments = append(segments, segs...)
				sortSegmentsByVcn(segments)
			}
		}
		if len(unreachable) == len(pending) {
			break // a whole pass resolved nothing; no further progress is possible
		}
		pending = unreachable
	}
	return segments, len(pending)
}

// readExtensionSegments reads the extension record at mftIdx, mapping its index
// through the segments known so far.
//
// located separates the one failure worth retrying: false means the record's disk
// location is still unknown, so a later segment may reveal it. true with no
// segments means it was reached but unusable (I/O error, bad signature, torn
// write), which re-reading cannot fix.
func readExtensionSegments(read readerAt, vol *volumeInfo, segments []mftSegment, mftIdx uint64) (segs []mftSegment, located bool) {
	streamOff := int64(mftIdx) * int64(vol.recordSize)
	disk, ok := mftStreamToDisk(segments, vol.bytesPerCluster, streamOff)
	if !ok {
		return nil, false
	}
	rec, err := readMFTRecord(read, vol, disk)
	if err != nil {
		return nil, true
	}
	segs, err = mftDataSegments(rec, vol)
	if err != nil {
		// An extension record is outside the trusted bootstrap.  Leave its range
		// unmapped so the scan can report an incomplete map, just as it does for
		// a torn or unreadable extension record, rather than discard known runs.
		return nil, true
	}
	return segs, true
}

// sortSegmentsByVcn keeps the list ordered while it is being assembled, so
// mftStreamToDisk can map indices against the segments found so far.
func sortSegmentsByVcn(segs []mftSegment) {
	slices.SortFunc(segs, func(a, b mftSegment) int {
		return cmp.Compare(a.lowestVcn, b.lowestVcn)
	})
}

// mftDataRuns returns the mapping-pairs (data run) bytes and LowestVcn of attr
// when it is the unnamed, non-resident, uncompressed $DATA attribute describing
// $MFT's own clusters, and ok=false for any other flavour of $DATA.
//
// The type code alone is not enough, because these runs become the disk offsets
// the scan reads as MFT records. Rejected:
//
//   - NameLength != 0: an alternate data stream, with its own unrelated extents.
//   - resident form: describes no clusters at all.
//   - compressed / sparse / encrypted: the runs are not plain byte extents.
//   - MappingPairsOffset inside the header: would decode header fields as runs.
//
// Offsets per ATTRIBUTE_RECORD_HEADER:
// https://learn.microsoft.com/en-us/windows/win32/devnotes/attribute-record-header
func mftDataRuns(attr []byte) (runs []byte, lowestVcn uint64, ok bool) {
	if len(attr) < attrNonresidentHeaderLen {
		return nil, 0, false
	}
	if attr[attrOffFormCode] != attrFormNonresident {
		return nil, 0, false
	}
	if attr[attrOffNameLength] != 0 {
		return nil, 0, false
	}
	flags := binary.LittleEndian.Uint16(attr[attrOffFlags : attrOffFlags+2])
	if flags&attrFlagsUnsupportedForMFT != 0 {
		return nil, 0, false
	}
	drOff := int(binary.LittleEndian.Uint16(attr[attrOffMappingPairs : attrOffMappingPairs+2]))
	if drOff < attrNonresidentHeaderLen || drOff > len(attr) {
		return nil, 0, false
	}
	lowVcn := binary.LittleEndian.Uint64(attr[attrOffLowestVcn : attrOffLowestVcn+8])
	// A VCN is a cluster index within the stream; one at or above 2^48 would
	// describe an $MFT vastly larger than any volume and would overflow the
	// byte arithmetic below, so treat it as corruption.
	if lowVcn > 1<<48 {
		return nil, 0, false
	}
	return attr[drOff:], lowVcn, true
}

// readNonResidentAttrList reads the content of a non-resident $ATTRIBUTE_LIST
// attribute by following its inline data runs directly by LCN. It applies a
// fixed size bound and validates the runlist before allocating.
func readNonResidentAttrList(read readerAt, attr []byte, bytesPerCluster, totalClusters int64) ([]byte, error) {
	// Non-resident attribute header: MappingPairsOffset at 0x20 (2 bytes),
	// real data size at 0x30 (8 bytes); the header is at least 0x40 bytes.
	if len(attr) < 0x38 {
		return nil, errors.New("attribute too short for non-resident header")
	}
	drOff := int(binary.LittleEndian.Uint16(attr[0x20:0x22]))
	dataSize := int64(binary.LittleEndian.Uint64(attr[0x30:0x38]))
	if drOff < 0x40 || drOff > len(attr) {
		return nil, fmt.Errorf("bad data-run offset %d", drOff)
	}
	if dataSize <= 0 || dataSize > maxAttrListBytes {
		return nil, fmt.Errorf("content size %d out of range", dataSize)
	}
	runs, err := decodeDataRuns(attr[drOff:], bytesPerCluster, totalClusters)
	if err != nil {
		return nil, fmt.Errorf("invalid data runs: %w", err)
	}
	if len(runs) == 0 {
		return nil, errors.New("no data runs")
	}
	var allocated int64
	for _, r := range runs {
		if r.byteLength <= 0 {
			return nil, errors.New("bad run length")
		}
		// Keep the fixed allocation bound checked before the addition. A
		// malicious runlist can use a very large individually-valid extent
		// after a small one; adding it first could wrap allocated negative and
		// bypass the post-addition upper-bound check below.
		if r.byteLength > maxAttrListBytes-allocated {
			return nil, fmt.Errorf("allocated data runs exceed %d", maxAttrListBytes)
		}
		allocated += r.byteLength
	}
	if allocated < dataSize {
		return nil, fmt.Errorf("runs cover %d < content size %d", allocated, dataSize)
	}
	buf := make([]byte, allocated)
	pos := 0
	for _, r := range runs {
		if err := read(buf[pos:pos+int(r.byteLength)], r.byteOffset); err != nil {
			return nil, err
		}
		pos += int(r.byteLength)
	}
	return buf[:dataSize], nil
}

// decodeDataRuns decodes an NTFS data run list into disk extents.
func decodeDataRuns(data []byte, bytesPerCluster, totalClusters int64) ([]extent, error) {
	if totalClusters <= 0 {
		return nil, fmt.Errorf("invalid volume cluster count %d", totalClusters)
	}
	var ext []extent
	var lcn int64
	pos := 0
	for pos < len(data) {
		hdr := data[pos]
		if hdr == 0 {
			return ext, nil
		}
		pos++
		lenSz := int(hdr & 0x0F)
		offSz := int((hdr >> 4) & 0x0F)
		if lenSz == 0 || lenSz > 8 || offSz > 8 {
			return nil, fmt.Errorf("invalid data-run field sizes length=%d offset=%d", lenSz, offSz)
		}
		if pos+lenSz+offSz > len(data) {
			return nil, errors.New("truncated data run")
		}
		var runLenU uint64
		for i := 0; i < lenSz; i++ {
			runLenU |= uint64(data[pos+i]) << (uint(i) * 8)
		}
		pos += lenSz
		if runLenU == 0 || runLenU > math.MaxInt64 {
			return nil, fmt.Errorf("invalid data-run length %d", runLenU)
		}
		runLen := int64(runLenU)
		if offSz == 0 {
			continue // sparse run, no offset
		}
		var runOff int64
		for i := 0; i < offSz; i++ {
			runOff |= int64(data[pos+i]) << (uint(i) * 8)
		}
		if data[pos+offSz-1]&0x80 != 0 {
			for i := offSz; i < 8; i++ {
				runOff |= int64(0xFF) << (uint(i) * 8)
			}
		}
		pos += offSz
		if (runOff > 0 && lcn > math.MaxInt64-runOff) || (runOff < 0 && lcn < (-math.MaxInt64-1)-runOff) {
			return nil, errors.New("data-run LCN addition overflows")
		}
		lcn += runOff
		if lcn < 0 {
			return nil, fmt.Errorf("negative data-run LCN %d", lcn)
		}
		if lcn >= totalClusters || runLen > totalClusters-lcn {
			return nil, fmt.Errorf("data run LCN %d length %d exceeds volume size %d clusters", lcn, runLen, totalClusters)
		}
		// runLen and lcn are assembled from up to 8 attacker-controlled on-disk
		// bytes; guard the cluster->byte multiplications against int64 overflow
		// (which would yield negative/garbage extents) and reject the runlist.
		byteOff, ok1 := mulNoOverflow(lcn, bytesPerCluster)
		byteLen, ok2 := mulNoOverflow(runLen, bytesPerCluster)
		if !ok1 || !ok2 {
			return nil, errors.New("data-run byte offset or length overflows")
		}
		ext = append(ext, extent{byteOffset: byteOff, byteLength: byteLen})
	}
	return nil, errors.New("unterminated data-run list")
}

// mulNoOverflow returns a*b and ok=false when the signed multiplication would
// overflow int64. b (bytesPerCluster) is always > 0 here.
func mulNoOverflow(a, b int64) (int64, bool) {
	if b == 0 {
		return 0, true
	}
	p := a * b
	if p/b != a {
		return 0, false
	}
	return p, true
}

// attrListEntry is one entry from an $ATTRIBUTE_LIST attribute.
type attrListEntry struct {
	attrType uint32
	mftRef   uint64
}

// parseAttributeList decodes the resident form of an $ATTRIBUTE_LIST. The
// non-resident form is read separately (see readNonResidentAttrList) and its
// content passed to parseAttributeListEntries.
func parseAttributeList(attr []byte) []attrListEntry {
	if len(attr) < 24 || attr[8] == 1 {
		return nil
	}
	contentOff := int(binary.LittleEndian.Uint16(attr[0x14:0x16]))
	contentLen := int(binary.LittleEndian.Uint32(attr[0x10:0x14]))
	if contentOff+contentLen > len(attr) {
		return nil
	}
	return parseAttributeListEntries(attr[contentOff : contentOff+contentLen])
}

// parseAttributeListEntries walks the raw entry list of an $ATTRIBUTE_LIST
// (the content bytes, resident or non-resident) and returns its entries.
func parseAttributeListEntries(c []byte) []attrListEntry {
	var out []attrListEntry
	pos := 0
	for pos+0x18 <= len(c) {
		entryType := binary.LittleEndian.Uint32(c[pos : pos+4])
		entryLen := int(binary.LittleEndian.Uint16(c[pos+4 : pos+6]))
		if entryLen < 0x18 || pos+entryLen > len(c) {
			break
		}
		mftRef := binary.LittleEndian.Uint64(c[pos+0x10 : pos+0x18])
		out = append(out, attrListEntry{attrType: entryType, mftRef: MFTIndex(mftRef)})
		pos += entryLen
	}
	return out
}
