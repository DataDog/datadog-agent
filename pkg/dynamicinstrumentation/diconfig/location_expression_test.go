// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
)

func TestLocationExpressionGeneration(t *testing.T) {
	testCases := []struct {
		Name              string
		ParameterMetadata bininspect.ParameterMetadata
		ExpectedOutput    []ditypes.LocationExpression
	}{
		{
			Name: "DirectlyAssignedRegisterUint",
			ParameterMetadata: bininspect.ParameterMetadata{
				TotalSize: 8,
				Kind:      reflect.Uint,
				Pieces: []bininspect.ParameterPiece{
					{Size: 8, InReg: true, StackOffset: 0, Register: 0},
				},
			},
			ExpectedOutput: []ditypes.LocationExpression{
				ditypes.ReadRegisterLocationExpression(0, 8),
				ditypes.PopLocationExpression(1, 8),
			},
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.Name, func(t *testing.T) {
			resultExpressions := GenerateLocationExpression(testcase.ParameterMetadata)

			for i := range resultExpressions {
				// make sure ID is set, then set to "" for sake of comparison for test
				require.NotEqual(t, "", resultExpressions[i].InstructionID)
				resultExpressions[i].InstructionID = ""
			}
			require.Equal(t, testcase.ExpectedOutput, resultExpressions)
		})
	}
}
