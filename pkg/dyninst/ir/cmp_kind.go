// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=CmpKind -trimprefix=CmpKind -output cmp_kind_string.go

// CmpKind tells ExprCmpBaseOp how to interpret the bytes being compared.
// Strings, equality, and inequality treat the buffer bytes verbatim — only
// ordering on multi-byte integers needs the kind to pick between unsigned
// and signed two's-complement semantics. The numeric values are part of
// the wire format.
type CmpKind uint8

const (
	// CmpKindUint compares bytes as an unsigned little-endian integer.
	// Also used for byte-equality (eq/ne) on any base type — including
	// floats, where bitwise equality matches Go's == semantics modulo the
	// signed-zero/NaN edge cases that aren't reachable with literal RHS.
	CmpKindUint CmpKind = 0
	// CmpKindInt compares bytes as a signed little-endian integer using
	// two's complement. Implemented in BPF by XOR-ing the sign bit of
	// the most-significant byte and comparing as unsigned.
	CmpKindInt CmpKind = 1
)
