// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=CmpOp -trimprefix=Cmp -output cmp_op_string.go

// CmpOp identifies which comparison an ExprCmpBaseOp / ExprCmpStringOp
// performs. The numeric values are part of the wire format encoded into
// the stack-machine bytecode — never reorder or remove members.
type CmpOp uint8

const (
	// CmpEq is the equality (==) comparison.
	CmpEq CmpOp = 0
	// CmpNe is the inequality (!=) comparison.
	CmpNe CmpOp = 1
	// CmpLt is the less-than (<) ordering.
	CmpLt CmpOp = 2
	// CmpLe is the less-than-or-equal (<=) ordering.
	CmpLe CmpOp = 3
	// CmpGt is the greater-than (>) ordering.
	CmpGt CmpOp = 4
	// CmpGe is the greater-than-or-equal (>=) ordering.
	CmpGe CmpOp = 5
)
