// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"bufio"
	"cmp"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// funcOffsetByNameIndex maps DWARF-form qualified names to DWARF offsets.
// The name keys are the linker-symbol form, where the package portion is
// escaped per cmd/internal/objabi.PathToPrefix (dots inside the last
// path segment become %2e, etc.). forPackage takes an *unescaped*
// package import path (matching DW_AT_name on the compile unit) and
// internally escapes it before searching, so callers don't deal with
// the escaping themselves. Used to find functions that belong to a
// package but appear in a different compile unit (e.g. generic shape
// instantiations placed in a foreign compile unit by the compiler).
type funcOffsetByNameIndex interface {
	// forPackage returns an iterator over all (canonicalName, funcOffset)
	// pairs whose owning package equals pkgName (compared in unescaped
	// form). Internally the lookup escapes pkgName via gosymname.EscapePkg
	// so that packages whose name contains a dot in a path segment (e.g.
	// "lib.v2", "gopkg.in/ini.v1") match their own entries and only
	// their own. Sibling packages that happen to share a dot-prefixed
	// segment (e.g. "lib" and "lib.v2") are not yielded across each
	// other.
	forPackage(pkgName string) iter.Seq2[string, dwarf.Offset]
	io.Closer
}

// funcOffsetByNameIndexBuilder accumulates name → offset entries and
// produces a sorted funcOffsetByNameIndex.
type funcOffsetByNameIndexBuilder interface {
	// add records an entry. name is the full canonicalized qualified name
	// (e.g. "lib.Filter[...]").
	add(name string, funcOffset dwarf.Offset) error
	build() (funcOffsetByNameIndex, error)
	io.Closer
}

// inMemFuncOffsetByNameEntry is a single entry in the in-memory index.
type inMemFuncOffsetByNameEntry struct {
	name       string
	funcOffset dwarf.Offset
}

// inMemFuncOffsetByNameIndexBuilder builds an in-memory name-keyed index.
type inMemFuncOffsetByNameIndexBuilder struct {
	entries []inMemFuncOffsetByNameEntry
}

var _ funcOffsetByNameIndexBuilder = (*inMemFuncOffsetByNameIndexBuilder)(nil)

func (b *inMemFuncOffsetByNameIndexBuilder) add(name string, funcOffset dwarf.Offset) error {
	b.entries = append(b.entries, inMemFuncOffsetByNameEntry{
		name:       name,
		funcOffset: funcOffset,
	})
	return nil
}

func (b *inMemFuncOffsetByNameIndexBuilder) build() (funcOffsetByNameIndex, error) {
	slices.SortFunc(b.entries, func(a, c inMemFuncOffsetByNameEntry) int {
		return cmp.Or(
			cmp.Compare(a.name, c.name),
			cmp.Compare(a.funcOffset, c.funcOffset),
		)
	})
	idx := &inMemFuncOffsetByNameIndex{entries: b.entries}
	b.entries = nil
	return idx, nil
}

func (b *inMemFuncOffsetByNameIndexBuilder) Close() error { return nil }

// inMemFuncOffsetByNameIndex is the in-memory implementation.
type inMemFuncOffsetByNameIndex struct {
	entries []inMemFuncOffsetByNameEntry // sorted by name
}

var _ funcOffsetByNameIndex = (*inMemFuncOffsetByNameIndex)(nil)

func (idx *inMemFuncOffsetByNameIndex) forPackage(pkgName string) iter.Seq2[string, dwarf.Offset] {
	prefix := gosymname.EscapePkg(pkgName) + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
		// Entries are stored in DWARF (escaped) form, so the prefix
		// must be escaped too.
		i, _ := slices.BinarySearchFunc(idx.entries, prefix, func(e inMemFuncOffsetByNameEntry, target string) int {
			return strings.Compare(e.name, target)
		})
		for ; i < len(idx.entries); i++ {
			e := &idx.entries[i]
			if !strings.HasPrefix(e.name, prefix) {
				break
			}
			if !yield(e.name, e.funcOffset) {
				return
			}
		}
	}
}

func (idx *inMemFuncOffsetByNameIndex) Close() error { return nil }

