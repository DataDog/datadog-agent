// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

// SymbolClass is a coarse classification of a Go symbol, computable from a
// quick scan without full parsing.
type SymbolClass uint8

const (
	// ClassFunction is any non-closure, non-init function or method.
	ClassFunction SymbolClass = iota
	// ClassClosure has .funcN, .gowrapN, .deferwrapN, or -rangeN.
	ClassClosure
	// ClassInit is pkg.init or pkg.init.N.
	ClassInit
	// ClassMapInit is pkg.map.init.N.
	ClassMapInit
	// ClassGlobalClosure is glob.funcN.
	ClassGlobalClosure
	// ClassCompilerGenerated has go:, type:, ..inittask, ..stmp_, ..dict. prefixes.
	ClassCompilerGenerated
	// ClassBareName has no package qualifier (assembly stubs like "indexbytebody").
	ClassBareName
	// ClassCFunction is a heuristic match for C function symbols: no '/' in
	// name, contains .isra., .part., or .constprop.
	ClassCFunction
)

// ReceiverKind indicates how a method receiver is declared.
type ReceiverKind uint8

const (
	// ReceiverNone means this is not a method.
	ReceiverNone ReceiverKind = iota
	// ReceiverPointer is a pointer receiver: (*Type).Method.
	ReceiverPointer
	// ReceiverValue is a value receiver: Type.Method.
	ReceiverValue
)

// WrapperKind indicates the type of compiler-generated wrapper.
type WrapperKind uint8

const (
	// WrapperNone means no wrapper.
	WrapperNone WrapperKind = iota
	// WrapperGoWrap is a go statement wrapper (.gowrapN).
	WrapperGoWrap
	// WrapperDeferWrap is a defer statement wrapper (.deferwrapN).
	WrapperDeferWrap
	// WrapperMethodExpr is a method expression wrapper (-fm).
	WrapperMethodExpr
)
