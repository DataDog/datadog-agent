// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

import "fmt"

// LogicalOp is the operator of a LogicalExpr.
type LogicalOp int

// Logical operators. The zero value is not a valid operator.
const (
	// LogicalAnd is the `&&` operator, also spelled `and`.
	LogicalAnd LogicalOp = iota + 1
	// LogicalOr is the `||` operator, also spelled `or`.
	LogicalOr
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

// Scalar comparison operators. The zero value is not a valid operator.
const (
	// ScalarEqual is the `==` operator.
	ScalarEqual ScalarOp = iota + 1
	// ScalarNotEqual is the `!=` operator.
	ScalarNotEqual
	// ScalarLesser is the `<` operator.
	ScalarLesser
	// ScalarLesserOrEqual is the `<=` operator.
	ScalarLesserOrEqual
	// ScalarGreater is the `>` operator.
	ScalarGreater
	// ScalarGreaterOrEqual is the `>=` operator.
	ScalarGreaterOrEqual
	// ScalarMatch is the `=~` operator, an equality forcing its pattern operand.
	ScalarMatch
	// ScalarNotMatch is the `!~` operator, an inequality forcing its pattern operand.
	ScalarNotMatch
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

// Array comparison operators. The zero value is not a valid operator.
const (
	// ArrayIn is the `in` operator, true when any element matches.
	ArrayIn ArrayOp = iota + 1
	// ArrayNotIn is the `not in` operator, the negation of ArrayIn.
	ArrayNotIn
	// ArrayAllIn is the `allin` operator, true when all the elements match.
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

// Bitwise operators. The zero value is not a valid operator.
const (
	// BitAnd is the `&` operator.
	BitAnd BitOp = iota + 1
	// BitOr is the `|` operator.
	BitOr
	// BitXor is the `^` operator.
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

// Arithmetic operators. The zero value is not a valid operator.
const (
	// ArithAdd is the `+` operator.
	ArithAdd ArithOp = iota + 1
	// ArithSub is the `-` operator.
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

// Unary operators. The zero value is not a valid operator.
const (
	// UnaryNot is the boolean negation `!`, also spelled `not`.
	UnaryNot UnaryOp = iota + 1
	// UnaryNeg is the arithmetic negation `-`.
	UnaryNeg
	// UnaryBitNot is the bitwise negation `^`.
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
