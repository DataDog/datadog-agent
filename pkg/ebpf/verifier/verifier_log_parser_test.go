// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package verifier

import (
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

// 5: R1_w=0 R10=fp0 fp-16_w=00000000
// 5: (63) *(u32 *)(r10 -16) = r2        ; R2_w=scalar() R10=fp0 fp-16_w=0000mmmm

func TestLogParsingWithRegisterState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected InstructionInfo
	}{
		{
			name:  "RegisterStateBeforeInsn",
			input: "5: R1_w=0 fp-16_w=00000000\n5: (63) *(u32 *)(r10 -16) = r2\n",
			expected: InstructionInfo{
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
				RegisterStateRaw: "5: R1_w=0 fp-16_w=00000000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vlp := newVerifierLogParser(nil)
			_, err := vlp.parseVerifierLog(tt.input)
			require.NoError(t, err)
			insInfo := vlp.complexity.InsnMap[5]
			require.Equal(t, &tt.expected, insInfo)
		})
	}
}
