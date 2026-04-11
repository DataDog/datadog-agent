// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command simple is a basic go program to be used with dyninst tests.
package main

import (
	"fmt"
	"log"
)

func main() {
	_, err := fmt.Scanln()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	intArg(0x0123456789abcdef)
	stringArg("d")
	intSliceArg([]int{1, 2, 3})
	intArrayArg([3]int{1, 2, 3})
	stringSliceArg([]string{"a", "b", "c"})
	stringArrayArg([3]string{"a", "b", "c"})
	stringArrayArgFrameless([3]string{"foo", "bar", "baz"})
	inlined(1)
	// Passing inlined function as an argument forces out-of-line instantation.
	funcArg(inlined)
	mapArg(map[string]int{"a": 1})
	bigMapArg(map[string]bigStruct{"b": {Field1: 1}})
	val := 17
	ptr1 := &val
	ptr2 := &ptr1
	ptr3 := &ptr2
	ptr4 := &ptr3
	ptr5 := &ptr4
	PointerChainArg(ptr5)
	PointerSmallChainArg(ptr2)
	noArgs()
	usesMapsOfMapsThatDoNotAppearAsArguments()

	// Condition test functions: each called twice — once with the value that
	// conditional probes match, once with a different value. Conditional probes
	// should fire only on the matching call; unconditional probes fire on both.
	// The "tag" parameter is a separate variable for log templates / captures.
	condInt8(42, "match")
	condInt8(7, "miss")
	condInt16(42, "match")
	condInt16(7, "miss")
	condInt32(42, "match")
	condInt32(7, "miss")
	condInt64(42, "match")
	condInt64(7, "miss")
	condUint8(42, "match")
	condUint8(7, "miss")
	condUint16(42, "match")
	condUint16(7, "miss")
	condUint32(42, "match")
	condUint32(7, "miss")
	condUint64(42, "match")
	condUint64(7, "miss")
	condFloat32(3.14, "match")
	condFloat32(1.0, "miss")
	condFloat64(3.14, "match")
	condFloat64(1.0, "miss")
	condBool(true, "match")
	condBool(false, "miss")
	condString("hello", "match")
	condString("other", "miss")

	// Struct with typed fields: called twice with different field values so
	// field-level conditions can distinguish the calls.
	condStructArg(condFields{
		I8: 10, I16: 200, I32: 300, I64: 400,
		U8: 50, U16: 600, U32: 700, U64: 800,
		F32: 1.5, F64: 2.5, B: true, S: "world",
	}, "match")
	condStructArg(condFields{
		I8: 1, I16: 2, I32: 3, I64: 4,
		U8: 5, U16: 6, U32: 7, U64: 8,
		F32: 0.1, F64: 0.2, B: false, S: "other",
	}, "miss")

	// Pointer-to-struct: called twice for the same reason.
	condPtrStructArg(&condFields{
		I8: 10, I16: 200, I32: 300, I64: 400,
		U8: 50, U16: 600, U32: 700, U64: 800,
		F32: 1.5, F64: 2.5, B: true, S: "world",
	}, "match")
	condPtrStructArg(&condFields{
		I8: 1, I16: 2, I32: 3, I64: 4,
		U8: 5, U16: 6, U32: 7, U64: 8,
		F32: 0.1, F64: 0.2, B: false, S: "other",
	}, "miss")

	// Return value and local: called twice so condition on parameter fires once.
	condReturnAndLocal(5, 3)
	condReturnAndLocal(1, 1)

	// Condition-only probe: called twice, condition matches only one.
	condOnly(99, "match")
	condOnly(0, "miss")

	// Line probe target: called twice with different values.
	condLine(7, "match")
	condLine(0, "miss")

	// Nil pointer in condition dereference chain: called twice.
	// First call: non-nil pointer matching condition → normal snapshot, no eval error.
	// Second call: nil pointer → condition eval error, snapshot still emitted.
	condNilPtrStruct(&condFields{I32: 300}, "match")
	condNilPtrStruct(nil, "nilptr")

	// Error case targets: called once each (conditions will fail at analysis).
	condSliceArg([]int{1, 2, 3}, "err")
	condMapArg(map[string]int{"a": 1}, "err")
	condStructDirect(condFields{I32: 42}, "err")

	// len/isEmpty test functions: each called twice — once with a value that
	// matches conditions, once with a different value.
	lenString("hello", "match")
	lenString("", "miss")
	lenSlice([]int{1, 2, 3, 4, 5}, "match")
	lenSlice(nil, "miss")
	lenMap(map[string]int{"a": 1, "b": 2, "c": 3}, "match")
	lenMap(nil, "miss")
	str := "hello"
	lenPtrString(&str, "match")
	emptyStr := ""
	lenPtrString(&emptyStr, "miss")

	// len/isEmpty on struct fields (getmember + len)
	sl := []int{1, 2, 3}
	mp := map[string]int{"a": 1, "b": 2}
	lenStructFields(lenFields{
		S: "hello", Items: sl, Dict: mp,
		PtrStr: &str, PtrSlice: &sl,
	}, "match")
	lenStructFields(lenFields{
		S: "", Items: nil, Dict: nil,
		PtrStr: &emptyStr, PtrSlice: nil,
	}, "miss")

	// len/isEmpty on pointer-to-struct fields
	lenPtrStructFields(&lenFields{
		S: "hello", Items: sl, Dict: mp,
		PtrStr: &str, PtrSlice: &sl,
	}, "match")
	lenPtrStructFields(&lenFields{
		S: "", Items: nil, Dict: nil,
		PtrStr: &emptyStr, PtrSlice: nil,
	}, "miss")

	// len/isEmpty error cases: unsupported types
	lenErrInt(42, "err")
	lenErrStruct(condFields{I32: 1}, "err")

	// Generic function called with two different shape instantiations.
	// int and string have different GC shapes, so the compiler emits two
	// distinct shape functions (go.shape.int, go.shape.string). A single
	// probe targeting genericIdentity[...] will match both, exercising
	// shared throttling across shapes and runtime dictionary resolution.
	genericIdentity(42)
	genericIdentity("hello")
}

