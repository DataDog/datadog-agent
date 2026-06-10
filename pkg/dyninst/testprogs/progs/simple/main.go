// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command simple is a basic go program to be used with dyninst tests.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"unsafe"
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
	// Negative value to exercise signed comparison: BPF cmp_kind_int
	// XORs the sign bit of the most-significant byte before comparing,
	// turning two's-complement compare into unsigned byte compare. A
	// bug in that trick surfaces here as `x < 0` either firing on
	// nothing (treats -5 as 0xfffffffb > 0) or firing on the wrong
	// calls.
	condInt32(-5, "neg")
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
	// Long-LHS / max-length-literal regression coverage. The literal
	// MaxStringLiteralLength (255) imposed by IR-gen used to be
	// indistinguishable from a longer LHS sharing the same first 255
	// bytes, because SM_OP_EXPR_READ_STRING capped the stored length at
	// 255. condString_long_a_then_b is 300 bytes ('a'*255 + 'b'*45),
	// condString_exact_a255 is 'a'*255 — both are compared against the
	// 255-byte literal 'a'*255 in simple.yaml.
	condString(strings.Repeat("a", 255)+strings.Repeat("b", 45), "long_a_then_b")
	condString(strings.Repeat("a", 255), "exact_a255")

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

	// Multi-return with two distinct-typed fields. Called with "ok" (→ r0=2,
	// r1="ok") and "other" (→ r0=5, r1="other") so compound conditions
	// referencing both @return.r0 and @return.r1 across different leaves
	// can be distinguished.
	condMultiReturn("ok")
	condMultiReturn("other")

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

	// condGuarded for split-condition short-circuit tests. Calls cover
	// match, no-match, and nil-pointer paths so probes can verify the
	// guard `(x != nil && x.I32 == 1)` correctly short-circuits without
	// nil-derefing.
	condGuarded(&condFields{I32: 1}, "match")
	condGuarded(&condFields{I32: 99}, "miss")
	condGuarded(nil, "nilptr")

	// `== null` condition targets: each called twice — once with nil (probe
	// matches) and once with non-nil (probe does not match). Covers all four
	// nullable Go types: pointer, slice, map, interface.
	condNullPtr(nil, "match")
	v := 7
	condNullPtr(&v, "miss")
	condNullSlice(nil, "match")
	condNullSlice([]int{1, 2, 3}, "miss")
	condNullMap(nil, "match")
	condNullMap(map[string]int{"k": 1}, "miss")
	condNullIface(nil, "match")
	condNullIface(errors.New("boom"), "miss")
	condNullUnsafePtr(nil, "match")
	u := 42
	condNullUnsafePtr(unsafe.Pointer(&u), "miss")

	// contains(map, key) condition targets. Called three times:
	// "present" — key is in the map and contains(...) is true.
	// "absent"  — key is not in the map; contains(...) is false.
	// "nil"     — map is nil; contains(...) is false.
	condContainsMap(
		map[string]int{"existing_key": 1},
		map[int]int{42: 1},
		"present",
	)
	condContainsMap(
		map[string]int{"other": 2},
		map[int]int{7: 1},
		"absent",
	)
	condContainsMap(nil, nil, "nil")

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

	// Big array tests: verify index expressions only read the single element,
	// not the entire array. These arrays are ~1MiB so copying them into the
	// scratch buffer would fail.
	var bigArr [131072]int64
	bigArr[0] = 0xdeadbeef
	bigArr[131071] = 0xcafebabe
	bigArrayArg(bigArr)
	bigArrayStructArg(&bigArrayStruct{tag: 42, data: bigArr})

	// Nil pointer + index: called twice — once with a valid pointer, once nil.
	// The non-nil call should produce a normal value; the nil call should
	// trigger a nil-deref evaluation error but still emit an event.
	indexNilPtrSlice(&lenFields{Items: []int{10, 20, 30}}, "match")
	indexNilPtrSlice(nil, "nilptr")

	// Index-then-getmember: index into array/slice of structs, then access field.
	structSliceArg([]indexMemberStruct{
		{Val: 100, Txt: "first"},
		{Val: 200, Txt: "second"},
	}, "idx-member")
	structArrayArg([2]indexMemberStruct{
		{Val: 300, Txt: "third"},
		{Val: 400, Txt: "fourth"},
	}, "idx-member")

	// Index-then-deref-then-getmember: index into array/slice of pointers to
	// structs, dereference the pointer, then access field.
	ptrStructSliceArg([]*indexMemberStruct{
		{Val: 500, Txt: "fifth"},
		{Val: 600, Txt: "sixth"},
	}, "idx-ptr-member")
	ptrStructArrayArg([2]*indexMemberStruct{
		{Val: 700, Txt: "seventh"},
		{Val: 800, Txt: "eighth"},
	}, "idx-ptr-member")

	// Deep dereference chains: struct field is pointer-to-array-of-structs or
	// pointer-to-array-of-pointers-to-structs.
	elem1 := indexMemberStruct{Val: 900, Txt: "ninth"}
	elem2 := indexMemberStruct{Val: 1000, Txt: "tenth"}
	indexMemberWrapperArg(&indexMemberWrapper{
		Arr:    &[2]indexMemberStruct{elem1, elem2},
		PtrArr: &[2]*indexMemberStruct{&elem1, &elem2},
	}, "wrapper")

	// Map index expression tests: small maps (≤8 entries, no directory),
	// large maps (>8 entries, uses directory + probing), and key-not-found.
	mapIndexIntKey(map[int]string{1: "one", 2: "two", 3: "three"})
	mapIndexStringKey(map[string]int{"alpha": 100, "beta": 200, "gamma": 300})
	largeMap := make(map[string]int)
	for i := 0; i < 10000; i++ {
		largeMap[fmt.Sprintf("key%d", i)] = i * 10
	}
	mapIndexLargeMap(largeMap)
	mapIndexMissing(map[string]int{"a": 1, "b": 2})

	// Map index tests for different key length tiers (AES hash lanes).
	// Each tier uses a large map (>8 entries) to exercise the full
	// hash → table → group → slot lookup path.
	mapIndexKeyLen8(makeLargeMapWithKeyLen(8, 1000))     // 1-16 byte tier
	mapIndexKeyLen24(makeLargeMapWithKeyLen(24, 1000))   // 17-32 byte tier
	mapIndexKeyLen48(makeLargeMapWithKeyLen(48, 1000))   // 33-64 byte tier
	mapIndexKeyLen96(makeLargeMapWithKeyLen(96, 1000))   // 65-128 byte tier
	mapIndexKeyLen200(makeLargeMapWithKeyLen(200, 1000)) // 129+ byte tier

	// Map index with struct/pointer values and field access.
	mapIndexStructVal(map[string]indexMemberStruct{
		"a": {Val: 111, Txt: "aaa"},
		"b": {Val: 222, Txt: "bbb"},
	})
	mapIndexPtrStructVal(map[string]*indexMemberStruct{
		"x": {Val: 333, Txt: "xxx"},
		"y": {Val: 444, Txt: "yyy"},
	})
	mapIndexEmptyKey(map[string]int{"": 999, "notempty": 0})

	// Boundary key-length tests: exercise both sides of the <=16 AES hash tier.
	mapIndexKeyLen16(makeLargeMapWithKeyLen(16, 1000)) // last key in 1-16 byte tier
	mapIndexKeyLen17(makeLargeMapWithKeyLen(17, 1000)) // first key in 17-32 byte tier

	// Bool key test.
	mapIndexBoolKey(map[bool]int{true: 42, false: 99})

	// Zero-value key: key exists but maps to the zero value.
	mapIndexZeroValue(map[string]int{"zero": 0, "one": 1})

	// Nil pointer value: key exists but value is nil pointer.
	mapIndexNilPtrVal(map[string]*indexMemberStruct{
		"nil_key": nil,
		"ok":      {Val: 1, Txt: "ok"},
	})

	// Large value struct: field past byte offset 255, exercises uint16 ValInSlotOffset.
	mapIndexLargeValue(map[string]largeValueStruct{
		"k": {Val: 12345},
	})

	// Generic function called with two different shape instantiations.
	// int and string have different GC shapes, so the compiler emits two
	// distinct shape functions (go.shape.int, go.shape.string). A single
	// probe targeting genericIdentity[...] will match both, exercising
	// shared throttling across shapes and runtime dictionary resolution.
	genericIdentity(42)
	genericIdentity("hello")

	// Method value: taking a method value of an inlined method creates a
	// trampoline (-fm) function. The inlined method only exists as an
	// inlined subroutine inside the trampoline. We must still be able to
	// probe it.
	mv := (&methodValueReceiver{val: 42}).inlinedMethod
	methodValueSink(mv)

	// condReturnPtr for split-condition return-side nil-deref tests:
	// returns a pointer that is nil on the "nilret" tag. A return-side
	// leaf like @return.I32 == 1 nil-derefs on the nil-returning call.
	condReturnPtr("match")
	condReturnPtr("nilret")

	// any / all targets. Each tag corresponds to an assertion in the
	// integration test snapshots. Placed at the end of main so adding /
	// removing them doesn't shift line numbers of probe targets earlier in
	// the file (which existing line-probes reference).
	//
	// Slices of base types.
	anyAllIntSlice([]int{1, 2, 42, 99}, "match")
	anyAllIntSlice([]int{1, 2, 3}, "anymiss_allmatch")
	anyAllIntSlice([]int{1, -1, 2}, "allmiss")
	anyAllIntSlice(nil, "empty")
	// 4096 elements exactly at the iteration cap, one match in the middle.
	anyAllIntSlice(makeIntSliceWithMatch(4096, 100, 42), "cap")
	// 4097 elements (one over the cap), all positive, match at index 5:
	// `any` short-circuits at iteration 6; `all` (no false element) hits
	// the cap and trips CollectionTooLarge.
	anyAllIntSlice(makeIntSliceWithMatch(4097, 5, 42), "capplus")
	// 5000 elements all positive (no match): both `any(==42)` and
	// `all(>0)` walk to the cap and trip CollectionTooLarge.
	anyAllIntSlice(makeIntSliceFilled(5000, 1), "toolarge")
	// 5000 elements, match at index 10: `any` short-circuits well before
	// the cap.
	anyAllIntSlice(makeIntSliceWithMatch(5000, 10, 42), "scany")
	// 5000 elements, single negative at index 10: `all(>0)` short-
	// circuits before the cap.
	anyAllIntSlice(makeIntSliceWithMatch(5000, 10, -1), "scall")

	// Predicate bodies over @it.field for slice-of-struct.
	anyAllStructSlice([]condFields{
		{I32: 100, S: "a"},
		{I32: 300, S: "b"},
	}, "match")
	anyAllStructSlice([]condFields{
		{I32: 1, S: "a"},
		{I32: 2, S: "b"},
	}, "miss")
	anyAllStructSlice(nil, "empty")

	// Slice of pointers — guarded vs unguarded predicate body. The
	// guarded probes must produce clean snapshots; the unguarded one
	// must surface condition_eval_error because dereferencing a nil
	// element faults.
	anyAllPtrSlice([]*condFields{nil, {I32: 1, S: "x"}, nil, {I32: 2, S: "y"}}, "guarded_match")
	anyAllPtrSlice([]*condFields{nil, {I32: 1, S: "y"}, nil}, "guarded_miss")
	anyAllPtrSlice(nil, "guarded_empty")
	anyAllPtrSlice([]*condFields{nil, {I32: 1, S: "y"}}, "unguarded_nil")
	anyAllPtrSlice([]*condFields{{I32: 1, S: "x"}, {I32: 2, S: "y"}}, "unguarded_clean")

	// Arrays.
	anyAllIntArray([5]int{1, 2, 42, 4, 5}, "match")
	anyAllIntArray([5]int{1, 2, 3, 4, 5}, "miss")

	// Maps.
	anyAllIntMap(map[string]int{"a": 1, "b": 42, "c": 3}, "match")
	anyAllIntMap(map[string]int{"a": 1, "b": 2, "c": 3}, "miss")
	anyAllIntMap(nil, "empty")
	// 4097 entries with all values == 1 and no match for ==42. The any
	// loop walks every slot and trips CollectionTooLarge; all(>0)
	// short-circuits never and also trips the cap.
	anyAllIntMapMassive(makeIntMapFilled(4097, 1, "", 0), "toolarge")
	// 100 entries (comfortably under the cap), with one match keyed
	// "match"=42 and one miss keyed "miss"=-1. any(==42) and all(>0)
	// both terminate well before the cap regardless of iteration order
	// — verifying short-circuit semantics over a map without depending
	// on cross-toolchain map layout near the cap boundary.
	{
		m := makeIntMapFilled(100, 1, "match", 42)
		m["miss"] = -1
		anyAllIntMap(m, "sc100")
	}

	// Map of string -> *condFields. Guarded body uses @value != null
	// before reading @value.I32; unguarded faults on the nil entry.
	anyAllPtrMap(map[string]*condFields{
		"a": nil,
		"b": {I32: 200, S: "match"},
	}, "guarded_match")
	anyAllPtrMap(map[string]*condFields{
		"a": nil,
		"b": {I32: 5, S: "miss"},
	}, "guarded_miss")
	anyAllPtrMap(nil, "guarded_empty")

	// Slice of *string. Verifies that null-comparison on a pointer
	// (@it != null) preserves the pointer, while a value comparison
	// (@it == "x") auto-derefs through the *string to compare the
	// underlying string bytes.
	{
		x := "x"
		y := "y"
		anyAllPtrStrSlice([]*string{nil, &x, nil, &y}, "guarded_match")
		anyAllPtrStrSlice([]*string{nil, &y}, "guarded_miss")
		anyAllPtrStrSlice([]*string{nil, &x}, "unguarded_nil")
		anyAllPtrStrSlice([]*string{&x, &y}, "unguarded_clean")
	}

	// Slice of *int. Same pattern, base type.
	{
		one := 1
		two := 2
		anyAllPtrIntSlice([]*int{nil, &one, nil, &two}, "guarded_match")
		anyAllPtrIntSlice([]*int{nil, &two}, "guarded_miss")
		anyAllPtrIntSlice([]*int{nil, &one}, "unguarded_nil")
		anyAllPtrIntSlice([]*int{&one, &two}, "unguarded_clean")
	}

	// Slice of structs whose element size exceeds the per-iteration
	// scratch budget. Probes over this target should be rejected at irgen
	// time with a typed Issue; the call here exists only to keep the
	// function in DWARF so the probe can resolve the method.
	anyAllOversizedSlice([]oversizedElem{{I32: 1, S: "a"}}, "match")

	// Map keyed by a struct (condFields). Lookup by struct key isn't
	// supported, but iteration via any/all should work — @key (the bare
	// key) is the struct value, @key.I32 accesses a field on it.
	anyAllStructKeyMap(map[condFields]int{
		{I32: 1, S: "a"}: 10,
		{I32: 2, S: "b"}: 20,
	}, "match")
	anyAllStructKeyMap(map[condFields]int{
		{I32: 1, S: "a"}: 10,
	}, "miss")
	anyAllStructKeyMap(nil, "empty")

	// Map keyed by a struct pointer. @key is a pointer; @key.I32 derefs.
	{
		a := condFields{I32: 1, S: "a"}
		b := condFields{I32: 2, S: "b"}
		anyAllPtrStructKeyMap(map[*condFields]int{
			&a: 10,
			&b: 20,
		}, "match")
		anyAllPtrStructKeyMap(map[*condFields]int{
			&a: 10,
		}, "miss")
		anyAllPtrStructKeyMap(nil, "empty")
	}

	// Map whose value type is large enough that Go stores it out-of-line
	// (the slot's `elem` field becomes *bigStruct). The body accesses
	// @value.Field1 — auto-deref should make this transparent to the user.
	anyAllBigValMap(map[string]bigStruct{
		"a": {Field1: 1},
		"b": {Field1: 42},
	}, "match")
	anyAllBigValMap(map[string]bigStruct{
		"a": {Field1: 1},
	}, "miss")
	anyAllBigValMap(nil, "empty")

	// Same, but the key is the large struct.
	anyAllBigKeyMap(map[bigStruct]int{
		{Field1: 1}:  10,
		{Field1: 42}: 20,
	}, "match")
	anyAllBigKeyMap(map[bigStruct]int{
		{Field1: 1}: 10,
	}, "miss")
	anyAllBigKeyMap(nil, "empty")

	// Slices and arrays of strings (used by both any/all and contains
	// probes).
	anyAllStringSlice([]string{"alpha", "match", "gamma"}, "match")
	anyAllStringSlice([]string{"alpha", "beta", "gamma"}, "miss")
	anyAllStringSlice(nil, "empty")
	anyAllStringArray([3]string{"alpha", "match", "gamma"}, "match")
	anyAllStringArray([3]string{"alpha", "beta", "gamma"}, "miss")

	// Negative targets for contains: slice element types that are not
	// comparable base types (slice-of-slice, slice-of-map). The probe must
	// be rejected at irgen.
	anyAllIntSliceOfSlice([][]int{{1, 2}, {3, 4}}, "match")
	anyAllIntSliceOfMap([]map[string]int{{"a": 1}, {"b": 2}}, "match")
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

