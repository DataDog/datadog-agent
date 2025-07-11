// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "fmt"

//nolint:all
//go:noinline
func doNothing(x []int) {}

//nolint:all
//go:noinline
func sprintSlice(x []int) string {
	return fmt.Sprintf("%v\n", x)
}

//nolint:all
//go:noinline
func expandSlice(x []int) {
	x = append(x, []int{9, 10, 11, 12}...)
	doNothing(x)
}

//nolint:all
//go:noinline
func testUintSlice(u []uint) {}

//nolint:all
//go:noinline
func testEmptySlice(u []uint) {}

//nolint:all
//go:noinline
func testSliceOfSlices(u [][]uint) {}

//nolint:all
//go:noinline
func testStructSlice(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func testEmptySliceOfStructs(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func testNilSliceOfStructs(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func testStringSlice(s []string) {}

//nolint:all
//go:noinline
func testNilSliceWithOtherParams(a int8, s []bool, x uint) {}

//nolint:all
//go:noinline
func testNilSlice(xs []uint16) {}

//nolint:all
//go:noinline
func testVeryLargeSlice(xs []uint) {}

//nolint:all
func executeSliceFuncs() {
	originalSlice := []int{1, 2, 3}
	expandSlice(originalSlice)
	sprintSlice(originalSlice)

	testStringSlice([]string{"abc", "xyz", "123"})
	testUintSlice([]uint{1, 2, 3})
	testStructSlice([]structWithNoStrings{{42, true}, {24, true}}, 3)
	testEmptySliceOfStructs([]structWithNoStrings{}, 2)
	testNilSliceOfStructs(nil, 5)
	testVeryLargeSlice([]uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100})
	testSliceOfSlices([][]uint{
		{4},
		{5, 6},
		{7, 8, 9},
	})
	testEmptySlice([]uint{})

	testNilSliceWithOtherParams(1, nil, 5)
	testNilSlice(nil)
}
