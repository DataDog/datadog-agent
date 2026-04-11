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

// genericFuncIndex maps canonicalized qualified names to DWARF offsets for
// generic shape functions found across all compile units. Used to find
// displaced generics that belong to a package but appear in a different CU.
type genericFuncIndex interface {
	// forPackage returns an iterator over all (canonicalName, funcOffset)
	// pairs where the function's qualified name starts with pkgName + ".".
	forPackage(pkgName string) iter.Seq2[string, dwarf.Offset]
	io.Closer
}

// genericFuncIndexBuilder accumulates generic shape function entries and
// produces a sorted genericFuncIndex.
type genericFuncIndexBuilder interface {
	// add records a generic shape function. name is the full canonicalized
	// qualified name (e.g. "lib.Filter[...]").
	add(name string, funcOffset dwarf.Offset) error
	build() (genericFuncIndex, error)
	io.Closer
}

// inMemGenericFuncEntry is a single entry in the in-memory generic function index.
type inMemGenericFuncEntry struct {
	name       string
	funcOffset dwarf.Offset
}

// inMemGenericFuncIndexBuilder builds an in-memory generic function index.
type inMemGenericFuncIndexBuilder struct {
	entries []inMemGenericFuncEntry
}

var _ genericFuncIndexBuilder = (*inMemGenericFuncIndexBuilder)(nil)

func (b *inMemGenericFuncIndexBuilder) add(name string, funcOffset dwarf.Offset) error {
	b.entries = append(b.entries, inMemGenericFuncEntry{
		name:       name,
		funcOffset: funcOffset,
	})
	return nil
}

func (b *inMemGenericFuncIndexBuilder) build() (genericFuncIndex, error) {
	slices.SortFunc(b.entries, func(a, c inMemGenericFuncEntry) int {
		return cmp.Or(
			cmp.Compare(a.name, c.name),
			cmp.Compare(a.funcOffset, c.funcOffset),
		)
	})
	idx := &inMemGenericFuncIndex{entries: b.entries}
	b.entries = nil
	return idx, nil
}

func (b *inMemGenericFuncIndexBuilder) Close() error { return nil }

// inMemGenericFuncIndex is the in-memory implementation of genericFuncIndex.
type inMemGenericFuncIndex struct {
	entries []inMemGenericFuncEntry // sorted by name
}

var _ genericFuncIndex = (*inMemGenericFuncIndex)(nil)

