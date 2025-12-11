// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"bufio"
	"cmp"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"runtime"
	"slices"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

type onDiskGoTypeIndexFactory struct {
	diskCache *object.DiskCache
}

var _ goTypeIndexFactory = (*onDiskGoTypeIndexFactory)(nil)

func (i *onDiskGoTypeIndexFactory) newGoTypeToOffsetIndexBuilder(
	programID ir.ProgramID, goTypeDataSize uint64,
) (goTypeToOffsetIndexBuilder, error) {
	f, err := openDiskFile(
		i.diskCache, "goTypeToOffsetIndex", programID, goTypeDataSize,
	)
	if err != nil {
		return nil, err
	}
	return &onDiskGoTypeToOffsetIndexBuilder{
		f: f,
		w: bufio.NewWriter(f),
	}, nil
}

func (i *onDiskGoTypeIndexFactory) newMethodToGoTypeIndexBuilder(
	programID ir.ProgramID, goTypeDataSize uint64,
) (methodToGoTypeIndexBuilder, error) {
	f, err := openDiskFile(
		i.diskCache, "methodToGoTypeIndex", programID, goTypeDataSize,
	)
	if err != nil {
		return nil, err
	}
	return &onDiskMethodToGoTypeIndexBuilder{
		diskCache:      i.diskCache,
		f:              f,
		w:              bufio.NewWriter(f),
		programID:      programID,
		goTypeDataSize: goTypeDataSize,
	}, nil
}

type onDiskGoTypeToOffsetIndexBuilder struct {
	f        *object.DiskFile
	w        *bufio.Writer
	entryBuf goTypeOffsetEntry
}

// AddType implements typeIndexBuilder.
func (o *onDiskGoTypeToOffsetIndexBuilder) addType(typeID gotype.TypeID, dwarfOffset dwarf.Offset) error {
	if o.w == nil {
		return errors.New("builder is closed")
	}
	o.entryBuf = goTypeOffsetEntry{typeID: typeID, dwarfOffset: dwarfOffset}
	buf := unsafe.Slice((*uint8)(unsafe.Pointer(&o.entryBuf)), unsafe.Sizeof(o.entryBuf))
	if _, err := o.w.Write(buf); err != nil {
		return err
	}
	return nil
}

