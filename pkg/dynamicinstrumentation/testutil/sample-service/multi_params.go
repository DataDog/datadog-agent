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
func test_combined_byte(w byte, x byte, y float32) {}

//nolint:all
//go:noinline
func test_combined_rune(w byte, x rune, y float32) {}

//nolint:all
//go:noinline
func test_combined_string(w byte, x string, y float32) {}

//nolint:all
//go:noinline
func test_combined_bool(w byte, x bool, y float32) {}

//nolint:all
//go:noinline
func test_combined_int(w byte, x int, y float32) {}

//nolint:all
//go:noinline
func test_combined_int8(w byte, x int8, y float32) {}

//nolint:all
//go:noinline
func test_combined_int16(w byte, x int16, y float32) {}

//nolint:all
//go:noinline
func test_combined_int32(w byte, x int32, y float32) {}

//nolint:all
//go:noinline
func test_combined_int64(w byte, x int64, y float32) {}

//nolint:all
//go:noinline
func test_combined_uint(w byte, x uint, y float32) {}

//nolint:all
//go:noinline
func test_combined_uint8(w byte, x uint8, y float32) {}

//nolint:all
//go:noinline
func test_combined_uint16(w byte, x uint16, y float32) {}

//nolint:all
//go:noinline
func test_combined_uint32(w byte, x uint32, y float32) {}

//nolint:all
//go:noinline
func test_combined_uint64(w byte, x uint64, y float32) {}

//nolint:all
//go:noinline
func test_multiple_simple_params(a bool, b byte, c rune, d uint, e string) {}

//nolint:all
//go:noinline
func test_multiple_composite_params(a [3]string, b aStruct, c []int, d map[string]string, e []nestedStruct) {
}

func executeMultiParamFuncs() {
	test_multiple_simple_params(false, 42, 'z', 1337, "xyz")

	// fails because slices and maps are not supported
	// also crashes because of stack overflow
	// test_multiple_composite_params(
	// 	[3]string{"one", "two", "three"},
	// 	aStruct{},
	// 	[]int{24, 42},
	// 	map[string]string{"foo": "bar"},
	// 	[]nestedStruct{{42, "ftwo"}, {42, "tfour"}},
	// )

	// all of these fail because floats are not supported
	test_combined_byte(2, 3, 3.0)
	test_combined_rune(2, 'b', 3.0)
	test_combined_string(2, "boo", 3.0)
	test_combined_bool(2, true, 3.0)
	test_combined_int(2, 3, 3.0)
	test_combined_int8(2, 38, 3.0)
	test_combined_int16(2, 316, 3.0)
	test_combined_int32(2, 332, 3.0)
	test_combined_int64(2, 364, 3.0)
	test_combined_uint(2, 12, 3.0)
	test_combined_uint8(2, 128, 3.0)
	test_combined_uint16(2, 1216, 3.0)
	test_combined_uint32(2, 1232, 3.0)
	test_combined_uint64(2, 1264, 3.0)
}
