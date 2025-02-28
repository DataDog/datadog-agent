// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventparser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

func TestCountBufferUsedByTypeDefinition(t *testing.T) {
	tests := []struct {
		name     string
		param    *ditypes.Param
		expected int
	}{
		{
			name: "Struct with nested structs and ints",
			param: &ditypes.Param{
				Kind: byte(reflect.Struct),
				Size: 2,
				Fields: []*ditypes.Param{
					{Kind: byte(reflect.Struct), Size: 2, Fields: []*ditypes.Param{
						{Kind: byte(reflect.Int), Size: 8},
						{Kind: byte(reflect.Int), Size: 8},
					}},
					{Kind: byte(reflect.Int), Size: 8},
				},
			},
			expected: 15,
		},
		{
			name: "Complex nested structure",
			param: &ditypes.Param{
				Type: "slice", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{Type: "struct", Size: 0x2, Kind: 0x19, Fields: []*ditypes.Param{
						{Type: "uint8", Size: 0x1, Kind: 0x8},
						{Type: "struct", Size: 0x2, Kind: 0x19, Fields: []*ditypes.Param{
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
						}},
					}},
				},
			},
			expected: 18,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countBufferUsedByTypeDefinition(tt.param)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestParseParamValue(t *testing.T) {
	tests := []struct {
		name            string
		inputBuffer     []byte
		inputDefinition *ditypes.Param
		expectedValue   *ditypes.Param
	}{
		{
			name: "Basic slice of structs",
			inputBuffer: []byte{
				1, 2, 0, 3, 0, 0, 0, // Content of slice element 1
				4, 5, 0, 6, 0, 0, 0, // Content of slice element 2
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // Extra padding
			},
			inputDefinition: &ditypes.Param{
				Type: "slice", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{Type: "uint8", Size: 0x1, Kind: 0x8},
						{Type: "uint16", Size: 0x2, Kind: 0x9},
						{Type: "uint32", Size: 0x4, Kind: 0xa},
					}},
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{Type: "uint8", Size: 0x1, Kind: 0x8},
						{Type: "uint16", Size: 0x2, Kind: 0x9},
						{Type: "uint32", Size: 0x4, Kind: 0xa},
					}},
				},
			},
			expectedValue: &ditypes.Param{
				Type: "slice", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{ValueStr: "1", Type: "uint8", Size: 0x1, Kind: 0x8},
						{ValueStr: "2", Type: "uint16", Size: 0x2, Kind: 0x9},
						{ValueStr: "3", Type: "uint32", Size: 0x4, Kind: 0xa},
					}},
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{ValueStr: "4", Type: "uint8", Size: 0x1, Kind: 0x8},
						{ValueStr: "5", Type: "uint16", Size: 0x2, Kind: 0x9},
						{ValueStr: "6", Type: "uint32", Size: 0x4, Kind: 0xa},
					}},
				},
			},
		},
		{
			name: "same sized string",
			inputBuffer: []byte{
				65, 65, 65,
			},
			inputDefinition: &ditypes.Param{
				Type: "string", Size: 0x3, Kind: 0x18,
			},
			expectedValue: &ditypes.Param{
				ValueStr: "AAA", Type: "string", Size: 0x3, Kind: 0x18,
			},
		},
		{
			name:        "empty string",
			inputBuffer: []byte{},
			inputDefinition: &ditypes.Param{
				Type: "string", Size: 0x0, Kind: 0x18,
			},
			expectedValue: &ditypes.Param{
				ValueStr: "", Type: "string", Size: 0x0, Kind: 0x18,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, _ := parseParamValue(tt.inputDefinition, tt.inputBuffer)
			if !reflect.DeepEqual(val, tt.expectedValue) {
				t.Errorf("Parsed incorrectly! Got %+v, expected %+v", val, tt.expectedValue)
			}
		})
	}
}

