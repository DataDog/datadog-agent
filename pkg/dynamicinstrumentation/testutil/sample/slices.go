// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

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
func test_uint_slice(u []uint) {}

//nolint:all
//go:noinline
func test_empty_slice(u []uint) {}

//nolint:all
//go:noinline
func test_slice_of_slices(u [][]uint) {}

//nolint:all
//go:noinline
func test_struct_slice(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func test_empty_slice_of_structs(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func test_nil_slice_of_structs(xs []structWithNoStrings, a int) {}

//nolint:all
//go:noinline
func test_string_slice(s []string) {}

//nolint:all
//go:noinline
func test_nil_slice(a int8, s []bool, x uint) {}

//nolint:all
func ExecuteSliceFuncs() {
	originalSlice := []int{1, 2, 3}
	expandSlice(originalSlice)
	sprintSlice(originalSlice)

	test_string_slice([]string{"abc", "xyz", "123"})
	test_uint_slice([]uint{1, 2, 3})
	test_struct_slice([]structWithNoStrings{{42, true}, {24, true}}, 3)
	test_empty_slice_of_structs([]structWithNoStrings{}, 2)
	test_nil_slice_of_structs([]structWithNoStrings{}, 5)

	test_slice_of_slices([][]uint{
		{4},
		{5, 6},
		{7, 8, 9},
	})
	test_empty_slice([]uint{})

	test_nil_slice(1, nil, 5)
}
