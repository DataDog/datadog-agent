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
// The operations on boolean literals are then folded, a rule that can't be
// decided by the field alone, or that doesn't use it, projects to `true`.
//
// fieldOf returns the field an identifier refers to, false for a constant.
func ProjectField(field string, fieldOf func(name string) (string, bool)) Pass {
	p := projection{field: field, fieldOf: fieldOf}
	return func(node ast.Node) ast.Node {
		ast.EditChildren(node, func(root ast.Node) ast.Node {
			return p.project(root, true)
		})
		return node
	}
}

type projection struct {
	field   string
	fieldOf func(name string) (string, bool)
}

// project projects a boolean expression, positive when it is under an even
// number of negations.
func (p projection) project(node ast.Node, positive bool) ast.Node {
	switch node := node.(type) {
	case *ast.LogicalExpr:
		node.Left = p.project(node.Left, positive)
		node.Right = p.project(node.Right, positive)
		return foldLogical(node)
	case *ast.UnaryExpr:
		if node.Op != ast.UnaryNot {
			break
		}
		node.Operand = p.project(node.Operand, !positive)
		if operand, ok := node.Operand.(*ast.BoolLiteral); ok {
			return &ast.BoolLiteral{Meta: node.Meta, Value: !operand.Value}
		}
		return node
	}

	if p.isDecidable(node) {
		return node
	}
	return &ast.BoolLiteral{Meta: ast.At(node.Position()), Value: positive}
}

// isDecidable returns whether the value of the tree only depends on the field.
func (p projection) isDecidable(node ast.Node) bool {
	decidable := true
	var walk func(ast.Node) ast.Node
	walk = func(node ast.Node) ast.Node {
		switch node := node.(type) {
		case *ast.Ident:
			if field, ok := p.fieldOf(node.Name); ok && field != p.field {
				decidable = false
			}
		case *ast.FieldRef:
			if node.Name != p.field {
				decidable = false
			}
		case *ast.VariableRef:
			decidable = false
		case *ast.StringLiteral:
			// an interpolation, `${variable}` or `%{field}`, is resolved at
			// evaluation; a literal that only looks like one is undecidable as
			// well, which is sound
			if strings.Contains(node.Value, "${") || strings.Contains(node.Value, "%{") {
				decidable = false
			}
		}
		ast.EditChildren(node, walk)
		return node
	}
	walk(node)
	return decidable
}

// foldLogical folds a logical operation having a boolean literal operand.
func foldLogical(node *ast.LogicalExpr) ast.Node {
	// the value making the operation equal to that value: false for &&, true
	// for ||
	absorbing := node.Op == ast.LogicalOr
	for _, operands := range [2][2]ast.Node{{node.Left, node.Right}, {node.Right, node.Left}} {
		literal, ok := operands[0].(*ast.BoolLiteral)
		if !ok {
			continue
		}
		if literal.Value == absorbing {
			return &ast.BoolLiteral{Meta: node.Meta, Value: absorbing}
		}
		return operands[1]
	}
	return node
}
