// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"cmp"
	"debug/dwarf"
	"iter"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

type inMemoryGoTypeIndexFactory struct{}

var _ goTypeIndexFactory = (*inMemoryGoTypeIndexFactory)(nil)

func (i *inMemoryGoTypeIndexFactory) newGoTypeToOffsetIndexBuilder(
	ir.ProgramID, uint64,
) (goTypeToOffsetIndexBuilder, error) {
	return &inMemoryGoTypeToOffsetIndexBuilder{}, nil
}
func (i inMemoryGoTypeIndexFactory) newMethodToGoTypeIndexBuilder(
	ir.ProgramID, uint64,
) (methodToGoTypeIndexBuilder, error) {
	return &inMemoryMethodToGoTypeIndexBuilder{}, nil
}

type inMemoryGoTypeToOffsetIndex []goTypeOffsetEntry

var _ goTypeToOffsetIndex = (*inMemoryGoTypeToOffsetIndex)(nil)

func (index inMemoryGoTypeToOffsetIndex) resolveDwarfOffset(typeID gotype.TypeID) (dwarf.Offset, bool) {
	if idx := slices.IndexFunc(index, func(entry goTypeOffsetEntry) bool {
		return entry.typeID == typeID
	}); idx != -1 {
		return index[idx].dwarfOffset, true
	}
	return 0, false
}
func (index inMemoryGoTypeToOffsetIndex) allGoTypes() iter.Seq[gotype.TypeID] {
	return func(yield func(gotype.TypeID) bool) {
		for _, entry := range index {
			if !yield(entry.typeID) {
				return
			}
		}
	}
}
func (index *inMemoryGoTypeToOffsetIndex) Close() error { return nil }

type inMemoryGoTypeToOffsetIndexBuilder inMemoryGoTypeToOffsetIndex

var _ goTypeToOffsetIndexBuilder = (*inMemoryGoTypeToOffsetIndexBuilder)(nil)

func (b *inMemoryGoTypeToOffsetIndexBuilder) addType(
	typeID gotype.TypeID, dwarfOffset dwarf.Offset,
) error {
	*b = append(*b, goTypeOffsetEntry{typeID: typeID, dwarfOffset: dwarfOffset})
	return nil
}
func (b *inMemoryGoTypeToOffsetIndexBuilder) build() (goTypeToOffsetIndex, error) {
	slices.SortFunc(*b, func(a, b goTypeOffsetEntry) int {
		return cmp.Compare(a.typeID, b.typeID)
	})
	return (*inMemoryGoTypeToOffsetIndex)(b), nil
}
func (b *inMemoryGoTypeToOffsetIndexBuilder) Close() error { return nil }

type goTypeOffsetEntry struct {
	typeID      gotype.TypeID
	dwarfOffset dwarf.Offset
}

type methodEntry struct {
	method   gotype.Method
	receiver gotype.TypeID
}

func cmpMethod(a, b gotype.Method) int {
	return cmp.Or(
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.Mtyp, b.Mtyp),
	)
}

func cmpMethodEntry(a, b methodEntry) int {
	return cmp.Or(
		cmpMethod(a.method, b.method),
		cmp.Compare(a.receiver, b.receiver),
	)
}

type inMemoryMethodToGoTypeIndexBuilder []methodEntry

var _ methodToGoTypeIndexBuilder = (*inMemoryMethodToGoTypeIndexBuilder)(nil)

func (b *inMemoryMethodToGoTypeIndexBuilder) addMethod(
	method gotype.Method, receiver gotype.TypeID,
) error {
	*b = append(*b, methodEntry{method: method, receiver: receiver})
	return nil
}
func (b inMemoryMethodToGoTypeIndexBuilder) build() (methodToGoTypeIndex, error) {
	slices.SortFunc(b, cmpMethodEntry)
	tids := make([]gotype.TypeID, 0, len(b))
	var methods []methodIndexEntry
	if len(b) == 0 {
		return &inMemoryMethodToGoTypeIndex{}, nil
	}

	var prevIdx uint32
	curMethod := b[0].method
	for _, entry := range b {
		if curMethod != entry.method {
			methods = append(methods, methodIndexEntry{
				method:  curMethod,
				offsets: [2]uint32{prevIdx, uint32(len(tids))},
			})
			prevIdx = uint32(len(tids))
			curMethod = entry.method
		}
		tids = append(tids, entry.receiver)
	}
	methods = append(methods, methodIndexEntry{
		method:  curMethod,
		offsets: [2]uint32{prevIdx, uint32(len(tids))},
	})
	return &inMemoryMethodToGoTypeIndex{methods: methods, tids: tids}, nil
}
func (b *inMemoryMethodToGoTypeIndexBuilder) Close() error {
	return nil
}

type inMemoryMethodToGoTypeIndex struct {
	methods []methodIndexEntry
	tids    []gotype.TypeID
}

var _ methodToGoTypeIndex = (*inMemoryMethodToGoTypeIndex)(nil)

func (b *inMemoryMethodToGoTypeIndex) Close() error {
	return nil
}
func (b *inMemoryMethodToGoTypeIndex) Iterator() methodToGoTypeIterator {
	return &inMemoryMethodToGoTypeIterator{index: b}
}

type inMemoryMethodToGoTypeIterator struct {
	index *inMemoryMethodToGoTypeIndex
	types []gotype.TypeID
}

var _ methodToGoTypeIterator = (*inMemoryMethodToGoTypeIterator)(nil)

func (ii *inMemoryMethodToGoTypeIterator) seek(method gotype.Method) {
	idx := slices.IndexFunc(ii.index.methods, func(e methodIndexEntry) bool {
		return e.method == method
	})
	if idx == -1 {
		ii.types = nil
		return
	}
	r := ii.index.methods[idx].offsets
	ii.types = ii.index.tids[r[0]:r[1]]
}
func (ii *inMemoryMethodToGoTypeIterator) valid() bool {
	return len(ii.types) > 0
}
func (ii *inMemoryMethodToGoTypeIterator) cur() gotype.TypeID {
	return ii.types[0]
}
func (ii *inMemoryMethodToGoTypeIterator) next() {
	if len(ii.types) > 0 {
		ii.types = ii.types[1:]
	}
}
