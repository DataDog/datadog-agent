// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

//nolint:all
//go:noinline
func testSingleString(x string) {}

//nolint:all
//go:noinline
func testThreeStrings(x, y, z string) {}

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
func testThreeStringsInStruct(a threeStringStruct) {}

//nolint:all
//go:noinline
func testThreeStringsInStructPointer(a *threeStringStruct) {}

//nolint:all
//go:noinline
func testOneStringInStructPointer(a *oneStringStruct) {}

//nolint:all
func executeStringFuncs() {
	testSingleString("abc")
	testThreeStrings("abc", "def", "ghi")
	testThreeStringsInStruct(threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testThreeStringsInStructPointer(&threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testOneStringInStructPointer(&oneStringStruct{a: "omg"})
}
