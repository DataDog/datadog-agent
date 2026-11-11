// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ast

import "fmt"

// LogicalOp is the operator of a LogicalExpr.
type LogicalOp int

// Logical operators.
const (
	LogicalAnd LogicalOp = iota + 1 // also spelled `and`
	LogicalOr                       // also spelled `or`
)

func (op LogicalOp) String() string {
	switch op {
	case LogicalAnd:
		return "&&"
	case LogicalOr:
		return "||"
	}
	return fmt.Sprintf("LogicalOp(%d)", int(op))
}

// ScalarOp is the operator of a ScalarCompareExpr.
type ScalarOp int

// Scalar comparison operators.
const (
	ScalarEqual ScalarOp = iota + 1
	ScalarNotEqual
	ScalarLesser
	ScalarLesserOrEqual
	ScalarGreater
	ScalarGreaterOrEqual
	ScalarMatch    // `==`, reading a string operand as a pattern
	ScalarNotMatch // `!=`, reading a string operand as a pattern
)

func (op ScalarOp) String() string {
	switch op {
	case ScalarEqual:
		return "=="
	case ScalarNotEqual:
		return "!="
	case ScalarLesser:
		return "<"
	case ScalarLesserOrEqual:
		return "<="
	case ScalarGreater:
		return ">"
	case ScalarGreaterOrEqual:
		return ">="
	case ScalarMatch:
		return "=~"
	case ScalarNotMatch:
		return "!~"
	}
	return fmt.Sprintf("ScalarOp(%d)", int(op))
}

// ArrayOp is the operator of an ArrayCompareExpr.
type ArrayOp int

// Array comparison operators.
const (
	ArrayIn ArrayOp = iota + 1
	ArrayNotIn
	ArrayAllIn
)

func (op ArrayOp) String() string {
	switch op {
	case ArrayIn:
		return "in"
	case ArrayNotIn:
		return "notin"
	case ArrayAllIn:
		return "allin"
	}
	return fmt.Sprintf("ArrayOp(%d)", int(op))
}

// BitOp is the operator of a BitExpr.
type BitOp int

// Bitwise operators.
const (
	BitAnd BitOp = iota + 1
	BitOr
	BitXor
)

func (op BitOp) String() string {
	switch op {
	case BitAnd:
		return "&"
	case BitOr:
		return "|"
	case BitXor:
		return "^"
	}
	return fmt.Sprintf("BitOp(%d)", int(op))
}

// ArithOp is the operator of an ArithExpr.
type ArithOp int

// Arithmetic operators.
const (
	ArithAdd ArithOp = iota + 1
	ArithSub
)

func (op ArithOp) String() string {
	switch op {
	case ArithAdd:
		return "+"
	case ArithSub:
		return "-"
	}
	return fmt.Sprintf("ArithOp(%d)", int(op))
}

// UnaryOp is the operator of a UnaryExpr.
type UnaryOp int

// Unary operators.
const (
	UnaryNot UnaryOp = iota + 1 // also spelled `not`
	UnaryNeg
	UnaryBitNot
)

func (op UnaryOp) String() string {
	switch op {
	case UnaryNot:
		return "!"
	case UnaryNeg:
		return "-"
	case UnaryBitNot:
		return "^"
	}
	return fmt.Sprintf("UnaryOp(%d)", int(op))
}
