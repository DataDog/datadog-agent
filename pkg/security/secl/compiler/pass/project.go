// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
package pass

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ProjectField returns the pass projecting a rule on one of its fields: the
// projected rule only depends on the value of that field, and is true for a value
// if the rule can be true for an event with that value, whatever the rest of the
// event. It is false for a value no event with it can match: a discarder.
//
// The leaves of the boolean expression, the comparisons and the boolean values,
// that depend on anything but the field are unknown. An unknown leaf is replaced
// by the value that lets the rule be true: true, or false under an odd number of
// negations. `!(a == 1 && b == 2)` projected on `a` is `!(a == 1 && false)`, true
// for any value of `a`. Only the logical operators and the boolean negation are
// walked through, a leaf is kept or replaced as a whole.
//
// The pass doesn't fold the boolean literals it introduces, FoldBools does:
// once folded, a rule that can't be decided by the field alone, or that doesn't
// use it, projects to `true`.
//
// fieldOf returns the field a field reference refers to.
func ProjectField(field string, fieldOf func(name string) string) Pass {
	p := projection{field: field, fieldOf: fieldOf}
	return func(node ast.Node) (ast.Node, error) {
		ast.EditChildren(node, func(root ast.Node) ast.Node {
			return p.project(root, true)
		})
		return node, nil
	}
}

type projection struct {
	field   string
	fieldOf func(name string) string
}

// project projects a boolean expression, positive when it is under an even
// number of negations.
func (p projection) project(node ast.Node, positive bool) ast.Node {
	switch n := node.(type) {
	case *ast.LogicalExpr:
		n.Left = p.project(n.Left, positive)
		n.Right = p.project(n.Right, positive)
		return node
	case *ast.UnaryExpr:
		if n.Op == ast.UnaryNot {
			n.Operand = p.project(n.Operand, !positive)
			return node
		}
	}

	if !ast.Any(node, p.isUnknown) {
		return node
	}
	return &ast.Literal{Meta: ast.At(node.Position()), Value: ast.MakeBool(positive)}
}

// isUnknown returns whether the value of the node depends on anything but the
// field.
func (p projection) isUnknown(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.FieldRef:
		return p.fieldOf(n.Name) != p.field
	case *ast.VariableRef:
		return true
	case *ast.Literal:
		// an interpolation, `${variable}` or `%{field}`, is resolved at
		// evaluation; a literal that only looks like one is unknown as well,
		// which is sound
		if n.Value.Kind() != ast.String {
			return false
		}
		value := ast.StringVal(n.Value)
		return strings.Contains(value, "${") || strings.Contains(value, "%{")
	}
	return false
}
