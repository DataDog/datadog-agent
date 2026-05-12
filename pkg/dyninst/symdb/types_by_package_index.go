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
	"iter"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// typesByPackageIndex maps a Go package import path to the DWARF offsets of
// every named user-type DIE belonging to that package.
//
// Unlike funcOffsetByNameIndex, which keys on canonical name and
// binary-searches by name prefix, this index keys directly on (escapedPackage,
// dwarf.Offset) — so forPackage(pkg) yields offsets in DWARF-offset order
// without any client-side sort. Names are not stored here; consumers chain
// through typeInfoByOffset.infoAt(offset) when they need the type's name.
//
// Package strings are stored escaped (per gosymname.EscapePkg) so
// dotted-segment packages like "lib.v2" don't leak into siblings like "lib",
// matching the semantics tested by TestForPackageEscapeRoundTrip.
type typesByPackageIndex interface {
	// forPackage returns an iterator over every DWARF offset whose owning
	// package equals pkgName (compared in unescaped form; internally escaped
	// before lookup). Offsets are yielded in DWARF-offset order.
	forPackage(pkgName string) iter.Seq[dwarf.Offset]
	io.Closer
}

// typesByPackageIndexBuilder accumulates (pkg, offset) entries during the
// prepass and produces a sorted typesByPackageIndex.
type typesByPackageIndexBuilder interface {
	// add records that the named user-type DIE at offset belongs to pkg. The
	// builder escapes pkg internally; callers pass the unescaped form (the same
	// form parseLinkFuncName produces).
	add(pkg string, offset dwarf.Offset) error
	build() (typesByPackageIndex, error)
	io.Closer
}

// emptyTypesByPackageIndex yields no offsets. Used when no entries
// are recorded.
type emptyTypesByPackageIndex struct{}

var _ typesByPackageIndex = emptyTypesByPackageIndex{}

func (emptyTypesByPackageIndex) forPackage(string) iter.Seq[dwarf.Offset] {
	return func(func(dwarf.Offset) bool) {}
}

func (emptyTypesByPackageIndex) Close() error { return nil }

// inMemTypesByPackageEntry is one (escapedPkg, dwarfOffset) pair.
type inMemTypesByPackageEntry struct {
	pkg    string
	offset dwarf.Offset
}

// inMemTypesByPackageIndexBuilder accumulates entries in memory.
type inMemTypesByPackageIndexBuilder struct {
	entries []inMemTypesByPackageEntry
}

var _ typesByPackageIndexBuilder = (*inMemTypesByPackageIndexBuilder)(nil)

func (b *inMemTypesByPackageIndexBuilder) add(pkg string, offset dwarf.Offset) error {
	b.entries = append(b.entries, inMemTypesByPackageEntry{
		pkg:    gosymname.EscapePkg(pkg),
		offset: offset,
	})
	return nil
}

func (b *inMemTypesByPackageIndexBuilder) build() (typesByPackageIndex, error) {
	if len(b.entries) == 0 {
		b.entries = nil
		return emptyTypesByPackageIndex{}, nil
	}
	slices.SortFunc(b.entries, func(a, c inMemTypesByPackageEntry) int {
		return cmp.Or(
			strings.Compare(a.pkg, c.pkg),
			cmp.Compare(a.offset, c.offset),
		)
	})
	idx := &inMemTypesByPackageIndex{entries: b.entries}
	b.entries = nil
	return idx, nil
}

func (b *inMemTypesByPackageIndexBuilder) Close() error {
	b.entries = nil
	return nil
}

// inMemTypesByPackageIndex is the in-memory implementation.
type inMemTypesByPackageIndex struct {
	entries []inMemTypesByPackageEntry // sorted by (pkg, offset)
}

var _ typesByPackageIndex = (*inMemTypesByPackageIndex)(nil)

func (idx *inMemTypesByPackageIndex) forPackage(pkgName string) iter.Seq[dwarf.Offset] {
	target := gosymname.EscapePkg(pkgName)
	return func(yield func(dwarf.Offset) bool) {
		i, _ := slices.BinarySearchFunc(idx.entries, target, func(e inMemTypesByPackageEntry, t string) int {
			return strings.Compare(e.pkg, t)
		})
		for ; i < len(idx.entries); i++ {
			e := &idx.entries[i]
			if e.pkg != target {
				break
			}
			if !yield(e.offset) {
				return
			}
		}
	}
}

