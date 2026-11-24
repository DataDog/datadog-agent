// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
package pass

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// NegationNormalForm returns the pass rewriting a rule in negation normal form:
// the negations only apply to the leaves of the boolean expression, the
// comparisons and the boolean values. `!(a == 1 && b)` is `a != 1 || !b`.
//
// A negated equality, match or membership test is the opposite test: `!(a == 1)`
// is `a != 1`, `!(a =~ "x*")` is `a !~ "x*"`, `!(a in [1, 2])` is
// `a not in [1, 2]`. The other negated leaves keep their negation: an ordering
// of an array field is true if one of the elements is ordered, `!(a < 1)` isn't
// `a >= 1`.
func NegationNormalForm() Pass {
	return func(node ast.Node) (ast.Node, error) {
		ast.EditChildren(node, func(root ast.Node) ast.Node {
			return negationNormalForm(root, false)
		})
		return node, nil
	}
}

// oppositeScalarOps are the scalar comparisons true when the other is false,
// whatever the operands.
var oppositeScalarOps = map[ast.ScalarOp]ast.ScalarOp{
	ast.ScalarEqual:    ast.ScalarNotEqual,
	ast.ScalarNotEqual: ast.ScalarEqual,
	ast.ScalarMatch:    ast.ScalarNotMatch,
	ast.ScalarNotMatch: ast.ScalarMatch,
}

// oppositeArrayOps are the membership tests true when the other is false.
var oppositeArrayOps = map[ast.ArrayOp]ast.ArrayOp{
	ast.ArrayIn:    ast.ArrayNotIn,
	ast.ArrayNotIn: ast.ArrayIn,
}

// negationNormalForm returns the negation normal form of a boolean expression,
// of its negation if negated.
func negationNormalForm(node ast.Node, negated bool) ast.Node {
	switch n := node.(type) {
	case *ast.LogicalExpr:
		if negated {
			if n.Op == ast.LogicalAnd {
				n.Op = ast.LogicalOr
			} else {
				n.Op = ast.LogicalAnd
			}
		}
		n.Left = negationNormalForm(n.Left, negated)
		n.Right = negationNormalForm(n.Right, negated)
		return n
	case *ast.UnaryExpr:
		if n.Op == ast.UnaryNot {
			return negationNormalForm(n.Operand, !negated)
		}
	case *ast.ScalarCompareExpr:
		if op, ok := oppositeScalarOps[n.Op]; ok && negated {
			n.Op = op
			return n
		}
	case *ast.ArrayCompareExpr:
		if op, ok := oppositeArrayOps[n.Op]; ok && negated {
			n.Op = op
			return n
		}
	}

	if negated {
		return &ast.UnaryExpr{Meta: ast.At(node.Position()), Op: ast.UnaryNot, Operand: node}
	}
	return node
}
