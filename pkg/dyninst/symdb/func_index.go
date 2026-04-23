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

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// funcOffsetByNameIndex maps canonicalized qualified names to DWARF offsets.
// The name keys are treated as package-prefixed identifiers; forPackage
// returns entries whose name starts with pkgName + ".". Used to find
// functions that belong to a package but appear in a different CU (e.g.
// generic shape instantiations placed in a foreign CU by the compiler).
type funcOffsetByNameIndex interface {
	// forPackage returns an iterator over all (canonicalName, funcOffset)
	// pairs where the function's qualified name starts with pkgName + ".".
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
	prefix := pkgName + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
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

// ─── On-disk implementation ────────────────────────────────────────────────

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

	// Handle empty index.
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
	prefix := pkgName + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
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
