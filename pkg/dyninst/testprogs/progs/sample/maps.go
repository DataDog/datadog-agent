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
func testSmallMap(m map[string]int) {}

//nolint:all
//go:noinline
func testArrayOfMaps(m [2]map[string]int) {}

//nolint:all
//go:noinline
func testPointerToMap(m *map[string]int) {}

//nolint:all
//go:noinline
func testMapIntToInt(m map[int]int) {}

//nolint:all
func executeMapFuncs() {
	testSmallMap(map[string]int{"AAA": 1, "BBB": 2})

	m := make(map[string]int, 10000)
	j := map[string]int{
		"AAA": 1,
		"BBB": 2,
		"CCC": 3,
		"DDD": 4,
		"EEE": 5,
		"FFF": 6,
		"GGG": 7,
		"HHH": 8,
		"III": 9,
		"JJJ": 10,
	}
	for k, v := range j {
		m[k] = v
	}
	testMapStringToInt(j)
	testMapStringToStruct(map[string]nestedStruct{"foo": {1, "one"}, "bar": {2, "two"}})

	testMapIntToInt(map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7, 8: 8, 9: 9, 10: 10})

	testArrayOfMaps([2]map[string]int{{"foo": 1, "bar": 2}, {"foo": 1, "bar": 2}})
	testStructWithMap(structWithMap{map[int]int{1: 1}})
	testPointerToMap(&map[string]int{"foo": 1, "bar": 2})
}
