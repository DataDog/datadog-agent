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
	// Call a function defined in lib from lib.v2. lib.v2's CU is
	// emitted later in the DWARF than lib's own CU; when the Go
	// compiler inlines this call, the abstract definition DIE for
	// lib.InlinedInLaterCU lands in lib.v2's CU. That places the
	// abstract after lib's CU has already been emitted by symdb —
	// the scenario the new cross-CU inline-instance index is
	// designed for.
	lib.InlinedInLaterCU()
}

type V2Type struct{}

func (v *V2Type) MyMethod() {
	fmt.Println("")
}
