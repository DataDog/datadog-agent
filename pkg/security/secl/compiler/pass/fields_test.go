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

func TestResolveFields(t *testing.T) {
	legacy := map[string]string{"container.id": "process.container.id"}

	for _, test := range []struct {
		expr     string
		expected *ast.FieldRef
	}{
		{`process.file.name`, &ast.FieldRef{Name: "process.file.name"}},
		{`async`, &ast.FieldRef{Name: "async"}},
		{`container.id`, &ast.FieldRef{Name: "process.container.id"}},
		{`process.ancestors[A].file.name`, &ast.FieldRef{Name: "process.ancestors.file.name", Iter: &ast.Iteration{Iterator: "process.ancestors", Register: "A"}}},
		{`process.argv[A]`, &ast.FieldRef{Name: "process.argv", Iter: &ast.Iteration{Iterator: "process.argv", Register: "A"}}},
	} {
		t.Run(test.expr, func(t *testing.T) {
			root, err := parser.ParseExpression(test.expr)
			require.NoError(t, err)
			original := ast.Sprint(root)

			resolved, err := Run(root, ResolveFields(legacy))
			require.NoError(t, err)
			field, ok := resolved.(*ast.FieldRef)
			require.True(t, ok, "%s is a field reference", ast.Sprint(resolved))
			require.Equal(t, ast.Position{Offset: 0, Line: 1, Column: 1}, field.Position())
			field.Meta = ast.Meta{}
			require.Equal(t, test.expected, field)
			require.Equal(t, original, ast.Sprint(root), "the parsed tree is left unchanged")

			// written as a field reference, the name refers to the same field
			root, err = parser.ParseExpression("%{" + test.expr + "}")
			require.NoError(t, err)
			resolved, err = Run(root, ResolveFields(legacy))
			require.NoError(t, err)
			field, ok = resolved.(*ast.FieldRef)
			require.True(t, ok, "%s is a field reference", ast.Sprint(resolved))
			field.Meta = ast.Meta{}
			require.Equal(t, test.expected, field)
		})
	}
}

// An integer subscript ending a name is an index in the array field, kept as an
// Index of the reference to the field.
func TestResolveFieldsIndex(t *testing.T) {
	legacy := map[string]string{"process.args_list": "process.argv"}

	for expr, expected := range map[string]struct {
		field string
		index int
	}{
		`process.argv[0]`:                {"process.argv", 0},
		`process.ancestors.file.name[1]`: {"process.ancestors.file.name", 1},
		`process.args_list[2]`:           {"process.argv", 2},
	} {
		t.Run(expr, func(t *testing.T) {
			root, err := parser.ParseExpression(expr)
			require.NoError(t, err)

			resolved, err := Run(root, ResolveFields(legacy))
			require.NoError(t, err)
			index, ok := resolved.(*ast.Index)
			require.True(t, ok, "%s is an index", ast.Sprint(resolved))
			field, ok := index.Base.(*ast.FieldRef)
			require.True(t, ok, "%s is a field reference", ast.Sprint(index.Base))
			require.Equal(t, expected.field, field.Name)
			require.Nil(t, field.Iter)
			literal, ok := index.Subscript.(*ast.Literal)
			require.True(t, ok)
			require.Equal(t, ast.MakeInt(expected.index), literal.Value)
		})
	}
}

// A tree already resolved, the one of a macro inserted in a rule, is left as is.
func TestResolveFieldsResolved(t *testing.T) {
	rule, err := parser.ParseRule(`process.argv[1] == "-c" && process.ancestors[A].uid == 0`)
	require.NoError(t, err)

	once, err := Run(rule, ResolveFields(nil))
	require.NoError(t, err)
	twice, err := Run(once, ResolveFields(nil))
	require.NoError(t, err)
	require.Equal(t, ast.Sprint(once), ast.Sprint(twice))
}

// The names left once the macros and the constants are resolved are fields,
// written the way they are in the rule.
func TestResolveFieldsAfterIdents(t *testing.T) {
	rule, err := parser.ParseRule(`process.ancestors[A].uid == 0 && open.flags & O_CREAT > 0 && process.argv[1] == "-c"`)
	require.NoError(t, err)

	resolved, err := Run(rule, ResolveIdents(testLookup), ResolveFields(nil))
	require.NoError(t, err)
	require.Equal(t, `(&& (&& (== process.ancestors[A].uid 0) (> (& open.flags 64) 0)) (== process.argv[1] "-c"))`, ast.Sprint(resolved))
	require.False(t, ast.Any(resolved, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.Ident, *ast.Selector, *ast.Index:
			return isName(node)
		}
		return false
	}), "no name is left")
}

func TestResolveFieldsErrors(t *testing.T) {
	for expr, expected := range map[string]string{
		`process.ancestors[A].argv[0] == "a"`: "`process.ancestors[A].argv[0]` has more than one subscript",
		`process.ancestors[A][B].uid == 0`:    "`process.ancestors[A][B].uid` has more than one subscript",
		`process.ancestors[0].uid == 0`:       "`process.ancestors[0].uid`: cannot index the iterator `process.ancestors` with `0`, use an iterator variable instead",
		`process.ancestors["a"].uid == 0`:     "`process.ancestors[\"a\"].uid`: cannot index the iterator `process.ancestors` with `\"a\"`, use an iterator variable instead",
		`process.argv[1s] == "a"`:             "`process.argv[1000000000ns]`: a subscript is an iterator variable or an integer index",
		`process.argv["a"] == "a"`:            "`process.argv[\"a\"]`: a subscript is an iterator variable or an integer index",
		`process.argv[a.b] == "a"`:            "`process.argv[a.b]`: a subscript is an iterator variable or an integer index",
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := parser.ParseRule(expr)
			require.NoError(t, err)

			_, err = Run(rule, ResolveFields(nil))
			require.EqualError(t, err, expected)
		})
	}
}
