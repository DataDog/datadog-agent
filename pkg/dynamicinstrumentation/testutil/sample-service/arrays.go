// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

/********************/
/* ARRAY PARAMETERs */
/********************/

//go:noinline
func test_byte_array(x [2]byte) {}

//go:noinline
func test_rune_array(x [2]rune) {}

//go:noinline
func test_string_array(x [2]string) {}

//go:noinline
func test_bool_array(x [2]bool) {}

//go:noinline
func test_int_array(x [2]int) {}

//go:noinline
func test_int8_array(x [2]int8) {}

//go:noinline
func test_int16_array(x [2]int16) {}

//go:noinline
func test_int32_array(x [2]int32) {}

//go:noinline
func test_int64_array(x [2]int64) {}

//go:noinline
func test_uint_array(x [2]uint) {}

//go:noinline
func test_uint8_array(x [2]uint8) {}

//go:noinline
func test_uint16_array(x [2]uint16) {}

//go:noinline
func test_uint32_array(x [2]uint32) {}

//go:noinline
func test_uint64_array(x [2]uint64) {}

//go:noinline
func test_array_of_arrays(a [2][2]int) {}

//go:noinline
func test_array_of_strings(a [2]string) {}

//go:noinline
func test_array_of_arrays_of_arrays(b [2][2][2]int) {}

//go:noinline
func test_array_of_structs(a [2]nestedStruct) {
}

func executeArrayFuncs() {
	test_byte_array([2]byte{1, 1})
	test_rune_array([2]rune{1, 2})
	test_string_array([2]string{"one", "two"})
	test_bool_array([2]bool{true, true})
	test_int_array([2]int{1, 2})
	test_int8_array([2]int8{1, 2})
	test_int16_array([2]int16{1, 2})
	test_int32_array([2]int32{1, 2})
	test_int64_array([2]int64{1, 2})
	test_uint_array([2]uint{1, 2})
	test_uint8_array([2]uint8{1, 2})
	test_uint16_array([2]uint16{1, 2})
	test_uint32_array([2]uint32{1, 2})
	test_uint64_array([2]uint64{1, 2})

	test_array_of_arrays([2][2]int{{1, 2}, {3, 4}})
	test_array_of_strings([2]string{"first", "second"})
	test_array_of_structs([2]nestedStruct{{42, "foo"}, {24, "bar"}})
	test_array_of_arrays_of_arrays([2][2][2]int{{[2]int{1, 2}, [2]int{3, 4}}, {[2]int{5, 6}, [2]int{7, 8}}})
}
