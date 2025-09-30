// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
)

//go:noinline
func testReturnsInt(i int) int {
	fmt.Println(i)
	return i
}

//go:noinline
func testReturnsIntPointer(i int) *int {
	fmt.Println(i)
	return &i
}

//go:noinline
func testReturnsIntAndFloat(i int) (int, float64) {
	f := float64(i)
	fmt.Println("returnsIntAndFloat", i, f)
	return i, f
}

//go:noinline
func testReturnsError(failed bool) error {
	if failed {
		return errors.New("error")
	}
	return nil
}

//go:noinline
func testReturnsAnyAndError(v any, failed bool) (any, error) {
	if failed {
		return nil, errors.New("error")
	}
	return v, nil
}

//go:noinline
func testReturnsAny(v any) any {
	fmt.Println("returnsAny", v)
	return v
}

//go:noinline
func testReturnsInterface(v any) behavior {
	b, ok := v.(behavior)
	if !ok {
		return nil
	}
	fmt.Println("returnsInterface", b)
	return b
}

//go:noinline
func testNamedReturn(i int) (result int) {
	result = i
	fmt.Println("testNamedReturn", result)
	return
}

//go:noinline
func testMultipleNamedReturn(i int) (result int, result2 int) {
	result = i
	result2 = i
	fmt.Println("testMultipleNamedReturn", result, result2)
	return
}

//go:noinline
func testSomeNamedReturn(i int) (_ int, result2 int, _ int) {
	fmt.Println("testSomeNamedReturn", i)
	return i - 1, i, i + 1
}

// Primitive types - exercise all basic types.
//
//go:noinline
func testReturnsPrimitives() (int8, int16, int32, int64, uint8, uint16, uint32, uint64) {
	fmt.Println("testReturnsPrimitives")
	return 1, 2, 3, 4, 5, 6, 7, 8
}

// Floating-point exhaustion - 16 floats exceeds 15 FP registers.
// Later floats should be stack-assigned (which we CAN read via CFA).
//
//go:noinline
func testReturnsManyFloats() (float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64) {
	fmt.Println("testReturnsManyFloats")
	return 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0, 13.0, 14.0, 15.0, 16.0
}

// Complex types - use FP registers.
//
//go:noinline
func testReturnsComplex() (complex64, complex128) {
	fmt.Println("testReturnsComplex")
	return complex(1, 2), complex(3, 4)
}

// Large struct - should be stack-assigned.
type largeStruct struct {
	a, b, c, d, e, f, g, h, i, j int
}

//go:noinline
func testReturnsLargeStruct() largeStruct {
	fmt.Println("testReturnsLargeStruct")
	return largeStruct{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
}

// Array - length > 1 should be stack-assigned.
//
//go:noinline
func testReturnsArray(a [3]int) [3]int {
	return [3]int{a[0] + 10, a[1] + 10, a[2] + 10}
}

// Array length 1 - should be register-assigned (same as element).
//
//go:noinline
func testReturnsArrayOne() [1]int {
	fmt.Println("testReturnsArrayOne")
	return [1]int{42}
}

// Array length 0 - edge case.
//
//go:noinline
func testReturnsArrayZero() [0]int {
	fmt.Println("testReturnsArrayZero")
	return [0]int{}
}

// Mixed int + float struct - should have both int reg pieces and FP pieces.
type mixedStruct struct {
	a int
	b float64
	c int
}

//go:noinline
func testReturnsMixedStruct() mixedStruct {
	fmt.Println("testReturnsMixedStruct")
	return mixedStruct{a: 1, b: 2.0, c: 3}
}

// Backtracking case 1: Exhaust integer registers mid-assignment.
// 8 ints (uses 8 int regs) + 1 string (needs 2 more = 10 total).
// Since we can't fit the string, ALL should be stack-assigned.
//
//go:noinline
func testReturnsBacktrackInt() (int, int, int, int, int, int, int, int, string) {
	fmt.Println("testReturnsBacktrackInt")
	return 1, 2, 3, 4, 5, 6, 7, 8, "overflow"
}

// Backtracking case 2: Struct with too many fields.
// First few fields fit in registers, but struct as a whole doesn't.
type wideStruct struct {
	a, b, c, d, e, f, g, h, i, j, k int // 11 fields > 9 int regs
}

//go:noinline
func testReturnsWideStruct() wideStruct {
	fmt.Println("testReturnsWideStruct")
	return wideStruct{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
}

// Multiple values with some floats - tests mixed scenarios.
//
//go:noinline
func testReturnsMixed() (int, float64, int, float64, string) {
	fmt.Println("testReturnsMixed")
	return 1, 2.0, 3, 4.0, "test"
}

// Struct with nested arrays - should be stack-assigned.
type structWithArray struct {
	arr [2]int
	x   int
}

//go:noinline
func testReturnsStructWithArray() structWithArray {
	fmt.Println("testReturnsStructWithArray")
	return structWithArray{arr: [2]int{1, 2}, x: 3}
}

// Edge case: Only floats (no int regs).
// Should NOT be augmented by ABI (keep DWARF).
//
//go:noinline
func testReturnsOnlyFloat() float64 {
	fmt.Println("testReturnsOnlyFloat")
	return 42.0
}

// Edge case: Multiple floats only.
//
//go:noinline
func testReturnsMultipleFloats() (float32, float64) {
	fmt.Println("testReturnsMultipleFloats")
	return 1.0, 2.0
}

// Bool and uintptr (should use int regs).
//
//go:noinline
func testReturnsBoolAndUintptr() (bool, uintptr) {
	fmt.Println("testReturnsBoolAndUintptr")
	return true, 0x1234
}

func executeReturns() {
	testReturnsInt(42)
	testReturnsIntPointer(42)
	testReturnsIntAndFloat(42)
	testReturnsError(true)
	testReturnsError(false)
	testReturnsAnyAndError(42, true)
	testReturnsAnyAndError(42, false)

	v := firstBehavior{"foo"}
	testReturnsAny(nil)
	testReturnsAny(42)
	fortyTwo := 42
	testReturnsAny(&fortyTwo)

	testReturnsInterface(42)
	testReturnsInterface(v)
	testReturnsInterface(&v)

	testNamedReturn(40)
	testMultipleNamedReturn(41)
	testSomeNamedReturn(42)

	// New ABI test cases.
	testReturnsPrimitives()
	testReturnsManyFloats()
	testReturnsComplex()
	testReturnsLargeStruct()
	testReturnsArray([3]int{1, 2, 3})
	testReturnsArrayOne()
	testReturnsArrayZero()
	testReturnsMixedStruct()
	testReturnsBacktrackInt()
	testReturnsWideStruct()
	testReturnsMixed()
	testReturnsStructWithArray()
	testReturnsOnlyFloat()
	testReturnsMultipleFloats()
	testReturnsBoolAndUintptr()
}