// emptyFuncOffsetByNameIndex is a no-op index that returns no results.
type emptyFuncOffsetByNameIndex struct{}

var _ funcOffsetByNameIndex = emptyFuncOffsetByNameIndex{}

func (emptyFuncOffsetByNameIndex) forPackage(string) iter.Seq2[string, dwarf.Offset] {
	return func(func(string, dwarf.Offset) bool) {}
}

func (emptyFuncOffsetByNameIndex) Close() error { return nil }

// onDiskFuncOffsetByNameEntry is the fixed-size entry stored in the index file.
// The strOffset field points into the string table file.
type onDiskFuncOffsetByNameEntry struct {
	strOffset  uint32
	funcOffset uint32
}

// onDiskFuncOffsetByNameIndexBuilder writes entries to two disk files (string
// table + index) and produces a sorted, mmap-backed index on build().
type onDiskFuncOffsetByNameIndexBuilder struct {
	strFile  *object.DiskFile
	strW     *bufio.Writer
	idxFile  *object.DiskFile
	idxW     *bufio.Writer
	strPos   uint32 // current write position in string table
	entryBuf onDiskFuncOffsetByNameEntry
	lenBuf   [4]byte
}

var _ funcOffsetByNameIndexBuilder = (*onDiskFuncOffsetByNameIndexBuilder)(nil)

const onDiskFuncOffsetByNameIndexMaxSize = 256 << 20 // 256 MiB per file
const onDiskFuncOffsetByNameIndexInitialSize = 1 << 20

func newOnDiskFuncOffsetByNameIndexBuilder(dc *object.DiskCache, suffix string) (*onDiskFuncOffsetByNameIndexBuilder, error) {
	strFile, err := dc.NewFile(
		"funcOffsetByNameIndex."+suffix+".strings",
		onDiskFuncOffsetByNameIndexMaxSize,
		onDiskFuncOffsetByNameIndexInitialSize,
	)
	if err != nil {
		return nil, err
	}
	idxFile, err := dc.NewFile(
		"funcOffsetByNameIndex."+suffix+".entries",
		onDiskFuncOffsetByNameIndexMaxSize,
		onDiskFuncOffsetByNameIndexInitialSize,
	)
	if err != nil {
		_ = strFile.Close()
		return nil, err
	}
	return &onDiskFuncOffsetByNameIndexBuilder{
		strFile: strFile,
		strW:    bufio.NewWriter(strFile),
		idxFile: idxFile,
		idxW:    bufio.NewWriter(idxFile),
	}, nil
}

func (b *onDiskFuncOffsetByNameIndexBuilder) add(name string, funcOffset dwarf.Offset) error {
	if b.strW == nil {
		return errors.New("builder is closed")
	}

	// Write string table entry: uint32 length + string bytes.
	nameOffset := b.strPos
	binary.NativeEndian.PutUint32(b.lenBuf[:], uint32(len(name)))
	if _, err := b.strW.Write(b.lenBuf[:]); err != nil {
		return err
	}
	if _, err := b.strW.WriteString(name); err != nil {
		return err
	}
	b.strPos += 4 + uint32(len(name))

	// Write index entry.
	b.entryBuf = onDiskFuncOffsetByNameEntry{
		strOffset:  nameOffset,
		funcOffset: uint32(funcOffset),
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&b.entryBuf)), unsafe.Sizeof(b.entryBuf))
	if _, err := b.idxW.Write(buf); err != nil {
		return err
	}
	return nil
}

