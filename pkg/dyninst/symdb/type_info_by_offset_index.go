// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"bufio"
	"cmp"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// typeInfoByOffsetIndex maps a DWARF type-DIE offset to the (raw AttrName, byte
// size) recorded for that DIE during the prepass. Variables, struct fields, and
// per-package type emission all read from this index instead of seeking the
// DWARF reader on demand.
//
// Names are stored in raw DWARF form (escaped, possibly containing
// "go.shape."); callers run unescapeSymbol / CanonicalizeGenerics themselves.
// An empty name is valid and means the indexed DIE has no DW_AT_name (anonymous
// structs etc.).
//
// Sizes follow Delve's godwarf.Common().Size() semantics: when the DIE itself
// has no DW_AT_byte_size, the builder back-substitutes a fallback at build()
// time. Pointer and subroutine types fall back to the binary's address size;
// typedef chains follow inner types until a non-zero size is found. Other tags
// without an explicit byte size are stored as size 0.
type typeInfoByOffsetIndex interface {
	// infoAt returns (name, size, true) for an indexed offset.
	// (_, _, false) means the offset wasn't recorded — usually a
	// caller bug or a DIE outside the six named-type tags.
	//
	// The returned name aliases mmap memory in the on-disk
	// implementation; callers that need to retain it beyond the
	// iterator's lifetime must clone.
	infoAt(offset dwarf.Offset) (name string, size int64, ok bool)
	io.Closer
}

// typeInfoByOffsetIndexBuilder accumulates entries during the prepass and
// produces a sorted typeInfoByOffsetIndex on build(), applying the size
// fallback pass-2 substitution described above.
type typeInfoByOffsetIndexBuilder interface {
	add(offset dwarf.Offset, info typeInfoEntry) error
	build() (typeInfoByOffsetIndex, error)
	io.Closer
}

// typeInfoEntry is the input to typeInfoByOffsetIndexBuilder.add. Only
// (name, size) end up in the on-disk entries; tag and inner are used
// at build() time for the size-fallback pass and then discarded.
type typeInfoEntry struct {
	// name is the raw DWARF AttrName, or "" if absent.
	name string
	// size is the raw DWARF AttrByteSize, or 0 if absent. The
	// builder may rewrite size during build() per the fallback
	// rules.
	size int64
	// tag is the DIE's DWARF tag; used during the size-fallback
	// pass to decide whether to apply the address-size fallback.
	tag dwarf.Tag
	// inner is the AttrType offset, used to walk the typedef chain
	// during the size-fallback pass. Zero if the DIE has no
	// AttrType.
	inner dwarf.Offset
}

// emptyTypeInfoByOffsetIndex returns (_, _, false) for every lookup.
// Used when the prepass produced no entries (e.g. a stripped binary).
type emptyTypeInfoByOffsetIndex struct{}

var _ typeInfoByOffsetIndex = emptyTypeInfoByOffsetIndex{}

func (emptyTypeInfoByOffsetIndex) infoAt(dwarf.Offset) (string, int64, bool) {
	return "", 0, false
}

func (emptyTypeInfoByOffsetIndex) Close() error { return nil }

// inMemTypeInfoByOffsetEntry is the in-memory entry shape, keyed by
// dwarfOffset for binary search.
type inMemTypeInfoByOffsetEntry struct {
	dwarfOffset dwarf.Offset
	name        string
	size        int64
}

// inMemTypeInfoByOffsetIndexBuilder builds an in-memory index. Used when
// no DiskCache is configured (typical for tests).
type inMemTypeInfoByOffsetIndexBuilder struct {
	pending     []typeInfoBuildEntry
	addressSize int64
}

// typeInfoBuildEntry is the per-entry state held during build().
// It carries the tag/inner needed for the size-fallback pass plus the
// final-form fields written into the index.
type typeInfoBuildEntry struct {
	dwarfOffset dwarf.Offset
	name        string
	size        int64
	tag         dwarf.Tag
	inner       dwarf.Offset
}

