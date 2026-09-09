// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

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
//go:noinline
func testMassiveString(x string) {}

//nolint:all
//go:noinline
func testUnitializedString(x string) {}

//nolint:all
//go:noinline
func testEmptyString(x string) {}

//nolint:all
//go:noinline
func testSubstrings(a string, b string, c string) {}

//nolint:all
//go:noinline
func testInvalidUtf8Strings(a string, b string) {}

//nolint:all
//go:noinline
func testMultibyteUtf8String(x string) {}

//nolint:all
func executeStringFuncs(ctx context.Context) {
	span, _ := tracer.StartSpanFromContext(ctx, "sample.strings")
	defer span.Finish()

	abc := "abc"
	testSingleString(abc)
	testThreeStrings(abc, "def", "ghi")
	testThreeStringsInStruct(threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testThreeStringsInStructPointer(&threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testOneStringInStructPointer(&oneStringStruct{a: "omg"})
	testMassiveString(x)

	var uninitializedString string
	testUnitializedString(uninitializedString)
	testEmptyString("")
	testEmptyString(abc[:0])

	// Check captures when multiple variables are aliasing the same underlying buffer.
	s := "abcdef"
	testSubstrings(s[:4], s[:2], s)

	// Go strings are not required to hold valid UTF-8.
	testInvalidUtf8Strings("\xff\xfe", "ok\xffbad")
	testMultibyteUtf8String(strings.Repeat("世界", 8))
}

var x = strings.Repeat("x", 100000)
