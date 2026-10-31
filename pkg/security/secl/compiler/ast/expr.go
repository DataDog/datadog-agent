// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

// LogicalExpr is a boolean operation: `a && b`, `a || b`.
type LogicalExpr struct {
	Meta

	Op    LogicalOp
	Left  Node
	Right Node
}

// ScalarCompareExpr is a comparison of two values: `a == b`, `a < b`, `a =~ b`.
type ScalarCompareExpr struct {
	Meta

	Op    ScalarOp
	Left  Node
	Right Node
}

// ArrayCompareExpr is a membership test: `a in [b, c]`, `a not in b`, `a allin b`.
type ArrayCompareExpr struct {
	Meta

	Op ArrayOp
	// Left is the value, or the array of values, being tested.
	Left Node
	// Right is the array of values being tested against. Beside the array
	// literals, it can be any node resolving to an array: an Ident referring to
	// a macro, a VariableRef, a FieldRef.
	Right Node
}

// BitExpr is a bitwise operation: `a & b`, `a | b`, `a ^ b`.
type BitExpr struct {
	Meta

	Op    BitOp
	Left  Node
	Right Node
}

// ArithExpr is an arithmetic operation: `a + b`, `a - b`.
type ArithExpr struct {
	Meta

	Op    ArithOp
	Left  Node
	Right Node
}

// UnaryExpr is a unary operation: `!a`, `-a`, `^a`.
type UnaryExpr struct {
	Meta

	Op      UnaryOp
	Operand Node
}
