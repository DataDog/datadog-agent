// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bytes"
	"testing"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// TestStreamingPackageScopeMatchesConverted verifies that streaming a
// symdb.Package through PackageScope.MarshalJSONTo produces the same JSON
// bytes as marshalling the equivalent uploader.Scope built by
// ConvertPackageToScope.
func TestStreamingPackageScopeMatchesConverted(t *testing.T) {
	cases := []struct {
		name         string
		pkg          symdb.Package
		agentVersion string
	}{
		{
			name: "rich",
			pkg: symdb.Package{
				Name: "github.com/example/foo",
				Functions: []symdb.Function{
					{
						Name:          "DoThing",
						QualifiedName: "github.com/example/foo.DoThing",
						File:          "/src/foo.go",
						InjectibleLines: []symdb.LineRange{
							{10, 12}, {20, 22},
						},
						Scope: symdb.Scope{
							StartLine: 5,
							EndLine:   30,
							Variables: []symdb.Variable{
								{
									Name:             "x",
									TypeName:         "int",
									FunctionArgument: true,
									DeclLine:         5,
								},
								{
									Name:     "tmp",
									TypeName: "*main . bigStruct",
									DeclLine: 12,
								},
							},
							Scopes: []symdb.Scope{
								{
									StartLine: 15,
									EndLine:   18,
									Variables: []symdb.Variable{
										{
											Name:     "y",
											TypeName: "string",
											DeclLine: 16,
										},
									},
								},
							},
						},
					},
					{
						Name:          "AnotherFn",
						QualifiedName: "github.com/example/foo.AnotherFn",
						File:          "/src/foo.go",
						Scope: symdb.Scope{
							StartLine: 40,
							EndLine:   45,
						},
					},
				},
				Types: map[string]*symdb.Type{
					"github.com/example/foo.Widget": {
						Name: "github.com/example/foo.Widget",
						Fields: []symdb.Field{
							{Name: "Field1", Type: "string"},
							{Name: "Field2", Type: "*main . bigStruct"},
						},
						Methods: []symdb.Function{
							{
								Name:          "Method1",
								QualifiedName: "github.com/example/foo.Widget.Method1",
								File:          "/src/widget.go",
								Scope: symdb.Scope{
									StartLine: 50,
									EndLine:   60,
								},
							},
						},
					},
				},
			},
			agentVersion: "7.72.0-test",
		},
		{
			name: "minimal_no_agent_version",
			pkg: symdb.Package{
				Name: "main",
				Functions: []symdb.Function{
					{
						Name: "f",
						Scope: symdb.Scope{
							StartLine: 1,
							EndLine:   2,
						},
					},
				},
			},
		},
		{
			name: "empty",
			pkg: symdb.Package{
				Name: "github.com/empty/pkg",
			},
			agentVersion: "1.0.0",
		},
		{
			// Function with empty QualifiedName: language_specifics must be
			// omitted (not emitted as `{}`) to match the v2 omitempty rule
			// applied to *LanguageSpecifics in convertFunctionToScope.
			name: "function_without_qualified_name",
			pkg: symdb.Package{
				Name: "main",
				Functions: []symdb.Function{
					{
						Name: "f",
						File: "/src/main.go",
						Scope: symdb.Scope{
							StartLine: 1,
							EndLine:   2,
							Variables: []symdb.Variable{
								{Name: "x", TypeName: "int", DeclLine: 1, FunctionArgument: true},
							},
						},
					},
				},
			},
		},
		{
			// Type-only package: no functions, only structs.
			name: "types_only",
			pkg: symdb.Package{
				Name: "main",
				Types: map[string]*symdb.Type{
					"main.A": {Name: "main.A", Fields: []symdb.Field{{Name: "x", Type: "int"}}},
					"main.B": {Name: "main.B"},
				},
			},
			agentVersion: "1.2.3",
		},
		{
			// Variables with AvailableLineRanges: exercises the
			// Symbol.LanguageSpecifics path so we know the streaming and
			// converted forms agree on it.
			name: "vars_with_available_ranges",
			pkg: symdb.Package{
				Name: "main",
				Functions: []symdb.Function{
					{
						Name: "f",
						File: "/src/main.go",
						Scope: symdb.Scope{
							StartLine: 1,
							EndLine:   10,
							Variables: []symdb.Variable{
								{
									Name:                "a",
									TypeName:            "int",
									DeclLine:            2,
									AvailableLineRanges: []symdb.LineRange{{2, 5}, {7, 9}},
								},
								{
									Name:             "b",
									TypeName:         "string",
									FunctionArgument: true,
									DeclLine:         1,
									// no AvailableLineRanges - should not emit
									// LanguageSpecifics at all
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			converted := ConvertPackageToScope(tc.pkg, tc.agentVersion)
			var convertedBuf, streamedBuf bytes.Buffer
			require.NoError(t, jsonv2.MarshalWrite(&convertedBuf, converted))
			require.NoError(t, jsonv2.MarshalWrite(&streamedBuf, NewPackageScope(tc.pkg, tc.agentVersion)))
			require.JSONEq(t, convertedBuf.String(), streamedBuf.String())
		})
	}
}
