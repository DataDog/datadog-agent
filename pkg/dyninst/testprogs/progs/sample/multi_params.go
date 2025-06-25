// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

/***********************/
/* Multiple Parameters */
/***********************/

//nolint:all
//go:noinline
func testOverLimitParameters(
	a, b, c, d, e, f, g,
	h, i, j, k, l, m, n,
	o, p, q, r, s, t, u byte) {
}

//nolint:all
//go:noinline
func testCombinedByte(w byte, x byte, y float32) {}

//nolint:all
//go:noinline
func testCombinedRune(w byte, x rune, y float32) {}

//nolint:all
//go:noinline
func testCombinedString(w byte, x string, y float32) {}

//nolint:all
//go:noinline
func testCombinedBool(w byte, x bool, y float32) {}

//nolint:all
//go:noinline
func testCombinedInt(w byte, x int, y float32) {}

//nolint:all
//go:noinline
func testCombinedInt8(w byte, x int8, y float32) {}

//nolint:all
//go:noinline
func testCombinedInt16(w byte, x int16, y float32) {}

//nolint:all
//go:noinline
func testCombinedInt32(w byte, x int32, y float32) {}

//nolint:all
//go:noinline
func testCombinedInt64(w byte, x int64, y float32) {}

//nolint:all
//go:noinline
func testCombinedUint(w byte, x uint, y float32) {}

//nolint:all
//go:noinline
func testCombinedUint8(w byte, x uint8, y float32) {}

//nolint:all
//go:noinline
func testCombinedUint16(w byte, x uint16, y float32) {}

//nolint:all
//go:noinline
func testCombinedUint32(w byte, x uint32, y float32) {}

//nolint:all
//go:noinline
func testCombinedUint64(w byte, x uint64, y float32) {}

//nolint:all
//go:noinline
func testMultipleSimpleParams(a bool, b byte, c rune, d uint, e string) {}

//nolint:all
//go:noinline
func testMultipleCompositeParams(a [3]string, b aStruct, c []int, d map[string]string, e []nestedStruct) {
}

//nolint:all
func executeMultiParamFuncs() {
	testMultipleSimpleParams(false, 42, 'z', 1337, "xyz")

	// fails because slices and maps are not supported
	// also crashes because of stack overflow
	// testMultipleCompositeParams(
	// 	[3]string{"one", "two", "three"},
	// 	aStruct{},
	// 	[]int{24, 42},
	// 	map[string]string{"foo": "bar"},
	// 	[]nestedStruct{{42, "ftwo"}, {42, "tfour"}},
	// )

	// all of these fail because floats are not supported
	testCombinedByte(2, 3, 3.0)
	testCombinedRune(2, 'b', 3.0)
	testCombinedString(2, "boo", 3.0)
	testCombinedBool(2, true, 3.0)
	testCombinedInt(2, 3, 3.0)
	testCombinedInt8(2, 38, 3.0)
	testCombinedInt16(2, 316, 3.0)
	testCombinedInt32(2, 332, 3.0)
	testCombinedInt64(2, 364, 3.0)
	testCombinedUint(2, 12, 3.0)
	testCombinedUint8(2, 128, 3.0)
	testCombinedUint16(2, 1216, 3.0)
	testCombinedUint32(2, 1232, 3.0)
	testCombinedUint64(2, 1264, 3.0)
	testOverLimitParameters(1, 2, 3, 4, 5, 6, 7,
		8, 9, 10, 11, 12, 13, 14,
		15, 16, 17, 18, 19, 20, 21)

}
