// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// TestInstrumentationOptions contains options for probes in tests
type TestInstrumentationOptions struct {
	CaptureDepth int
}

// CapturedValueMapWithOptions pairs instrumentaiton options with expected values
type CapturedValueMapWithOptions struct {
	CapturedValueMap ditypes.CapturedValueMap
	Options          TestInstrumentationOptions
}

type funcName = string
type fixtures = map[funcName][]CapturedValueMapWithOptions
type fieldMap = map[string]*ditypes.CapturedValue

func strPtr(s string) *string { return &s }
func capturedValue(t string, v string) *ditypes.CapturedValue {
	return &ditypes.CapturedValue{Type: t, Value: strPtr(v)}
}

var basicCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_bool": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("bool", "true")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					NotCapturedReason: "depth",
					Type:              "bool",
				}},
			Options: TestInstrumentationOptions{CaptureDepth: 0},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int", "-1512")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint", "1512")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int8": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int8", "-8")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int16": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int16", "-16")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int32": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int32", "-32")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int64": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int64", "-64")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint8": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint8", "8")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint16": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint16", "16")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint32": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint32", "32")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_uint64": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint64", "64")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_byte": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("uint8", "97")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_rune": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("int32", "1")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	// everything with float errors out
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_float32": {"x": capturedValue("float", "1.32")},
	// "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_float64": {"x": capturedValue("float", "-1.646464")},
}

var stringCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_string": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": capturedValue("string", "abc")},
			Options:          TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var arrayCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_byte_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]uint8", Elements: []ditypes.CapturedValue{
				{Type: "uint8", Value: strPtr("1")},
				{Type: "uint8", Value: strPtr("1")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_rune_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]int32", Elements: []ditypes.CapturedValue{
				{Type: "int32", Value: strPtr("1")},
				{Type: "int32", Value: strPtr("2")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]string", Elements: []ditypes.CapturedValue{
				{Type: "string", Value: strPtr("one")},
				{Type: "string", Value: strPtr("two")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]int", Elements: []ditypes.CapturedValue{
				{Type: "int", Value: strPtr("1")},
				{Type: "int", Value: strPtr("2")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int8_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]int8", Elements: []ditypes.CapturedValue{
				{Type: "int8", Value: strPtr("1")},
				{Type: "int8", Value: strPtr("2")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "[2]uint", Elements: []ditypes.CapturedValue{
				{Type: "uint", Value: strPtr("1")},
				{Type: "uint", Value: strPtr("2")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"arg_0": {Type: "array", Elements: []ditypes.CapturedValue{
				{Type: "array", Elements: []ditypes.CapturedValue{
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "int", Value: strPtr("1")},
						{Type: "int", Value: strPtr("2")},
					}},
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "int", Value: strPtr("3")},
						{Type: "int", Value: strPtr("4")},
					}},
				}},
				{Type: "array", Elements: []ditypes.CapturedValue{
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "int", Value: strPtr("5")},
						{Type: "int", Value: strPtr("6")},
					}},
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "int", Value: strPtr("7")},
						{Type: "int", Value: strPtr("8")},
					}},
				}},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_structs": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"a": {Type: "array", Elements: []ditypes.CapturedValue{
				{Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct", Elements: []ditypes.CapturedValue{
					{Type: "int", Value: strPtr("42")},
					{Type: "string", Value: strPtr("foo")},
				}},
				{Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct", Elements: []ditypes.CapturedValue{
					{Type: "int", Value: strPtr("24")},
					{Type: "string", Value: strPtr("bar")},
				}},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays_of_arrays": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"b": {Type: "array", Elements: []ditypes.CapturedValue{
				{Type: "array", Elements: []ditypes.CapturedValue{
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "array", Elements: []ditypes.CapturedValue{
							{Type: "int", Value: strPtr("1")},
							{Type: "int", Value: strPtr("2")},
						}},
						{Type: "array", Elements: []ditypes.CapturedValue{
							{Type: "int", Value: strPtr("3")},
							{Type: "int", Value: strPtr("4")},
						}},
					}},
					{Type: "array", Elements: []ditypes.CapturedValue{
						{Type: "array", Elements: []ditypes.CapturedValue{
							{Type: "int", Value: strPtr("5")},
							{Type: "int", Value: strPtr("6")},
						}},
						{Type: "array", Elements: []ditypes.CapturedValue{
							{Type: "int", Value: strPtr("7")},
							{Type: "int", Value: strPtr("8")},
						}},
					}},
				}},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var sliceCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"u": {Type: "[]uint", Elements: []ditypes.CapturedValue{
				{Type: "uint", Value: strPtr("1")},
				{Type: "uint", Value: strPtr("2")},
				{Type: "uint", Value: strPtr("3")},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": capturedValue("int", "3"),
				"xs": {
					Type: "[]struct",
					Elements: []ditypes.CapturedValue{
						{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
							Fields: fieldMap{
								"aUint8": capturedValue("uint8", "42"),
								"aBool":  capturedValue("bool", "true"),
							},
						},
						{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
							Fields: fieldMap{
								"aUint8": capturedValue("uint8", "24"),
								"aBool":  capturedValue("bool", "true"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_empty_slice_of_structs": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": capturedValue("int", "2"),
				"xs": {
					Type:   "[]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
					Fields: nil,
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_slice_of_structs": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": capturedValue("int", "5"),
				"xs": {
					Type:   "[]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
					Fields: nil,
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_slice_with_other_params": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": capturedValue("int8", "1"),
				"s": {
					Type:   "[]bool",
					Fields: nil,
				},
				"x": capturedValue("uint", "5"),
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var structCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_unsupported_fields": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.hasUnsupportedFields",
					Fields: fieldMap{
						"a": capturedValue("int", "1"),
						"b": capturedValue("float32", ""),
						"c": {
							Type: "[]uint8",
							Elements: []ditypes.CapturedValue{
								*capturedValue("uint8", "3"),
								*capturedValue("uint8", "4"),
								*capturedValue("uint8", "5"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_method_receiver": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"r": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver",
					Fields: fieldMap{
						"arg_0": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver",
							Fields: fieldMap{
								"u": capturedValue("uint", "3"),
							},
						},
					},
				},
				"a": capturedValue("int", "4"),
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_method_receiver": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"r": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver",
					Fields: fieldMap{
						"u": capturedValue("uint", "1"),
					},
				},
				"a": capturedValue("int", "2"),
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithAnArray",
					Fields: fieldMap{
						"arr": {
							Type: "[5]uint8",
							Elements: []ditypes.CapturedValue{
								*capturedValue("uint8", "1"),
								*capturedValue("uint8", "2"),
								*capturedValue("uint8", "3"),
								*capturedValue("uint8", "4"),
								*capturedValue("uint8", "5"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_a_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
					Fields: fieldMap{
						"x": capturedValue("int", "1"),
						"slice": {
							Type: "[]uint8",
							Elements: []ditypes.CapturedValue{
								*capturedValue("uint8", "2"),
								*capturedValue("uint8", "3"),
								*capturedValue("uint8", "4"),
							},
						},
						"z": capturedValue("int", "5"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_an_empty_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
					Fields: fieldMap{
						"x": capturedValue("int", "9"),
						"slice": {
							Type:     "[]uint8",
							Elements: []ditypes.CapturedValue{},
						},
						"z": capturedValue("uint64", "5"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_a_nil_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
					Fields: fieldMap{
						"x":     capturedValue("int", "9"),
						"slice": {Type: "[]uint8"},
						"z":     capturedValue("uint64", "5"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
					Fields: fieldMap{
						"arg_0": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
							Fields: fieldMap{
								"x": capturedValue("int", "5"),
								"slice": {
									Type: "[]int",
									Elements: []ditypes.CapturedValue{
										*capturedValue("int", "2"),
										*capturedValue("int", "3"),
										*capturedValue("int", "4"),
									},
								},
								"z": capturedValue("int", "5"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_string": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithAString",
					Fields: fieldMap{
						"arg_0": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithAString",
							Fields: fieldMap{
								"x": capturedValue("int", "5"),
								"s": capturedValue("string", "abcdef"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.aStruct",
					Fields: fieldMap{
						"aBool":   capturedValue("bool", "true"),
						"aString": capturedValue("string", "one"),
						"aNumber": capturedValue("int", "2"),
						"nested": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct",
							Fields: fieldMap{
								"anotherInt":    capturedValue("int", "3"),
								"anotherString": capturedValue("string", "four"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_with_arrays": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithTwoArrays",
					Fields: fieldMap{
						"a": {
							Type: "[3]uint64",
							Elements: []ditypes.CapturedValue{
								*capturedValue("uint64", "1"),
								*capturedValue("uint64", "2"),
								*capturedValue("uint64", "3"),
							},
						},
						"b": capturedValue("int", "4"),
						"c": {
							Type: "[5]int64",
							Elements: []ditypes.CapturedValue{
								*capturedValue("int64", "6"),
								*capturedValue("int64", "7"),
								*capturedValue("int64", "8"),
								*capturedValue("int64", "9"),
								*capturedValue("int64", "10"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nonembedded_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct",
					Fields: fieldMap{
						"aBool":  capturedValue("bool", "true"),
						"aInt":   capturedValue("int", "1"),
						"aInt16": capturedValue("int16", "2"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_multiple_embedded_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"b": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.bStruct",
					Fields: fieldMap{
						"aInt16": capturedValue("int16", "42"),
						"nested": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.aStruct",
							Fields: fieldMap{
								"aBool":   capturedValue("bool", "true"),
								"aString": capturedValue("string", "one"),
								"aNumber": capturedValue("int", "2"),
								"nested": {
									Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct",
									Fields: fieldMap{
										"anotherInt":    capturedValue("int", "3"),
										"anotherString": capturedValue("string", "four"),
									},
								},
							},
						},
						"aBool":  capturedValue("bool", "true"),
						"aInt32": capturedValue("int32", "31"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_no_string_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"c": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.cStruct",
					Fields: fieldMap{
						"aInt32": capturedValue("int32", "4"),
						"aUint":  capturedValue("uint", "1"),
						"nested": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
							Fields: fieldMap{
								"aUint8": capturedValue("uint8", "9"),
								"aBool":  capturedValue("bool", "true"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_and_byte": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"w": capturedValue("uint8", "97"),
				"x": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.aStruct",
					Fields: fieldMap{
						"aBool":   capturedValue("bool", "true"),
						"aString": capturedValue("string", "one"),
						"aNumber": capturedValue("int", "2"),
						"nested": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct",
							Fields: fieldMap{
								"anotherInt":    capturedValue("int", "3"),
								"anotherString": capturedValue("string", "four"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nested_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.anotherStruct",
					Fields: fieldMap{
						"arg_0": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.anotherStruct",
							Fields: fieldMap{
								"nested": {
									Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct",
									Fields: fieldMap{
										"arg_0": {
											Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct",
											Fields: fieldMap{
												"anotherInt":    capturedValue("int", "42"),
												"anotherString": capturedValue("string", "xyz"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_ten_strings": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tenStrings",
					Fields: fieldMap{
						"first":   capturedValue("string", "one"),
						"second":  capturedValue("string", "two"),
						"third":   capturedValue("string", "three"),
						"fourth":  capturedValue("string", "four"),
						"fifth":   capturedValue("string", "five"),
						"sixth":   capturedValue("string", "six"),
						"seventh": capturedValue("string", "seven"),
						"eighth":  capturedValue("string", "eight"),
						"ninth":   capturedValue("string", "nine"),
						"tenth":   capturedValue("string", "ten"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"t": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.threestrings",
					Fields: fieldMap{
						"a": capturedValue("string", "a"),
						"b": capturedValue("string", "bb"),
						"c": capturedValue("string", "ccc"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_deep_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"t": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct1",
					Fields: fieldMap{
						"a": capturedValue("int", "1"),
						"b": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct2",
							Fields: fieldMap{
								"c": capturedValue("int", "2"),
								"d": {
									Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct3",
									Fields: fieldMap{
										"e": capturedValue("int", "3"),
										"f": {
											Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct4",
											Fields: fieldMap{
												"g": capturedValue("int", "4"),
												"h": {
													Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct5",
													Fields: fieldMap{
														"i": capturedValue("int", "5"),
														"j": {
															Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.deepStruct6",
															Fields: fieldMap{
																"k": capturedValue("int", "6"),
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_empty_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"e": {
					Type:   "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.emptyStruct",
					Fields: fieldMap{},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_lots_of_fields": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"l": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.lotsOfFields",
					Fields: fieldMap{
						"a": capturedValue("uint8", "1"),
						"b": capturedValue("uint8", "2"),
						"c": capturedValue("uint8", "3"),
						"d": capturedValue("uint8", "4"),
						"e": capturedValue("uint8", "5"),
						"f": capturedValue("uint8", "6"),
						"g": capturedValue("uint8", "7"),
						"h": capturedValue("uint8", "8"),
						"i": capturedValue("uint8", "9"),
						"j": capturedValue("uint8", "10"),
						"k": capturedValue("uint8", "11"),
						"l": capturedValue("uint8", "12"),
						"m": capturedValue("uint8", "13"),
						"n": capturedValue("uint8", "14"),
						"o": capturedValue("uint8", "15"),
						"p": capturedValue("uint8", "16"),
						"q": capturedValue("uint8", "17"),
						"r": capturedValue("uint8", "18"),
						"s": capturedValue("uint8", "19"),
						"t": capturedValue("uint8", "20"),
						"u": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
						"v": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
						"w": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
						"x": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
						"y": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
						"z": &ditypes.CapturedValue{
							Type:              "uint8",
							NotCapturedReason: "fieldCount",
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var pointerCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "*uint",
					Fields: fieldMap{
						"arg_0": capturedValue("uint", "1"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					NotCapturedReason: "depth",
					Type:              "*uint",
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 0},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "uint", Value: strPtr("1")},
				"z": {Type: "*bool"},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct",
					Fields: fieldMap{
						"arg_0": &ditypes.CapturedValue{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct",
							Fields: fieldMap{
								"aBool":  capturedValue("bool", "true"),
								"aInt":   capturedValue("int", "1"),
								"aInt16": capturedValue("int16", "2"),
							},
						},
					}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nil_struct_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "int", Value: strPtr("5")},
				"x": {Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct"},
				"z": {Type: "uint", Value: strPtr("4")},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"z": {
					Type: "*string",
					Fields: fieldMap{
						"arg_0": capturedValue("string", "abc"),
					},
				}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_string": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithAString",
					Fields: fieldMap{
						"arg_0": &ditypes.CapturedValue{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithAString",
							Fields: fieldMap{
								"x": capturedValue("int", "5"),
								"s": capturedValue("string", "abcdef"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_pointer_to_struct_with_a_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"s": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
					Fields: fieldMap{
						"arg_0": &ditypes.CapturedValue{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithASlice",
							Fields: fieldMap{
								"x": capturedValue("int", "5"),
								"slice": &ditypes.CapturedValue{
									Type: "[]uint8",
									Elements: []ditypes.CapturedValue{
										{Type: "uint8", Value: strPtr("2")},
										{Type: "uint8", Value: strPtr("3")},
										{Type: "uint8", Value: strPtr("4")},
									},
								},
								"z": capturedValue("uint64", "5"),
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var captureDepthCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_multiple_struct_tiers": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierA", Fields: fieldMap{
					"a": capturedValue("int", "1"),
					"b": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierB", Fields: fieldMap{
						"c": capturedValue("int", "2"),
						"d": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierC", Fields: fieldMap{
							"e": capturedValue("int", "3"),
							"f": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierD", Fields: fieldMap{
								"g": capturedValue("int", "4"),
							}},
						}},
					}},
				}},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 5},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierA", Fields: fieldMap{
					"a": capturedValue("int", "1"),
					"b": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierB", Fields: fieldMap{
						"c": capturedValue("int", "2"),
						"d": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierC", Fields: fieldMap{
							"e": capturedValue("int", "3"),
							"f": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierD", NotCapturedReason: "depth"},
						}},
					}},
				}},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 4},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierA", Fields: fieldMap{
					"a": capturedValue("int", "1"),
					"b": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierB", Fields: fieldMap{
						"c": capturedValue("int", "2"),
						"d": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierC", NotCapturedReason: "depth"},
					}},
				}},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 3},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierA", Fields: fieldMap{
					"a": capturedValue("int", "1"),
					"b": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierB", NotCapturedReason: "depth"},
				}},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 2},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type:              "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.tierA",
					NotCapturedReason: "depth",
				},
			}, Options: TestInstrumentationOptions{CaptureDepth: 1},
		},
	},
}

var interfaceCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_interface_complexity": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.interfaceComplexityA", Fields: fieldMap{
					"b": capturedValue("int", "1"),
					"c": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.interfaceComplexityB", Fields: fieldMap{
						"d": capturedValue("int", "2"),
						"e": {Type: "runtime.iface", Value: nil, NotCapturedReason: "unsupported"},
						"f": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.interfaceComplexityC", Fields: fieldMap{
							"g": capturedValue("int", "4"),
						}},
					}},
				}},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_interface_and_int": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": capturedValue("int", "1"),
				"b": {Type: "runtime.iface", Value: nil, NotCapturedReason: "unsupported"},
				"c": capturedValue("uint", "3"),
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var linkedListCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_linked_list": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
					Fields: fieldMap{
						"val": capturedValue("int", "1"),
						"b": {
							Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
							Fields: fieldMap{
								"arg_0": {
									Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
									Fields: fieldMap{
										"val": capturedValue("int", "2"),
										"b": {
											Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
											Fields: fieldMap{
												"arg_0": {
													Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
													Fields: fieldMap{
														"val": capturedValue("int", "3"),
														"b":   {Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 4},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
					Fields: fieldMap{
						"val": capturedValue("int", "1"),
						"b": {
							Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
							Fields: fieldMap{
								"arg_0": {
									Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
									Fields: fieldMap{
										"val": capturedValue("int", "2"),
										"b": {
											Type:   "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
											Fields: fieldMap{},
										},
									},
								},
							},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 3},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
					Fields: fieldMap{
						"val": capturedValue("int", "1"),
						"b": {
							Type:   "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
							Fields: fieldMap{},
						},
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 2},
		},
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"a": {
					Type:              "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.node",
					NotCapturedReason: "depth",
					Fields:            fieldMap{},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 1},
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
	captureDepthCaptures,
	interfaceCaptures,
	linkedListCaptures,
)