//go:noinline
func intArg(x int) {
	fmt.Println(x)
}

//go:noinline
func stringArg(s string) {
	fmt.Println(s)
}

//go:noinline
func intSliceArg(s []int) {
	fmt.Println(s)
}

//go:noinline
func intArrayArg(s [3]int) {
	fmt.Println(s)
}

//go:noinline
func stringSliceArg(s []string) {
	fmt.Println(s)
}

//go:noinline
func stringArrayArg(s [3]string) {
	fmt.Println(s)
}

//go:noinline
func stringArrayArgFrameless(s [3]string) {
}

func inlined(x int) {
	fmt.Println(x)
}

//go:noinline
func funcArg(f func(int)) {
	f(2)
}

//go:noinline
func mapArg(m map[string]int) {
	fmt.Println(m)
}

type bigStruct struct {
	Field1 int
	Field2 int
	Field3 int
	Field4 int
	Field5 int
	Field6 int
	Field7 int

	data [128]byte
}

//go:noinline
func bigMapArg(m map[string]bigStruct) {
	v, ok := m["b"]
	if ok {
		v.data[0] = 1 // use data
	}
	fmt.Println(m)
}

func PointerChainArg(ptr *****int) {
	fmt.Println(ptr)
}

func PointerSmallChainArg(ptr **int) {
	fmt.Println(ptr)
}

//go:noinline
func noArgs() {
	fmt.Println("noArgs")
}

// condFields is a struct with fields of every base type, used for condition
// tests involving member access (getmember) and type coercion.
type condFields struct {
	I8  int8
	I16 int16
	I32 int32
	I64 int64
	U8  uint8
	U16 uint16
	U32 uint32
	U64 uint64
	F32 float32
	F64 float64
	B   bool
	S   string

	arr [3]int16 // prevent this struct from being split to registers
}

// --- Condition test functions: one per base type ---
//
// Every function takes a second parameter "tag" so that probes can reference
// a variable other than the condition variable in their log template or
// capture expressions.

