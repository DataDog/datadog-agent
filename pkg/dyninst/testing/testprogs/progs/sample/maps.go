// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

type structWithMap struct {
	m map[int]int
}

//nolint:all
//go:noinline
func testStructWithMap(s structWithMap) {}

//nolint:all
//go:noinline
func testMapStringToStruct(m map[string]nestedStruct) {}

//nolint:all
//go:noinline
func testMapStringToInt(m map[string]int) {}

//nolint:all
//go:noinline
func testArrayOfMaps(m [2]map[string]int) {}

//nolint:all
//go:noinline
func testPointerToMap(m *map[string]int) {}

//nolint:all
func executeMapFuncs() {

	testMapStringToInt(map[string]int{"foo": 1, "bar": 2})
	testMapStringToStruct(map[string]nestedStruct{"foo": {1, "one"}, "bar": {2, "two"}})

	testArrayOfMaps([2]map[string]int{{"foo": 1, "bar": 2}, {"foo": 1, "bar": 2}})
	testStructWithMap(structWithMap{map[int]int{1: 1}})
	testPointerToMap(&map[string]int{"foo": 1, "bar": 2})
}
