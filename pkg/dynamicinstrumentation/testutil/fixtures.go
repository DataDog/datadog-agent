// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type funcName = string
type fixtures = map[funcName]ditypes.CapturedValueMap
type fieldMap = map[string]*ditypes.CapturedValue

func strPtr(s string) *string { return &s }
func capturedValue(t string, v string) *ditypes.CapturedValue {
	return &ditypes.CapturedValue{Type: t, Value: strPtr(v)}
}

var basicCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_bool":   {"x": capturedValue("bool", "true")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int":    {"x": capturedValue("int", "-1512")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint":   {"x": capturedValue("uint", "1512")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int8":   {"x": capturedValue("int8", "-8")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int16":  {"x": capturedValue("int16", "-16")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int32":  {"x": capturedValue("int32", "-32")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int64":  {"x": capturedValue("int64", "-64")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint8":  {"x": capturedValue("uint8", "8")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint16": {"x": capturedValue("uint16", "16")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint32": {"x": capturedValue("uint32", "32")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint64": {"x": capturedValue("uint64", "64")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_byte":   {"x": capturedValue("uint8", "97")},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_rune":   {"x": capturedValue("int32", "1")},

	// everything with float errors out
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_float32": {"x": capturedValue("float", "1.32")},
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_float64": {"x": capturedValue("float", "-1.646464")},
}

var stringCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_string": {"x": capturedValue("string", "abc")},
}

var arrayCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_byte_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("uint8", "1"),
		"arg_1": capturedValue("uint8", "1"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_rune_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("int32", "1"),
		"arg_1": capturedValue("int32", "2"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("string", "one"),
		"arg_1": capturedValue("string", "two"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("int", "1"),
		"arg_1": capturedValue("int", "2"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int8_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("int8", "1"),
		"arg_1": capturedValue("int8", "2"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_array": {"x": {Type: "array", Fields: fieldMap{
		"arg_0": capturedValue("uint", "1"),
		"arg_1": capturedValue("uint", "2"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays": {"arg_0": {Type: "array", Fields: fieldMap{
		"arg_0": {Type: "array", Fields: fieldMap{
			"arg_0": capturedValue("int", "1"),
			"arg_1": capturedValue("int", "2"),
		}},
		"arg_1": {Type: "array", Fields: fieldMap{
			"arg_0": capturedValue("int", "3"),
			"arg_1": capturedValue("int", "4"),
		}},
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_structs": {"a": {Type: "array", Fields: fieldMap{
		"[2]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct[0]": {Type: "struct", Fields: fieldMap{
			"anotherInt":    capturedValue("int", "42"),
			"anotherString": capturedValue("string", "foo"),
		}},
		"[2]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct[1]": {Type: "struct", Fields: fieldMap{
			"anotherInt":    capturedValue("int", "24"),
			"anotherString": capturedValue("string", "bar"),
		}},
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays_of_arrays": {"b": {Type: "array", Fields: fieldMap{
		"[2][2][2]int[0]": {Type: "array", Fields: fieldMap{
			"[2][2]int[0]": {Type: "array", Fields: fieldMap{
				"[2]int[0]": capturedValue("int", "1"),
				"[2]int[1]": capturedValue("int", "2"),
			}},
			"[2][2]int[1]": {Type: "array", Fields: fieldMap{
				"[2]int[0]": capturedValue("int", "3"),
				"[2]int[1]": capturedValue("int", "4"),
			}},
		}},
		"[2][2][2]int[1]": {Type: "array", Fields: fieldMap{
			"[2][2]int[0]": {Type: "array", Fields: fieldMap{
				"[2]int[0]": capturedValue("int", "5"),
				"[2]int[1]": capturedValue("int", "6"),
			}},
			"[2][2]int[1]": {Type: "array", Fields: fieldMap{
				"[2]int[0]": capturedValue("int", "7"),
				"[2]int[1]": capturedValue("int", "8"),
			}},
		}},
	}}},
}

var sliceCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_slice": {"u": {Type: "[]uint", Fields: fieldMap{
		"[0]uint": capturedValue("uint", "1"),
		"[1]uint": capturedValue("uint", "2"),
		"[2]uint": capturedValue("uint", "3"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_slice": {
		"a": capturedValue("int", "3"),
		"xs": {
			Type: "[]struct",
			Fields: fieldMap{
				"[0]struct": &ditypes.CapturedValue{
					Type: "struct",
					Fields: fieldMap{
						"arg_0": capturedValue("uint8", "42"),
						"arg_1": capturedValue("bool", "true"),
					},
				},
				"[1]struct": &ditypes.CapturedValue{
					Type: "struct",
					Fields: fieldMap{
						"arg_0": capturedValue("uint8", "24"),
						"arg_1": capturedValue("bool", "true"),
					},
				},
			},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_empty_slice_of_structs": {
		"a": capturedValue("int", "2"),
		"xs": {
			Type:   "[]struct",
			Fields: nil,
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_slice_of_structs": {
		"a": capturedValue("int", "5"),
		"xs": {
			Type:   "[]struct",
			Fields: nil,
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_slice_with_other_params": {
		"a": capturedValue("int8", "1"),
		"s": {
			Type:   "[]bool",
			Fields: nil,
		},
		"x": capturedValue("uint", "5"),
	},
}

var structCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_struct": {"t": {Type: "struct", Fields: fieldMap{
		"arg_0": capturedValue("string", "a"),
		"arg_1": capturedValue("string", "bb"),
		"arg_2": capturedValue("string", "ccc"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver.test_method_receiver": {
		"r": {
			Type: "struct", Fields: fieldMap{
				"arg_0": capturedValue("uint", "1"),
			}},
		"a": capturedValue("int", "2"),
	},
	// TODO: re-enable when fixing pointer method receivers
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.(*receiver).test_pointer_method_receiver": {
	// 	"r": {
	// 		Type: "struct", Fields: fieldMap{
	// 			"u": capturedValue("uint", "3"),
	// 		}},
	// 	"a": capturedValue("int", "4"),
	// },
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_lots_of_fields": {"l": {Type: "struct", Fields: fieldMap{
	// 	"a": capturedValue("uint8", "1"),
	// 	"b": capturedValue("uint8", "2"),
	// 	"c": capturedValue("uint8", "3"),
	// 	"d": capturedValue("uint8", "4"),
	// 	"e": capturedValue("uint8", "5"),
	// 	"f": capturedValue("uint8", "6"),
	// 	"g": capturedValue("uint8", "7"),
	// 	"h": capturedValue("uint8", "8"),
	// 	"i": capturedValue("uint8", "9"),
	// 	"j": capturedValue("uint8", "10"),
	// 	"k": capturedValue("uint8", "11"),
	// 	"l": capturedValue("uint8", "12"),
	// 	"m": capturedValue("uint8", "13"),
	// 	"n": capturedValue("uint8", "14"),
	// 	"o": capturedValue("uint8", "15"),
	// 	"p": capturedValue("uint8", "16"),
	// 	"q": capturedValue("uint8", "17"),
	// 	"r": capturedValue("uint8", "18"),
	// 	"s": capturedValue("uint8", "19"),
	// 	"t": capturedValue("uint8", "20"),
	// 	"u": capturedValue("CutFieldLimit", "reached field limit"),
	// 	"v": capturedValue("CutFieldLimit", "reached field limit"),
	// 	"w": capturedValue("CutFieldLimit", "reached field limit"),
	// 	"x": capturedValue("CutFieldLimit", "reached field limit"),
	// 	"y": capturedValue("CutFieldLimit", "reached field limit"),
	// 	"z": capturedValue("CutFieldLimit", "reached field limit"),
	// }}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nonembedded_struct": {"x": {Type: "struct", Fields: fieldMap{
		"arg_0": capturedValue("bool", "true"),
		"arg_1": capturedValue("int", "1"),
		"arg_2": capturedValue("int16", "2"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_pointer": {"x": {Type: "ptr", Fields: fieldMap{
		"arg_0": {
			Type: "struct", Fields: fieldMap{
				"arg_0": capturedValue("bool", "true"),
				"arg_1": capturedValue("int", "1"),
				"arg_2": capturedValue("int16", "2"),
			}},
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_multiple_embedded_struct": {"b": {Type: "struct", Fields: fieldMap{
		"aBool":  capturedValue("bool", "true"),
		"aInt16": capturedValue("int16", "42"),
		"aInt32": capturedValue("int32", "31"),
		"nested": {Type: "struct", Fields: fieldMap{
			"aBool":   capturedValue("bool", "true"),
			"aString": capturedValue("string", "one"),
			"aNumber": capturedValue("int", "2"),
			"nested": {Type: "struct", Fields: fieldMap{
				"anotherInt":    capturedValue("int", "3"),
				"anotherString": capturedValue("string", "four"),
			}},
		}},
	}}},
}

var pointerCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_pointer": {"x": {Type: "*uint", Fields: fieldMap{
		"arg_0": capturedValue("uint", "1"),
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_pointer": {"z": {Type: "*bool"}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_pointer": {"x": {Type: "*struct", Fields: fieldMap{
		"arg_0": &ditypes.CapturedValue{
			Type: "struct",
			Fields: fieldMap{
				"arg_0": capturedValue("bool", "true"),
				"arg_1": capturedValue("int", "1"),
				"arg_2": capturedValue("int16", "2"),
			},
		},
	}}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_struct_pointer": {"x": {Type: "*struct"}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_pointer": {"z": {
		Type: "*string",
		Fields: fieldMap{
			"arg_0": capturedValue("string", "abc"),
		},
	}},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_string": {
		"s": &ditypes.CapturedValue{
			Type: "*struct",
			Fields: fieldMap{
				"arg_0": &ditypes.CapturedValue{
					Type: "struct",
					Fields: fieldMap{
						"arg_0": capturedValue("int", "5"),
						"arg_1": capturedValue("string", "abcdef"),
					},
				},
			},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_slice": {
		"s": &ditypes.CapturedValue{
			Type: "*struct",
			Fields: fieldMap{
				"arg_0": &ditypes.CapturedValue{
					Type: "struct",
					Fields: fieldMap{
						"arg_0": capturedValue("int", "5"),
						"arg_1": &ditypes.CapturedValue{
							Type: "[]uint8",
							Fields: fieldMap{
								"[0]uint8": capturedValue("uint8", "2"),
								"[1]uint8": capturedValue("uint8", "3"),
								"[2]uint8": capturedValue("uint8", "4"),
							},
						},
						"arg_2": capturedValue("uint64", "5"),
					},
				},
			},
		},
	},
}

// mergeMaps combines multiple fixture maps into a single map
func mergeMaps(maps ...fixtures) fixtures {
	result := make(fixtures)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

var expectedCaptures = mergeMaps(
	basicCaptures,
	stringCaptures,
	arrayCaptures,
	structCaptures,
	sliceCaptures,
	pointerCaptures,
	// mapCaptures,
	// genericCaptures,
	// multiParamCaptures,
)