func TestReadParams(t *testing.T) {
	tests := []struct {
		name           string
		inputBuffer    []byte
		expectedResult []*ditypes.Param
	}{
		{
			name: "Basic slice of structs",
			inputBuffer: []byte{
				23, 2, 0, // Slice with 2 elements
				25, 3, 0, // Slice elements are each a struct with 3 fields
				8, 1, 0, // Struct field 1 is a uint8 (size 1)
				9, 2, 0, // Struct field 2 is a uint16 (size 2)
				8, 1, 0, // Struct field 3 is a uint8 (size 1)
				1, 2, 0, 3, // Content of slice element 1 (not relevant for this function)
				4, 5, 0, 6, // Content of slice element 2 (not relevant for this function)
				// Padding
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0,
			},
			expectedResult: []*ditypes.Param{{
				Type: "[]struct", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{ValueStr: "1", Type: "uint8", Size: 0x1, Kind: 0x8},
						{ValueStr: "2", Type: "uint16", Size: 0x2, Kind: 0x9},
						{ValueStr: "3", Type: "uint8", Size: 0x1, Kind: 0x8},
					}},
					{Type: "struct", Size: 0x3, Kind: 0x19, Fields: []*ditypes.Param{
						{ValueStr: "4", Type: "uint8", Size: 0x1, Kind: 0x8},
						{ValueStr: "5", Type: "uint16", Size: 0x2, Kind: 0x9},
						{ValueStr: "6", Type: "uint8", Size: 0x1, Kind: 0x8},
					}},
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := readParams(tt.inputBuffer)
			assert.Equal(t, output, tt.expectedResult)
		})
	}
}
func TestParseTypeDefinition(t *testing.T) {
	tests := []struct {
		name           string
		inputBuffer    []byte
		expectedResult *ditypes.Param
	}{
		{
			name: "Slice of structs with uint8 and uint16 fields",
			inputBuffer: []byte{
				23, 2, 0, // Slice with 2 elements

				25, 3, 0, // Slice elements are each a struct with 3 fields

				8, 1, 0, // Struct field 1 is a uint8 (size 1)
				9, 2, 0, // Struct field 2 is a uint16 (size 2)
				8, 1, 0, // Struct field 3 is a uint8 (size 1)

				25, 3, 0, // Slice elements are each a struct with 3 fields

				8, 1, 0, // Struct field 1 is a uint8 (size 1)
				9, 2, 0, // Struct field 2 is a uint16 (size 2)
				8, 1, 0, // Struct field 3 is a uint8 (size 1)

				// Padding
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			expectedResult: &ditypes.Param{
				Type: "[]struct", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{
						Type: "struct", Size: 0x3, Kind: 0x19,
						Fields: []*ditypes.Param{
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint16", Size: 0x2, Kind: 0x9},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
						},
					},
					{
						Type: "struct", Size: 0x3, Kind: 0x19,
						Fields: []*ditypes.Param{
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint16", Size: 0x2, Kind: 0x9},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
						},
					},
				},
			},
		},
		{
			name: "Nested struct fields",
			inputBuffer: []byte{
				23, 2, 0, // Slice with 2 elements
				25, 4, 0, // Slice elements are each a struct with 2 fields
				8, 1, 0, // Struct field 1 is a uint8 (size 1)
				8, 1, 0, // Struct field 2 is a uint8 (size 1)
				8, 1, 0, // Struct field 3 is a uint8 (size 1)
				25, 2, 0, // Struct field 4 is a struct with 2 fields
				8, 1, 0, // Nested struct field 1 is a uint8 (size 1)
				8, 1, 0, // Nested struct field 2 is a uint8 (size 1)
				1, 2, 3, // Content of slice element 1 (top-level uint8, then 2 second tier uint8s)
				4, 5, 6, // Content of slice element 2 (top-level uint8, then 2 second tier uint8s)
				// Padding
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			expectedResult: &ditypes.Param{
				Type: "[]struct", Size: 0x2, Kind: 0x17,
				Fields: []*ditypes.Param{
					{
						Type: "struct", Size: 0x4, Kind: 0x19,
						Fields: []*ditypes.Param{
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{
								Type: "struct", Size: 0x2, Kind: 0x19,
								Fields: []*ditypes.Param{
									{Type: "uint8", Size: 0x1, Kind: 0x8},
									{Type: "uint8", Size: 0x1, Kind: 0x8},
								},
							},
						},
					},
					{
						Type: "struct", Size: 0x4, Kind: 0x19,
						Fields: []*ditypes.Param{
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{Type: "uint8", Size: 0x1, Kind: 0x8},
							{
								Type: "struct", Size: 0x2, Kind: 0x19,
								Fields: []*ditypes.Param{
									{Type: "uint8", Size: 0x1, Kind: 0x8},
									{Type: "uint8", Size: 0x1, Kind: 0x8},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeDefinition := parseTypeDefinition(tt.inputBuffer)
			if !paramsAreEqual(typeDefinition, tt.expectedResult) {
				t.Errorf("params are not equal\nExpected: %s\nReceived: %s\n", pretty.Sprint(tt.expectedResult), pretty.Sprint(typeDefinition))
			}
		})
	}
}

func paramsAreEqual(p1, p2 *ditypes.Param) bool {
	if p1 == nil && p2 == nil {
		return true
	}
	if p1 == nil || p2 == nil {
		return false
	}
	if p1.ValueStr != p2.ValueStr || p1.Type != p2.Type || p1.Size != p2.Size || p1.Kind != p2.Kind {
		return false
	}
	if len(p1.Fields) != len(p2.Fields) {
		return false
	}
	for i := range p1.Fields {
		if !paramsAreEqual(p1.Fields[i], p2.Fields[i]) {
			return false
		}
	}
	return true
}

func TestParseParams(t *testing.T) {
	type testCase struct {
		Name           string
		Buffer         []byte
		ExpectedOutput []*ditypes.Param
	}

	testCases := []testCase{
		{
			Name:   "uint slice ok",
			Buffer: []byte{23, 3, 0, 7, 8, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedOutput: []*ditypes.Param{
				{
					Type: "[]uint",
					Size: 3,
					Kind: byte(reflect.Slice),
					Fields: []*ditypes.Param{
						{
							Kind:     byte(reflect.Uint),
							ValueStr: "1",
							Type:     "uint",
							Size:     8,
						},
						{
							Kind:     byte(reflect.Uint),
							ValueStr: "2",
							Type:     "uint",
							Size:     8,
						},
						{
							Kind:     byte(reflect.Uint),
							ValueStr: "3",
							Type:     "uint",
							Size:     8,
						},
					},
				},
			},
		},
		{
			Name:   "uint pointer ok",
			Buffer: []byte{22, 8, 0, 7, 8, 0, 248, 60, 128, 0, 64, 0, 0, 0, 123, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedOutput: []*ditypes.Param{
				{
					Type: "*uint",
					Size: 8,
					Kind: byte(reflect.Pointer),
					Fields: []*ditypes.Param{
						{
							Kind:     byte(reflect.Uint),
							ValueStr: "123",
							Type:     "uint",
							Size:     8,
						},
					},
				},
			},
		},
		{
			Name:   "struct pointer ok",
			Buffer: []byte{22, 8, 0, 25, 2, 0, 7, 8, 0, 1, 1, 0, 248, 60, 128, 0, 64, 0, 0, 0, 9, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedOutput: []*ditypes.Param{
				{
					Type: "*struct",
					Size: 8,
					Kind: byte(reflect.Pointer),
					Fields: []*ditypes.Param{
						{
							Type: "struct",
							Size: 2,
							Kind: byte(reflect.Struct),
							Fields: []*ditypes.Param{
								{
									Kind:     byte(reflect.Uint),
									ValueStr: "9",
									Type:     "uint",
									Size:     8,
								},
								{
									Kind:     byte(reflect.Bool),
									ValueStr: "true",
									Type:     "bool",
									Size:     1,
								},
							},
						},
					},
				},
			},
		},
		{
			Name:   "struct pointer nil",
			Buffer: []byte{22, 8, 0, 25, 3, 0, 1, 1, 0, 2, 8, 0, 4, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedOutput: []*ditypes.Param{
				{
					Type:     "*struct",
					Size:     8,
					Kind:     byte(reflect.Pointer),
					ValueStr: "0x0",
					Fields:   nil,
				},
			},
		},
	}

	for i := range testCases {
		t.Run(testCases[i].Name, func(t *testing.T) {
			result := readParams(testCases[i].Buffer)
			for i := range result {
				if strings.HasPrefix(result[i].Type, "*") && result[i].ValueStr != "0x0" {
					result[i].ValueStr = ""
				}
			}
			assert.Equal(t, testCases[i].ExpectedOutput, result)
		})
	}
}