func (idx *inMemGenericFuncIndex) forPackage(pkgName string) iter.Seq2[string, dwarf.Offset] {
	prefix := pkgName + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
		i, _ := slices.BinarySearchFunc(idx.entries, prefix, func(e inMemGenericFuncEntry, target string) int {
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

func (idx *inMemGenericFuncIndex) Close() error { return nil }

// emptyGenericFuncIndex is a no-op index that returns no results.
type emptyGenericFuncIndex struct{}

var _ genericFuncIndex = emptyGenericFuncIndex{}

func (emptyGenericFuncIndex) forPackage(string) iter.Seq2[string, dwarf.Offset] {
	return func(func(string, dwarf.Offset) bool) {}
}

func (emptyGenericFuncIndex) Close() error { return nil }

// ─── On-disk implementation ────────────────────────────────────────────────

// onDiskGenericFuncIndexEntry is the fixed-size entry stored in the index file.
// The strOffset field points into the string table file.
type onDiskGenericFuncIndexEntry struct {
	strOffset  uint32
	funcOffset uint32
}

// onDiskGenericFuncIndexBuilder writes entries to two disk files (string table
// + index) and produces a sorted, mmap-backed index on build().
type onDiskGenericFuncIndexBuilder struct {
	strFile  *object.DiskFile
	strW     *bufio.Writer
	idxFile  *object.DiskFile
	idxW     *bufio.Writer
	strPos   uint32 // current write position in string table
	entryBuf onDiskGenericFuncIndexEntry
	lenBuf   [4]byte
}

var _ genericFuncIndexBuilder = (*onDiskGenericFuncIndexBuilder)(nil)

const onDiskGenericIndexMaxSize = 256 << 20 // 256 MiB per file
const onDiskGenericIndexInitialSize = 1 << 20

func newOnDiskGenericFuncIndexBuilder(dc *object.DiskCache, suffix string) (*onDiskGenericFuncIndexBuilder, error) {
	strFile, err := dc.NewFile(
		"genericIndex."+suffix+".strings",
		onDiskGenericIndexMaxSize,
		onDiskGenericIndexInitialSize,
	)
	if err != nil {
		return nil, err
	}
	idxFile, err := dc.NewFile(
		"genericIndex."+suffix+".entries",
		onDiskGenericIndexMaxSize,
		onDiskGenericIndexInitialSize,
	)
	if err != nil {
		_ = strFile.Close()
		return nil, err
	}
	return &onDiskGenericFuncIndexBuilder{
		strFile: strFile,
		strW:    bufio.NewWriter(strFile),
		idxFile: idxFile,
		idxW:    bufio.NewWriter(idxFile),
	}, nil
}

func (b *onDiskGenericFuncIndexBuilder) add(name string, funcOffset dwarf.Offset) error {
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
	b.entryBuf = onDiskGenericFuncIndexEntry{
		strOffset:  nameOffset,
		funcOffset: uint32(funcOffset),
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&b.entryBuf)), unsafe.Sizeof(b.entryBuf))
	if _, err := b.idxW.Write(buf); err != nil {
		return err
	}
	return nil
}

func (b *onDiskGenericFuncIndexBuilder) build() (_ genericFuncIndex, retErr error) {
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
		return emptyGenericFuncIndex{}, nil
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
		(*onDiskGenericFuncIndexEntry)(unsafe.Pointer(unsafe.SliceData(idxData))),
		uintptr(len(idxData))/unsafe.Sizeof(onDiskGenericFuncIndexEntry{}),
	)

	// readName extracts the string from the string table at the given offset.
	readName := func(offset uint32) string {
		nameLen := binary.NativeEndian.Uint32(strData[offset:])
		return string(strData[offset+4 : offset+4+nameLen])
	}

	// Sort index entries by their string table names, then by funcOffset.
	slices.SortFunc(entries, func(a, c onDiskGenericFuncIndexEntry) int {
		return cmp.Or(
			strings.Compare(readName(a.strOffset), readName(c.strOffset)),
			cmp.Compare(a.funcOffset, c.funcOffset),
		)
	})

	return &onDiskGenericFuncIndex{
		strMM:   strMM,
		idxMM:   idxMM,
		strData: strData,
		entries: entries,
	}, nil
}

func (b *onDiskGenericFuncIndexBuilder) cleanup() {
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

func (b *onDiskGenericFuncIndexBuilder) Close() error {
	b.cleanup()
	return nil
}

// onDiskGenericFuncIndex is the mmap-backed implementation of genericFuncIndex.
type onDiskGenericFuncIndex struct {
	strMM   object.SectionData
	idxMM   object.SectionData
	strData []byte
	entries []onDiskGenericFuncIndexEntry // sorted by name
}

var _ genericFuncIndex = (*onDiskGenericFuncIndex)(nil)

func (idx *onDiskGenericFuncIndex) readName(offset uint32) string {
	nameLen := binary.NativeEndian.Uint32(idx.strData[offset:])
	return string(idx.strData[offset+4 : offset+4+nameLen])
}

func (idx *onDiskGenericFuncIndex) forPackage(pkgName string) iter.Seq2[string, dwarf.Offset] {
	prefix := pkgName + "."
	return func(yield func(string, dwarf.Offset) bool) {
		// Binary search for the first entry whose name >= prefix.
		i, _ := slices.BinarySearchFunc(idx.entries, prefix, func(e onDiskGenericFuncIndexEntry, target string) int {
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

func (idx *onDiskGenericFuncIndex) Close() error {
	if idx.strMM == nil {
		return nil
	}
	defer func() { *idx = onDiskGenericFuncIndex{} }()
	return errors.Join(idx.strMM.Close(), idx.idxMM.Close())
}
