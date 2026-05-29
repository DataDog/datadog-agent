//go:build windows

// Package du computes per-immediate-child disk usage for a target directory
// on an NTFS volume by reading the raw $MFT. It is the production-grade
// successor to the experiments under poc/mft_du, poc/ntfs_du, poc/usn_du.
//
// Scan pipeline (see Scan in du_windows.go for section markers):
//
//   - Setup: open \\.\<drive>:, resolve the target and its immediate children
//     to MFT indices via the Windows API (CreateFile, GetFileInformationByHandle,
//     FindFirstFile). Exclusion paths are resolved to indices before the MFT
//     walks so out-of-scope subtrees short-circuit cheaply.
//
//   - Pass 1 (modeAll, one full MFT stream): build dirParent (directory →
//     parent idx), plus extSize and extParents per file base. Extension records
//     are folded into this pass so their $DATA sizes and spillover $FILE_NAME
//     parents are not rescanned. The bulk walk does not decode UTF-16 names.
//
//   - Classify scope: TreeDepth == 0 precomputes dirBucket via walkUp from
//     target and immediate-child buckets; TreeDepth > 0 retains dirParent and
//     tree-dir anchor maps for per-file chain walks in pass 2.
//
//   - Pass 2 (modeFileBaseOnly, or modeAll when TreeDepth > 0): tally in-use
//     file base records into bucket / subtree / loose totals; optional top-N
//     files, extension aggregation, and find predicates run inline in this
//     callback. Tree mode opportunistically decodes names only for dirs at
//     depth ≤ TreeDepth.
//
//   - Post-scan: assemble Result.Buckets and optional Result.Tree; resolve
//     top-file paths via OpenFileByID (bounded, not part of the MFT stream).
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
package du

import (
	"encoding/binary"
	"errors"
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

	// allocatedSize / dataSize come from $DATA. fnAllocSize / fnDataSize are
	// the cached sizes from the highest-priority $FILE_NAME, used as a
	// fallback when $DATA sizes are missing (typically when $DATA is in an
	// extension record — covered by pass 2).
	allocatedSize int64
	dataSize      int64
	fnAllocSize   int64
	fnDataSize    int64

	isInUse      bool
	isDir        bool
	isSparse     bool
	isCompressed bool
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
			parseFileNameParents(record[offset:offset+attrLen], entry, &bestNS)
		case attrData:
			nonResident := record[offset+8]
			if nonResident == 1 {
				parseNonResidentData(record[offset:offset+attrLen], entry)
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
func parseFileNameParents(attr []byte, entry *mftEntry, bestNS *int) {
	contentOffset := int(binary.LittleEndian.Uint16(attr[0x14:0x16]))
	contentLen := int(binary.LittleEndian.Uint32(attr[0x10:0x14]))
	if contentOffset+contentLen > len(attr) || contentLen < 0x42 {
		return
	}
	c := attr[contentOffset : contentOffset+contentLen]

	parentRef := MFTIndex(binary.LittleEndian.Uint64(c[0x00:0x08]))
	allocSize := int64(binary.LittleEndian.Uint64(c[0x28:0x30]))
	realSize := int64(binary.LittleEndian.Uint64(c[0x30:0x38]))
	namespace := int(c[0x41])

	if namespace == nsDOS {
		return
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
// Allocated == data size (no separate cluster allocation).
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
	entry.dataSize += contentLen
	entry.allocatedSize += contentLen
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
func parseNonResidentData(attr []byte, entry *mftEntry) {
	if len(attr) < 0x40 {
		return
	}
	dataFlags := binary.LittleEndian.Uint16(attr[0x0C:0x0E])
	isCompressed := dataFlags&0x0001 != 0
	isSparse := dataFlags&0x8000 != 0

	lowestVcn := binary.LittleEndian.Uint64(attr[0x10:0x18])
	if lowestVcn != 0 {
		return // continuation run — sizes are invalid
	}

	entry.dataSize += int64(binary.LittleEndian.Uint64(attr[0x30:0x38]))
	if isSparse {
		entry.isSparse = true
	}
	if isCompressed {
		entry.isCompressed = true
	}

	if (isSparse || isCompressed) && len(attr) >= 0x48 {
		entry.allocatedSize += int64(binary.LittleEndian.Uint64(attr[0x40:0x48]))
	} else {
		entry.allocatedSize += int64(binary.LittleEndian.Uint64(attr[0x28:0x30]))
	}
}

// -------------------------------------------------------------------------
// Multi-sector transfer protection (fixups)
// -------------------------------------------------------------------------

// errTornWrite is returned by applyFixups when a sector-end USN does not
// match the header USN, indicating the write that produced this record
// did not complete atomically. The record's content is in an
// indeterminate state and must not be parsed.
var errTornWrite = errors.New("torn write detected (USN mismatch)")

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
	if fixupCount < 2 || int(fixupOffset)+int(fixupCount)*2 > recordSize {
		return nil
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