func (b *onDiskFuncOffsetByNameIndexBuilder) build() (_ funcOffsetByNameIndex, retErr error) {
	if b.strW == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() {
		if retErr != nil {
			b.cleanup()
		}
	}()

	// Flush and mmap the string table.
	if err := b.strW.Flush(); err != nil {
		return nil, err
	}
	b.strW = nil

	if b.strPos == 0 {
		b.cleanup()
		return emptyFuncOffsetByNameIndex{}, nil
	}

	strMM, err := b.strFile.IntoMMap(syscall.PROT_READ)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = strMM.Close()
		}
	}()
	b.strFile = nil
	strData := strMM.Data()

	// Flush and mmap the index (read-write for in-place sort).
	if err := b.idxW.Flush(); err != nil {
		return nil, err
	}
	b.idxW = nil
	idxMM, err := b.idxFile.IntoMMap(syscall.PROT_READ | syscall.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = idxMM.Close()
		}
	}()
	b.idxFile = nil
	idxData := idxMM.Data()

	entries := unsafe.Slice(
		(*onDiskFuncOffsetByNameEntry)(unsafe.Pointer(unsafe.SliceData(idxData))),
		uintptr(len(idxData))/unsafe.Sizeof(onDiskFuncOffsetByNameEntry{}),
	)

	// readName extracts the string from the string table at the given offset.
	readName := func(offset uint32) string {
		nameLen := binary.NativeEndian.Uint32(strData[offset:])
		return string(strData[offset+4 : offset+4+nameLen])
	}

	// Sort index entries by their string table names, then by funcOffset.
	slices.SortFunc(entries, func(a, c onDiskFuncOffsetByNameEntry) int {
		return cmp.Or(
			strings.Compare(readName(a.strOffset), readName(c.strOffset)),
			cmp.Compare(a.funcOffset, c.funcOffset),
		)
	})

	return &onDiskFuncOffsetByNameIndex{
		strMM:   strMM,
		idxMM:   idxMM,
		strData: strData,
		entries: entries,
	}, nil
}

func (b *onDiskFuncOffsetByNameIndexBuilder) cleanup() {
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

func (b *onDiskFuncOffsetByNameIndexBuilder) Close() error {
	b.cleanup()
	return nil
}

// onDiskFuncOffsetByNameIndex is the mmap-backed implementation.
type onDiskFuncOffsetByNameIndex struct {
	strMM   object.SectionData
	idxMM   object.SectionData
	strData []byte
	entries []onDiskFuncOffsetByNameEntry // sorted by name
}

var _ funcOffsetByNameIndex = (*onDiskFuncOffsetByNameIndex)(nil)

func (idx *onDiskFuncOffsetByNameIndex) readName(offset uint32) string {
	nameLen := binary.NativeEndian.Uint32(idx.strData[offset:])
	return string(idx.strData[offset+4 : offset+4+nameLen])
}

func (idx *onDiskFuncOffsetByNameIndex) forPackage(pkgName string) iter.Seq2[string, dwarf.Offset] {
	prefix := gosymname.EscapePkg(pkgName) + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
		// Entries are stored in DWARF (escaped) form, so the prefix
		// must be escaped too.
		i, _ := slices.BinarySearchFunc(idx.entries, prefix, func(e onDiskFuncOffsetByNameEntry, target string) int {
			return strings.Compare(idx.readName(e.strOffset), target)
		})
		for ; i < len(idx.entries); i++ {
			e := &idx.entries[i]
			name := idx.readName(e.strOffset)
			if !strings.HasPrefix(name, prefix) {
				break
			}
			if !yield(name, dwarf.Offset(e.funcOffset)) {
				return
			}
		}
		runtime.KeepAlive(idx)
	}
}

func (idx *onDiskFuncOffsetByNameIndex) Close() error {
	if idx.strMM == nil {
		return nil
	}
	defer func() { *idx = onDiskFuncOffsetByNameIndex{} }()
	return errors.Join(idx.strMM.Close(), idx.idxMM.Close())
}

// funcOffsetByOriginIndex maps a DWARF offset (the "origin") to zero or more
// other DWARF offsets. Used to look up every inlined instance of an abstract
// function definition across all compile units: key = abstract-origin offset,
// value = instance offset. Multiple entries for the same origin are
// permitted and all yielded by forOrigin.
type funcOffsetByOriginIndex interface {
	// forOrigin returns an iterator over all instance offsets whose origin
	// matches the given origin offset.
	forOrigin(origin dwarf.Offset) iter.Seq[dwarf.Offset]
	io.Closer
}