var _ typeInfoByOffsetIndexBuilder = (*inMemTypeInfoByOffsetIndexBuilder)(nil)

// newInMemTypeInfoByOffsetIndexBuilder constructs an in-memory builder
// for a binary whose pointer width is addressSize bytes (used by the
// pass-2 fallback for TagPointerType / TagSubroutineType DIEs that
// lack a DW_AT_byte_size).
func newInMemTypeInfoByOffsetIndexBuilder(addressSize int64) *inMemTypeInfoByOffsetIndexBuilder {
	return &inMemTypeInfoByOffsetIndexBuilder{addressSize: addressSize}
}

func (b *inMemTypeInfoByOffsetIndexBuilder) add(offset dwarf.Offset, info typeInfoEntry) error {
	b.pending = append(b.pending, typeInfoBuildEntry{
		dwarfOffset: offset,
		name:        info.name,
		size:        info.size,
		tag:         info.tag,
		inner:       info.inner,
	})
	return nil
}

func (b *inMemTypeInfoByOffsetIndexBuilder) build() (typeInfoByOffsetIndex, error) {
	if len(b.pending) == 0 {
		b.pending = nil
		return emptyTypeInfoByOffsetIndex{}, nil
	}
	// Pass 2: resolve sizes. Build an offset → index map first so
	// the typedef chain walk can find downstream entries by offset.
	byOffset := make(map[dwarf.Offset]int, len(b.pending))
	for i := range b.pending {
		byOffset[b.pending[i].dwarfOffset] = i
	}
	for i := range b.pending {
		if b.pending[i].size != 0 {
			continue
		}
		b.pending[i].size = resolveSizeFallback(b.pending, byOffset, i, b.addressSize)
	}
	// Convert pending → entries, sort by dwarfOffset for binary search.
	entries := make([]inMemTypeInfoByOffsetEntry, len(b.pending))
	for i, p := range b.pending {
		entries[i] = inMemTypeInfoByOffsetEntry{
			dwarfOffset: p.dwarfOffset,
			name:        p.name,
			size:        p.size,
		}
	}
	slices.SortFunc(entries, func(a, c inMemTypeInfoByOffsetEntry) int {
		return cmp.Compare(a.dwarfOffset, c.dwarfOffset)
	})
	b.pending = nil
	return &inMemTypeInfoByOffsetIndex{entries: entries}, nil
}

func (b *inMemTypeInfoByOffsetIndexBuilder) Close() error {
	b.pending = nil
	return nil
}

// resolveSizeFallback applies Delve's Common().Size() rules to the
// entry at index i: pointer/subroutine types get the architecture
// pointer size; typedef chains follow inner offsets through pending
// entries until a non-zero size is found; everything else stays 0.
func resolveSizeFallback(
	pending []typeInfoBuildEntry,
	byOffset map[dwarf.Offset]int,
	i int,
	addressSize int64,
) int64 {
	switch pending[i].tag {
	case dwarf.TagPointerType, dwarf.TagSubroutineType:
		return addressSize
	case dwarf.TagTypedef:
		// Walk inner chain through pending entries. Bounded by
		// maxTypeUnwrapDepth as a defense against malformed DWARF cycles
		// (matching the runtime walk in symdb.go).
		cur := i
		for step := 0; step < maxTypeUnwrapDepth; step++ {
			next := pending[cur].inner
			if next == 0 {
				return 0
			}
			j, ok := byOffset[next]
			if !ok {
				return 0
			}
			if pending[j].size != 0 {
				return pending[j].size
			}
			// If we're following a chain through pointer/subroutine fallbacks,
			// terminate here too — the inner DIE will itself get the
			// address-size fallback.
			if pending[j].tag == dwarf.TagPointerType ||
				pending[j].tag == dwarf.TagSubroutineType {
				return addressSize
			}
			cur = j
		}
		return 0
	default:
		return 0
	}
}

