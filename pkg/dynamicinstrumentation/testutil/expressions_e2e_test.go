// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

var packageName = "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample"

func TestExpressionsInstrumentation(t *testing.T) {
	testCases := []struct {
		Name            string
		Expressions     []ditypes.LocationExpression
		FuncName        string
		ExpectedArgData []*ditypes.Param
	}{
		{
			Name:     "test1",
			FuncName: "test_single_uint",
			Expressions: []ditypes.LocationExpression{
				ditypes.ReadRegisterLocationExpression(0, 8),
				ditypes.PopLocationExpression(1, 8),
			},
			ExpectedArgData: []*ditypes.Param{
				{
					ValueStr: "1512",
					Type:     "uint",
					Size:     0x8,
					Kind:     0x7,
					Fields:   nil,
				},
			},
		},

		{
			Name:        "test2",
			FuncName:    "test_uint_slice",
			Expressions: []ditypes.LocationExpression{
				/*
					Read reg 0 ptr
					Apply offset 0
					Read reg 1 len
					Dynamic dereference 8
					Read reg 0 ptr
					Apply offset 8
					Read reg 1 len
					Dynamic dereference 8
					Read reg 0 ptr
					Apply offset 16
					Read reg 1 len
					Dynamic dereference 8
				*/
			},
			ExpectedArgData: []*ditypes.Param{
				{
					ValueStr: "",
					Type:     "slice",
					Size:     0x3,
					Kind:     0x17,
					Fields: []*ditypes.Param{
						{
							ValueStr: "1",
							Type:     "uint",
							Size:     0x8,
							Kind:     0x7,
							Fields:   nil,
						},
						{
							ValueStr: "2",
							Type:     "uint",
							Size:     0x8,
							Kind:     0x7,
							Fields:   nil,
						},
						{
							ValueStr: "3",
							Type:     "uint",
							Size:     0x8,
							Kind:     0x7,
							Fields:   nil,
						},
					},
				},
			},
		},
		{
			Name:        "test3",
			FuncName:    "test_single_string",
			Expressions: []ditypes.LocationExpression{},
			ExpectedArgData: []*ditypes.Param{
				{
					ValueStr: "abc",
					Type:     "string",
					Size:     0x3,
					Kind:     0x18,
					Fields:   nil,
				},
			},
		},
		{
			Name:        "test4",
			FuncName:    "test_pointer_to_simple_struct",
			Expressions: []ditypes.LocationExpression{},
			ExpectedArgData: []*ditypes.Param{
				&ditypes.Param{
					ValueStr: "0x400059FD50",
					Type:     "ptr",
					Size:     0x8,
					Kind:     0x16,
					Fields: []*ditypes.Param{
						&ditypes.Param{
							ValueStr: "",
							Type:     "struct",
							Size:     0x2,
							Kind:     0x19,
							Fields: []*ditypes.Param{
								&ditypes.Param{
									ValueStr: "9",
									Type:     "uint",
									Size:     0x8,
									Kind:     0x7,
									Fields:   nil,
								},
								&ditypes.Param{
									ValueStr: "true",
									Type:     "bool",
									Size:     0x1,
									Kind:     0x1,
									Fields:   nil,
								},
							},
						},
					},
				},
			},
		},
	}
	for _, testcase := range testCases {
		t.Run(testcase.Name, func(t *testing.T) {
			// Compile/run sample service
			// Go from list of expressions to fully formed bpf code, to compilation and attachment
		})
	}
}
