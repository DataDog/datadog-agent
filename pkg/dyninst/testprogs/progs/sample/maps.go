// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
)

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
func testMapStringToSlice(m map[string][]string) {}

//nolint:all
//go:noinline
func testMapArrayToArray(m map[[4]string][2]int) {}

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
//go:noinline
func testMapMassive(redactMyEntries map[string][]structWithMap) {}

//nolint:all
//go:noinline
func testMapEmbeddedMaps(m map[string][]structWithMap) {}

//nolint:all
//go:noinline
func testMapWithLinkedList(m map[bool]node) {}

// generateEmbeddedMaps creates a map for testMapEmbeddedMaps programmatically
func generateEmbeddedMaps(entriesCount int) map[string][]structWithMap {
	result := make(map[string][]structWithMap)
	for i := range entriesCount {
		key := fmt.Sprintf("key%d", i)
		slice := make([]structWithMap, 10)
		for j := range 5 {
			slice[j] = structWithMap{map[int]int{j + 1: j + 1}}
		}
		result[key] = slice
	}
	return result
}

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
	testMapStringToSlice(map[string][]string{"foo": {"one", "two"}, "bar": {"three", "four"}})

	b := node{
		val: 1,
		b: &node{
			val: 2,
			b: &node{
				val: 3,
				b: &node{
					val: 4,
					b: &node{
						val: 5,
						b: &node{
							val: 6,
							b:   nil,
						},
					},
				},
			},
		},
	}
	testMapWithLinkedList(
		map[bool]node{
			true: b,
		},
	)
	testMapArrayToArray(map[[4]string][2]int{
		[4]string{"foo", "bar", "baz", "qux"}:        {1, 2},
		[4]string{"quux", "quuz", "corge", "grault"}: {3, 4},
		[4]string{"bluh", "blur", "baw23", "aaaa"}:   {5, 6},
	})

	testMapEmbeddedMaps(generateEmbeddedMaps(5))
	testMapMassive(generateEmbeddedMaps(150))
}