// inMemTypeInfoByOffsetIndex is the in-memory implementation.
type inMemTypeInfoByOffsetIndex struct {
	entries []inMemTypeInfoByOffsetEntry // sorted by dwarfOffset
}

var _ typeInfoByOffsetIndex = (*inMemTypeInfoByOffsetIndex)(nil)

func (idx *inMemTypeInfoByOffsetIndex) infoAt(offset dwarf.Offset) (string, int64, bool) {
	i, found := slices.BinarySearchFunc(idx.entries, offset, func(e inMemTypeInfoByOffsetEntry, t dwarf.Offset) int {
		return cmp.Compare(e.dwarfOffset, t)
	})
	if !found {
		return "", 0, false
	}
	e := &idx.entries[i]
	return e.name, e.size, true
}

func (idx *inMemTypeInfoByOffsetIndex) Close() error { return nil }

// onDiskTypeInfoByOffsetEntry is the fixed-size record stored in the .entries
// file, sorted by dwarfOffset on build() for binary search.
//
// tag and inner are written during pass 1 and consumed by the build()-time
// size-fallback pass; they are not exposed at lookup time. We carry them on
// disk so the prepass walk can stream every entry to the file as soon as it's
// seen instead of buffering on the heap. The padding byte keeps the struct
// 4-byte-aligned and makes the entry size a power of 2 (16 bytes), which is
// friendly to the mmap'd binary-search and to the unsafe.Slice
// reinterpretation.
type onDiskTypeInfoByOffsetEntry struct {
	dwarfOffset uint32
	strOffset   uint32
	size        int32
	inner       uint32
	tag         uint8
	_           [3]uint8 // padding to 20 bytes; struct alignment rounds to 20
}

// onDiskTypeInfoByOffsetIndexBuilder streams entries to two disk files
// (.strings + .entries) as add() is called during the prepass walk.  All
// write-path state lives on disk or in fixed-size scratch buffers — there is no
// heap-resident map or slice that scales with input size, so prepass heap RSS
// stays flat regardless of how many type DIEs the binary contains.
//
// Names are *not* deduplicated: each add() writes a length-prefixed copy to the
// strings file. This trades a few MB of disk (page- cache-resident, not
// heap-resident) for predictable heap behaviour on large binaries.  On a binary
// with ~100k indexed DIEs averaging ~30 B names, the strings file lands around
// 3-4 MB.
//
// The size-fallback pass runs in build() over the mmap'd entries file: after
// sorting by dwarfOffset, we walk the entries in place and rewrite the size
// column for any entry whose pass-1 size was zero, looking up referenced inner
// offsets by binary search over the same mmap. The tag and inner columns become
// dead data after this pass — infoAt reads only (dwarfOffset, strOffset, size)
// — but they stay on disk; reclaiming them would require a second
// rewrite pass that's not worth the complexity.
type onDiskTypeInfoByOffsetIndexBuilder struct {
	strFile     *object.DiskFile
	strW        *bufio.Writer
	idxFile     *object.DiskFile
	idxW        *bufio.Writer
	strPos      uint32
	numEntries  uint32
	addressSize int64
	lenBuf      [4]byte
	entryBuf    onDiskTypeInfoByOffsetEntry
}

var _ typeInfoByOffsetIndexBuilder = (*onDiskTypeInfoByOffsetIndexBuilder)(nil)

const onDiskTypeInfoByOffsetIndexMaxSize = 256 << 20 // 256 MiB per file
const onDiskTypeInfoByOffsetIndexInitialSize = 1 << 20

