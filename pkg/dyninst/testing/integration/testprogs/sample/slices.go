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
func executeSliceFuncs() {
	originalSlice := []int{1, 2, 3}
	expandSlice(originalSlice)
	sprintSlice(originalSlice)

	testStringSlice([]string{"abc", "xyz", "123"})
	testUintSlice([]uint{1, 2, 3})
	testStructSlice([]structWithNoStrings{{42, true}, {24, true}}, 3)
	testEmptySliceOfStructs([]structWithNoStrings{}, 2)
	testNilSliceOfStructs([]structWithNoStrings{}, 5)

	testSliceOfSlices([][]uint{
		{4},
		{5, 6},
		{7, 8, 9},
	})
	testEmptySlice([]uint{})

	testNilSliceWithOtherParams(1, nil, 5)
	testNilSlice(nil)
}