func (idx *inMemTypesByPackageIndex) Close() error { return nil }

// onDiskTypesByPackageEntry is the fixed-size record stored in the
// .entries file. pkgStrOffset references a per-package entry in the
// .pkgs string table — multiple types from one package share one
// string-table entry, so the table stays compact (one entry per
// distinct package).
type onDiskTypesByPackageEntry struct {
	pkgStrOffset uint32
	dwarfOffset  uint32
}

// onDiskTypesByPackageIndexBuilder streams (pkgStrOffset, dwarfOffset) entries
// to disk during the prepass. All write-path state lives on disk or in
// fixed-size scratch buffers; nothing scales with input size on the Go heap.
//
// Package strings are *not* deduplicated: the same package gets a fresh entry
// in the .pkgs file every time it's seen. On a binary with thousands of indexed
// type DIEs spread across hundreds of packages this costs an extra ~MB of
// (page-cache-resident) disk to avoid an unbounded heap-resident map.
// forPackage's binary search works regardless of duplication: identical package
// strings sort adjacent after build()'s lex-then-offset sort, so the binary
// search lands on the first one and yields contiguously through all matches.
type onDiskTypesByPackageIndexBuilder struct {
	pkgsFile   *object.DiskFile
	pkgsW      *bufio.Writer
	idxFile    *object.DiskFile
	idxW       *bufio.Writer
	pkgsPos    uint32
	numEntries uint32
	lenBuf     [4]byte
	entryBuf   onDiskTypesByPackageEntry
}

var _ typesByPackageIndexBuilder = (*onDiskTypesByPackageIndexBuilder)(nil)

const onDiskTypesByPackageIndexMaxSize = 256 << 20 // 256 MiB per file
const onDiskTypesByPackageIndexInitialSize = 1 << 20

func newOnDiskTypesByPackageIndexBuilder(
	dc *object.DiskCache, suffix string,
) (*onDiskTypesByPackageIndexBuilder, error) {
	pkgsFile, err := dc.NewFile(
		"typesByPackageIndex."+suffix+".pkgs",
		onDiskTypesByPackageIndexMaxSize,
		onDiskTypesByPackageIndexInitialSize,
	)
	if err != nil {
		return nil, err
	}
	idxFile, err := dc.NewFile(
		"typesByPackageIndex."+suffix+".entries",
		onDiskTypesByPackageIndexMaxSize,
		onDiskTypesByPackageIndexInitialSize,
	)
	if err != nil {
		_ = pkgsFile.Close()
		return nil, err
	}
	return &onDiskTypesByPackageIndexBuilder{
		pkgsFile: pkgsFile,
		pkgsW:    bufio.NewWriter(pkgsFile),
		idxFile:  idxFile,
		idxW:     bufio.NewWriter(idxFile),
	}, nil
}

// writePkg appends a length-prefixed copy of pkg to the .pkgs file
// and returns the offset. No deduplication; see the type-level doc
// for the rationale.
func (b *onDiskTypesByPackageIndexBuilder) writePkg(pkg string) (uint32, error) {
	off := b.pkgsPos
	binary.NativeEndian.PutUint32(b.lenBuf[:], uint32(len(pkg)))
	if _, err := b.pkgsW.Write(b.lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := b.pkgsW.WriteString(pkg); err != nil {
		return 0, err
	}
	b.pkgsPos += 4 + uint32(len(pkg))
	return off, nil
}

func (b *onDiskTypesByPackageIndexBuilder) add(pkg string, offset dwarf.Offset) error {
	if b.idxW == nil {
		return errors.New("builder is closed")
	}
	strOff, err := b.writePkg(gosymname.EscapePkg(pkg))
	if err != nil {
		return err
	}
	b.entryBuf = onDiskTypesByPackageEntry{
		pkgStrOffset: strOff,
		dwarfOffset:  uint32(offset),
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&b.entryBuf)), unsafe.Sizeof(b.entryBuf))
	if _, err := b.idxW.Write(buf); err != nil {
		return err
	}
	b.numEntries++
	return nil
}