// condGuarded mixes a pointer arg (may be nil) with a return value so a
// split-event-kind condition can guard a potentially-nil-derefing entry
// leaf with another entry leaf, e.g. (x != nil && x.I32 == 1) and pair
// it with a return-side leaf like @return == 0. Used by tests that
// verify the guard short-circuits correctly when x is nil.
//
//go:noinline
func condGuarded(x *condFields, tag string) int {
	if x == nil {
		fmt.Println("condGuarded nil", tag)
		return 0
	}
	fmt.Println("condGuarded", x.I32, tag)
	return int(x.I32)
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

// condNullPtr is a target for `p == null` conditions on pointer values.
//
//go:noinline
func condNullPtr(p *int, tag string) {
	sink(p, tag)
	fmt.Println(p, tag)
}

// condNullSlice is a target for `s == null` conditions on slice values.
//
//go:noinline
func condNullSlice(s []int, tag string) {
	sink(s, tag)
	fmt.Println(s, tag)
}

// condNullMap is a target for `m == null` conditions on map values.
//
//go:noinline
func condNullMap(m map[string]int, tag string) {
	sink(m, tag)
	fmt.Println(m, tag)
}

// condNullIface is a target for `i == null` conditions on interface values.
//
//go:noinline
func condNullIface(i error, tag string) {
	sink(i, tag)
	fmt.Println(i, tag)
}

// condNullUnsafePtr is a target for `p == null` conditions on unsafe.Pointer.
//
//go:noinline
func condNullUnsafePtr(p unsafe.Pointer, tag string) {
	sink(p, tag)
	fmt.Println(p, tag)
}

// condContainsMap is a target for contains(m, key) conditions. Takes both a
// string-keyed and int-keyed map so a single test function can exercise both
// key-type flavors.
//
//go:noinline
func condContainsMap(m map[string]int, mi map[int]int, tag string) {
	sink(m, mi, tag)
	fmt.Println(m, mi, tag)
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

// indexNilPtrSlice is a target for testing index expressions when the
// pointer-to-struct is nil. When x is nil, x.Items[0] causes a nil-pointer
// dereference in the eBPF expression evaluation chain. The expression should
// fail gracefully with an evaluation error.
//
//go:noinline
func indexNilPtrSlice(x *lenFields, tag string) {
	fmt.Println("indexNilPtrSlice", x, tag)
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

// bigArrayStruct wraps a large array behind a pointer for testing index
// expressions that traverse pointer->struct->array.
type bigArrayStruct struct {
	tag  int
	data [131072]int64
}

// bigArrayArg takes a large array by value. Index expressions must only
// read the single element, not the entire ~1MiB array.
//
//go:noinline
func bigArrayArg(s [131072]int64) {
	fmt.Println(s[0], s[131071])
}

// bigArrayStructArg takes a pointer to a struct containing a large array.
//
//go:noinline
func bigArrayStructArg(s *bigArrayStruct) {
	fmt.Println(s.data[0], s.tag)
}

// indexMemberStruct is a small struct for testing index-then-getmember
// expressions (e.g., slice[0].Val). The pad field prevents register splitting.
type indexMemberStruct struct {
	Val int32
	Txt string
	pad [3]int16
}

// largeValueStruct has a field past byte offset 255, exercising uint16
// ValInSlotOffset when used as a map value type.
type largeValueStruct struct {
	Pad [256]byte
	Val int // offset 256
}

// indexMemberWrapper holds pointer-to-array fields for testing deep
// dereference chains: ptr → array → struct and ptr → array → ptr → struct.
type indexMemberWrapper struct {
	Arr    *[2]indexMemberStruct
	PtrArr *[2]*indexMemberStruct
	pad    [3]int16
}

// structSliceArg takes a slice of structs for testing index-then-getmember.
//
//go:noinline
func structSliceArg(s []indexMemberStruct, tag string) {
	fmt.Println(s[0].Val, tag)
}

// structArrayArg takes an array of structs for testing index-then-getmember.
//
//go:noinline
func structArrayArg(s [2]indexMemberStruct, tag string) {
	fmt.Println(s[0].Val, tag)
}

// ptrStructSliceArg takes a slice of pointers to structs for testing
// index-then-deref-then-getmember.
//
//go:noinline
func ptrStructSliceArg(s []*indexMemberStruct, tag string) {
	fmt.Println(s[0].Val, tag)
}

// ptrStructArrayArg takes an array of pointers to structs for testing
// index-then-deref-then-getmember.
//
//go:noinline
func ptrStructArrayArg(s [2]*indexMemberStruct, tag string) {
	fmt.Println(s[0].Val, tag)
}

// indexMemberWrapperArg takes a pointer to a wrapper struct with
// pointer-to-array fields for testing deep dereference + index + getmember.
//
//go:noinline
func indexMemberWrapperArg(s *indexMemberWrapper, tag string) {
	fmt.Println(s.Arr[0].Val, s.PtrArr[0].Val, tag)
}

// mapIndexIntKey takes a small map with int keys for map index tests.
//
//go:noinline
func mapIndexIntKey(m map[int]string) {
	fmt.Println(m)
}

// mapIndexStringKey takes a small map with string keys for map index tests.
//
//go:noinline
func mapIndexStringKey(m map[string]int) {
	fmt.Println(m)
}

// mapIndexLargeMap takes a map with >8 entries, forcing the runtime to use
// directory + table indirection (dirLen > 0).
//
//go:noinline
func mapIndexLargeMap(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexMissing is used to test key-not-found cases.
//
//go:noinline
func mapIndexMissing(m map[string]int) {
	fmt.Println(m)
}

// makeLargeMapWithKeyLen creates a map with n entries where keys are
// zero-padded strings of exactly keyLen bytes. Key "0" is padded to keyLen.
// makeKey generates a string of exactly length bytes for map key i.
// The key is a zero-padded hex representation, e.g. makeKey(8, 42) = "0000002a".
func makeKey(length int, i int) string {
	paddedDigits := min(length, 10)
	extraPad := max(length-paddedDigits, 0)
	pad := strings.Repeat("0", extraPad)
	format := fmt.Sprintf("%%0%dx", paddedDigits)
	return pad + fmt.Sprintf(format, i)
}

func makeLargeMapWithKeyLen(keyLen, n int) map[string]int {
	m := make(map[string]int, n)
	for i := range n {
		m[makeKey(keyLen, i)] = i * 7
	}
	return m
}

// mapIndexKeyLen8 tests 1-16 byte key tier (8-byte keys).
//
//go:noinline
func mapIndexKeyLen8(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexKeyLen24 tests 17-32 byte key tier (24-byte keys).
//
//go:noinline
func mapIndexKeyLen24(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexKeyLen48 tests 33-64 byte key tier (48-byte keys).
//
//go:noinline
func mapIndexKeyLen48(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexKeyLen96 tests 65-128 byte key tier (96-byte keys).
//
//go:noinline
func mapIndexKeyLen96(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexKeyLen200 tests 129+ byte key tier (200-byte keys).
//
//go:noinline
func mapIndexKeyLen200(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexStructVal takes a map with struct values for testing
// map-index-then-getmember (e.g., m["key"].Val).
//
//go:noinline
func mapIndexStructVal(m map[string]indexMemberStruct) {
	fmt.Println(m["a"].Val)
}

// mapIndexPtrStructVal takes a map with pointer-to-struct values for testing
// map-index-then-deref-then-getmember (e.g., m["key"].Val).
//
//go:noinline
func mapIndexPtrStructVal(m map[string]*indexMemberStruct) {
	fmt.Println(m["x"].Val)
}

// mapIndexEmptyKey takes a map where the empty string is a valid key.
//
//go:noinline
func mapIndexEmptyKey(m map[string]int) {
	fmt.Println(m[""])
}

// mapIndexKeyLen16 tests the boundary of the 1-16 byte AES hash tier.
//
//go:noinline
func mapIndexKeyLen16(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexKeyLen17 tests the first key in the 17-32 byte AES hash tier.
//
//go:noinline
func mapIndexKeyLen17(m map[string]int) {
	fmt.Println(len(m))
}

// mapIndexBoolKey tests map index with bool keys.
//
//go:noinline
func mapIndexBoolKey(m map[bool]int) {
	fmt.Println(m)
}

// mapIndexZeroValue tests a key that exists but maps to the zero value.
//
//go:noinline
func mapIndexZeroValue(m map[string]int) {
	fmt.Println(m)
}

// mapIndexNilPtrVal tests a key that maps to a nil pointer. Accessing a
// field through the nil pointer should produce ExprStatusNilDeref.
//
//go:noinline
func mapIndexNilPtrVal(m map[string]*indexMemberStruct) {
	fmt.Println(m)
}

// mapIndexLargeValue tests a map with a large value struct where the target
// field sits past byte offset 255, exercising uint16 ValInSlotOffset.
//
//go:noinline
func mapIndexLargeValue(m map[string]largeValueStruct) {
	fmt.Println(m["k"].Val)
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

// methodValueReceiver is used to test probing inlined methods that are only
// reachable through a method value trampoline (-fm wrapper). When Go creates
// a method value (e.g. obj.Method), the compiler generates a trampoline
// function. If the method is small enough to be inlined, the only concrete
// instance lives inside the trampoline.
type methodValueReceiver struct {
	val int
}

// inlinedMethod is intentionally NOT marked //go:noinline so it will be
// inlined into the trampoline wrapper.
func (m *methodValueReceiver) inlinedMethod() int {
	return m.val
}

//go:noinline
func methodValueSink(f func() int) {
	fmt.Println(f())
}

// condMultiReturn has two distinct-typed named returns so compound
// conditions can reference both @return.r0 (int) and @return.r1 (string)
// from different leaves.
//
//go:noinline
func condMultiReturn(tag string) (r0 int, r1 string) {
	r0 = len(tag)
	r1 = tag
	fmt.Println("condMultiReturn", r0, r1)
	return r0, r1
}

// condReturnPtr returns a *condFields that is nil on the "nilret" tag.
// Used by split-condition probes that put the nil-deref leaf on the
// return side, exercising the abort-path arming of condition_eval_error
// when no ConditionBeginOp ran (return-side AST replay inlines its
// return leaves).
//
//go:noinline
func condReturnPtr(tag string) *condFields {
	fmt.Println("condReturnPtr", tag)
	if tag == "nilret" {
		return nil
	}
	return &condFields{I32: 1}
}

// makeIntSliceFilled returns a slice of length n with every element set
// to fill. Helper for any/all integration probes — kept out of main() so
// the surrounding DWARF location lists stay consistent across toolchains.
//
//go:noinline
func makeIntSliceFilled(n int, fill int) []int {
	xs := make([]int, n)
	for i := range xs {
		xs[i] = fill
	}
	return xs
}

// makeIntSliceWithMatch returns a slice of length n filled with 1s except
// for index matchIdx, which is set to matchVal.
//
//go:noinline
func makeIntSliceWithMatch(n, matchIdx, matchVal int) []int {
	xs := makeIntSliceFilled(n, 1)
	xs[matchIdx] = matchVal
	return xs
}

// makeIntMapFilled returns a map of length n with keys "k_0000".."k_NNNN"
// all valued fill. If specialKey is non-empty, that key is added with
// specialVal (overwriting any same-name k_NNNN entry).
//
//go:noinline
func makeIntMapFilled(n, fill int, specialKey string, specialVal int) map[string]int {
	m := make(map[string]int, n)
	for i := range n {
		m[fmt.Sprintf("k_%04d", i)] = fill
	}
	if specialKey != "" {
		m[specialKey] = specialVal
	}
	return m
}

// anyAllIntSlice is the target for `any` / `all` slice-predicate probes
// over a slice of base-typed elements.
//
//go:noinline
func anyAllIntSlice(xs []int, tag string) {
	fmt.Println("anyAllIntSlice", xs, tag)
}

// anyAllStructSlice is the target for `any` / `all` over a slice of
// structs, exercising `@it.field` predicate bodies.
//
//go:noinline
func anyAllStructSlice(xs []condFields, tag string) {
	fmt.Println("anyAllStructSlice", len(xs), tag)
}

// anyAllPtrSlice is the target for `any` / `all` over a slice of struct
// pointers. Exercises guarded (@it != null && @it.S == "x") and unguarded
// (@it.S == "x") predicate bodies against nil-containing inputs.
//
//go:noinline
func anyAllPtrSlice(xs []*condFields, tag string) {
	fmt.Println("anyAllPtrSlice", len(xs), tag)
}

// anyAllIntArray is the target for `any` / `all` over a Go array (fixed
// compile-time length).
//
//go:noinline
func anyAllIntArray(xs [5]int, tag string) {
	fmt.Println("anyAllIntArray", xs, tag)
}

// anyAllIntMap is the target for `any` / `all` over a Go map.
// Exercises the swiss-map iteration walk.
//
//go:noinline
func anyAllIntMap(m map[string]int, tag string) {
	fmt.Println("anyAllIntMap", len(m), tag)
}

// anyAllIntMapMassive carries the same payload as anyAllIntMap but lives
// in its own function so probes that need to fire on a too-large map can
// target it independently. The parameter is named `redactMyEntries` so
// the test JSON redactor (defaultRedactors in json_redaction_test.go)
// replaces the captured entries with a placeholder — necessary because
// chased_slices truncation makes the captured key set non-deterministic
// across Go toolchains for maps with > MAX_CHASED_SLICES entries.
//
//go:noinline
func anyAllIntMapMassive(redactMyEntries map[string]int, tag string) {
	fmt.Println("anyAllIntMapMassive", len(redactMyEntries), tag)
}

// anyAllPtrMap is the target for `any` / `all` over a map[string]*condFields,
// exercising short-circuit through the @value pointer access.
//
//go:noinline
func anyAllPtrMap(m map[string]*condFields, tag string) {
	fmt.Println("anyAllPtrMap", len(m), tag)
}

// anyAllPtrStrSlice is the target for any/all over a slice of *string.
// Exercises (a) null-comparison preserving the pointer and (b) value
// comparison auto-derefing the pointer to read the underlying string.
//
//go:noinline
func anyAllPtrStrSlice(xs []*string, tag string) {
	fmt.Println("anyAllPtrStrSlice", len(xs), tag)
}

// anyAllPtrIntSlice is the target for any/all over a slice of *int.
// Same pattern as anyAllPtrStrSlice but with a base-type pointee.
//
//go:noinline
func anyAllPtrIntSlice(xs []*int, tag string) {
	fmt.Println("anyAllPtrIntSlice", len(xs), tag)
}

// oversizedElem is a struct deliberately larger than
// ir.CollectionPredicateMaxElemBytes (256). Used to verify that irgen
// rejects any/all over a slice/array with elem_size > the per-iteration
// scratch budget.
type oversizedElem struct {
	I32 int32
	S   string
	pad [300]byte
}

// anyAllOversizedSlice is the target for any/all over a slice of structs
// whose size exceeds the per-iteration scratch budget. The probe should
// fail to load with a typed Issue.
//
//go:noinline
func anyAllOversizedSlice(xs []oversizedElem, tag string) {
	fmt.Println("anyAllOversizedSlice", len(xs), tag)
}

// anyAllStructKeyMap is the target for any/all over a map with a struct
// key. Iteration is supported even though m[k] lookup with a struct key
// is not — the loop walks every slot and exposes @key (the bare key
// reference) as the struct value, with @key.field accessing fields on it.
//
//go:noinline
func anyAllStructKeyMap(m map[condFields]int, tag string) {
	fmt.Println("anyAllStructKeyMap", len(m), tag)
}

// anyAllPtrStructKeyMap is the target for any/all over a map with a
// struct-pointer key. @key is a pointer; @key.field auto-derefs it.
//
//go:noinline
func anyAllPtrStructKeyMap(m map[*condFields]int, tag string) {
	fmt.Println("anyAllPtrStructKeyMap", len(m), tag)
}

// anyAllBigValMap exercises any/all over a map whose values exceed Go's
// in-slot size threshold (~128 bytes), forcing the runtime to store
// values out-of-line as pointers in the slot. DWARF's slot type reflects
// this rewrite: the slot's `elem` field is *bigStruct, which means
// iteration reads an 8-byte pointer per slot and the body auto-derefs
// to access fields.
//
//go:noinline
func anyAllBigValMap(m map[string]bigStruct, tag string) {
	fmt.Println("anyAllBigValMap", len(m), tag)
}

// anyAllBigKeyMap is the same but with the *key* being the large struct.
// The slot's `key` field becomes *bigStruct.
//
//go:noinline
func anyAllBigKeyMap(m map[bigStruct]int, tag string) {
	fmt.Println("anyAllBigKeyMap", len(m), tag)
}

// anyAllStringSlice is the target for any/all (and contains) over a slice
// of strings.
//
//go:noinline
func anyAllStringSlice(xs []string, tag string) {
	fmt.Println("anyAllStringSlice", len(xs), tag)
}

// anyAllStringArray is the target for any/all (and contains) over a Go
// array of strings.
//
//go:noinline
func anyAllStringArray(xs [3]string, tag string) {
	fmt.Println("anyAllStringArray", xs, tag)
}

// anyAllIntSliceOfSlice exists so we can attach `contains` probes whose
// element type is `[]int` and verify the resulting Issue surfaces at irgen
// (slice-of-slice elements are not a comparable base type).
//
//go:noinline
func anyAllIntSliceOfSlice(xs [][]int, tag string) {
	fmt.Println("anyAllIntSliceOfSlice", len(xs), tag)
}

// anyAllIntSliceOfMap is the same negative-target shape, for slice elements
// of map type.
//
//go:noinline
func anyAllIntSliceOfMap(xs []map[string]int, tag string) {
	fmt.Println("anyAllIntSliceOfMap", len(xs), tag)
}
