// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

/********************/
/* ARRAY PARAMETERs */
/********************/

//nolint:all
//go:noinline
func testByteArray(x [2]byte) {}

//nolint:all
//go:noinline
func testRuneArray(x [2]rune) {}

//nolint:all
//go:noinline
func testStringArray(x [2]string) {}

//nolint:all
//go:noinline
func testBoolArray(x [2]bool) {}

//nolint:all
//go:noinline
func testIntArray(x [2]int) {}

//nolint:all
//go:noinline
func testInt8Array(x [2]int8) {}

//nolint:all
//go:noinline
func testInt16Array(x [2]int16) {}

//nolint:all
//go:noinline
func testInt32Array(x [2]int32) {}

//nolint:all
//go:noinline
func testInt64Array(x [2]int64) {}

//nolint:all
//go:noinline
func testUintArray(x [2]uint) {}

//nolint:all
//go:noinline
func testUint8Array(x [2]uint8) {}

//nolint:all
//go:noinline
func testUint16Array(x [2]uint16) {}

//nolint:all
//go:noinline
func testUint32Array(x [2]uint32) {}

//nolint:all
//go:noinline
func testUint64Array(x [2]uint64) {}

//nolint:all
//go:noinline
func testArrayOfArrays(a [2][2]int) {}

//nolint:all
//go:noinline
func testArrayOfStrings(a [2]string) {}

//nolint:all
//go:noinline
func testArrayOfArraysOfArrays(b [2][2][2]int) {}

//nolint:all
//go:noinline
func testArrayOfStructs(a [2]nestedStruct) {
}

//nolint:all
//go:noinline
func testOverLimitArrayParameters(
	a, b, c, d, e, f, g,
	h, i, j, k, l, m, n,
	o, p, q, r, s, t, u [3]uint32) {
}

//nolint:all
func executeArrayFuncs() {
	testByteArray([2]byte{1, 1})
	testRuneArray([2]rune{1, 2})
	testStringArray([2]string{"one", "two"})
	testBoolArray([2]bool{true, true})
	testIntArray([2]int{1, 2})
	testInt8Array([2]int8{1, 2})
	testInt16Array([2]int16{1, 2})
	testInt32Array([2]int32{1, 2})
	testInt64Array([2]int64{1, 2})
	testUintArray([2]uint{1, 2})
	testUint8Array([2]uint8{1, 2})
	testUint16Array([2]uint16{1, 2})
	testUint32Array([2]uint32{1, 2})
	testUint64Array([2]uint64{1, 2})

	testArrayOfArrays([2][2]int{{1, 2}, {3, 4}})
	testArrayOfStrings([2]string{"first", "second"})
	testArrayOfStructs([2]nestedStruct{{42, "foo"}, {24, "bar"}})
	testArrayOfArraysOfArrays([2][2][2]int{{[2]int{1, 2}, [2]int{3, 4}}, {[2]int{5, 6}, [2]int{7, 8}}})

	testOverLimitArrayParameters([3]uint32{1, 2, 1}, [3]uint32{1, 2, 2}, [3]uint32{1, 2, 3}, [3]uint32{1, 2, 4}, [3]uint32{1, 2, 5}, [3]uint32{1, 2, 6}, [3]uint32{1, 2, 7},
		[3]uint32{1, 2, 8}, [3]uint32{1, 2, 9}, [3]uint32{1, 2, 10}, [3]uint32{1, 2, 11}, [3]uint32{1, 2, 12}, [3]uint32{1, 2, 13}, [3]uint32{1, 2, 14},
		[3]uint32{1, 2, 15}, [3]uint32{1, 2, 16}, [3]uint32{1, 2, 17}, [3]uint32{1, 2, 18}, [3]uint32{1, 2, 19}, [3]uint32{1, 2, 20}, [3]uint32{1, 2, 21})
}