//go:noinline
func condInt8(x int8, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condInt16(x int16, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condInt32(x int32, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condInt64(x int64, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condUint8(x uint8, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condUint16(x uint16, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condUint32(x uint32, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condUint64(x uint64, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condFloat32(x float32, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condFloat64(x float64, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condBool(x bool, tag string) {
	fmt.Println(x, tag)
}

//go:noinline
func condString(x string, tag string) {
	fmt.Println(x, tag)
}

// condStructArg takes a struct by value for field-level conditions.
// The tag parameter is available for templates/captures independent of x.
//
//go:noinline
func condStructArg(x condFields, tag string) {
	fmt.Println(x.I32, x.S, x.B, x.F64, tag)
}

// condPtrStructArg takes a pointer to struct for pointer-dereference + member
// access conditions. The tag parameter is for template use.
//
//go:noinline
func condPtrStructArg(x *condFields, tag string) {
	fmt.Println(x.I32, x.S, x.B, x.F64, tag)
}

// condNilPtrStruct takes a pointer-to-struct that may be nil. When x is nil,
// a condition like x.I32 == 300 causes a nil-pointer dereference in the eBPF
// condition evaluation chain. The condition should be treated as passed
// (snapshot emitted) with a condition evaluation error reported.
//
//go:noinline
func condNilPtrStruct(x *condFields, tag string) {
	fmt.Println("condNilPtrStruct", x, tag)
}

// condReturnAndLocal has parameters a and b, a local (sum), and a return
// value so we can test condition event-kind assignment: parameter→entry,
// local/return→return. Parameter b serves as the template variable.
//
//go:noinline
func condReturnAndLocal(a int, b int) int {
	sum := a + b
	fmt.Println("sum", sum)
	return sum
}

// condOnly is a target for condition-only probes (no capture expressions, no
// template). This exercises the code path where the condition's root variable
// type must still be added to exploration roots. The tag parameter exists so
// that non-condition-only probes on this function have a second variable.
//
//go:noinline
func condOnly(x int, tag string) {
	fmt.Println(x, tag)
}

// condLine is a target for line-probe conditions. Both n and tag are passed
// through to fmt.Println after the target line so the compiler keeps them
// live at the line probe site.
//
//go:noinline
func condLine(n int, tag string) {
	// Both n and tag are read below the target line, so the compiler must
	// keep them in registers/stack at the probe site.
	result := n * 2 // target for line probe conditions on n
	fmt.Println(result, n, tag)
}

// condSliceArg is a target for unsupported-type condition errors (slice).
//
//go:noinline
func condSliceArg(x []int, tag string) {
	fmt.Println(x, tag)
}

// condMapArg is a target for unsupported-type condition errors (map).
//
//go:noinline
func condMapArg(x map[string]int, tag string) {
	fmt.Println(x, tag)
}

// condStructDirect is a target for unsupported-type condition errors (struct
// compared directly, not a field of it).
//
//go:noinline
func condStructDirect(x condFields, tag string) {
	fmt.Println(x, tag)
}

// --- len/isEmpty test functions ---

//go:noinline
func sink(args ...any) {
	// Prevent the compiler from optimizing away arguments.
	_ = args
}

//go:noinline
func lenString(s string, tag string) {
	sink(s, tag)
	fmt.Println(len(s), tag)
}

//go:noinline
func lenSlice(s []int, tag string) {
	sink(s, tag)
	fmt.Println(len(s), tag)
}

//go:noinline
func lenMap(m map[string]int, tag string) {
	sink(m, tag)
	fmt.Println(len(m), tag)
}

//go:noinline
func lenPtrString(s *string, tag string) {
	sink(s, tag)
	fmt.Println(s, tag)
}

// lenFields is a struct with collection-typed fields for testing len/isEmpty
// on field accesses (getmember + len), including pointer-to-collection fields.
type lenFields struct {
	S        string
	Items    []int
	Dict     map[string]int
	PtrStr   *string
	PtrSlice *[]int

	pad [3]int16 // prevent this struct from being split to registers
}

//go:noinline
func lenStructFields(x lenFields, tag string) {
	sink(x, tag)
	fmt.Println(x.S, x.Items, x.Dict, tag)
}

//go:noinline
func lenPtrStructFields(x *lenFields, tag string) {
	sink(x, tag)
	fmt.Println(x.S, x.Items, x.Dict, tag)
}

// lenErrInt is a target for unsupported len on base type (int).
//
//go:noinline
func lenErrInt(x int, tag string) {
	sink(x, tag)
	fmt.Println(x, tag)
}

// lenErrStruct is a target for unsupported len on a plain struct.
//
//go:noinline
func lenErrStruct(x condFields, tag string) {
	sink(x, tag)
	fmt.Println(x, tag)
}

type aStructNotUsedAsAnArgument struct {
	a int
}

// This test tickles a bug where we didn't explore variable types when we
// but we were adding them. At that point we violated an invariant regarding
// the completion of internals of map types. This test reproduced that bug.
//
//go:noinline
func usesMapsOfMapsThatDoNotAppearAsArguments() map[byte]map[int]aStructNotUsedAsAnArgument {
	// The bug required a map of maps. We make a new type here to ensure
	// that it's not a map type that could possibly exist elsewhere.
	m := map[string]map[int]aStructNotUsedAsAnArgument{
		"a": {0: aStructNotUsedAsAnArgument{a: 1}},
	}
	if m["b"] != nil {
		m["b"][0] = aStructNotUsedAsAnArgument{a: 2}
	}
	return map[byte]map[int]aStructNotUsedAsAnArgument{
		'a': m["a"],
	}
}

// genericIdentity is a generic function called with different shape types
// (int vs string) to exercise dictionary-based type resolution and shared
// throttling across shape instantiations.
//
//go:noinline
func genericIdentity[T any](x T) T {
	fmt.Println(x)
	return x
}
