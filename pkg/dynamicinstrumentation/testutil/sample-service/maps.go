package main

type structWithMap struct {
	m map[int]int
}

//go:noinline
func test_struct_with_map(s structWithMap) {}

//go:noinline
func test_map_string_to_struct(m map[string]nestedStruct) {}

//go:noinline
func test_map_string_to_int(m map[string]int) {}

//go:noinline
func test_array_of_maps(m [2]map[string]int) {}

//go:noinline
func test_pointer_to_map(m *map[string]int) {}

func executeMapFuncs() {

	test_map_string_to_int(map[string]int{"foo": 1, "bar": 2})
	test_map_string_to_struct(map[string]nestedStruct{"foo": {1, "one"}, "bar": {2, "two"}})

	test_array_of_maps([2]map[string]int{{"foo": 1, "bar": 2}, {"foo": 1, "bar": 2}})
	test_struct_with_map(structWithMap{map[int]int{1: 1}})
	test_pointer_to_map(&map[string]int{"foo": 1, "bar": 2})
}
