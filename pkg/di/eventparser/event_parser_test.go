package eventparser

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/kr/pretty"
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
		expectedResult []ditypes.Param
	}{
		{
			name: "Basic slice of structs",
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
				1, 2, 0, 3, // Content of slice element 1 (not relevant for this function)
				4, 5, 0, 6, // Content of slice element 2 (not relevant for this function)
				// Padding
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0,
			},
			expectedResult: []ditypes.Param{{
				Type: "slice", Size: 0x2, Kind: 0x17,
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
			if !reflect.DeepEqual(output, tt.expectedResult) {
				pretty.Log(output)
				pretty.Log(tt.expectedResult)
				t.Errorf("Didn't read correctly!")
			}
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
				1, 2, 0, 3, // Content of slice element 1 (not relevant for this function)
				4, 5, 0, 6, // Content of slice element 2 (not relevant for this function)
				// Padding
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			expectedResult: &ditypes.Param{
				Type: "slice", Size: 0x2, Kind: 0x17,
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
			},
			expectedResult: &ditypes.Param{
				Type: "slice", Size: 0x2, Kind: 0x17,
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
			if !reflect.DeepEqual(typeDefinition, tt.expectedResult) {
				pretty.Log(typeDefinition)
				pretty.Log(tt.expectedResult)
				t.Errorf("Not equal!")
			}
		})
	}
}
