// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ast

// IntConstant returns the value of a constant integer expression: an integer,
// or an operation on constant integers, `O_CREAT | O_EXCL`.
func IntConstant(node Node) (int, bool) {
	switch n := node.(type) {
	case *Literal:
		if n.Value.Kind() == Int {
			return IntVal(n.Value), true
		}
	case *BitExpr:
		left, leftOk := IntConstant(n.Left)
		right, rightOk := IntConstant(n.Right)
		if !leftOk || !rightOk {
			break
		}
		switch n.Op {
		case BitAnd:
			return left & right, true
		case BitOr:
			return left | right, true
		case BitXor:
			return left ^ right, true
		}
	case *UnaryExpr:
		operand, ok := IntConstant(n.Operand)
		if !ok {
			break
		}
		switch n.Op {
		case UnaryNeg:
			return -operand, true
		case UnaryBitNot:
			return ^operand, true
		}
	}
	return 0, false
}
