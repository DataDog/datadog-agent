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
)

func TestLocationExpressionGeneration(t *testing.T) {
	testCases := []struct {
		Name           string
		Parameter      *ditypes.Parameter
		Limits         *ditypes.InstrumentationInfo
		ExpectedOutput []ditypes.LocationExpression
	}{
		{
			Name: "DirectlyAssignedRegisterUint",
			Parameter: &ditypes.Parameter{
				Type:      "uint",
				Kind:      uint(reflect.Uint),
				TotalSize: 8,
				Location: &ditypes.Location{
					InReg:            true,
					Register:         1,
					PointerOffset:    9999, // should not be used
					StackOffset:      8888, // should not be used
					NeedsDereference: true, // should not be used
				},
			},
			ExpectedOutput: []ditypes.LocationExpression{
				ditypes.ReadRegisterLocationExpression(1, 8),
				ditypes.PopLocationExpression(1, 8),
			},
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.Name, func(t *testing.T) {
			GenerateLocationExpression(testcase.Limits, testcase.Parameter)
			resultExpressions := collectAllLocationExpressions(testcase.Parameter, true)
			require.Equal(t, testcase.ExpectedOutput, resultExpressions)
		})
	}
}
