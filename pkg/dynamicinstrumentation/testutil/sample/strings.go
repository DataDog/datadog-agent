// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

//nolint:all
//go:noinline
func test_single_string(x string) {}

//nolint:all
//go:noinline
func test_three_strings(x, y, z string) {}

type threeStringStruct struct {
	a string
	b string
	c string
}

type oneStringStruct struct {
	a string
}

//nolint:all
//go:noinline
func test_three_strings_in_struct(a threeStringStruct) {}

//nolint:all
//go:noinline
func test_three_strings_in_struct_pointer(a *threeStringStruct) {}

//nolint:all
//go:noinline
func test_one_string_in_struct_pointer(a *oneStringStruct) {}

//nolint:all
func ExecuteStringFuncs() {
	test_single_string("abc")
	test_three_strings("abc", "def", "ghi")
	test_three_strings_in_struct(threeStringStruct{a: "abc", b: "def", c: "ghi"})
	test_three_strings_in_struct_pointer(&threeStringStruct{a: "abc", b: "def", c: "ghi"})
	test_one_string_in_struct_pointer(&oneStringStruct{a: "omg"})
}