// newOnDiskTypeInfoByOffsetIndexBuilder constructs an on-disk builder rooted at
// dc. addressSize is captured from the DWARF reader once at prepass entry and
// applied uniformly during the build()-time size fallback pass.
func newOnDiskTypeInfoByOffsetIndexBuilder(
	dc *object.DiskCache, suffix string, addressSize int64,
) (*onDiskTypeInfoByOffsetIndexBuilder, error) {
	strFile, err := dc.NewFile(
		"typeInfoByOffsetIndex."+suffix+".strings",
		onDiskTypeInfoByOffsetIndexMaxSize,
		onDiskTypeInfoByOffsetIndexInitialSize,
	)
	if err != nil {
		return nil, err
	}
	idxFile, err := dc.NewFile(
		"typeInfoByOffsetIndex."+suffix+".entries",
		onDiskTypeInfoByOffsetIndexMaxSize,
		onDiskTypeInfoByOffsetIndexInitialSize,
	)
	if err != nil {
		_ = strFile.Close()
		return nil, err
	}
	return &onDiskTypeInfoByOffsetIndexBuilder{
		strFile:     strFile,
		strW:        bufio.NewWriter(strFile),
		idxFile:     idxFile,
		idxW:        bufio.NewWriter(idxFile),
		addressSize: addressSize,
	}, nil
}

// writeName appends a length-prefixed copy of name to the strings file and
// returns its offset.
func (b *onDiskTypeInfoByOffsetIndexBuilder) writeName(name string) (uint32, error) {
	off := b.strPos
	binary.NativeEndian.PutUint32(b.lenBuf[:], uint32(len(name)))
	if _, err := b.strW.Write(b.lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := b.strW.WriteString(name); err != nil {
		return 0, err
	}
	b.strPos += 4 + uint32(len(name))
	return off, nil
}

func (b *onDiskTypeInfoByOffsetIndexBuilder) add(offset dwarf.Offset, info typeInfoEntry) error {
	if b.idxW == nil {
		return errors.New("builder is closed")
	}
	strOff, err := b.writeName(info.name)
	if err != nil {
		return err
	}
	b.entryBuf = onDiskTypeInfoByOffsetEntry{
		dwarfOffset: uint32(offset),
		strOffset:   strOff,
		size:        int32(info.size),
		inner:       uint32(info.inner),
		tag:         uint8(info.tag),
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&b.entryBuf)), unsafe.Sizeof(b.entryBuf))
	if _, err := b.idxW.Write(buf); err != nil {
		return err
	}
	b.numEntries++
	return nil
}

