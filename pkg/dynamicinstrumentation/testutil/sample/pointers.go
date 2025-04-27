// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

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
func test_pointer_to_simple_struct(a *structWithTwoValues) {}

//nolint:all
//go:noinline
func test_linked_list(a node) {}

type node struct {
	val int
	b   *node
}

//nolint:all
//go:noinline
func test_unsafe_pointer(x unsafe.Pointer) {}

//nolint:all
//go:noinline
func test_array_pointer(x *[2]uint) {}

//nolint:all
//go:noinline
func test_uint_pointer(x *uint) {}

//nolint:all
//go:noinline
func test_struct_pointer(x *nStruct) {}

//nolint:all
//go:noinline
func test_nil_struct_pointer(z uint, x *nStruct, a int) {}

//nolint:all
//go:noinline
func test_complex_type(z *reallyComplexType) {}

//nolint:all
//go:noinline
func test_struct_with_pointer(x structWithPointer) {}

//nolint:all
//go:noinline
func test_struct_with_struct_pointer(b swsp) {}

//nolint:all
//go:noinline
func test_struct_with_string_pointer(z spws) {}

//nolint:all
//go:noinline
func test_string_pointer(z *string) {}

//nolint:all
//go:noinline
func test_string_slice_pointer(a *[]string) {}

//nolint:all
//go:noinline
func test_nil_pointer(z *bool, a uint) {}

//nolint:all
//go:noinline
func test_pointer_to_pointer(u **int) {}

//nolint:all
func ExecutePointerFuncs() {
	var u64F uint64 = 5
	swp := structWithPointer{a: &u64F}
	test_struct_with_pointer(swp)

	r := "abc"
	z := spws{3, &r}

	var uintToPointTo uint = 1
	test_uint_pointer(&uintToPointTo)

	n := nStruct{true, 1, 2}
	test_struct_pointer(&n)
	test_nil_struct_pointer(4, nil, 5)
	ssaw := swsp{
		a: 1,
		b: &n,
	}
	test_struct_with_struct_pointer(ssaw)
	test_struct_with_string_pointer(z)
	test_string_pointer(&r)

	x := structWithTwoValues{9, true}
	test_pointer_to_simple_struct(&x)

	rct := reallyComplexType{
		pointerToStructWithAPointerToAStruct: &ssaw,
		anArray:                              [1]nStruct{n},
		aString:                              "hello",
		aStringPtr:                           &r,
	}
	test_complex_type(&rct)

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
	test_linked_list(b)

	test_unsafe_pointer(unsafe.Pointer(&b))

	aruint := [2]uint{1, 2}
	test_array_pointer(&aruint)

	stringSlice := []string{"aaa", "bbb", "ccc", "ddd"}
	test_string_slice_pointer(&stringSlice)

	test_nil_pointer(nil, 1)

	u := 9
	up := &u
	upp := &up
	test_pointer_to_pointer(upp)
}