// Build implements typeIndexBuilder.
func (o *onDiskGoTypeToOffsetIndexBuilder) build() (_ goTypeToOffsetIndex, retErr error) {
	if o.w == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() {
		if retErr != nil {
			_ = o.f.Close()
		}
	}()
	if err := o.w.Flush(); err != nil {
		return nil, err
	}
	o.w = nil
	defer func() { o.f = nil }()
	mm, err := o.f.IntoMMap(syscall.PROT_READ | syscall.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	bytes := mm.Data()
	entries := unsafe.Slice(
		(*goTypeOffsetEntry)(unsafe.Pointer(unsafe.SliceData(bytes))),
		uintptr(len(bytes))/unsafe.Sizeof(goTypeOffsetEntry{}),
	)
	slices.SortFunc(entries, func(a, b goTypeOffsetEntry) int {
		return cmp.Compare(a.typeID, b.typeID)
	})
	return &onDiskGoTypeToOffsetIndex{mm: mm, entries: inMemoryGoTypeToOffsetIndex(entries)}, nil
}

func (o *onDiskGoTypeToOffsetIndexBuilder) Close() error {
	if o.w == nil {
		return nil
	}
	defer func() { *o = onDiskGoTypeToOffsetIndexBuilder{} }()
	return o.f.Close()
}

type onDiskGoTypeToOffsetIndex struct {
	mm      object.SectionData
	entries inMemoryGoTypeToOffsetIndex
}

func (o *onDiskGoTypeToOffsetIndex) resolveDwarfOffset(typeID gotype.TypeID) (dwarf.Offset, bool) {
	return o.entries.resolveDwarfOffset(gotype.TypeID(typeID))
}
func (o *onDiskGoTypeToOffsetIndex) allGoTypes() iter.Seq[gotype.TypeID] {
	return func(yield func(gotype.TypeID) bool) {
		for _, entry := range o.entries {
			if !yield(entry.typeID) {
				return
			}
		}
		runtime.KeepAlive(o)
	}
}
func (o *onDiskGoTypeToOffsetIndex) Close() error {
	if o.mm == nil {
		return nil
	}
	defer func() { *o = onDiskGoTypeToOffsetIndex{} }()
	return o.mm.Close()
}

type onDiskMethodToGoTypeIndexBuilder struct {
	diskCache      *object.DiskCache
	f              *object.DiskFile
	w              *bufio.Writer
	programID      ir.ProgramID
	goTypeDataSize uint64
	entryBuf       [3]uint32
}

var _ methodToGoTypeIndexBuilder = (*onDiskMethodToGoTypeIndexBuilder)(nil)

func (o *onDiskMethodToGoTypeIndexBuilder) addMethod(method gotype.Method, receiver gotype.TypeID) error {
	if o.w == nil {
		return errors.New("builder is closed")
	}
	o.entryBuf = [3]uint32{uint32(method.Name), uint32(method.Mtyp), uint32(receiver)}
	buf := unsafe.Slice((*uint8)(unsafe.Pointer(&o.entryBuf)), unsafe.Sizeof(o.entryBuf))
	if _, err := o.w.Write(buf); err != nil {
		return err
	}
	return nil
}
func (o *onDiskMethodToGoTypeIndexBuilder) build() (_ methodToGoTypeIndex, retErr error) {
	if o.w == nil {
		return nil, errors.New("builder is closed")
	}
	defer func() { o.w = nil }()
	if err := o.w.Flush(); err != nil {
		return nil, err
	}
	o.w = nil
	defer func() { o.f = nil }()
	mm, err := o.f.IntoMMap(syscall.PROT_READ | syscall.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	defer func() { _ = mm.Close() }()
	entries := unsafe.Slice(
		(*[3]uint32)(unsafe.Pointer(unsafe.SliceData(mm.Data()))),
		uintptr(len(mm.Data()))/unsafe.Sizeof([3]uint32{}),
	)
	slices.SortFunc(entries, func(a, b [3]uint32) int {
		return cmp.Or(
			cmp.Compare(a[0], b[0]),
			cmp.Compare(a[1], b[1]),
			cmp.Compare(a[2], b[2]),
		)
	})

	methodToOffsetFile, err := openDiskFile(
		o.diskCache, "methodToOffsets", o.programID, o.goTypeDataSize,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = methodToOffsetFile.Close() }()
	methodToOffsetWriter := bufio.NewWriter(methodToOffsetFile)

	typeIDsFile, err := openDiskFile(
		o.diskCache, "implementorsTypeIDs", o.programID, o.goTypeDataSize,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = typeIDsFile.Close() }()
	typeIDsWriter := bufio.NewWriter(typeIDsFile)

	var prevIdx uint32
	entry := func(i uint32) (gotype.Method, gotype.TypeID) {
		return gotype.Method{
			Name: gotype.NameOff(entries[i][0]),
			Mtyp: gotype.TypeID(entries[i][1]),
		}, gotype.TypeID(entries[i][2])
	}
	var methodEntryBuf methodIndexEntry
	curMethod, _ := entry(0)
	var b [4]byte
	for i := range uint32(len(entries)) {
		method, tid := entry(i)
		if curMethod != method {
			methodEntryBuf.method = curMethod
			methodEntryBuf.offsets = [2]uint32{prevIdx, i}
			buf := unsafe.Slice(
				(*uint8)(unsafe.Pointer(&methodEntryBuf)),
				unsafe.Sizeof(methodEntryBuf),
			)
			if _, err := methodToOffsetWriter.Write(buf); err != nil {
				return nil, err
			}
			prevIdx = i
			curMethod = method
		}
		binary.NativeEndian.PutUint32(b[:], tid)
		if _, err := typeIDsWriter.Write(b[:]); err != nil {
			return nil, err
		}
	}

	methodEntryBuf.method = curMethod
	methodEntryBuf.offsets = [2]uint32{prevIdx, uint32(len(entries))}
	buf := unsafe.Slice(
		(*uint8)(unsafe.Pointer(&methodEntryBuf)),
		unsafe.Sizeof(methodEntryBuf),
	)
	if _, err := methodToOffsetWriter.Write(buf); err != nil {
		return nil, err
	}
	if err := methodToOffsetWriter.Flush(); err != nil {
		return nil, err
	}
	if err := typeIDsWriter.Flush(); err != nil {
		return nil, err
	}
	typeIDsData, err := typeIDsFile.IntoMMap(syscall.PROT_READ)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = typeIDsData.Close()
		}
	}()
	methodsData, err := methodToOffsetFile.IntoMMap(syscall.PROT_READ)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = methodsData.Close()
		}
	}()
	methodsDataBytes := methodsData.Data()
	typeIDsDataBytes := typeIDsData.Data()
	methodEntries := unsafe.Slice(
		(*methodIndexEntry)(unsafe.Pointer(unsafe.SliceData(methodsDataBytes))),
		uintptr(len(methodsDataBytes))/unsafe.Sizeof(methodIndexEntry{}),
	)
	typeIDsEntries := unsafe.Slice(
		(*gotype.TypeID)(unsafe.Pointer(unsafe.SliceData(typeIDsDataBytes))),
		uintptr(len(typeIDsDataBytes))/unsafe.Sizeof(gotype.TypeID(0)),
	)

	return &onDiskMethodToGoTypeIndex{
		typeIDsData: typeIDsData,
		methodsData: methodsData,
		inMem: inMemoryMethodToGoTypeIndex{
			methods: methodEntries,
			tids:    typeIDsEntries,
		},
	}, nil
}

func (o *onDiskMethodToGoTypeIndexBuilder) Close() error {
	if o.w == nil {
		return nil
	}
	defer func() { o.w = nil }()
	return o.f.Close()
}

type methodIndexEntry struct {
	method  gotype.Method
	offsets [2]uint32
}

type onDiskMethodToGoTypeIndex struct {
	typeIDsData object.SectionData
	methodsData object.SectionData
	inMem       inMemoryMethodToGoTypeIndex
}

func (o *onDiskMethodToGoTypeIndex) Close() error {
	if o.typeIDsData == nil {
		return nil
	}
	defer func() { *o = onDiskMethodToGoTypeIndex{} }()
	return errors.Join(
		o.typeIDsData.Close(),
		o.methodsData.Close(),
	)
}

var _ methodToGoTypeIndex = (*onDiskMethodToGoTypeIndex)(nil)

func (o *onDiskMethodToGoTypeIndex) Iterator() methodToGoTypeIterator {
	return o.inMem.Iterator()
}

func openDiskFile(
	d *object.DiskCache, name string, programID ir.ProgramID, goTypeDataSize uint64,
) (*object.DiskFile, error) {
	const initialSize = 1 << 20
	return d.NewFile(
		fmt.Sprintf("%s.%d", name, programID),
		goTypeDataSize,
		min(initialSize, goTypeDataSize),
	)
}
