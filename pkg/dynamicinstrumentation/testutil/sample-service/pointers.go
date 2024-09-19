package main

import "unsafe"

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

//go:noinline
func test_linked_list(a node) {}

type node struct {
	val int
	b   *node
}

//go:noinline
func test_unsafe_pointer(x unsafe.Pointer) {}

//go:noinline
func test_uint_pointer(x *uint) {}

//go:noinline
func test_struct_pointer(x *nStruct) {}

//go:noinline
func test_complex_type(z *reallyComplexType) {}

//go:noinline
func test_struct_with_pointer(x structWithPointer) {}

//go:noinline
func test_struct_with_struct_pointer(b swsp) {}

//go:noinline
func test_struct_with_string_pointer(z spws) {}

//go:noinline
func test_string_pointer(z *string) {}

func executePointerFuncs() {
	var u64F uint64 = 5
	swp := structWithPointer{a: &u64F}
	test_struct_with_pointer(swp)

	r := "abc"
	z := spws{3, &r}

	var uintToPointTo uint = 123
	test_uint_pointer(&uintToPointTo)

	n := nStruct{true, 1, 2}
	test_struct_pointer(&n)

	ssaw := swsp{
		a: 1,
		b: &n,
	}
	test_struct_with_struct_pointer(ssaw)
	test_struct_with_string_pointer(z)
	test_string_pointer(&r)

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
			b:   nil,
		},
	}
	test_linked_list(b)

	test_unsafe_pointer(unsafe.Pointer(&b))
}