func (b *onDiskTypesByPackageIndexBuilder) build() (_ typesByPackageIndex, retErr error) {
	if b.idxW == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() {
		if retErr != nil {
			b.cleanup()
		}
	}()

	if err := b.pkgsW.Flush(); err != nil {
		return nil, err
	}
	b.pkgsW = nil
	if err := b.idxW.Flush(); err != nil {
		return nil, err
	}
	b.idxW = nil

	if b.numEntries == 0 {
		b.cleanup()
		return emptyTypesByPackageIndex{}, nil
	}

	pkgsMM, err := b.pkgsFile.IntoMMap(syscall.PROT_READ)
	if err != nil {
		return nil, err
	}
	b.pkgsFile = nil
	defer func() {
		if retErr != nil {
			_ = pkgsMM.Close()
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
		(*onDiskTypesByPackageEntry)(unsafe.Pointer(unsafe.SliceData(idxData))),
		uintptr(len(idxData))/unsafe.Sizeof(onDiskTypesByPackageEntry{}),
	)

	pkgsData := pkgsMM.Data()
	readPkg := func(off uint32) string {
		nameLen := binary.NativeEndian.Uint32(pkgsData[off:])
		return string(pkgsData[off+4 : off+4+nameLen])
	}

	// Sort in-place on the writable mmap by (pkg lex order,
	// dwarfOffset). Dereferencing pkgStrOffset for the comparison
	// gives us the package-grouped, then offset-ordered layout
	// forPackage expects.
	slices.SortFunc(entries, func(a, c onDiskTypesByPackageEntry) int {
		return cmp.Or(
			strings.Compare(readPkg(a.pkgStrOffset), readPkg(c.pkgStrOffset)),
			cmp.Compare(a.dwarfOffset, c.dwarfOffset),
		)
	})

	return &onDiskTypesByPackageIndex{
		pkgsMM:   pkgsMM,
		idxMM:    idxMM,
		pkgsData: pkgsData,
		entries:  entries,
	}, nil
}

func (b *onDiskTypesByPackageIndexBuilder) cleanup() {
	if b.pkgsFile != nil {
		_ = b.pkgsFile.Close()
		b.pkgsFile = nil
	}
	if b.idxFile != nil {
		_ = b.idxFile.Close()
		b.idxFile = nil
	}
	b.pkgsW = nil
	b.idxW = nil
}

func (b *onDiskTypesByPackageIndexBuilder) Close() error {
	b.cleanup()
	return nil
}

// onDiskTypesByPackageIndex is the mmap-backed implementation.
type onDiskTypesByPackageIndex struct {
	pkgsMM   object.SectionData
	idxMM    object.SectionData
	pkgsData []byte
	entries  []onDiskTypesByPackageEntry // sorted by (pkg lex, dwarfOffset)
}

var _ typesByPackageIndex = (*onDiskTypesByPackageIndex)(nil)

func (idx *onDiskTypesByPackageIndex) readPkg(off uint32) string {
	nameLen := binary.NativeEndian.Uint32(idx.pkgsData[off:])
	return string(idx.pkgsData[off+4 : off+4+nameLen])
}

func (idx *onDiskTypesByPackageIndex) forPackage(pkgName string) iter.Seq[dwarf.Offset] {
	target := gosymname.EscapePkg(pkgName)
	return func(yield func(dwarf.Offset) bool) {
		i, _ := slices.BinarySearchFunc(idx.entries, target, func(e onDiskTypesByPackageEntry, t string) int {
			return strings.Compare(idx.readPkg(e.pkgStrOffset), t)
		})
		for ; i < len(idx.entries); i++ {
			e := &idx.entries[i]
			if idx.readPkg(e.pkgStrOffset) != target {
				break
			}
			if !yield(dwarf.Offset(e.dwarfOffset)) {
				return
			}
		}
		runtime.KeepAlive(idx)
	}
}

func (idx *onDiskTypesByPackageIndex) Close() error {
	if idx.pkgsMM == nil {
		return nil
	}
	defer func() { *idx = onDiskTypesByPackageIndex{} }()
	return errors.Join(idx.pkgsMM.Close(), idx.idxMM.Close())
}
