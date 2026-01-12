// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"debug/dwarf"
	"io"
	"iter"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// goTypeIndexFactory is a factory for type index builders.
type goTypeIndexFactory interface {
	newGoTypeToOffsetIndexBuilder(
		programID ir.ProgramID,
		goTypeDataSize uint64,
	) (goTypeToOffsetIndexBuilder, error)

	newMethodToGoTypeIndexBuilder(
		programID ir.ProgramID,
		goTypeDataSize uint64,
	) (methodToGoTypeIndexBuilder, error)
}

// goTypeToOffsetIndexBuilder is a builder for goTypeToOffsetIndex.
type goTypeToOffsetIndexBuilder interface {
	addType(tid gotype.TypeID, dwarfOffset dwarf.Offset) error
	build() (goTypeToOffsetIndex, error)
	io.Closer
}

// goTypeToOffsetIndex is an index of the go types to their dwarf offsets.
type goTypeToOffsetIndex interface {
	allGoTypes() iter.Seq[gotype.TypeID]
	resolveDwarfOffset(tid gotype.TypeID) (dwarf.Offset, bool)
	io.Closer
}

// methodToGoTypeIndexBuilder is a builder for methodToGoTypeIndex.
type methodToGoTypeIndexBuilder interface {
	addMethod(method gotype.Method, receiver gotype.TypeID) error
	build() (methodToGoTypeIndex, error)
	io.Closer
}

// methodToGoTypeIndex is an index of the methods to the types that implement them.
type methodToGoTypeIndex interface {
	Iterator() methodToGoTypeIterator
	io.Closer
}

// methodToGoTypeIterator is an iterator over the methods to the types that
// implement them.
type methodToGoTypeIterator = iterator[gotype.Method, gotype.TypeID]

type iterator[Key any, Item any] interface {
	seek(key Key)
	valid() bool
	cur() Item
	next()
}
