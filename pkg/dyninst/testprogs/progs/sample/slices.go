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
//go:noinline
func testSliceEmptyStructs(xs []struct{}) {}

//nolint:all
//go:noinline
func testSubslices(as []uint, bs []uint, cs []uint) {}

//nolint:all
//go:noinline
func testByteSliceAscii(x []byte) {}

//nolint:all
//go:noinline
func testByteSliceBinary(x []byte) {}

//nolint:all
//go:noinline
func testByteSliceEmbeddedNul(x []byte) {}

//nolint:all
//go:noinline
func testByteSliceInvalidUtf8(x []byte) {}

//nolint:all
//go:noinline
func testByteSliceMultibyteUtf8(x []byte) {}

//nolint:all
//go:noinline
func testEmptyByteSlice(x []byte) {}

//nolint:all
//go:noinline
func testNilByteSlice(x []byte) {}

//nolint:all
//go:noinline
func testUint8SliceFromString(x []uint8) {}

//nolint:all
//go:noinline
func testUint8SliceHighBytes(x []uint8) {}

//nolint:all
//go:noinline
func testTwoByteSlicesAsciiAndBinary(a, b []byte) {}

//nolint:all
//go:noinline
func testByteAndUint8Slices(a []byte, b []uint8) {}

//nolint:all
//go:noinline
func testByteSliceOfSlices(x [][]byte) {}

//nolint:all
//go:noinline
func testByteSliceWithOtherParams(a int, x []byte, b string) {}

//nolint:all
//go:noinline
func testByteSliceAndIntSlice(a []byte, b []int) {}

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
	s := make([]uint, 10000)
	for i := range s {
		s[i] = uint(i)
	}
	testVeryLargeSlice(s)
	testSliceOfSlices([][]uint{
		{4},
		{5, 6},
		{7, 8, 9},
	})
	testEmptySlice([]uint{})

	testNilSliceWithOtherParams(1, nil, 5)
	testNilSlice(nil)
	testSliceEmptyStructs([]struct{}{{}, {}})

	// Check captures when multiple variables are aliasing the same underlying buffer.
	s2 := []uint{1, 2, 3}
	testSubslices(s2[:2], s2[:1], s2)

	testByteSliceAscii([]byte("Hello!"))
	testByteSliceBinary([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	testByteSliceEmbeddedNul([]byte("hello\x00world"))
	testByteSliceInvalidUtf8([]byte{0xFF, 0xFE, 0xC0, 0x80})
	testByteSliceMultibyteUtf8([]byte("héllo, 世界"))
	testEmptyByteSlice([]byte{})
	testNilByteSlice(nil)
	testUint8SliceFromString([]uint8("ascii via uint8"))
	testUint8SliceHighBytes([]uint8{0x80, 0xFE, 0xFF})
	testTwoByteSlicesAsciiAndBinary([]byte("Hello!"), []byte{0xDE, 0xAD, 0xBE, 0xEF})
	testByteAndUint8Slices([]byte("Hello!"), []uint8{0x80, 0x81, 0x82})
	testByteSliceOfSlices([][]byte{
		[]byte("Hello!"),
		{0xDE, 0xAD, 0xBE, 0xEF},
		[]byte("héllo, 世界"),
		{},
	})
	testByteSliceWithOtherParams(1, []byte("Hello!"), "world")
	testByteSliceAndIntSlice([]byte("Hello!"), []int{1, 2, 3})
}