// funcOffsetByOriginIndexBuilder accumulates (origin, instance) pairs and
// produces a sorted funcOffsetByOriginIndex.
type funcOffsetByOriginIndexBuilder interface {
	add(origin, instance dwarf.Offset) error
	build() (funcOffsetByOriginIndex, error)
	io.Closer
}

// funcOffsetByOriginEntry is the fixed-size entry shared by both in-memory
// and on-disk implementations.
type funcOffsetByOriginEntry struct {
	origin   uint32
	instance uint32
}

// inMemFuncOffsetByOriginIndexBuilder builds an in-memory origin-keyed index.
type inMemFuncOffsetByOriginIndexBuilder struct {
	entries []funcOffsetByOriginEntry
}

var _ funcOffsetByOriginIndexBuilder = (*inMemFuncOffsetByOriginIndexBuilder)(nil)

func (b *inMemFuncOffsetByOriginIndexBuilder) add(origin, instance dwarf.Offset) error {
	b.entries = append(b.entries, funcOffsetByOriginEntry{
		origin:   uint32(origin),
		instance: uint32(instance),
	})
	return nil
}

func (b *inMemFuncOffsetByOriginIndexBuilder) build() (funcOffsetByOriginIndex, error) {
	sortFuncOffsetByOriginEntries(b.entries)
	idx := &inMemFuncOffsetByOriginIndex{entries: b.entries}
	b.entries = nil
	return idx, nil
}

func (b *inMemFuncOffsetByOriginIndexBuilder) Close() error { return nil }

// inMemFuncOffsetByOriginIndex is the in-memory implementation.
type inMemFuncOffsetByOriginIndex struct {
	entries []funcOffsetByOriginEntry // sorted by origin, then instance
}

var _ funcOffsetByOriginIndex = (*inMemFuncOffsetByOriginIndex)(nil)

func (idx *inMemFuncOffsetByOriginIndex) forOrigin(origin dwarf.Offset) iter.Seq[dwarf.Offset] {
	return forOriginFromEntries(idx.entries, origin, nil)
}

func (idx *inMemFuncOffsetByOriginIndex) Close() error { return nil }

// emptyFuncOffsetByOriginIndex is a no-op index.
type emptyFuncOffsetByOriginIndex struct{}

var _ funcOffsetByOriginIndex = emptyFuncOffsetByOriginIndex{}

func (emptyFuncOffsetByOriginIndex) forOrigin(dwarf.Offset) iter.Seq[dwarf.Offset] {
	return func(func(dwarf.Offset) bool) {}
}

func (emptyFuncOffsetByOriginIndex) Close() error { return nil }

// onDiskFuncOffsetByOriginIndexBuilder writes (origin, instance) pairs to a
// single disk file and produces a sorted, mmap-backed index on build().
type onDiskFuncOffsetByOriginIndexBuilder struct {
	idxFile    *object.DiskFile
	idxW       *bufio.Writer
	numEntries uint32
	entryBuf   funcOffsetByOriginEntry
}

var _ funcOffsetByOriginIndexBuilder = (*onDiskFuncOffsetByOriginIndexBuilder)(nil)

const onDiskFuncOffsetByOriginIndexMaxSize = 256 << 20 // 256 MiB
const onDiskFuncOffsetByOriginIndexInitialSize = 1 << 20

func newOnDiskFuncOffsetByOriginIndexBuilder(dc *object.DiskCache, suffix string) (*onDiskFuncOffsetByOriginIndexBuilder, error) {
	idxFile, err := dc.NewFile(
		"funcOffsetByOriginIndex."+suffix+".entries",
		onDiskFuncOffsetByOriginIndexMaxSize,
		onDiskFuncOffsetByOriginIndexInitialSize,
	)
	if err != nil {
		return nil, err
	}
	return &onDiskFuncOffsetByOriginIndexBuilder{
		idxFile: idxFile,
		idxW:    bufio.NewWriter(idxFile),
	}, nil
}

func (b *onDiskFuncOffsetByOriginIndexBuilder) add(origin, instance dwarf.Offset) error {
	if b.idxW == nil {
		return errors.New("builder is closed")
	}
	b.entryBuf = funcOffsetByOriginEntry{
		origin:   uint32(origin),
		instance: uint32(instance),
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&b.entryBuf)), unsafe.Sizeof(b.entryBuf))
	if _, err := b.idxW.Write(buf); err != nil {
		return err
	}
	b.numEntries++
	return nil
}

