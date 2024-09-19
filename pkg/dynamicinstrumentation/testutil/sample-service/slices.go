package main

import "fmt"

//go:noinline
func doNothing(x []int) {}

//go:noinline
func sprintSlice(x []int) string {
	return fmt.Sprintf("%v\n", x)
}

//go:noinline
func expandSlice(x []int) {
	x = append(x, []int{9, 10, 11, 12}...)
	doNothing(x)
}

//go:noinline
func test_uint_slice(u []uint) {}

//go:noinline
func test_struct_slice(xs []nestedStruct) {}

func executeSliceFuncs() {
	originalSlice := []int{1, 2, 3}
	expandSlice(originalSlice)
	sprintSlice(originalSlice)

	test_uint_slice([]uint{9, 8, 7})
	test_struct_slice([]nestedStruct{{42, "foo"}, {24, "bar"}})
}
