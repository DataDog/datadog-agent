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

//nolint:all
//go:noinline
func testMapWithSmallValue(m map[int]uint8) {}

//nolint:all
//go:noinline
func testMapWithSmallValueMassive(redactMyEntries map[int]uint8) {}

//nolint:all
//go:noinline
func testMapWithSmallKeyAndValue(m map[uint8]uint8) {}

//nolint:all
//go:noinline
func testMapWithSmallKeyAndValueMassive(m map[uint8]uint8) {}

//nolint:all
//go:noinline
func testMapSmallKeySmallValue(m map[uint8]uint8) {}

//nolint:all
//go:noinline
func testMapSmallKeyLargeValue(m map[uint8][4]int) {}

//nolint:all
//go:noinline
func testMapLargeKeySmallValue(m map[[4]int]uint8) {}

//nolint:all
//go:noinline
func testMapLargeKeyLargeValue(m map[[4]int][4]int) {}

//nolint:all
//go:noinline
func testMapEmptyKey(m map[struct{}]int) {}

//nolint:all
//go:noinline
func testMapEmptyValue(m map[int]struct{}) {}

//nolint:all
//go:noinline
func testMapEmptyKeyAndValue(m map[struct{}]struct{}) {}

// generateEmbeddedMaps creates a map for testMapEmbeddedMaps programmatically
func generateEmbeddedMaps(entriesCount int) map[string][]structWithMap {
	result := make(map[string][]structWithMap)
	for i := range entriesCount {
		key := fmt.Sprintf("key%d", i)
		slice := make([]structWithMap, 5)
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

	b := &node{val: 1}
	current := b
	for i := 2; i <= 20; i++ {
		current.b = &node{val: i}
		current = current.b
	}
	testMapWithLinkedList(
		map[bool]node{
			true: *b,
		},
	)
	testMapArrayToArray(map[[4]string][2]int{
		{"foo", "bar", "baz", "qux"}:        {1, 2},
		{"quux", "quuz", "corge", "grault"}: {3, 4},
		{"bluh", "blur", "baw23", "aaaa"}:   {5, 6},
	})

	testMapEmbeddedMaps(generateEmbeddedMaps(5))
	testMapMassive(generateEmbeddedMaps(150))

	smallMap := make(map[int]uint8)
	largeMap := make(map[int]uint8)
	for i := 1; i <= 10; i++ {
		smallMap[i] = uint8(i)
	}
	for i := 1; i <= 10000; i++ {
		largeMap[i] = uint8(i)
	}
	testMapWithSmallValue(smallMap)
	testMapWithSmallValueMassive(largeMap)

	smallKeyValueMap := make(map[uint8]uint8)
	largeKeyValueMap := make(map[uint8]uint8)
	for i := range 10 {
		smallKeyValueMap[uint8(i)] = uint8(i) * 2
	}
	for i := range 255 {
		largeKeyValueMap[uint8(i)] = uint8(i) * 2
	}
	testMapWithSmallKeyAndValue(smallKeyValueMap)
	testMapWithSmallKeyAndValueMassive(largeKeyValueMap)
	testMapSmallKeySmallValue(smallKeyValueMap)

	smallKeyLargeValueMap := make(map[uint8][4]int)
	for i := range 3 {
		smallKeyLargeValueMap[uint8(i)] = [4]int{int(i), int(i) * 2, int(i) * 3, int(i) * 4}
	}
	testMapSmallKeyLargeValue(smallKeyLargeValueMap)

	largeKeyLargeValueMap := make(map[[4]int][4]int)
	for i := range 10 {
		key := [4]int{i, i * 2, i * 3, i * 4}
		value := [4]int{i * 10, i * 20, i * 30, i * 40}
		largeKeyLargeValueMap[key] = value
	}
	testMapLargeKeyLargeValue(largeKeyLargeValueMap)

	largeKeySmallValueMap := make(map[[4]int]uint8)
	for i := 0; i < 3; i++ {
		key := [4]int{1 + i*4, 2 + i*4, 3 + i*4, 4 + i*4}
		largeKeySmallValueMap[key] = uint8(i + 1)
	}
	testMapLargeKeySmallValue(largeKeySmallValueMap)

	testMapEmptyKey(map[struct{}]int{{}: 1})
	testMapEmptyValue(map[int]struct{}{1: {}})
	testMapEmptyKeyAndValue(map[struct{}]struct{}{{}: {}})
}
