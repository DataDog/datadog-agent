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
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("uint8", "1"),
				"arg_1": capturedValue("uint8", "1"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_rune_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("int32", "1"),
				"arg_1": capturedValue("int32", "2"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("string", "one"),
				"arg_1": capturedValue("string", "two"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("int", "1"),
				"arg_1": capturedValue("int", "2"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_int8_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("int8", "1"),
				"arg_1": capturedValue("int8", "2"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_array": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"x": {Type: "array", Fields: fieldMap{
				"arg_0": capturedValue("uint", "1"),
				"arg_1": capturedValue("uint", "2"),
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"arg_0": {Type: "array", Fields: fieldMap{
				"arg_0": {Type: "array", Fields: fieldMap{
					"arg_0": capturedValue("int", "1"),
					"arg_1": capturedValue("int", "2"),
				}},
				"arg_1": {Type: "array", Fields: fieldMap{
					"arg_0": capturedValue("int", "3"),
					"arg_1": capturedValue("int", "4"),
				}},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_structs": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"a": {Type: "array", Fields: fieldMap{
				"[2]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct[0]": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct", Fields: fieldMap{
					"anotherInt":    capturedValue("int", "42"),
					"anotherString": capturedValue("string", "foo"),
				}},
				"[2]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct[1]": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct", Fields: fieldMap{
					"anotherInt":    capturedValue("int", "24"),
					"anotherString": capturedValue("string", "bar"),
				}},
			}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_array_of_arrays_of_arrays": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"b": {Type: "array", Fields: fieldMap{
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
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
}

var sliceCaptures = fixtures{
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_slice": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{"u": {Type: "[]uint", Fields: fieldMap{
				"[0]uint": capturedValue("uint", "1"),
				"[1]uint": capturedValue("uint", "2"),
				"[2]uint": capturedValue("uint", "3"),
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
					Fields: fieldMap{
						"[0]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings": &ditypes.CapturedValue{
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings",
							Fields: fieldMap{
								"aUint8": capturedValue("uint8", "42"),
								"aBool":  capturedValue("bool", "true"),
							},
						},
						"[1]github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.structWithNoStrings": &ditypes.CapturedValue{
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
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_string_struct": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"t": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.threestrings", Fields: fieldMap{
						"a": capturedValue("string", "a"),
						"b": capturedValue("string", "bb"),
						"c": capturedValue("string", "ccc"),
					},
				},
			},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver.test_method_receiver": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"r": {
					Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.receiver", Fields: fieldMap{
						"u": capturedValue("uint", "1"),
					}},
				"a": capturedValue("int", "2"),
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
					}}},
			Options: TestInstrumentationOptions{CaptureDepth: 10},
		},
	},
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct_pointer": []CapturedValueMapWithOptions{
		{
			CapturedValueMap: map[string]*ditypes.CapturedValue{
				"x": {
					Type: "*github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct",
					Fields: fieldMap{
						"arg_0": {
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nStruct", Fields: fieldMap{
								"aBool":  capturedValue("bool", "true"),
								"aInt":   capturedValue("int", "1"),
								"aInt16": capturedValue("int16", "2"),
							}},
					}}},
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
							Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.aStruct", Fields: fieldMap{
								"aBool":   capturedValue("bool", "true"),
								"aString": capturedValue("string", "one"),
								"aNumber": capturedValue("int", "2"),
								"nested": {Type: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.nestedStruct", Fields: fieldMap{
									"anotherInt":    capturedValue("int", "3"),
									"anotherString": capturedValue("string", "four"),
								}},
							}},
						"aBool":  capturedValue("bool", "true"),
						"aInt32": capturedValue("int32", "31"),
					}}},
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
									Fields: fieldMap{
										"[0]uint8": capturedValue("uint8", "2"),
										"[1]uint8": capturedValue("uint8", "3"),
										"[2]uint8": capturedValue("uint8", "4"),
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
	// mapCaptures,
	// genericCaptures,
	// multiParamCaptures,
)
