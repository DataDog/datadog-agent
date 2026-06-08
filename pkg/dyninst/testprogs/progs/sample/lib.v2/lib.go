// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lib_v2 is a package in a directory with a dot in its name (package
// names do not need to correspond to directory names necessarily). This dot
// gets escaped in DWARF, making this useful for our tests.
package lib_v2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib"
)

var dummy int

//go:noinline
func FooV2() {
	dummy++
	// Call a function defined in lib from lib.v2. lib.v2's compile
	// unit is emitted later in the DWARF than lib's own compile unit;
	// when the Go compiler inlines this call, the abstract definition
	// DIE for lib.InlinedInLaterCU lands in lib.v2's compile unit.
	// That places the abstract after lib's compile unit has already
	// been emitted by symdb — the scenario the cross-compile-unit
	// inline-instance index is designed for.
	lib.InlinedInLaterCU()
}

type V2Type struct{}

func (v *V2Type) MyMethod() {
	fmt.Println("")
}

// V2GenericBox is a generic struct in a package whose last path segment
// contains a dot. It exercises the index-key escape contract: the entry
// stored in genericTypes uses the DWARF (escaped) package form, matching
// forPackage's escaped-prefix lookup.
type V2GenericBox[T any] struct {
	Value T
}

// Get is a method so that the Go compiler emits a DW_TAG_structure_type
// DIE for V2GenericBox. Without a method-using-the-receiver, no struct
// DIE would be emitted and genericTypes would be empty for lib.v2.
//
//go:noinline
func (b V2GenericBox[T]) Get() T { return b.Value }

//go:noinline
func UseV2GenericBox() {
	b := V2GenericBox[int]{Value: 42}
	dummy += b.Get()
}
