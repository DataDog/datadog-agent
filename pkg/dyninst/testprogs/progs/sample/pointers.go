// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "unsafe"

type structWithTwoValues struct {
	a uint
	b bool
}

type structWithPointer struct {
	a *uint64
}

type spws struct {
	a int
	x *string
}

type swsp struct {
	a int
	b *nStruct
}

type reallyComplexType struct {
	pointerToStructWithAPointerToAStruct *swsp
	anArray                              [1]nStruct
	aString                              string
	aStringPtr                           *string
}

//nolint:all
//go:noinline
func testPointerToSimpleStruct(a *structWithTwoValues) {}

//nolint:all
//go:noinline
func testLinkedList(a node) {}

type node struct {
	val int
	b   *node
}

//nolint:all
//go:noinline
func testPointerLoop(a *node) {}

//nolint:all
//go:noinline
func testUnsafePointer(x unsafe.Pointer) {}

//nolint:all
//go:noinline
func testArrayPointer(x *[2]uint) {}

//nolint:all
//go:noinline
func testUintPointer(x *uint) {}

//nolint:all
//go:noinline
func testStructPointer(x *nStruct) {}

//nolint:all
//go:noinline
func testNilStructPointer(z uint, x *nStruct, a int) {}

//nolint:all
//go:noinline
func testComplexType(z *reallyComplexType) {}

//nolint:all
//go:noinline
func testStructWithPointer(x structWithPointer) {}

//nolint:all
//go:noinline
func testStructWithStructPointer(b swsp) {}

//nolint:all
//go:noinline
func testStructWithStringPointer(z spws) {}

//nolint:all
//go:noinline
func testStringPointer(z *string) {}

//nolint:all
//go:noinline
func testStringSlicePointer(a *[]string) {}

//nolint:all
//go:noinline
func testNilPointer(z *bool, a uint) {}

//nolint:all
//go:noinline
func testPointerToPointer(u **int) {}

//nolint:all
func executePointerFuncs() {
	var u64F uint64 = 5
	swp := structWithPointer{a: &u64F}
	testStructWithPointer(swp)

	r := "abc"
	z := spws{3, &r}

	var uintToPointTo uint = 1
	testUintPointer(&uintToPointTo)

	n := nStruct{true, 1, 2}
	testStructPointer(&n)
	testNilStructPointer(4, nil, 5)
	ssaw := swsp{
		a: 1,
		b: &n,
	}
	testStructWithStructPointer(ssaw)
	testStructWithStringPointer(z)
	testStringPointer(&r)

	x := structWithTwoValues{9, true}
	testPointerToSimpleStruct(&x)

	rct := reallyComplexType{
		pointerToStructWithAPointerToAStruct: &ssaw,
		anArray:                              [1]nStruct{n},
		aString:                              "hello",
		aStringPtr:                           &r,
	}
	testComplexType(&rct)

	b := node{
		val: 1,
		b: &node{
			val: 2,
			b: &node{
				val: 3,
				b:   nil,
			},
		},
	}
	testLinkedList(b)

	testUnsafePointer(unsafe.Pointer(&b))

	aruint := [2]uint{1, 2}
	testArrayPointer(&aruint)

	stringSlice := []string{"aaa", "bbb", "ccc", "ddd"}
	testStringSlicePointer(&stringSlice)

	testNilPointer(nil, 1)

	u := 9
	up := &u
	upp := &up
	testPointerToPointer(upp)

	self := &node{
		val: 1,
	}
	self.b = self
	testPointerLoop(self)
}
