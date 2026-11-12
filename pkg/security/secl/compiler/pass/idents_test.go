// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pass

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
)

var testConstants = map[string]ast.Node{
	"true":    &ast.Literal{Value: ast.MakeBool(true)},
	"false":   &ast.Literal{Value: ast.MakeBool(false)},
	"O_CREAT": &ast.Literal{Value: ast.MakeInt(64)},
	"O_EXCL":  &ast.Literal{Value: ast.MakeInt(128)},
	"NAMES":   &ast.Array{Members: []*ast.Literal{{Value: ast.MakeString("a")}, {Value: ast.MakeString("b")}}},
}

// testLookup returns the test constant a name refers to.
func testLookup(name string) (ast.Node, bool) {
	value, ok := testConstants[name]
	return value, ok
}

// An array of identifiers is replaced by the array of the values of its
// constants, and the parsed tree is left unchanged.
func TestResolveIdents(t *testing.T) {
	rule, err := parser.ParseRule(`open.flags in [O_CREAT, O_EXCL]`)
	require.NoError(t, err)
	original := ast.Sprint(rule)

	resolved, err := Run(rule, ResolveIdents(testLookup))
	require.NoError(t, err)
	require.Equal(t, `(in open.flags [64 128])`, ast.Sprint(resolved))
	require.Equal(t, original, ast.Sprint(rule), "the parsed tree is left unchanged")
}

func TestResolveIdentsErrors(t *testing.T) {
	for expr, expected := range map[string]string{
		`open.flags in [NAMES, O_CREAT]`: "`NAMES` can't be a member of an array, it isn't a literal",
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := parser.ParseRule(expr)
			require.NoError(t, err)

			_, err = Run(rule, ResolveIdents(testLookup))
			require.EqualError(t, err, expected)
		})
	}
}

// The root of a macro is positioned at its reference, its other nodes in the
// macro. The value of a constant is positioned at its reference, and each
// reference gets its own copy.
func TestResolveIdentsPositions(t *testing.T) {
	macro, err := parser.ParseMacro("creat", `open.flags & O_CREAT > 0`)
	require.NoError(t, err)
	// the tree of a macro is resolved before it's referred to
	tree, err := Run(macro, ResolveIdents(testLookup))
	require.NoError(t, err)
	lookup := func(name string) (ast.Node, bool) {
		if name == "creat" {
			return tree.(*ast.Macro).Root, true
		}
		value, ok := testConstants[name]
		return value, ok
	}

	rule, err := parser.ParseRule(`creat && process.name in NAMES && O_CREAT > 0`)
	require.NoError(t, err)

	resolved, err := Run(rule, ResolveIdents(lookup))
	require.NoError(t, err)
	require.Equal(t, `(&& (&& {creat (> (& open.flags 64) 0)} (in process.name ["a" "b"])) (> 64 0))`, ast.Sprint(resolved))

	root := resolved.(*ast.Rule).Root.(*ast.LogicalExpr)
	expanded := root.Left.(*ast.LogicalExpr).Left
	inMacro := expanded.(*ast.ScalarCompareExpr).Left.(*ast.BitExpr).Right
	inRule := root.Right.(*ast.ScalarCompareExpr).Left
	names := root.Left.(*ast.LogicalExpr).Right.(*ast.ArrayCompareExpr).Right.(*ast.Array)

	require.Equal(t, ast.Position{Offset: 0, Line: 1, Column: 1}, expanded.Position())
	require.Equal(t, ast.Position{Offset: 13, Line: 1, Column: 14, Source: "creat"}, inMacro.Position())
	require.Equal(t, ast.Position{Offset: 34, Line: 1, Column: 35}, inRule.Position())
	require.NotSame(t, inMacro, inRule)
	require.NotSame(t, testConstants["O_CREAT"], inRule)
	for _, member := range names.Members {
		require.Equal(t, names.Position(), member.Position())
	}
}
