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
	result := i + 100
	fmt.Println(result)
	return result
}

//go:noinline
func testReturnsIntPointer(i int) *int {
	result := i + 100
	fmt.Println(result)
	return &result
}

//go:noinline
func testReturnsIntAndFloat(i int) (int, float64) {
	resultInt := i + 100
	resultFloat := float64(i) + 0.5
	fmt.Println("returnsIntAndFloat", resultInt, resultFloat)
	return resultInt, resultFloat
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
	// Transform the value to ensure we're not reading argument memory.
	switch val := v.(type) {
	case int:
		return val + 1000, nil
	case string:
		return val + "_modified", nil
	default:
		return []int{999}, nil
	}
}

//go:noinline
func testReturnsAny(v any) any {
	fmt.Println("returnsAny", v)
	// Transform the value to ensure we're not reading argument memory.
	switch val := v.(type) {
	case int:
		return val + 1000
	case *int:
		result := *val + 1000
		return &result
	case nil:
		return nil
	default:
		return []any{999, fmt.Sprintf("%T", v)}
	}
}

//go:noinline
func testReturnsInterface(v any) behavior {
	b, ok := v.(behavior)
	if !ok {
		return nil
	}
	fmt.Println("returnsInterface", b)
	// Return a new instance to avoid reading argument memory.
	return firstBehavior{s: b.DoSomething() + "_modified"}
}

//go:noinline
func testNamedReturn(i int) (result int) {
	result = i * 2
	fmt.Println("testNamedReturn", result)
	return
}

//go:noinline
func testMultipleNamedReturn(i int) (result int, result2 int) {
	result = i * 2
	result2 = i * 3
	fmt.Println("testMultipleNamedReturn", result, result2)
	return
}

//go:noinline
func testSomeNamedReturn(i int) (_ int, result2 int, _ int) {
	fmt.Println("testSomeNamedReturn", i)
	return i * 10, i * 20, i * 30
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
func testReturnsManyFloats() (
	_ float64, _ float64, _ float64, _ float64, _ float64, _ float64, _ float64,
	_ float64, _ float64, _ float64, _ float64, _ float64, _ float64, _ float64,
	_ float64,
	// This overflows the 15 FP registers on amd64 but is still in a register on
	// arm64.
	onlyOnAmd64_16 float64,
	// This one is available and is a register on both amd64 and arm64.
	bothArmAndAmd64 float64,
) {
	fmt.Println("testReturnsManyFloats")
	return 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0, 13.0,
		14.0, 15.0, 16.0, 17.0
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
func testReturnsArray() [3]int {
	fmt.Println("testReturnsArray")
	return [3]int{1, 2, 3}
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

// Argument stack space tests - verify return offsets account for arguments.

// Large argument + large return - both on stack.
// Argument uses stack space, return offset must be after it.
//
//go:noinline
func testLargeArgAndReturn(arg largeStruct) largeStruct {
	fmt.Println("testLargeArgAndReturn", arg.a)
	arg.a *= 10
	arg.b *= 10
	arg.c *= 10
	arg.d *= 10
	arg.e *= 10
	arg.f *= 10
	arg.g *= 10
	arg.h *= 10
	return arg
}

// Array argument + array return - both on stack.
//
//go:noinline
func testArrayArgAndReturn(arg [3]int) [3]int {
	fmt.Println("testArrayArgAndReturn", arg[0])
	arg[0] *= 10
	arg[1] *= 10
	arg[2] *= 10
	return arg
}

// Multiple large arguments + large return.
//
//go:noinline
func testMultipleLargeArgs(s1 largeStruct, s2 largeStruct) largeStruct {
	fmt.Println("testMultipleLargeArgs", s1.a, s2.a)
	return largeStruct{
		a: s1.a*10 + s2.a*2,
		b: s1.b*10 + s2.b*2,
		c: s1.c*10 + s2.c*2,
		d: s1.d*10 + s2.d*2,
		e: s1.e*10 + s2.e*2,
		f: s1.f*10 + s2.f*2,
		g: s1.g*10 + s2.g*2,
		h: s1.h*10 + s2.h*2,
		i: s1.i*10 + s2.i*2,
		j: s1.j*10 + s2.j*2,
	}
}

// Exhaust registers with arguments, then return on stack.
// 9 int args (fill all int regs) + array return (on stack).
//
//go:noinline
func testManyArgsArrayReturn(a, b, c, d, e, f, g, h, i int) [2]int {
	fmt.Println("testManyArgsArrayReturn", a, b, c, d, e, f, g, h, i)
	return [2]int{i, a * 10}
}

// Some args on stack, some in regs, return on stack.
// 8 ints (in regs) + string (on stack) → large struct return (on stack).
//
//go:noinline
func testMixedArgsStackReturn(a, b, c, d, e, f, g, h int, s string) largeStruct {
	fmt.Println("testMixedArgsStackReturn", a, s)
	return largeStruct{
		a: a * 10,
		b: b * 10,
		c: c * 10,
		d: d * 10,
		e: e * 10,
		f: f * 10,
		g: g * 10,
		h: h * 10,
		i: int(len(s)),
		j: 0,
	}
}

// Stack-assigned arg + register-assigned returns.
//
//go:noinline
func testStackArgRegReturns(arg [5]int) (int, int, int) {
	fmt.Println("testStackArgRegReturns", arg[0])
	return arg[4], arg[1], arg[2]
}

// Multiple array arguments (stack) + multiple returns (some stack, some reg).
//
//go:noinline
func testMultipleArrayArgs(a [2]int, b [3]int) (int, [2]int) {
	fmt.Println("testMultipleArrayArgs", a[0], b[0])
	return a[0] + b[0], [2]int{a[0] * 10, a[1] * 10}
}

// Struct with array argument + mixed returns.
//
//go:noinline
func testStructWithArrayArg(arg structWithArray) (int, structWithArray) {
	fmt.Println("testStructWithArrayArg", arg.x)
	x := arg.x + 1
	ret := structWithArray{
		x:   arg.x * 10,
		arr: [2]int{arg.arr[0] * 10, arg.arr[1] * 10},
	}
	return x, ret
}

// Edge case: Zero-size argument (shouldn't affect offsets).
//
//go:noinline
func testZeroSizeArgAndReturn(a [0]int, b int) (int, [0]int) {
	fmt.Println("testZeroSizeArgAndReturn", b)
	b += 900
	return b, a
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

	// New ABI test cases - return values only.
	testReturnsPrimitives()
	testReturnsManyFloats()
	testReturnsComplex()
	testReturnsLargeStruct()
	testReturnsArray()
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

	// ABI test cases - argument + return interaction.
	//
	// Use different arguments to really make sure we aren't just getting lucky
	// with stack slots.
	testLargeArgAndReturn(largeStruct{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	testArrayArgAndReturn([3]int{11, 12, 13})
	testMultipleLargeArgs(
		largeStruct{101, 102, 103, 104, 105, 106, 107, 108, 109, 110},
		largeStruct{111, 112, 113, 114, 115, 116, 117, 118, 119, 120},
	)
	testManyArgsArrayReturn(21, 22, 23, 24, 25, 26, 27, 28, 29)
	testMixedArgsStackReturn(31, 32, 33, 34, 35, 36, 37, 38, "overflow")
	testStackArgRegReturns([5]int{41, 42, 43, 44, 45})
	testMultipleArrayArgs([2]int{51, 52}, [3]int{63, 64, 65})
	testStructWithArrayArg(structWithArray{arr: [2]int{71, 72}, x: 73})
	testZeroSizeArgAndReturn([0]int{}, 82)
}