func (b *onDiskTypeInfoByOffsetIndexBuilder) build() (_ typeInfoByOffsetIndex, retErr error) {
	if b.idxW == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() {
		if retErr != nil {
			b.cleanup()
		}
	}()

	if err := b.strW.Flush(); err != nil {
		return nil, err
	}
	b.strW = nil
	if err := b.idxW.Flush(); err != nil {
		return nil, err
	}
	b.idxW = nil

	if b.numEntries == 0 {
		b.cleanup()
		return emptyTypeInfoByOffsetIndex{}, nil
	}

	strMM, err := b.strFile.IntoMMap(syscall.PROT_READ)
	if err != nil {
		return nil, err
	}
	b.strFile = nil
	defer func() {
		if retErr != nil {
			_ = strMM.Close()
		}
	}()

	idxMM, err := b.idxFile.IntoMMap(syscall.PROT_READ | syscall.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	b.idxFile = nil
	defer func() {
		if retErr != nil {
			_ = idxMM.Close()
		}
	}()

	idxData := idxMM.Data()
	entries := unsafe.Slice(
		(*onDiskTypeInfoByOffsetEntry)(unsafe.Pointer(unsafe.SliceData(idxData))),
		uintptr(len(idxData))/unsafe.Sizeof(onDiskTypeInfoByOffsetEntry{}),
	)

	// Sort by dwarfOffset in place on the writable mmap.
	slices.SortFunc(entries, func(a, c onDiskTypeInfoByOffsetEntry) int {
		return cmp.Compare(a.dwarfOffset, c.dwarfOffset)
	})

	// Pass 2: rewrite size for entries whose pass-1 size was zero.
	// Inner-offset lookups go through binary search over the just- sorted
	// entries, so this pass is O(N log N) and entirely on the mmap — no
	// heap-resident pending slice.
	resolveOnDiskSizes(entries, b.addressSize)

	return &onDiskTypeInfoByOffsetIndex{
		strMM:   strMM,
		idxMM:   idxMM,
		strData: strMM.Data(),
		entries: entries,
	}, nil
}

// resolveOnDiskSizes applies the same Delve-equivalent fallback rules as
// resolveSizeFallback, but walks the typedef chain through the sorted on-disk
// entries via binary search. Bounded by maxTypeUnwrapDepth as a defense against
// malformed-DWARF cycles.
func resolveOnDiskSizes(entries []onDiskTypeInfoByOffsetEntry, addressSize int64) {
	findIndex := func(off uint32) (int, bool) {
		i, ok := slices.BinarySearchFunc(entries, off, func(
			e onDiskTypeInfoByOffsetEntry, t uint32,
		) int {
			return cmp.Compare(e.dwarfOffset, t)
		})
		return i, ok
	}
	for i := range entries {
		if entries[i].size != 0 {
			continue
		}
		switch dwarf.Tag(entries[i].tag) {
		case dwarf.TagPointerType, dwarf.TagSubroutineType:
			entries[i].size = int32(addressSize)
		case dwarf.TagTypedef:
			cur := i
			for step := 0; step < maxTypeUnwrapDepth; step++ {
				next := entries[cur].inner
				if next == 0 {
					break
				}
				j, ok := findIndex(next)
				if !ok {
					break
				}
				if entries[j].size != 0 {
					entries[i].size = entries[j].size
					break
				}
				if dwarf.Tag(entries[j].tag) == dwarf.TagPointerType ||
					dwarf.Tag(entries[j].tag) == dwarf.TagSubroutineType {
					entries[i].size = int32(addressSize)
					break
				}
				cur = j
			}
		}
	}
}

func (b *onDiskTypeInfoByOffsetIndexBuilder) cleanup() {
	if b.strFile != nil {
		_ = b.strFile.Close()
		b.strFile = nil
	}
	if b.idxFile != nil {
		_ = b.idxFile.Close()
		b.idxFile = nil
	}
	b.strW = nil
	b.idxW = nil
}

func (b *onDiskTypeInfoByOffsetIndexBuilder) Close() error {
	b.cleanup()
	return nil
}

// onDiskTypeInfoByOffsetIndex is the mmap-backed implementation.
type onDiskTypeInfoByOffsetIndex struct {
	strMM   object.SectionData
	idxMM   object.SectionData
	strData []byte
	entries []onDiskTypeInfoByOffsetEntry // sorted by dwarfOffset
}

var _ typeInfoByOffsetIndex = (*onDiskTypeInfoByOffsetIndex)(nil)

func (idx *onDiskTypeInfoByOffsetIndex) readName(strOffset uint32) string {
	nameLen := binary.NativeEndian.Uint32(idx.strData[strOffset:])
	return string(idx.strData[strOffset+4 : strOffset+4+nameLen])
}

func (idx *onDiskTypeInfoByOffsetIndex) infoAt(offset dwarf.Offset) (string, int64, bool) {
	target := uint32(offset)
	i, found := slices.BinarySearchFunc(idx.entries, target, func(
		e onDiskTypeInfoByOffsetEntry, t uint32,
	) int {
		return cmp.Compare(e.dwarfOffset, t)
	})
	if !found {
		return "", 0, false
	}
	e := &idx.entries[i]
	return idx.readName(e.strOffset), int64(e.size), true
}

func (idx *onDiskTypeInfoByOffsetIndex) Close() error {
	if idx.strMM == nil {
		return nil
	}
	defer func() { *idx = onDiskTypeInfoByOffsetIndex{} }()
	return errors.Join(idx.strMM.Close(), idx.idxMM.Close())
}
