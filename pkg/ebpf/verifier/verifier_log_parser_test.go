// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package verifier

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRegisterState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *RegisterState
	}{
		{
			name:  "SingleScalar",
			input: "R0=inv0",
			expected: &RegisterState{
				Register: 0,
				Live:     "",
				Type:     "scalar",
				Value:    "0",
				Precise:  false,
			},
		},
		{
			name:  "WithOnlyMaxValues",
			input: "R2_w=inv(id=2,smax_value=9223372032559808512,umax_value=18446744069414584320,var_off=(0x0;0xffffffff00000000),s32_min_value=0,s32_max_value=0,u32_max_value=0)",
			expected: &RegisterState{
				Register: 2,
				Live:     "written",
				Type:     "scalar",
				Value:    "[0, 2^63 - 1]",
				Precise:  false,
			},
		},
		{
			name:  "ScalarWithoutValue",
			input: "R2_w=scalar()",
			expected: &RegisterState{
				Register: 2,
				Live:     "written",
				Type:     "scalar",
				Value:    "?",
				Precise:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := singleRegStateRegex.FindStringSubmatch(tt.input)
			result, err := parseRegisterState(parts)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogParsingWithRegisterState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[int]*InstructionInfo
	}{
		{
			name:  "RegisterStateSeparate",
			input: "5: (63) *(u32 *)(r10 -16) = r2\n6: R1_w=0 fp-16_w=00000000\n",
			expected: map[int]*InstructionInfo{
				5: {
					Index:          5,
					TimesProcessed: 1,
					Source:         nil,
					Code:           "*(u32 *)(r10 -16) = r2",
					RegisterState: map[int]*RegisterState{
						1: {
							Register: 1,
							Live:     "written",
							Type:     "scalar",
							Value:    "0",
							Precise:  false,
						},
					},
					RegisterStateRaw: "R1_w=0 fp-16_w=00000000",
				},
			},
		},
		{
			name:  "RegisterStateAttachedToInsn",
			input: "5: (63) *(u32 *)(r10 -16) = r2     ; R2_w=scalar() fp-16_w=0000mmmm\n",
			expected: map[int]*InstructionInfo{
				5: {
					Index:          5,
					TimesProcessed: 1,
					Source:         nil,
					Code:           "*(u32 *)(r10 -16) = r2",
					RegisterState: map[int]*RegisterState{
						2: {
							Register: 2,
							Live:     "written",
							Type:     "scalar",
							Value:    "?",
							Precise:  false,
						},
					},
					RegisterStateRaw: "R2_w=scalar() fp-16_w=0000mmmm",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vlp := newVerifierLogParser(nil)
			_, err := vlp.parseVerifierLog(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, vlp.complexity.InsnMap)
		})
	}
}

func TestLogParsingWith(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		progSourceMap map[int]*SourceLine
		expected      ComplexityInfo
	}{
		{
			name:  "SingleInstructionForLine",
			input: "5: (63) *(u32 *)(r10 -16) = r2\n6: R1_w=0 fp-16_w=00000000",
			progSourceMap: map[int]*SourceLine{
				5: {
					LineInfo: "file.c:5",
					Line:     "int a = 2",
				},
			},
			expected: ComplexityInfo{
				InsnMap: map[int]*InstructionInfo{
					5: {
						Index:          5,
						TimesProcessed: 1,
						Source: &SourceLine{
							LineInfo: "file.c:5",
							Line:     "int a = 2",
						},
						Code: "*(u32 *)(r10 -16) = r2",
						RegisterState: map[int]*RegisterState{
							1: {
								Register: 1,
								Live:     "written",
								Type:     "scalar",
								Value:    "0",
								Precise:  false,
							},
						},
						RegisterStateRaw: "R1_w=0 fp-16_w=00000000",
					},
				},
				SourceMap: map[string]*SourceLineStats{
					"file.c:5": {
						NumInstructions:            1,
						MaxPasses:                  1,
						MinPasses:                  1,
						TotalInstructionsProcessed: 1,
						AssemblyInsns:              []int{5},
					},
				},
			},
		},
		{
			name:  "MultipleInstructionsForLine",
			input: "4: (63) *(u32 *)(r10 -16) = r3\n5: (63) *(u32 *)(r10 -16) = r2\n",
			progSourceMap: map[int]*SourceLine{
				5: {
					LineInfo: "file.c:5",
					Line:     "int a = 2",
				},
				4: {
					LineInfo: "file.c:5",
					Line:     "int a = 2",
				},
			},
			expected: ComplexityInfo{
				InsnMap: map[int]*InstructionInfo{
					4: {
						Index:          4,
						TimesProcessed: 1,
						Source: &SourceLine{
							LineInfo: "file.c:5",
							Line:     "int a = 2",
						},
						Code:             "*(u32 *)(r10 -16) = r3",
						RegisterState:    nil,
						RegisterStateRaw: "",
					},
					5: {
						Index:          5,
						TimesProcessed: 1,
						Source: &SourceLine{
							LineInfo: "file.c:5",
							Line:     "int a = 2",
						},
						Code:             "*(u32 *)(r10 -16) = r2",
						RegisterState:    nil,
						RegisterStateRaw: "",
					},
				},
				SourceMap: map[string]*SourceLineStats{
					"file.c:5": {
						NumInstructions:            2,
						MaxPasses:                  1,
						MinPasses:                  1,
						TotalInstructionsProcessed: 2,
						AssemblyInsns:              []int{4, 5},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vlp := newVerifierLogParser(tt.progSourceMap)
			_, err := vlp.parseVerifierLog(tt.input)
			require.NoError(t, err)

			// Fix to avoid flakiness, sort the assembly instructions
			// as they might be in a different order, due to dict iteration
			// order being random.
			for _, lineData := range vlp.complexity.SourceMap {
				sort.Ints(lineData.AssemblyInsns)
			}

			require.Equal(t, tt.expected, vlp.complexity)
		})
	}
}

func TestTryPowerOfTwoRepresentation(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "MaxInt64",
			input:    math.MaxInt64,
			expected: "2^63 - 1",
		},
		{
			name:     "MinInt64",
			input:    math.MinInt64,
			expected: "-2^63",
		},
		{
			name:     "ExactPowerOfTwo",
			input:    1024,
			expected: "2^10",
		},
		{
			name:     "NegativeExactPowerOfTwo",
			input:    -1024,
			expected: "-2^10",
		},
		{
			name:     "PowerOfTwoMinusOne",
			input:    1023,
			expected: "2^10 - 1",
		},
		{
			name:     "NotPowerOfTwo",
			input:    0x5,
			expected: "5 (0x5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tryPowerOfTwoRepresentation(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
