// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSliceHeader(t *testing.T) {
	tests := []struct {
		name          string
		slice         *ditypes.Parameter
		expectedError error
	}{
		{
			name:          "nil slice",
			slice:         nil,
			expectedError: errors.New("malformed slice type"),
		},
		{
			name: "malformed slice",
			slice: &ditypes.Parameter{
				Name: "foo",
				ParameterPieces: []*ditypes.Parameter{
					nil,
					nil,
					nil,
				},
			},
			expectedError: errors.New("malformed slice type"),
		},
		{
			name: "invalid uint slice, no cap",
			slice: &ditypes.Parameter{
				Kind: uint(reflect.Slice),
				ParameterPieces: []*ditypes.Parameter{
					{
						Name: "array",
						Type: "*uint",
						ParameterPieces: []*ditypes.Parameter{
							{
								Type:            "uint",
								Kind:            uint(reflect.Uint),
								TotalSize:       8,
								ParameterPieces: nil,
							},
						},
					},
					{
						Name: "len",
						Type: "int",
						Kind: uint(reflect.Int),
					},
				},
			},
			expectedError: errors.New("malformed slice type"),
		},
		{
			name: "invalid uint slice, no underlying type",
			slice: &ditypes.Parameter{
				Kind: uint(reflect.Slice),
				ParameterPieces: []*ditypes.Parameter{
					{
						Name:            "array",
						Type:            "*uint",
						ParameterPieces: []*ditypes.Parameter{},
					},
					{
						Name: "len",
						Type: "int",
						Kind: uint(reflect.Int),
					},
					{
						Name: "cap",
						Type: "int",
						Kind: uint(reflect.Int),
					},
				},
			},
			expectedError: errors.New("malformed slice type"),
		},
		{
			name: "valid uint slice",
			slice: &ditypes.Parameter{
				Kind: uint(reflect.Slice),
				ParameterPieces: []*ditypes.Parameter{
					{
						Name: "array",
						Type: "*uint",
						ParameterPieces: []*ditypes.Parameter{
							{
								Type:            "uint",
								Kind:            uint(reflect.Uint),
								TotalSize:       8,
								ParameterPieces: nil,
							},
						},
					},
					{
						Name: "len",
						Type: "int",
						Kind: uint(reflect.Int),
					},
					{
						Name: "cap",
						Type: "int",
						Kind: uint(reflect.Int),
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := generateSliceHeader(tt.slice, &out)
			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