func (b *onDiskFuncOffsetByOriginIndexBuilder) build() (_ funcOffsetByOriginIndex, retErr error) {
	if b.idxW == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() {
		if retErr != nil {
			b.cleanup()
		}
	}()

	if err := b.idxW.Flush(); err != nil {
		return nil, err
	}
	b.idxW = nil

	// IntoMMap rejects empty files, so short-circuit before the mmap
	// call. This handles binaries that have no inline-subroutine
	// instances at all.
	if b.numEntries == 0 {
		b.cleanup()
		return emptyFuncOffsetByOriginIndex{}, nil
	}

	idxMM, err := b.idxFile.IntoMMap(syscall.PROT_READ | syscall.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = idxMM.Close()
		}
	}()
	b.idxFile = nil
	idxData := idxMM.Data()

	entries := unsafe.Slice(
		(*funcOffsetByOriginEntry)(unsafe.Pointer(unsafe.SliceData(idxData))),
		uintptr(len(idxData))/unsafe.Sizeof(funcOffsetByOriginEntry{}),
	)
	sortFuncOffsetByOriginEntries(entries)

	return &onDiskFuncOffsetByOriginIndex{
		idxMM:   idxMM,
		entries: entries,
	}, nil
}

func (b *onDiskFuncOffsetByOriginIndexBuilder) cleanup() {
	if b.idxFile != nil {
		_ = b.idxFile.Close()
		b.idxFile = nil
	}
	b.idxW = nil
}

func (b *onDiskFuncOffsetByOriginIndexBuilder) Close() error {
	b.cleanup()
	return nil
}

// onDiskFuncOffsetByOriginIndex is the mmap-backed implementation.
type onDiskFuncOffsetByOriginIndex struct {
	idxMM object.SectionData
	// entries is an unsafe view onto idxMM's mmap'ed data, sorted by
	// origin, then instance. It must not be used after idxMM is closed.
	entries []funcOffsetByOriginEntry
}

var _ funcOffsetByOriginIndex = (*onDiskFuncOffsetByOriginIndex)(nil)

func (idx *onDiskFuncOffsetByOriginIndex) forOrigin(origin dwarf.Offset) iter.Seq[dwarf.Offset] {
	return forOriginFromEntries(idx.entries, origin, idx)
}

func (idx *onDiskFuncOffsetByOriginIndex) Close() error {
	if idx.idxMM == nil {
		return nil
	}
	defer func() { *idx = onDiskFuncOffsetByOriginIndex{} }()
	return idx.idxMM.Close()
}

// sortFuncOffsetByOriginEntries sorts entries by origin then instance.
func sortFuncOffsetByOriginEntries(entries []funcOffsetByOriginEntry) {
	slices.SortFunc(entries, func(a, b funcOffsetByOriginEntry) int {
		return cmp.Or(
			cmp.Compare(a.origin, b.origin),
			cmp.Compare(a.instance, b.instance),
		)
	})
}

// forOriginFromEntries is shared by in-memory and on-disk implementations.
// keepAlive is kept referenced for the lifetime of the iteration to prevent
// the underlying mmap from being finalized while the caller iterates. Pass
// nil for in-memory indexes.
func forOriginFromEntries(entries []funcOffsetByOriginEntry, origin dwarf.Offset, keepAlive any) iter.Seq[dwarf.Offset] {
	return func(yield func(dwarf.Offset) bool) {
		target := uint32(origin)
		i, _ := slices.BinarySearchFunc(entries, target, func(e funcOffsetByOriginEntry, t uint32) int {
			return cmp.Compare(e.origin, t)
		})
		for ; i < len(entries); i++ {
			e := &entries[i]
			if e.origin != target {
				break
			}
			if !yield(dwarf.Offset(e.instance)) {
				return
			}
		}
		if keepAlive != nil {
			runtime.KeepAlive(keepAlive)
		}
	}
}
