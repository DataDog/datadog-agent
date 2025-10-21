// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
	"unsafe"

	lib_v2 "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2"
)

type behavior interface {
	DoSomething() string
}

type firstBehavior struct {
	s string
}

type secondBehavior struct {
	i int
}

func (b firstBehavior) DoSomething() string {
	return fmt.Sprintln(b)
}

func (b secondBehavior) DoSomething() string {
	return fmt.Sprintf("%10d\n", b.i)
}

type iface struct {
	tab  *itab
	data unsafe.Pointer
}
type itab struct {
	inter uintptr
	_type uintptr
	hash  uint32
	_     [4]byte
	fun   [1]uintptr
}

//nolint:all
//go:noinline
func testInterface(b behavior) {}

//nolint:all
//go:noinline
func testAny(a any) string {
	return fmt.Sprintf("%v", a)
}

//nolint:all
//go:noinline
func testError(e error) {}

//nolint:all
//go:noinline
func testAnyPtr(a *any) string {
	return fmt.Sprintf("%v", a)
}

type structWithAny struct {
	a any
}

//go:noinline
func testStructWithAny(s structWithAny) {}

//nolint:all
func executeInterfaceFuncs() {
	testInterface(firstBehavior{"foo"})
	testInterface(&firstBehavior{"foo"})
	testInterface(secondBehavior{42})
	testInterface(&secondBehavior{42})
	testInterface((*secondBehavior)(nil))
	testInterface(nil)
	// Use types that don't implement the interface so that we won't sometimes
	// have the type information if the testInterface probe happens to be
	// present.
	type otherFirstBehavior firstBehavior
	type otherSecondBehavior secondBehavior
	testAny(otherFirstBehavior{"foo"})
	testAny(&otherFirstBehavior{"foo"})
	testAny(otherSecondBehavior{42})
	testAny(&otherSecondBehavior{42})
	testAny(lib_v2.V2Type{})
	testAny(&lib_v2.V2Type{})
	testAny(nil)
	one := 1
	testAny(one)
	testAny(&one)
	foo := "foo"
	testAny(foo)
	testAny(&foo)
	testError(errors.New("blah"))
	boxedOne := any(one)
	boxedOnePtr := any(&boxedOne)
	boxedPtrToBoxedOne := any(&boxedOnePtr)
	boxedNil := any(nil)
	boxedTypedNil := any((*int)(nil))
	testAnyPtr(nil)
	testAnyPtr(&boxedNil)
	testAnyPtr(&boxedTypedNil)
	testAnyPtr(&boxedOne)
	testAnyPtr(&boxedOnePtr)
	testAnyPtr(&boxedPtrToBoxedOne)

	{
		var (
			structWithAny0 structWithAny
			structWithAny1 = structWithAny{a: structWithAny0}
			structWithAny2 = structWithAny{a: structWithAny1}
			structWithAny3 = structWithAny{a: structWithAny2}
			structWithAny4 = structWithAny{a: structWithAny3}
		)
		testStructWithAny(structWithAny4)
	}
}
