// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen is used to generate bpf program source code based on probe definitions
package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

func TestApplyCaptureDepth(t *testing.T) {
	tests := []struct {
		name           string
		parameters     []*ditypes.Parameter
		targetDepth    int
		expectedResult []*ditypes.Parameter
	}{
		{
			name:           "Capture Depth Larger Than Tree Depth",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    20,
		},
		{
			name:           "Capture Depth 0",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    0,
		},
		{
			name:           "Capture Depth All Get Cut Off",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    1,
		},
		{
			name:           "Capture Depth One Leaf Cut Off",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    1,
		},
		{
			name:           "Nils",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    2,
		},
		{
			name:           "Empties",
			parameters:     []*ditypes.Parameter{},
			expectedResult: []*ditypes.Parameter{},
			targetDepth:    2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

		})
	}
}
