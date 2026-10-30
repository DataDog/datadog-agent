// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
package pass

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// FoldBools returns the pass folding the boolean operations having a boolean
// literal operand: `a && true` is `a`, `a || true` is `true`, `!false` is
// `true`.
//
// Only the logical operators and the boolean negation are folded, the other
// operations on literals are left to the compiler.
func FoldBools() Pass {
	return func(node ast.Node) (ast.Node, error) {
		return foldBools(node), nil
	}
}

// foldBools returns the node, the boolean operations of its tree folded.
func foldBools(node ast.Node) ast.Node {
	ast.EditChildren(node, foldBools)

	switch n := node.(type) {
	case *ast.LogicalExpr:
		return foldLogical(n)
	case *ast.UnaryExpr:
		if value, ok := boolValue(n.Operand); ok && n.Op == ast.UnaryNot {
			return &ast.Literal{Meta: n.Meta, Value: ast.MakeBool(!value)}
		}
	}
	return node
}

// foldLogical folds a logical operation having a boolean literal operand.
func foldLogical(node *ast.LogicalExpr) ast.Node {
	// the value making the operation equal to that value: false for &&, true
	// for ||
	absorbing := node.Op == ast.LogicalOr
	for _, operands := range [2][2]ast.Node{{node.Left, node.Right}, {node.Right, node.Left}} {
		value, ok := boolValue(operands[0])
		if !ok {
			continue
		}
		if value == absorbing {
			return &ast.Literal{Meta: node.Meta, Value: ast.MakeBool(absorbing)}
		}
		return operands[1]
	}
	return node
}

// boolValue returns the value of a boolean literal, false if the node isn't one.
func boolValue(node ast.Node) (bool, bool) {
	if literal, ok := node.(*ast.Literal); ok && literal.Value.Kind() == ast.Bool {
		return ast.BoolVal(literal.Value), true
	}
	return false, false
}
