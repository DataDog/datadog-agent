// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdbprinter

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
)

// TestSerializeScope_Function spot-checks the function/scope/symbol output
// against a hand-crafted expected string. The intent is to keep the format
// pinned to what the symdb golden tests expect; any format change here
// requires regenerating goldens.
func TestSerializeScope_Function(t *testing.T) {
	pkg := symdb.Package{
		Name: "main",
		Functions: []symdb.Function{
			{
				Name:          "f",
				QualifiedName: "main.f",
				File:          "/src/main.go",
				InjectibleLines: []symdb.LineRange{
					{1, 3}, {5, 5},
				},
				Scope: symdb.Scope{
					StartLine: 1,
					EndLine:   10,
					Variables: []symdb.Variable{
						{
							Name:                "x",
							TypeName:            "int",
							FunctionArgument:    true,
							DeclLine:            1,
							AvailableLineRanges: []symdb.LineRange{{1, 5}},
						},
						{
							Name:     "y",
							TypeName: "string",
							DeclLine: 2,
							// no AvailableLineRanges
						},
					},
					Scopes: []symdb.Scope{
						{
							StartLine: 4, EndLine: 6,
							Variables: []symdb.Variable{
								{
									Name:     "z",
									TypeName: "bool",
									DeclLine: 5,
								},
							},
						},
					},
				},
			},
		},
	}
	scope := uploader.ConvertPackageToScope(pkg, "")
	var buf bytes.Buffer
	require.NoError(t, SerializeScope(&buf, scope))
	const expected = "Package: main\n" +
		"\tFunction: f (main.f) in /src/main.go [1:10] injectible: [1-3], [5-5]\n" +
		"\t\tArg: x: int (declared at line 1, available: [1-5])\n" +
		"\t\tVar: y: string (declared at line 2, available: )\n" +
		"\t\tScope: 4-6\n" +
		"\t\t\tVar: z: bool (declared at line 5, available: )\n"
	require.Equal(t, expected, buf.String())
}

// TestSerializeScope_Type pins the type/method/field output.
func TestSerializeScope_Type(t *testing.T) {
	pkg := symdb.Package{
		Name: "main",
		Types: map[string]*symdb.Type{
			"main.T": {
				Name: "main.T",
				Fields: []symdb.Field{
					{Name: "a", Type: "int"},
					{Name: "b", Type: "*main.inner"},
				},
				Methods: []symdb.Function{
					{
						Name:          "M",
						QualifiedName: "main.T.M",
						File:          "/src/main.go",
						Scope: symdb.Scope{
							StartLine: 20, EndLine: 25,
						},
					},
				},
			},
		},
	}
	scope := uploader.ConvertPackageToScope(pkg, "")
	var buf bytes.Buffer
	require.NoError(t, SerializeScope(&buf, scope))
	const expected = "Package: main\n" +
		"\tType: main.T\n" +
		"\t\tField: a: int\n" +
		"\t\tField: b: *main.inner\n" +
		"\t\tFunction: M (main.T.M) in /src/main.go [20:25] injectible: \n"
	require.Equal(t, expected, buf.String())
}
